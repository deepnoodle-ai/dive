package dive

import (
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

func dateTimeString(t time.Time) string {
	prompt := "The current date is " + t.Format("January 2, 2006") + "."
	prompt += " The current time is " + t.Format("3:04 PM") + "."
	prompt += " It is a " + t.Format("Monday") + "."
	return prompt
}

func getToolResultContent(callResults []*ToolCallResult) []*llm.ToolResultContent {
	results := make([]*llm.ToolResultContent, len(callResults))
	for i, callResult := range callResults {
		results[i] = &llm.ToolResultContent{
			ToolUseID: callResult.ID,
			Content:   callResult.Result.Content,
			IsError:   callResult.Error != nil || callResult.Result.IsError,
		}
	}
	return results
}

func ptr[T any](t T) *T {
	return &t
}
