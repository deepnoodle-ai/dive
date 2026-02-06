package dive

import (
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// DateTimeString returns a human-readable string describing the given time,
// including the date, time of day, and day of the week.
func DateTimeString(t time.Time) string {
	prompt := "The current date is " + t.Format("January 2, 2006") + "."
	prompt += " The current time is " + t.Format("3:04 PM") + "."
	prompt += " It is a " + t.Format("Monday") + "."
	return prompt
}

func getToolResultContent(callResults []*ToolCallResult) []*llm.ToolResultContent {
	results := make([]*llm.ToolResultContent, len(callResults))
	for i, callResult := range callResults {
		var content any
		var isError bool
		if callResult.Result != nil {
			content = callResult.Result.Content
			isError = callResult.Result.IsError
		}
		// IsError is true if either the tool crashed (Error) or the tool
		// reported a protocol-level error (Result.IsError).
		results[i] = &llm.ToolResultContent{
			ToolUseID: callResult.ID,
			Content:   content,
			IsError:   callResult.Error != nil || isError,
		}
	}
	return results
}

// Ptr returns a pointer to the given value. This is useful for setting
// optional pointer fields in structs like ModelSettings.
func Ptr[T any](t T) *T {
	return &t
}
