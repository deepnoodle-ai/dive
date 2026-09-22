package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

const okResponse = `{
	"id": "msg_1", "type": "message", "role": "assistant",
	"model": "claude-opus-5-5", "content": [{"type": "text", "text": "ok"}],
	"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 1}
}`

// capturedRequest is what a capturingServer received.
type capturedRequest struct {
	body  map[string]any
	betas []string
}

// capturingServer answers every request with response and records the last
// request's body and beta headers.
func capturingServer(t *testing.T, contentType, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		captured.body = nil
		assert.NoError(t, json.Unmarshal(data, &captured.body))
		captured.betas = nil
		if header := r.Header.Get("anthropic-beta"); header != "" {
			captured.betas = strings.Split(header, ",")
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func generateCaptured(t *testing.T, response string, providerOpts []Option, opts ...llm.Option) (*llm.Response, *capturedRequest) {
	t.Helper()
	server, captured := capturingServer(t, "application/json", response)
	provider := New(append([]Option{WithEndpoint(server.URL), WithAPIKey("test-key")}, providerOpts...)...)
	resp, err := provider.Generate(context.Background(), opts...)
	assert.NoError(t, err)
	return resp, captured
}

// Opus 5.5 thinks without a thinking config, so asking only for the updates
// display must still send a thinking object to carry it, with its beta header.
func TestThinkingDisplayUpdatesOnModelThatThinksByDefault(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithThinkingDisplay(llm.ThinkingDisplayUpdates),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Equal(t, map[string]any{"type": "adaptive", "display": "updates"}, captured.body["thinking"])
	assert.Equal(t, []string{FeatureThinkingDisplayUpdates}, captured.betas)
}

// Opus 4.8 does not think unless asked, so a display setting alone must not
// turn thinking on.
func TestThinkingDisplayDoesNotEnableThinking(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48, llm.WithThinkingDisplay(llm.ThinkingDisplaySummarized))
	assert.Nil(t, req.Thinking)
}

func TestThinkingDisplayOtherValuesNeedNoBeta(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithThinkingDisplay(llm.ThinkingDisplaySummarized),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Equal(t, map[string]any{"type": "adaptive", "display": "summarized"}, captured.body["thinking"])
	assert.Len(t, captured.betas, 0)
}

func TestPrefixMismatchBehaviorSetsBlockBinding(t *testing.T) {
	response := `{
		"id": "msg_1", "type": "message", "role": "assistant",
		"model": "claude-opus-5-5", "content": [{"type": "text", "text": "ok"}],
		"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 1},
		"input_transformations": [
			{"type": "thinking_dropped", "path": "messages.1.content.0", "reason": "prefix_binding_mismatch"}
		]
	}`
	resp, captured := generateCaptured(t, response,
		[]Option{WithPrefixMismatchBehavior(PrefixMismatchDropBlock)},
		llm.WithModel(ModelClaudeOpus55),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Equal(t, map[string]any{
		"type":          "adaptive",
		"block_binding": map[string]any{"prefix_mismatch_behavior": "drop_block"},
	}, captured.body["thinking"])
	assert.Equal(t, []string{FeatureThinkingBindingControls}, captured.betas)
	assert.Equal(t, []llm.InputTransformation{{
		Type:   "thinking_dropped",
		Path:   "messages.1.content.0",
		Reason: "prefix_binding_mismatch",
	}}, resp.InputTransformations)
}

// block_binding is part of the thinking object, so it cannot go on a request
// that disables thinking.
func TestPrefixMismatchBehaviorSkippedWhenThinkingDisabled(t *testing.T) {
	_, captured := generateCaptured(t, okResponse,
		[]Option{WithPrefixMismatchBehavior(PrefixMismatchError)},
		llm.WithModel(ModelClaudeOpus5),
		llm.WithThinking(llm.ThinkingTypeDisabled),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Equal(t, map[string]any{"type": "disabled"}, captured.body["thinking"])
	assert.Len(t, captured.betas, 0)
}

// The binding-controls header on its own reports mismatches without
// enforcing anything, so enabling the feature must send it.
func TestThinkingBindingControlsFeatureAlone(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithFeatures(FeatureThinkingBindingControls),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Nil(t, captured.body["thinking"])
	assert.Equal(t, []string{FeatureThinkingBindingControls}, captured.betas)
}

// After a mid-stream fallback, message_delta carries input_transformations
// again with the serving model's entries, which replace message_start's.
func TestStreamInputTransformations(t *testing.T) {
	stream := `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":10,"output_tokens":1},"input_transformations":[{"type":"thinking_mismatch_allowed","path":"messages.1.content.0","reason":"prefix_binding_mismatch"}]}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2},"input_transformations":[{"type":"thinking_dropped","path":"messages.1.content.0","reason":"model_binding_mismatch"}]}

data: {"type":"message_stop"}

`
	server, _ := capturingServer(t, "text/event-stream", stream)
	provider := New(WithEndpoint(server.URL), WithAPIKey("test-key"))
	iterator, err := provider.Stream(context.Background(),
		llm.WithModel(ModelClaudeOpus55),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	resp := consumeAnthropicStream(t, iterator).Response()
	assert.Equal(t, []llm.InputTransformation{{
		Type:   "thinking_dropped",
		Path:   "messages.1.content.0",
		Reason: "model_binding_mismatch",
	}}, resp.InputTransformations)
}

func effortConversation(effort llm.ReasoningEffort) llm.Option {
	return llm.WithMessages(
		llm.NewUserTextMessage("Plan the migration."),
		llm.NewAssistantTextMessage("Here is the plan."),
		llm.NewEffortMessage(effort),
		llm.NewUserTextMessage("Rename the file."),
	)
}

func TestEffortMessageSentAsOutputConfig(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithReasoningEffort(llm.ReasoningEffortHigh),
		effortConversation(llm.ReasoningEffortLow),
	)
	messages := captured.body["messages"].([]any)
	assert.Len(t, messages, 4)
	assert.Equal(t, map[string]any{
		"role":          "system",
		"content":       []any{},
		"output_config": map[string]any{"effort": "low"},
	}, messages[2])
	// The other messages keep their usual form, with no output_config.
	assert.Equal(t, "user", messages[3].(map[string]any)["role"])
	_, hasOutputConfig := messages[3].(map[string]any)["output_config"]
	assert.False(t, hasOutputConfig)
	assert.Equal(t, map[string]any{"effort": "high"}, captured.body["output_config"])
	assert.Equal(t, []string{FeatureMidConversationOutputConfig}, captured.betas)
}

func TestEffortMessageClampedToModelLevels(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		effortConversation(llm.ReasoningEffortMinimal),
	)
	messages := captured.body["messages"].([]any)
	assert.Equal(t, map[string]any{"effort": "low"}, messages[2].(map[string]any)["output_config"])
}

// A session can move to a model without per-message effort. The effort message
// is dropped there, not sent to fail the request.
func TestEffortMessageDroppedWithoutSupport(t *testing.T) {
	for _, model := range []string{ModelClaudeSonnet5, ModelClaudeFable5, "custom-model"} {
		t.Run(model, func(t *testing.T) {
			_, captured := generateCaptured(t, okResponse, nil,
				llm.WithModel(model),
				effortConversation(llm.ReasoningEffortLow),
			)
			messages := captured.body["messages"].([]any)
			assert.Len(t, messages, 3)
			for _, message := range messages {
				assert.NotEqual(t, "system", message.(map[string]any)["role"])
			}
			assert.Len(t, captured.betas, 0)
		})
	}
}

func TestEffortMessageDoesNotMutateCaller(t *testing.T) {
	effort := llm.NewEffortMessage(llm.ReasoningEffortMinimal)
	generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithMessages(llm.NewUserTextMessage("hi"), effort, llm.NewUserTextMessage("again")),
	)
	assert.Equal(t, llm.ReasoningEffortMinimal, effort.Effort)
}
