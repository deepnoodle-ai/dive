package llm

import "strings"

// StopKind groups the stop reasons providers report by what an agent does
// with the response. Response.StopReason keeps the provider's raw value;
// ClassifyStopReason maps it onto a StopKind.
type StopKind string

const (
	// StopKindFinished is a response the model ended on its own: end_turn,
	// stop, stop_sequence. Its tool calls, if any, are meant to run.
	StopKindFinished StopKind = "finished"

	// StopKindToolUse is a response that ended in order to run its tool
	// calls.
	StopKindToolUse StopKind = "tool_use"

	// StopKindOutputLimit is a response cut off at the output token limit:
	// max_tokens, length. A tool call in it may be truncated.
	StopKindOutputLimit StopKind = "output_limit"

	// StopKindContextLimit is a response cut off because the conversation
	// filled the model's context window (Anthropic's
	// model_context_window_exceeded, Mistral's model_length).
	// Unlike an output limit, the same history cannot be sent again as is.
	StopKindContextLimit StopKind = "context_limit"

	// StopKindRefusal is a response the provider or model declined to
	// complete: refusal, content_filter, and Gemini's safety and recitation
	// reasons.
	StopKindRefusal StopKind = "refusal"

	// StopKindPause is a server tool loop the provider paused (pause_turn).
	// The conversation is sent again, unchanged, to let it continue.
	StopKindPause StopKind = "pause"

	// StopKindIncomplete is a response the provider ended early for a reason
	// it did not name as a limit or a refusal: an incomplete, cancelled,
	// timed-out or failed response, or a malformed tool call.
	StopKindIncomplete StopKind = "incomplete"

	// StopKindOther is a stop reason Dive does not recognize, including an
	// empty one. An agent treats such a response as finished only when it
	// carries no client tool call, and never runs a call on it.
	StopKindOther StopKind = "other"
)

// stopKinds maps every stop reason Dive's providers report, lowercased, to
// its kind. Adapters that normalize a provider's values say so where they
// map them; the raw spellings below are the ones that reach Response.
var stopKinds = map[string]StopKind{
	// Anthropic, and Ollama through its Anthropic-compatible API.
	"end_turn":                      StopKindFinished,
	"stop_sequence":                 StopKindFinished,
	"tool_use":                      StopKindToolUse,
	"max_tokens":                    StopKindOutputLimit,
	"model_context_window_exceeded": StopKindContextLimit,
	"refusal":                       StopKindRefusal,
	"pause_turn":                    StopKindPause,

	// Chat Completions (OpenAI, Mistral, OpenRouter, DeepInfra). The adapter
	// reports tool_calls and function_call as tool_use.
	"stop":           StopKindFinished,
	"length":         StopKindOutputLimit,
	"model_length":   StopKindContextLimit, // Mistral: the model's context length was reached
	"content_filter": StopKindRefusal,
	"tool_calls":     StopKindToolUse,
	"function_call":  StopKindToolUse,
	"error":          StopKindIncomplete,

	// OpenAI Responses, as the adapter maps a response's status and
	// incomplete_details (end_turn, tool_use, max_tokens and content_filter
	// are above).
	"incomplete": StopKindIncomplete,
	"cancelled":  StopKindIncomplete,
	"timeout":    StopKindIncomplete,

	// Gemini finish reasons, lowercased by the adapter (stop and max_tokens
	// are above). LANGUAGE, OTHER, NO_IMAGE, IMAGE_OTHER and
	// FINISH_REASON_UNSPECIFIED stay StopKindOther.
	"safety":                   StopKindRefusal,
	"recitation":               StopKindRefusal,
	"blocklist":                StopKindRefusal,
	"prohibited_content":       StopKindRefusal,
	"spii":                     StopKindRefusal,
	"image_safety":             StopKindRefusal,
	"image_prohibited_content": StopKindRefusal,
	"image_recitation":         StopKindRefusal,
	"malformed_function_call":  StopKindIncomplete,
	"unexpected_tool_call":     StopKindIncomplete,
	"too_many_tool_calls":      StopKindIncomplete,
}

// ClassifyStopReason maps a provider's stop reason, as Response.StopReason
// carries it, onto the kind an agent acts on. The match ignores case. A
// value Dive does not recognize, and an empty one, is StopKindOther.
func ClassifyStopReason(reason string) StopKind {
	if kind, ok := stopKinds[strings.ToLower(reason)]; ok {
		return kind
	}
	return StopKindOther
}
