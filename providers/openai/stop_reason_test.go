package openai

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

func responseWith(t *testing.T, raw string) *responses.Response {
	t.Helper()
	var resp responses.Response
	assert.NoError(t, json.Unmarshal([]byte(raw), &resp))
	return &resp
}

const functionCallItem = `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":","status":"incomplete"}`

func TestDetermineStopReasonStatusBeforeToolCalls(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"cut off with a call", `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[` + functionCallItem + `]}`, "max_tokens"},
		{"filtered with a call", `{"status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[` + functionCallItem + `]}`, "content_filter"},
		{"incomplete without reason", `{"status":"incomplete","output":[` + functionCallItem + `]}`, "incomplete"},
		{"failed with a call", `{"status":"failed","output":[` + functionCallItem + `]}`, "error"},
		{"cancelled", `{"status":"cancelled","output":[]}`, "cancelled"},
		{"completed with a call", `{"status":"completed","output":[` + functionCallItem + `]}`, "tool_use"},
		{"completed text", `{"status":"completed","output":[{"type":"message","id":"m","role":"assistant","status":"completed","content":[]}]}`, "end_turn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, determineStopReason(responseWith(t, tc.raw)), tc.want)
		})
	}
}
