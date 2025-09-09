package agent

import (
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

func getToolResultContent(callResults []*dive.ToolCallResult) []*llm.ToolResultContent {
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
