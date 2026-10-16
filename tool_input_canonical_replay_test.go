package dive_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Tool inputs as a model writes them: keys out of order at every depth,
// spacing, characters json.Marshal escapes, and \u escapes it does not keep.
var modelToolInputs = []string{
	uesc(`{"repo_full_name": "a/b",  "opts": {"z": [3, {"y": null, "x": "<b>&amp;</b>"}], "a": true}, "dest": "caf~u00e9 ~u2028 ok"}`),
	`{ "pr_number": 42, "repo_full_name": "a/b", "ids": [9007199254740992, -7] }`,
}

// Number literals a float64 decode rewrites: past 2^53, a trailing zero, a
// decimal point on an integer. A store matches them only when it keeps number
// text, as json.Decoder.UseNumber does.
const numberLiteralInput = `{"id": 12345678901234567890, "ratio": 0.10, "n": 1.0}`

// A live turn's last request and the next turn's first request, rebuilt from
// a transcript store that decodes each block into a Go map and marshals it
// again, carry the turn's tool calls as identical bytes. That byte-for-byte
// prefix is what the provider's prompt cache needs to read instead of
// rewrite.
func TestToolInputsReplayByteForByte(t *testing.T) {
	for _, useNumber := range []bool{false, true} {
		t.Run(fmt.Sprintf("useNumber=%v", useNumber), func(t *testing.T) {
			inputs := modelToolInputs
			if useNumber {
				inputs = append(inputs[:len(inputs):len(inputs)], numberLiteralInput)
			}
			server := &scriptedAnthropic{streams: []string{
				toolUseStream(inputs...),
				textStream("done"),
				textStream("again"),
			}}
			ts := httptest.NewServer(server)
			defer ts.Close()

			received := map[string]any{}
			var mu sync.Mutex
			tool := FuncTool("clone", "Clone a repository.",
				func(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
					mu.Lock()
					defer mu.Unlock()
					received[ToolCallID(ctx)] = decodeNumbers(t, input)
					return NewToolResultText("ok"), nil
				},
				WithFuncToolSchema(&Schema{Type: Object}),
			)
			agent, err := NewAgent(AgentOptions{
				SystemPrompt: "You clone repositories.",
				Model:        anthropic.New(anthropic.WithAPIKey("test"), anthropic.WithEndpoint(ts.URL)),
				Tools:        []Tool{tool},
			})
			assert.NoError(t, err)

			input := llm.NewUserTextMessage("clone it")
			resp, err := agent.CreateResponse(context.Background(), WithMessages(input))
			assert.NoError(t, err)

			// Each tool receives the values the model wrote, number text
			// included.
			assert.Equal(t, len(inputs), len(received))
			for i, raw := range inputs {
				want := decodeNumbers(t, json.RawMessage(raw))
				assert.True(t, reflect.DeepEqual(want, received[fmt.Sprintf("toolu_%d", i)]), "tool input %d changed", i)
			}

			// Rebuild the turn the way a structured store replays it.
			history := []*llm.Message{input}
			for _, msg := range resp.OutputMessages {
				history = append(history, replayThroughMap(t, msg, useNumber))
			}
			history = append(history, llm.NewUserTextMessage("and again"))
			_, err = agent.CreateResponse(context.Background(), WithMessages(history...))
			assert.NoError(t, err)

			bodies := server.requestBodies()
			assert.Equal(t, 3, len(bodies))
			live, replay := bodies[1], bodies[2]
			assert.Equal(t, string(live.System), string(replay.System))
			assert.Equal(t, string(live.Tools), string(replay.Tools))
			// messages[0] is the user's input and messages[1] the assistant's
			// tool calls. Later messages differ only by where each request
			// puts its cache breakpoint.
			for i := 0; i < 2; i++ {
				assert.Equal(t, string(live.Messages[i]), string(replay.Messages[i]))
			}
			calls := string(live.Messages[1])
			assert.Contains(t, calls, uesc(`"input":{"dest":"caf`+"\xc3\xa9"+` ~u2028 ok","opts":{"a":true,"z":[3,{"x":"~u003cb~u003e~u0026amp;~u003c/b~u003e","y":null}]},"repo_full_name":"a/b"}`))
			assert.Contains(t, calls, `"input":{"ids":[9007199254740992,-7],"pr_number":42,"repo_full_name":"a/b"}`)
			if useNumber {
				assert.Contains(t, calls, `"input":{"id":12345678901234567890,"n":1.0,"ratio":0.10}`)
			}
		})
	}
}

