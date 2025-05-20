package agent

import (
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/groq"
	"github.com/diveagents/dive/llm/providers/openai"
)

func detectProvider() (llm.LLM, bool) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return anthropic.New(), true
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return openai.New(), true
	}
	if key := os.Getenv("GROQ_API_KEY"); key != "" {
		return groq.New(), true
	}
	return nil, false
}

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
