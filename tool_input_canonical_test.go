package dive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// jsonEsc turns each ~u in s into a JSON \u escape, so the test source stays
// free of escapes an editor might decode.
func jsonEsc(s string) string { return strings.ReplaceAll(s, "~u", "\x5cu") }

func TestCanonicalJSON(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"sorts keys":                {`{"repo_full_name": "a/b", "dest": "x"}`, `{"dest":"x","repo_full_name":"a/b"}`},
		"sorts nested keys":         {`{"z": {"b": 1, "a": [ {"d": 2, "c": 3} ]}}`, `{"z":{"a":[{"c":3,"d":2}],"b":1}}`},
		"sorts objects in arrays":   {`[{"b": 1, "a": 2}, [{"d": 3, "c": 4}]]`, `[{"a":2,"b":1},[{"c":4,"d":3}]]`},
		"sorts keys by byte order":  {"{\"\xc3\xa9\": 1, \"z\": 2, \"Z\": 3}", "{\"Z\":3,\"z\":2,\"\xc3\xa9\":1}"},
		"keeps number literals":     {`{"n": 12345678901234567890, "f": 0.10, "e": 1E+2, "i": 1.0}`, `{"e":1E+2,"f":0.10,"i":1.0,"n":12345678901234567890}`},
		"escapes html":              {`{"x": "<a href='q?a=1&b=2'>"}`, jsonEsc(`{"x":"~u003ca href='q?a=1~u0026b=2'~u003e"}`)},
		"decodes unicode escapes":   {jsonEsc(`{"x": "caf~u00e9 ~u0041"}`), "{\"x\":\"caf\xc3\xa9 A\"}"},
		"keeps separators escaped":  {jsonEsc(`{"x": "a~u2028b"}`), jsonEsc(`{"x":"a~u2028b"}`)},
		"duplicate keys keep last":  {`{"a": 1, "a": 2}`, `{"a":2}`},
		"null":                      {` null `, `null`},
		"already canonical":         {`{"a":1}`, `{"a":1}`},
		"invalid json untouched":    {`{"a":`, `{"a":`},
		"trailing value untouched":  {`{"a":1} {"b":2}`, `{"a":1} {"b":2}`},
		"trailing brace untouched":  {`{"a":1} }`, `{"a":1} }`},
		"empty untouched":           {``, ``},
		"whitespace only untouched": {`  `, `  `},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(canonicalJSON(json.RawMessage(tc.in))))
		})
	}
}

// Canonical form is the form a transcript store produces when it decodes a
// tool input into Go values and marshals it again: the same bytes when the
// store keeps number text, and the same bytes for every number a float64
// holds exactly in its shortest form.
func TestCanonicalJSONMatchesMapReplay(t *testing.T) {
	replay := func(raw string, useNumber bool) string {
		decoder := json.NewDecoder(strings.NewReader(raw))
		if useNumber {
			decoder.UseNumber()
		}
		var value any
		assert.NoError(t, decoder.Decode(&value))
		out, err := json.Marshal(value)
		assert.NoError(t, err)
		return string(out)
	}
	floatSafe := []string{
		`{"repo_full_name": "a/b", "branch": "main", "dest": "/tmp/x"}`,
		`{"z": {"b": [1, -2.5, {"y": null, "x": false}], "a": "<&>"}, "m": []}`,
		jsonEsc(`{"q": "caf~u00e9 ~u2028 ~ud83d~ude00 tab\t", "n": 9007199254740992}`),
		`[]`, `{}`, `"text"`, `null`,
	}
	for _, raw := range floatSafe {
		got := string(canonicalJSON(json.RawMessage(raw)))
		assert.Equal(t, replay(raw, false), got, "float64 replay of %s", raw)
		assert.Equal(t, replay(raw, true), got, "UseNumber replay of %s", raw)
	}
	numberText := `{"id": 12345678901234567890, "ratio": 0.10, "n": 1.0}`
	assert.Equal(t, replay(numberText, true), string(canonicalJSON(json.RawMessage(numberText))))
	assert.NotEqual(t, replay(numberText, false), string(canonicalJSON(json.RawMessage(numberText))))
}

func BenchmarkCanonicalJSON(b *testing.B) {
	big, _ := json.Marshal(map[string]any{
		"path":    "/src/main.go",
		"content": strings.Repeat("func main() { fmt.Println(\"<hello>\") }\n", 25000),
		"opts":    map[string]any{"mode": 420, "create": true},
	})
	b.SetBytes(int64(len(big)))
	for b.Loop() {
		canonicalJSON(big)
	}
}

// Both tool-call content types are canonicalized in place; other content is
// left alone.
func TestCanonicalizeToolInputs(t *testing.T) {
	toolUse := &llm.ToolUseContent{ID: "t1", Name: "clone", Input: json.RawMessage(`{"repo": "a/b", "branch": "main"}`)}
	mcpToolUse := &llm.MCPToolUseContent{ID: "m1", Name: "get_pr", ServerName: "github", Input: json.RawMessage(`{"repo": "a/b", "pr": {"number": 7, "draft": false}}`)}
	text := &llm.TextContent{Text: `{"b": 1, "a": 2}`}

	canonicalizeToolInputs([]llm.Content{text, toolUse, mcpToolUse})

	assert.Equal(t, `{"branch":"main","repo":"a/b"}`, string(toolUse.Input))
	assert.Equal(t, `{"pr":{"draft":false,"number":7},"repo":"a/b"}`, string(mcpToolUse.Input))
	assert.Equal(t, `{"b": 1, "a": 2}`, text.Text)
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
