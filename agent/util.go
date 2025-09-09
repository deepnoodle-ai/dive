package agent

import (
	"os"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
	"github.com/deepnoodle-ai/dive/llm/providers/google"
	"github.com/deepnoodle-ai/dive/llm/providers/grok"
	"github.com/deepnoodle-ai/dive/llm/providers/groq"
	"github.com/deepnoodle-ai/dive/llm/providers/openai"
)

func detectProvider() (llm.LLM, bool) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return anthropic.New(), true
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return openai.New(), true
	}
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		return google.New(), true
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return google.New(), true
	}
	if key := os.Getenv("XAI_API_KEY"); key != "" {
		return grok.New(), true
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

func ptr[T any](t T) *T {
	return &t
}