// A finished tool call kept from an interrupted stream is recorded in the
// same canonical form as one from a complete response.
func TestInterruptedStreamRecordsToolInputCanonically(t *testing.T) {
	events := []*llm.Event{
		{Type: llm.EventTypeMessageStart, Message: &llm.Response{ID: "msg_1", Role: llm.Assistant}},
		{Type: llm.EventTypeContentBlockStart, Index: idx(0), ContentBlock: &llm.EventContentBlock{Type: llm.ContentTypeToolUse, ID: "toolu_1", Name: "work"}},
		{Type: llm.EventTypeContentBlockDelta, Index: idx(0), Delta: &llm.EventDelta{Type: llm.EventDeltaTypeInputJSON, PartialJSON: `{"b": 1, "a": {"d": 2, "c": 3}}`}},
		{Type: llm.EventTypeContentBlockStop, Index: idx(0)},
	}
	mock := &eventStreamLLM{streams: [][]*llm.Event{events}, errs: []error{errors.New("connection reset")}}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	genErr := assertIncomplete(t, resp, err, TurnReasonStreamInterrupted, TurnNextContinue)
	call, ok := genErr.OutputMessages[0].Content[0].(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, `{"a":{"c":3,"d":2},"b":1}`, string(call.Input))
}

// uesc turns each ~u in s into a JSON \u escape, so the test source stays
// free of escapes an editor might decode.
func uesc(s string) string { return strings.ReplaceAll(s, "~u", "\x5cu") }

func decodeNumbers(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	assert.NoError(t, decoder.Decode(&value))
	return value
}

// replayThroughMap round-trips each content block of msg through
// map[string]any, as a Postgres jsonb column read into Go maps does, and
// decodes it back into typed content.
func replayThroughMap(t *testing.T, msg *llm.Message, useNumber bool) *llm.Message {
	t.Helper()
	out := &llm.Message{ID: msg.ID, Role: msg.Role}
	for _, content := range msg.Content {
		raw, err := json.Marshal(content)
		assert.NoError(t, err)
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if useNumber {
			decoder.UseNumber()
		}
		var block map[string]any
		assert.NoError(t, decoder.Decode(&block))
		raw, err = json.Marshal(block)
		assert.NoError(t, err)
		replayed, err := llm.UnmarshalContent(raw)
		assert.NoError(t, err)
		out.Content = append(out.Content, replayed)
	}
	return out
}

type anthropicRequestBody struct {
	System   json.RawMessage   `json:"system"`
	Tools    json.RawMessage   `json:"tools"`
	Messages []json.RawMessage `json:"messages"`
}

// scriptedAnthropic answers each Messages API request with the next scripted
// event stream and keeps the request bodies.
type scriptedAnthropic struct {
	mu      sync.Mutex
	streams []string
	bodies  [][]byte
}

func (s *scriptedAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	n := len(s.bodies)
	s.bodies = append(s.bodies, body)
	s.mu.Unlock()
	if n >= len(s.streams) {
		http.Error(w, "unexpected request", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, s.streams[n])
}

func (s *scriptedAnthropic) requestBodies() []anthropicRequestBody {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]anthropicRequestBody, len(s.bodies))
	for i, body := range s.bodies {
		_ = json.Unmarshal(body, &out[i])
	}
	return out
}

func sseEvents(events ...map[string]any) string {
	var sb strings.Builder
	for _, event := range events {
		data, _ := json.Marshal(event)
		sb.WriteString("data: ")
		sb.Write(data)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func messageStart() map[string]any {
	return map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "test-model",
		"content": []any{}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 1},
	}}
}

func messageEnd(stopReason string) []map[string]any {
	return []map[string]any{
		{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason}, "usage": map[string]any{"output_tokens": 5}},
		{"type": "message_stop"},
	}
}

// toolUseStream streams one tool call per input, each input split into two
// partial_json chunks.
func toolUseStream(inputs ...string) string {
	events := []map[string]any{messageStart()}
	for i, input := range inputs {
		half := len(input) / 2
		events = append(events,
			map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{
				"type": "tool_use", "id": fmt.Sprintf("toolu_%d", i), "name": "clone", "input": map[string]any{},
			}},
			map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "input_json_delta", "partial_json": input[:half]}},
			map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "input_json_delta", "partial_json": input[half:]}},
			map[string]any{"type": "content_block_stop", "index": i},
		)
	}
	return sseEvents(append(events, messageEnd("tool_use")...)...)
}

func textStream(text string) string {
	events := []map[string]any{
		messageStart(),
		{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}},
		{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}},
		{"type": "content_block_stop", "index": 0},
	}
	return sseEvents(append(events, messageEnd("end_turn")...)...)
}
