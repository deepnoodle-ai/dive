//go:build integration

package meta

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if getAPIKey() == "" {
		t.Skip("Skipping integration test: no MODEL_API_KEY or META_API_KEY set")
	}
}

func testContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func TestIntegration_Generate(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := testContext(t, 90*time.Second)

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say hello and nothing else."),
	))
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, response.Role, llm.Assistant)
	text := response.Message().Text()
	t.Logf("response: %q", text)
	assert.True(t, len(text) > 0, "expected some visible output")
}

func TestIntegration_Stream(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := testContext(t, 90*time.Second)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Count from 1 to 5, separated by spaces."),
	))
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		assert.NoError(t, accumulator.AddEvent(iterator.Event()))
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	text := accumulator.Response().Message().Text()
	t.Logf("streamed: %q", text)
	assert.True(t, len(text) > 0)
}

// TestIntegration_UsageAndCost checks that Meta reports the usage fields the
// pricing table is keyed on, so spend is attributed rather than silently zero.
func TestIntegration_UsageAndCost(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := testContext(t, 90*time.Second)

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Name one primary color."),
	))
	assert.NoError(t, err)

	usage := response.Usage
	t.Logf("usage: input=%d output=%d cache_read=%d",
		usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens)
	assert.True(t, usage.InputTokens > 0, "expected input tokens to be reported")
	assert.True(t, usage.OutputTokens > 0, "expected output tokens to be reported")
}

// TestIntegration_EffortLadder probes what the API actually accepts, which is
// the contract modelcaps documents for its tables: every entry verified by
// sending the parameter and recording 200 vs 400, never inferred from a name or
// a docs page. Muse Spark is documented as rejecting "none" and not offering
// "max"; this is where that stops being a claim.
func TestIntegration_EffortLadder(t *testing.T) {
	skipIfNoAPIKey(t)

	for _, effort := range []llm.ReasoningEffort{
		llm.ReasoningEffortMinimal,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
	} {
		t.Run("accepts_"+string(effort), func(t *testing.T) {
			provider := New()
			ctx := testContext(t, 120*time.Second)
			_, err := provider.Generate(ctx,
				llm.WithMessages(llm.NewUserTextMessage("Reply with the word ok.")),
				llm.WithReasoningEffort(effort),
			)
			assert.NoError(t, err, "expected effort "+string(effort)+" to be accepted")
		})
	}

	// "none" is documented as HTTP 400 rather than ignored, and "max" is not
	// offered. Dive clamps both before they reach the wire, so a caller never
	// sees either; these assertions pin the clamp that makes that true.
	t.Run("clamps_none_up_to_minimal", func(t *testing.T) {
		got, send := modelcaps.ResolveEffort("meta", DefaultModel, llm.ReasoningEffortNone, nil)
		assert.True(t, send)
		assert.Equal(t, got, llm.ReasoningEffortMinimal)
	})
	t.Run("clamps_max_down_to_xhigh", func(t *testing.T) {
		got, send := modelcaps.ResolveEffort("meta", DefaultModel, llm.ReasoningEffortMax, nil)
		assert.True(t, send)
		assert.Equal(t, got, llm.ReasoningEffortXHigh)
	})
}

// TestIntegration_ToolCallAndReasoningReplay is the test the whole provider
// exists for. Muse Spark is on the Responses API precisely because Chat
// Completions redacts reasoning between turns; this confirms that encrypted
// reasoning comes back, survives a JSON round trip the way a stored session
// would, and is accepted when replayed alongside the tool results.
func TestIntegration_ToolCallAndReasoningReplay(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New(WithMaxTokens(4000))
	ctx := testContext(t, 180*time.Second)

	weather := llm.NewToolDefinition().
		WithName("get_weather").
		WithDescription("Get the current weather for a city.").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"city"},
			Properties: map[string]*schema.Property{
				"city": {Type: "string", Description: "City name"},
			},
		})

	prompt := "What is the weather in Paris? Use the tool, then answer in one sentence."

	first, err := provider.Generate(ctx,
		llm.WithUserTextMessage(prompt),
		llm.WithTools(weather),
		llm.WithReasoningEffort(llm.ReasoningEffortLow),
	)
	assert.NoError(t, err)

	// Persist and reload the assistant turn the way a durable session would.
	stored, err := json.Marshal(first.Message())
	assert.NoError(t, err)
	var reloaded llm.Message
	assert.NoError(t, json.Unmarshal(stored, &reloaded))

	var toolResults []llm.Content
	var signatures int
	for _, content := range reloaded.Content {
		switch c := content.(type) {
		case *llm.ToolUseContent:
			t.Logf("tool call: %s(%s)", c.Name, string(c.Input))
			toolResults = append(toolResults, &llm.ToolResultContent{
				ToolUseID: c.ID,
				Content:   "18C, partly cloudy",
			})
		case *llm.ThinkingContent:
			if c.Signature != "" {
				signatures++
			}
		}
	}
	if len(toolResults) == 0 {
		t.Skip("model answered without calling the tool; nothing to replay")
	}

	// The reason this provider is on Responses rather than Chat Completions.
	assert.True(t, signatures > 0,
		"expected encrypted reasoning to survive the round trip; without it "+
			"the Responses API buys nothing over Chat Completions here")
	t.Logf("replaying %d reasoning item(s) with %d tool result(s)", signatures, len(toolResults))

	second, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage(prompt),
			&reloaded,
			llm.NewMessage(llm.User, toolResults),
		),
		llm.WithTools(weather),
		llm.WithReasoningEffort(llm.ReasoningEffortLow),
	)
	assert.NoError(t, err, "Meta must accept the replayed reasoning item")
	final := second.Message().Text()
	t.Logf("final: %q", final)
	assert.True(t, strings.TrimSpace(final) != "", "expected a concluding answer")
}
