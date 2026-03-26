package dive

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// SetSystemReminder inserts or replaces a named <system-reminder> block as a
// separate text content block in the first user message. Each reminder gets
// its own content block, keeping user text pristine.
//
// This provides a stable, cache-friendly location for injecting context that
// persists across generations. Placing reminders in the first user message
// ensures they sit right after the system prompt in the conversation, forming
// a stable prefix for prompt caching.
//
// If there is no user message in the conversation, one is created and prepended.
//
// The block format is:
//
//	<system-reminder name="skills">
//	...content...
//	</system-reminder>
func SetSystemReminder(messages []*llm.Message, name, content string) []*llm.Message {
	block := formatBlock(name, content)

	// Find the first user message
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		// Look for an existing block with the same name
		for i, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			if isReminderBlock(tc.Text, name) {
				// Replace in place
				msg.Content[i] = &llm.TextContent{Text: block}
				return messages
			}
		}
		// No existing block — append a new content block
		msg.Content = append(msg.Content, &llm.TextContent{Text: block})
		return messages
	}

	// No user message — create one with the reminder as the sole content block
	return append([]*llm.Message{
		{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: block}}},
	}, messages...)
}

// RemoveSystemReminder removes a named <system-reminder> block from the
// first user message. Returns the messages unchanged if the block is not found.
func RemoveSystemReminder(messages []*llm.Message, name string) []*llm.Message {
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		for i, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			if isReminderBlock(tc.Text, name) {
				msg.Content = append(msg.Content[:i], msg.Content[i+1:]...)
				return messages
			}
		}
		return messages
	}
	return messages
}

// HasSystemReminder returns true if a named <system-reminder> block exists
// in the first user message.
func HasSystemReminder(messages []*llm.Message, name string) bool {
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			if isReminderBlock(tc.Text, name) {
				return true
			}
		}
		return false
	}
	return false
}

func formatBlock(name, content string) string {
	return fmt.Sprintf("<system-reminder name=%q>\n%s\n</system-reminder>", name, content)
}

// isReminderBlock returns true if the text is a system-reminder block with
// the given name. It checks for the opening tag at the start of the text.
func isReminderBlock(text, name string) bool {
	prefix := fmt.Sprintf("<system-reminder name=%q>", name)
	return strings.HasPrefix(text, prefix)
}
