package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Sonnet 4.5 has no native effort parameter, so a requested effort is served
// as a manual thinking budget. The clamp is silent on the wire; the response is
// where the caller can see it.
const clampedControlsAnthropicStream = `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"usage":{"input_tokens":14,"output_tokens":4}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":14,"output_tokens":7}}

data: {"type":"message_stop"}

`

func TestGenerateReportsEffectiveControls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":14,"output_tokens":7}}`)
	}))
	defer server.Close()

	plan, ok := modelcaps.Preview("anthropic", llm.Config{
		Model:           ModelClaudeSonnet45,
		ReasoningEffort: llm.ReasoningEffortMedium,
		Temperature:     floatPointer(0.7),
	})
	assert.True(t, ok)

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL), WithModel(ModelClaudeSonnet45))
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithReasoningEffort(llm.ReasoningEffortMedium),
		llm.WithTemperature(0.7),
	)
	assert.NoError(t, err)

	controls := response.Usage.Controls
	assert.NotNil(t, controls)
	assert.True(t, controls.Equal(plan.Effective))
	// The effort was translated into a budget, and temperature was dropped
	// because thinking is active — neither is visible without this field.
	assert.Equal(t, llm.ReasoningEffort(""), controls.ReasoningEffort)
	assert.NotNil(t, controls.ReasoningBudget)
	assert.Nil(t, controls.Temperature)
}

func TestStreamReportsEffectiveControls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, clampedControlsAnthropicStream)
	}))
	defer server.Close()

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL), WithModel(ModelClaudeSonnet45))
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithReasoningEffort(llm.ReasoningEffortMedium),
		llm.WithTemperature(0.7),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	accumulator := consumeAnthropicStream(t, iterator)
	assert.True(t, accumulator.IsComplete())

	controls := accumulator.Response().Usage.Controls
	assert.NotNil(t, controls)
	assert.NotNil(t, controls.ReasoningBudget)
	assert.Nil(t, controls.Temperature)
}
