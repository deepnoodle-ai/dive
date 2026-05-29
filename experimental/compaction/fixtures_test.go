package compaction

// Realistic, larger compaction fixtures and a deterministic end-to-end test of
// the non-destructive compaction cycle over them.
//
// The fixtures in testdata/*.json are full multi-turn agent transcripts (a SWE
// debugging session, a codebase-onboarding exploration, and a production
// incident triage) — far closer to a real session than the tiny unit-test
// messages elsewhere in this package. They are the source of truth; edit them
// directly. This test loads each one and runs it through the real
// CompactMessages + session.Compact path with a deterministic stub summarizer,
// so it asserts the mechanics (active window collapses, originals are retained,
// history is recorded) without needing network access.
//
// The companion capture_test.go runs the same fixtures against a live model and
// writes the generated summaries to testdata/captured/ for human review.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// fixture is one loaded transcript plus its human-facing description.
type fixture struct {
	name        string // file name without extension, e.g. "swe_debugging"
	title       string
	description string
	messages    []*llm.Message
}

// fixtureManifest describes each transcript. Keyed by file stem.
var fixtureManifest = map[string]struct{ title, description string }{
	"swe_debugging": {
		title:       "SWE debugging session (multi-phase)",
		description: "The largest fixture: a long coding session spanning three phases. The agent fixes a failing discount test, adds a multi-item regression test, then investigates and fixes a separate per-line tax-rounding (off-by-a-cent) bug — with its own reproduction test — verifies the fix is localized across the repo, and runs the full suite. Exercises a realistic debug→fix→test→verify loop across several files with a hard 'don't weaken the tests' constraint.",
	},
	"codebase_onboarding": {
		title:       "Codebase onboarding / auth exploration",
		description: "An agent answers an architecture question — 'how does request auth work and where do I add API-key auth?' — by reading the router, middleware, JWT verifier, and identity helpers. Read-only; the payload is dominated by several source-file tool results, the kind of context that drives compaction.",
	},
	"incident_triage": {
		title:       "Production incident triage",
		description: "An SRE-style read-only investigation of a 500-rate spike: pulls Cloud Run error logs, finds pool exhaustion, locates a connection leak in a repo, and corroborates with a read-only pg_stat_activity query. Carries explicit production constraints (no restarts/writes without approval) that a good summary must preserve.",
	},
}

// loadFixtures reads every testdata/*.json transcript, sorted by name.
func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	assert.NoError(t, err)
	assert.True(t, len(paths) > 0, "expected at least one fixture in testdata/")
	sort.Strings(paths)

	var out []fixture
	for _, p := range paths {
		data, err := os.ReadFile(p)
		assert.NoError(t, err)
		var msgs []*llm.Message
		assert.NoError(t, json.Unmarshal(data, &msgs), "fixture %s should parse", p)
		assert.True(t, len(msgs) > 0, "fixture %s should have messages", p)

		stem := stemOf(p)
		meta := fixtureManifest[stem]
		out = append(out, fixture{
			name:        stem,
			title:       meta.title,
			description: meta.description,
			messages:    msgs,
		})
	}
	return out
}

func stemOf(path string) string {
	base := filepath.Base(path)
	return base[:len(base)-len(filepath.Ext(base))]
}

// stubLLM is a deterministic summarizer: it ignores the prompt and returns a
// fixed <summary> block, echoing how many messages it was handed so the test
// can confirm the whole active window reached the model.
type stubLLM struct {
	sawMessages int
	sawTokens   int // estimated size of the transcript handed to the summarizer
}

func (s *stubLLM) Name() string { return "stub" }

func (s *stubLLM) Generate(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	// The last message is the injected summary instruction; the rest is the
	// transcript handed to the summarizer.
	s.sawMessages = len(cfg.Messages) - 1
	s.sawTokens = 0
	for _, m := range cfg.Messages {
		s.sawTokens += estimateTokens(m)
	}
	return &llm.Response{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{Text: "<summary>STUB SUMMARY — saw transcript and produced a continuation handoff.</summary>"},
		},
	}, nil
}

// TestFixturesAreWellFormed guards the transcripts themselves: every tool_use
// must have a matching tool_result so compaction's pairing assumptions hold.
func TestFixturesAreWellFormed(t *testing.T) {
	for _, f := range loadFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			assert.NotEmpty(t, f.title, "fixture %s missing manifest entry", f.name)

			pendingToolUse := map[string]bool{}
			for _, m := range f.messages {
				for _, c := range m.Content {
					switch cc := c.(type) {
					case *llm.ToolUseContent:
						assert.NotEmpty(t, cc.ID)
						assert.NotEmpty(t, cc.Name)
						assert.True(t, json.Valid(cc.Input), "tool_use %s has invalid JSON input", cc.ID)
						pendingToolUse[cc.ID] = true
					case *llm.ToolResultContent:
						assert.True(t, pendingToolUse[cc.ToolUseID],
							"tool_result %s has no preceding tool_use", cc.ToolUseID)
						delete(pendingToolUse, cc.ToolUseID)
					}
				}
			}
			assert.Equal(t, 0, len(pendingToolUse), "every tool_use should be answered by a tool_result")

			// The transcripts end on assistant text (a final answer), so nothing
			// is dropped by filterPendingToolUse during compaction.
			last := f.messages[len(f.messages)-1]
			assert.Equal(t, llm.Assistant, last.Role)
		})
	}
}

// TestNonDestructiveCompactionCycle runs each fixture through the real
// compaction path with a deterministic summarizer and asserts the
// non-destructive contract end to end.
func TestNonDestructiveCompactionCycle(t *testing.T) {
	ctx := context.Background()
	for _, f := range loadFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			sess := session.New(f.name)
			assert.NoError(t, sess.SaveTurn(ctx, f.messages, nil))

			before, err := sess.Messages(ctx)
			assert.NoError(t, err)
			assert.Equal(t, len(f.messages), len(before), "active window starts as the full transcript")

			stub := &stubLLM{}
			err = sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
				out, _, cerr := CompactMessages(ctx, stub, msgs, "", "", 123456)
				return out, cerr
			})
			assert.NoError(t, err)
			assert.Equal(t, len(f.messages), stub.sawMessages, "summarizer should receive the whole active window")

			// Active window collapsed to the single summary message, framed as a
			// predecessor's handoff and authored in the User role.
			after, err := sess.Messages(ctx)
			assert.NoError(t, err)
			assert.Len(t, after, 1)
			assert.Equal(t, llm.User, after[0].Role)
			assert.Contains(t, after[0].Text(), "STUB SUMMARY")
			assert.Contains(t, after[0].Text(), "handoff notes")
			assert.True(t, estimateTokens(after[0]) < activeTokens(before),
				"compacted window should be smaller than the original")

			// Originals retained: AllMessages = full transcript + appended summary.
			all, err := sess.AllMessages(ctx)
			assert.NoError(t, err)
			assert.Equal(t, len(f.messages)+1, len(all))

			// Exactly one checkpoint, replacing the whole transcript.
			hist, err := sess.CompactionHistory(ctx)
			assert.NoError(t, err)
			assert.Len(t, hist, 1)
			assert.Len(t, hist[0].Summary, 1)
			assert.Equal(t, len(f.messages), len(hist[0].ReplacedMessages))
			assert.False(t, hist[0].CompactedAt.IsZero())
		})
	}
}

func activeTokens(msgs []*llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokens(m)
	}
	return total
}
