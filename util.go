package dive

import (
	"fmt"
	"strings"
	"time"

	"github.com/diveagents/dive/llm"
	petname "github.com/dustinkirkland/golang-petname"
)

func init() {
	petname.NonDeterministicMode()
}

func FormatMessages(messages []*llm.Message) string {
	var lines []string
	for i, message := range messages {
		lines = append(lines, "========")
		lines = append(lines, fmt.Sprintf("Message %d | Role: %s | Contents: %d", i+1, message.Role, len(message.Content)))
		for j, content := range message.Content {
			lines = append(lines, " | ----")
			lines = append(lines, fmt.Sprintf(" | Content %d (%s)", j+1, content.Type))
			switch content.Type {
			case llm.ContentTypeText:
				contentLines := strings.Split(content.Text, "\n")
				for _, cl := range contentLines {
					lines = append(lines, fmt.Sprintf(" | %s", cl))
				}
			case llm.ContentTypeImage:
				lines = append(lines, fmt.Sprintf(" | <data len=%d>", len(content.Data)))
			case llm.ContentTypeToolUse:
				lines = append(lines, fmt.Sprintf(" | id=%s name=%s", content.ID, content.Name))
				lines = append(lines, fmt.Sprintf(" | %s", string(content.Input)))
			case llm.ContentTypeToolResult:
				lines = append(lines, fmt.Sprintf(" | id=%s name=%s", content.ToolUseID, content.Name))
				var truncated bool
				resultLines := strings.Split(content.Text, "\n")
				if len(resultLines) > 4 {
					resultLines = resultLines[:4]
					truncated = true
				}
				for _, rl := range resultLines {
					lines = append(lines, fmt.Sprintf(" | %s", rl))
				}
				if truncated {
					lines = append(lines, " | ...")
				}
			default:
				lines = append(lines, " | <unknown>")
			}
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

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

func RandomName() string {
	return fmt.Sprintf("%s-%s", petname.Adjective(), petname.Name())
}

func DateString(t time.Time) string {
	prompt := "The current date is " + t.Format("January 2, 2006") + "."
	prompt += " It is a " + t.Format("Monday") + "."
	return prompt
}

func AgentNames(agents []Agent) []string {
	var agentNames []string
	for _, agent := range agents {
		agentNames = append(agentNames, agent.Name())
	}
	return agentNames
}

// func TaskNames(tasks []Task) []string {
// 	var taskNames []string
// 	for _, task := range tasks {
// 		taskNames = append(taskNames, task.Name())
// 	}
// 	return taskNames
// }
