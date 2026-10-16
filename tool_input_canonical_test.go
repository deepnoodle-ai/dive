package dive

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestCanonicalJSON(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"sorts keys":              {`{"repo_full_name": "a/b", "dest": "x"}`, `{"dest":"x","repo_full_name":"a/b"}`},
		"sorts nested keys":       {`{"z": {"b": 1, "a": [ {"d": 2, "c": 3} ]}}`, `{"z":{"a":[{"c":3,"d":2}],"b":1}}`},
		"keeps number literals":   {`{"n": 12345678901234567890, "f": 0.10}`, `{"f":0.10,"n":12345678901234567890}`},
		"already canonical":       {`{"a":1}`, `{"a":1}`},
		"invalid json untouched":  {`{"a":`, `{"a":`},
		"trailing data untouched": {`{"a":1} {"b":2}`, `{"a":1} {"b":2}`},
		"empty untouched":         {``, ``},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(canonicalJSON(json.RawMessage(tc.in))))
		})
	}
}

// The request that follows a tool call carries the call in canonical form, the
// same bytes a replay from a structured transcript store produces, and the
// recorded output messages match it.
func TestAgentRecordsToolCallInputsCanonically(t *testing.T) {
	var requests [][]*llm.Message
	calls := 0
	model := &mockLLM{generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		var config llm.Config
		config.Apply(opts...)
		requests = append(requests, config.Messages)
		calls++
		if calls == 1 {
			return &llm.Response{
				Role:       llm.Assistant,
				StopReason: "tool_use",
				Content: []llm.Content{&llm.ToolUseContent{
					ID: "toolu_1", Name: "clone", Input: json.RawMessage(`{"repo_full_name": "a/b", "dest": "x"}`),
				}},
			}, nil
		}
		return &llm.Response{Role: llm.Assistant, StopReason: "end_turn", Content: []llm.Content{&llm.TextContent{Text: "done"}}}, nil
	}}
	tool := &mockTool{name: "clone", callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewToolResultText("ok"), nil
	}}
	agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{tool}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("clone it"))
	assert.NoError(t, err)
	assert.Equal(t, 2, len(requests))

	var sent *llm.ToolUseContent
	for _, msg := range requests[1] {
		for _, c := range msg.Content {
			if tu, ok := c.(*llm.ToolUseContent); ok {
				sent = tu
			}
		}
	}
	assert.NotNil(t, sent)
	assert.Equal(t, `{"dest":"x","repo_full_name":"a/b"}`, string(sent.Input))

	var recorded *llm.ToolUseContent
	for _, msg := range resp.OutputMessages {
		for _, c := range msg.Content {
			if tu, ok := c.(*llm.ToolUseContent); ok {
				recorded = tu
			}
		}
	}
	assert.NotNil(t, recorded)
	assert.Equal(t, string(sent.Input), string(recorded.Input))
}
