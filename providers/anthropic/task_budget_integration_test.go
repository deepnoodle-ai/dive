//go:build integration

package anthropic_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/wonton/assert"
)

// archiveSection returns about 3,000 tokens of synthetic incident log.
func archiveSection(index int) string {
	services := []string{"checkout", "auth", "search", "inventory", "billing", "notifications"}
	notes := []string{"retry storm", "cache miss", "slow query", "pool exhausted", "timeout", "ok", "failover", "GC pause"}
	var b strings.Builder
	fmt.Fprintf(&b, "=== Incident archive, section %d of 12 ===\n", index)
	seed := uint32(index*7919 + 17)
	for line := 0; line < 100; line++ {
		seed = seed*1664525 + 1013904223
		fmt.Fprintf(&b, "2026-03-%02dT%02d:%02d:%02dZ host-%03d service=%s latency_ms=%d status=%d note=%q\n",
			index+1, line/4, (line*7)%60, int(seed%60), int(seed%250),
			services[int(seed>>8)%len(services)], 20+int(seed>>4)%4000,
			[]int{200, 200, 200, 500, 503, 429}[int(seed>>12)%6], notes[int(seed>>16)%len(notes)])
	}
	return b.String()
}

type readSectionInput struct {
	Index int `json:"index" description:"Section number, 0 to 11"`
}

// callRecord is one model call's outcome, read from its message_delta event.
type callRecord struct {
	stopReason   string
	outputTokens int
}

// A Haiku 5.5 run with adaptive thinking at medium effort reads about 36,000
// tokens of tool output under a 40,000-token task budget and is asked for a
// long report. The API must accept the budget, and the model must finish the
// turn rather than run into max_tokens.
func TestIntegration_TaskBudgetHaiku55(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	readSection := dive.FuncTool("read_section", "Read one section of the incident archive.",
		func(ctx context.Context, input *readSectionInput) (*dive.ToolResult, error) {
			if input.Index < 0 || input.Index > 11 {
				return dive.NewToolResultError("index must be 0 to 11"), nil
			}
			return dive.NewToolResultText(archiveSection(input.Index)), nil
		})

	sentBudget := false
	maxTokens := 32000
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model: anthropic.New(anthropic.WithModel(anthropic.ModelClaudeHaiku55)),
		ModelSettings: &dive.ModelSettings{
			Thinking:        llm.ThinkingTypeAdaptive,
			ReasoningEffort: llm.ReasoningEffortMedium,
			MaxTokens:       &maxTokens,
			TaskBudget:      &llm.TaskBudget{Total: 40000},
		},
		Tools: []dive.Tool{readSection},
		LLMHooks: llm.Hooks{{Type: llm.BeforeGenerate, Func: func(ctx context.Context, hctx *llm.HookContext) error {
			if strings.Contains(string(hctx.Request.Body), `"task_budget":{"type":"tokens","total":40000}`) {
				sentBudget = true
			}
			return nil
		}}},
	})
	assert.NoError(t, err)

	var calls []callRecord
	response, err := agent.CreateResponse(ctx,
		dive.WithInput("The incident archive has 12 sections, numbered 0 to 11. Read every section with "+
			"read_section, then write a long, detailed incident report of at least 3,000 words that covers "+
			"each section: the services affected, error rates, latency outliers, and a timeline."),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent && item.Event.Type == llm.EventTypeMessageDelta {
				record := callRecord{stopReason: item.Event.Delta.StopReason}
				if item.Event.Usage != nil {
					record.outputTokens = item.Event.Usage.OutputTokens
				}
				calls = append(calls, record)
			}
			return nil
		}),
	)
	assert.NoError(t, err)
	assert.True(t, sentBudget, "the request body should carry the task budget")

	total := 0
	for i, call := range calls {
		total += call.outputTokens
		t.Logf("call %d: output_tokens=%d stop_reason=%s", i+1, call.outputTokens, call.stopReason)
	}
	t.Logf("calls=%d output_tokens=%d status=%s report_words=%d",
		len(calls), total, response.Status, len(strings.Fields(response.OutputText())))

	assert.True(t, len(calls) > 0)
	assert.Equal(t, "end_turn", calls[len(calls)-1].stopReason)
	for _, call := range calls {
		assert.NotEqual(t, "max_tokens", call.stopReason)
	}
	assert.Equal(t, dive.ResponseStatusCompleted, response.Status)
}
