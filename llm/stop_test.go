package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestClassifyStopReason(t *testing.T) {
	cases := map[string]StopKind{
		"end_turn":                      StopKindFinished,
		"stop":                          StopKindFinished,
		"stop_sequence":                 StopKindFinished,
		"tool_use":                      StopKindToolUse,
		"tool_calls":                    StopKindToolUse,
		"max_tokens":                    StopKindOutputLimit,
		"length":                        StopKindOutputLimit,
		"model_length":                  StopKindOutputLimit,
		"model_context_window_exceeded": StopKindContextLimit,
		"refusal":                       StopKindRefusal,
		"content_filter":                StopKindRefusal,
		"safety":                        StopKindRefusal,
		"SAFETY":                        StopKindRefusal,
		"pause_turn":                    StopKindPause,
		"incomplete":                    StopKindIncomplete,
		"cancelled":                     StopKindIncomplete,
		"unexpected_tool_call":          StopKindIncomplete,
		"too_many_tool_calls":           StopKindIncomplete,
		"other":                         StopKindOther,
		"language":                      StopKindOther,
		"some_new_reason":               StopKindOther,
		"":                              StopKindOther,
	}
	for reason, want := range cases {
		assert.Equal(t, ClassifyStopReason(reason), want, "reason %q", reason)
	}
}
