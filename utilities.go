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
		resultContent := &llm.ToolResultContent{
			ToolUseID: callResult.ID,
			Content:   content,
			IsError:   callResult.Error != nil || isError,
		}
		results[i] = resultContent
	}
	return results
}

func getAdditionalContextContent(callResults []*ToolCallResult) []*llm.TextContent {
	var contexts []*llm.TextContent
	for _, callResult := range callResults {
		if callResult.AdditionalContext != "" {
			contexts = append(contexts, &llm.TextContent{
				Text: callResult.AdditionalContext,
			})
		}
	}
	return contexts
}

func toolResultsBeforeAuxiliaryContent(content []llm.Content) []llm.Content {
	if len(content) < 2 {
		return content
	}
	var results []llm.Content
	var auxiliary []llm.Content
	for _, c := range content {
		if _, ok := c.(*llm.ToolResultContent); ok {
			results = append(results, c)
		} else {
			auxiliary = append(auxiliary, c)
		}
	}
	if len(results) == 0 || len(auxiliary) == 0 {
		return content
	}
	out := make([]llm.Content, 0, len(content))
	out = append(out, results...)
	out = append(out, auxiliary...)
	return out
}

// Ptr returns a pointer to the given value. This is useful for setting
// optional pointer fields in structs like ModelSettings.
func Ptr[T any](t T) *T {
	return &t
}
