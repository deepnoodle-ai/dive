package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Anthropic repeats the full cumulative usage object in message_delta that
// message_start already carried. The accumulated usage must equal the final
// frame's values, not the sum of both frames.
const cumulativeUsageAnthropicStream = `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":14,"cache_creation_input_tokens":8012,"cache_read_input_tokens":120000,"output_tokens":4}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":14,"cache_creation_input_tokens":8012,"cache_read_input_tokens":120000,"output_tokens":7}}

data: {"type":"message_stop"}

`

func TestStreamCumulativeUsageNotDoubleCounted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, cumulativeUsageAnthropicStream)
	}))
	defer server.Close()

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
	)
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	accumulator := consumeAnthropicStream(t, iterator)
	assert.True(t, accumulator.IsComplete())

	usage := accumulator.Response().Usage
	assert.Equal(t, 14, usage.InputTokens)
	assert.Equal(t, 8012, usage.CacheCreationInputTokens)
	assert.Equal(t, 120000, usage.CacheReadInputTokens)
	assert.Equal(t, 7, usage.OutputTokens)
	assert.Equal(t, 128026, usage.TotalInputTokens())
}
