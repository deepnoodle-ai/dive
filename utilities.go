package dive

import (
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

func TruncateText(text string, maxWords int) string {
	// Split into lines while preserving newlines
	lines := strings.Split(text, "\n")
	wordCount := 0
	var result []string
	// Process each line
	for _, line := range lines {
		words := strings.Fields(line)
		// If we haven't reached maxWords, add words from this line
		if wordCount < maxWords {
			remaining := maxWords - wordCount
			if len(words) <= remaining {
				// Add entire line if it fits
				if len(words) > 0 {
					result = append(result, line)
				} else {
					// Preserve empty lines
					result = append(result, "")
				}
				wordCount += len(words)
			} else {
				// Add partial line up to remaining words
				result = append(result, strings.Join(words[:remaining], " "))
				wordCount = maxWords
			}
		}
	}
	truncated := strings.Join(result, "\n")
	if wordCount >= maxWords {
		truncated += " ..."
	}
	return truncated
}

func dateString(t time.Time) string {
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
