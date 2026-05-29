package arena

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// fakeLLM is a deterministic, offline stand-in for a real provider. It drives
// the agent's tool loop exactly like a real model would: when handed a task it
// returns a single valid tool call; when handed the tool's acknowledgement it
// returns plain text to end the turn. This lets us exercise the whole arena —
// the shared template, the referee hook, capture, sessions, and the transcript
// — without any network calls or API keys.
type fakeLLM struct {
	name    string
	self    string
	counter atomic.Int64
}

func (f *fakeLLM) Name() string { return f.name }

func (f *fakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	var cfg llm.Config
	cfg.Apply(opts...)
	if len(cfg.Messages) == 0 {
		return nil, fmt.Errorf("no messages")
	}
	last := cfg.Messages[len(cfg.Messages)-1]

	// A tool result means we already acted this turn — just acknowledge.
	if containsToolResult(last) {
		return f.text("Understood."), nil
	}

	task := lastUserText(cfg.Messages)
	switch {
	case strings.Contains(task, "'speak' tool"):
		return f.toolCall("speak", map[string]string{
			"message":   f.self + " has nothing incriminating to share.",
			"reasoning": "Staying low-key this round.",
		}), nil
	case strings.Contains(task, "'vote' tool"):
		return f.toolCall("vote", map[string]string{
			"target":    f.pickTarget(task),
			"reasoning": "Voting for the most suspicious player.",
		}), nil
	case strings.Contains(task, "'night_action' tool"):
		return f.toolCall("night_action", map[string]string{
			"target":    f.pickTarget(task),
			"reasoning": "Best night target available.",
		}), nil
	default:
		// Unknown prompt: produce harmless text so the turn ends (the arena
		// will treat it as a no-action and default to abstain).
		return f.text("..."), nil
	}
}

// pickTarget parses the "one of: a, b, c." candidate list from the task and
// returns the first candidate that is not the player itself (so votes form a
// clear majority and the game converges quickly under test).
func (f *fakeLLM) pickTarget(task string) string {
	idx := strings.Index(task, "one of:")
	if idx < 0 {
		return ""
	}
	rest := task[idx+len("one of:"):]
	if end := strings.IndexByte(rest, '.'); end >= 0 {
		rest = rest[:end]
	}
	var first string
	for _, c := range strings.Split(rest, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if first == "" {
			first = c
		}
		if c != f.self {
			return c
		}
	}
	return first
}

func (f *fakeLLM) text(s string) *llm.Response {
	return &llm.Response{
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: s}},
		StopReason: "end_turn",
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

func (f *fakeLLM) toolCall(name string, input map[string]string) *llm.Response {
	raw, _ := json.Marshal(input)
	id := fmt.Sprintf("call-%s-%d", f.self, f.counter.Add(1))
	return &llm.Response{
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.ToolUseContent{ID: id, Name: name, Input: raw}},
		StopReason: "tool_use",
		Usage:      llm.Usage{InputTokens: 50, OutputTokens: 20},
	}
}

func containsToolResult(m *llm.Message) bool {
	for _, c := range m.Content {
		if _, ok := c.(*llm.ToolResultContent); ok {
			return true
		}
	}
	return false
}

func lastUserText(msgs []*llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.User && !containsToolResult(msgs[i]) {
			return msgs[i].Text()
		}
	}
	return ""
}

func fakeContestants(ids ...string) []Contestant {
	out := make([]Contestant, len(ids))
	for i, id := range ids {
		out[i] = Contestant{
			ID:       id,
			Provider: id,
			Model:    "fake-" + id,
			LLM:      &fakeLLM{name: "fake-" + id, self: id},
		}
	}
	return out
}

func TestArenaRunsFullMatch(t *testing.T) {
	var buf bytes.Buffer
	gm, err := New(context.Background(), fakeContestants("claude", "gpt", "gemini", "grok"), Options{
		Seed:             7,
		MaxRounds:        8,
		DiscussionRounds: 1,
		Reveal:           false,
		Out:              io.Discard,
		Transcript:       &buf,
	})
	assert.NoError(t, err)
	assert.Equal(t, 4, gm.PlayerCount())

	winner, err := gm.Run(context.Background())
	assert.NoError(t, err)
	// With deterministic fakes and a fixed seed the village should reach a
	// decisive outcome inside the round cap.
	assert.NotEqual(t, "", string(winner), "match should produce a winner")

	events, err := transcript.Read(&buf)
	assert.NoError(t, err)
	types := eventTypeSet(events)
	for _, want := range []string{"match_start", "phase_start", "speak", "vote", "night_action", "match_end"} {
		assert.True(t, types[want], "transcript should contain a %q event", want)
	}

	// The transcript is the reveal artifact: it must record private reasoning.
	assert.True(t, hasReasoning(events), "transcript must capture private reasoning")

	// match_start must record every player's role for replay.
	start := firstEvent(events, "match_start")
	assert.NotNil(t, start)
	roles, _ := start.Data["roles"].(map[string]any)
	assert.Equal(t, 4, len(roles), "all four roles recorded")

	// match_end must name the winner.
	end := firstEvent(events, "match_end")
	assert.NotNil(t, end)
	assert.Equal(t, string(winner), end.Data["winner"])
}

func TestArenaRejectsMissingTranscript(t *testing.T) {
	_, err := New(context.Background(), fakeContestants("a", "b", "c"), Options{Out: io.Discard})
	assert.Error(t, err)
}

func TestArenaRejectsTooFewPlayers(t *testing.T) {
	var buf bytes.Buffer
	_, err := New(context.Background(), fakeContestants("a", "b"), Options{Out: io.Discard, Transcript: &buf})
	assert.Error(t, err)
}

// --- transcript helpers ---

func eventTypeSet(events []transcript.Event) map[string]bool {
	set := map[string]bool{}
	for _, e := range events {
		set[e.Type] = true
	}
	return set
}

func firstEvent(events []transcript.Event, typ string) *transcript.Event {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}
	return nil
}

func hasReasoning(events []transcript.Event) bool {
	for _, e := range events {
		if strings.TrimSpace(e.Reasoning) != "" {
			return true
		}
	}
	return false
}
