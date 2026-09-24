package google

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// TestConvertFinishReasonKeepsReasonsDistinct verifies that each Gemini
// finish reason keeps its own stop reason, so a safety stop classifies as a
// refusal and a malformed call as incomplete instead of both being "other".
func TestConvertFinishReasonKeepsReasonsDistinct(t *testing.T) {
	cases := []struct {
		reason genai.FinishReason
		want   string
		kind   llm.StopKind
	}{
		{genai.FinishReasonStop, "stop", llm.StopKindFinished},
		{genai.FinishReasonMaxTokens, "max_tokens", llm.StopKindOutputLimit},
		{genai.FinishReasonSafety, "safety", llm.StopKindRefusal},
		{genai.FinishReasonRecitation, "recitation", llm.StopKindRefusal},
		{genai.FinishReasonProhibitedContent, "prohibited_content", llm.StopKindRefusal},
		{genai.FinishReasonMalformedFunctionCall, "malformed_function_call", llm.StopKindIncomplete},
		{genai.FinishReasonUnexpectedToolCall, "unexpected_tool_call", llm.StopKindIncomplete},
		{genai.FinishReasonTooManyToolCalls, "too_many_tool_calls", llm.StopKindIncomplete},
		{genai.FinishReasonOther, "other", llm.StopKindOther},
		{"", "other", llm.StopKindOther},
	}
	for _, tc := range cases {
		got := convertFinishReason(tc.reason)
		assert.Equal(t, got, tc.want)
		assert.Equal(t, llm.ClassifyStopReason(got), tc.kind)
	}
}
