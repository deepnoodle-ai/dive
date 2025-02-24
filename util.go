package dive

import (
	"fmt"
	"strings"

	"github.com/getstingrai/dive/llm"
)

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
				lines = append(lines, fmt.Sprintf(" | <unknown>"))
			}
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
