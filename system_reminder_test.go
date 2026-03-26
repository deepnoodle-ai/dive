package dive

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSetSystemReminder_AddsNewContentBlock(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = SetSystemReminder(messages, "skills", "Available skills:\n- reviewer")

	// User text should be unchanged
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
	// Reminder should be a separate content block
	assert.Equal(t, 2, len(messages[0].Content))
	reminderText := messages[0].Content[1].(*llm.TextContent).Text
	assert.Contains(t, reminderText, `<system-reminder name="skills">`)
	assert.Contains(t, reminderText, "Available skills:")
	assert.Contains(t, reminderText, "</system-reminder>")
}

func TestSetSystemReminder_ReplacesExistingBlock(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.User, Content: []llm.Content{
			&llm.TextContent{Text: "Hello"},
			&llm.TextContent{Text: "<system-reminder name=\"skills\">\nold content\n</system-reminder>"},
		}},
	}

	messages = SetSystemReminder(messages, "skills", "new content")

	// Still two content blocks
	assert.Equal(t, 2, len(messages[0].Content))
	// User text unchanged
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
	// Reminder replaced
	reminderText := messages[0].Content[1].(*llm.TextContent).Text
	assert.Contains(t, reminderText, "new content")
	assert.NotContains(t, reminderText, "old content")
}

func TestSetSystemReminder_MultipleNamedBlocks(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = SetSystemReminder(messages, "skills", "skill catalog")
	messages = SetSystemReminder(messages, "context", "CLAUDE.md contents")

	// Three content blocks: user text + two reminders
	assert.Equal(t, 3, len(messages[0].Content))
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
	assert.Contains(t, messages[0].Content[1].(*llm.TextContent).Text, "skill catalog")
	assert.Contains(t, messages[0].Content[2].(*llm.TextContent).Text, "CLAUDE.md contents")
}

func TestSetSystemReminder_ReplaceOnlyTargetedBlock(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = SetSystemReminder(messages, "skills", "original skills")
	messages = SetSystemReminder(messages, "context", "context info")
	messages = SetSystemReminder(messages, "skills", "updated skills")

	// Still three blocks
	assert.Equal(t, 3, len(messages[0].Content))
	// Skills block updated
	assert.Contains(t, messages[0].Content[1].(*llm.TextContent).Text, "updated skills")
	assert.NotContains(t, messages[0].Content[1].(*llm.TextContent).Text, "original skills")
	// Context block untouched
	assert.Contains(t, messages[0].Content[2].(*llm.TextContent).Text, "context info")
}

func TestSetSystemReminder_NoUserMessage(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hi"}}},
	}

	messages = SetSystemReminder(messages, "skills", "skill catalog")

	// Should have created a user message at the front
	assert.Equal(t, 2, len(messages))
	assert.Equal(t, llm.User, messages[0].Role)
	assert.Equal(t, 1, len(messages[0].Content))
	assert.Contains(t, messages[0].Content[0].(*llm.TextContent).Text, "skill catalog")
}

func TestSetSystemReminder_EmptyMessages(t *testing.T) {
	var messages []*llm.Message
	messages = SetSystemReminder(messages, "skills", "catalog")

	assert.Equal(t, 1, len(messages))
	assert.Equal(t, llm.User, messages[0].Role)
}

func TestSetSystemReminder_SkipsNonUserMessages(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.System, Content: []llm.Content{&llm.TextContent{Text: "System"}}},
		{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Assistant"}}},
		llm.NewUserTextMessage("User message"),
	}

	messages = SetSystemReminder(messages, "skills", "catalog")

	// System and assistant messages unchanged
	assert.Equal(t, "System", messages[0].Content[0].(*llm.TextContent).Text)
	assert.Equal(t, "Assistant", messages[1].Content[0].(*llm.TextContent).Text)
	// Reminder is a separate content block on the user message
	assert.Equal(t, 2, len(messages[2].Content))
	assert.Equal(t, "User message", messages[2].Content[0].(*llm.TextContent).Text)
	assert.Contains(t, messages[2].Content[1].(*llm.TextContent).Text, "catalog")
}

func TestRemoveSystemReminder(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.User, Content: []llm.Content{
			&llm.TextContent{Text: "Hello"},
			&llm.TextContent{Text: "<system-reminder name=\"skills\">\ncatalog\n</system-reminder>"},
		}},
	}

	messages = RemoveSystemReminder(messages, "skills")

	// Only user text remains
	assert.Equal(t, 1, len(messages[0].Content))
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
}

func TestRemoveSystemReminder_LeavesOtherBlocks(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.User, Content: []llm.Content{
			&llm.TextContent{Text: "Hello"},
			&llm.TextContent{Text: "<system-reminder name=\"skills\">\ncatalog\n</system-reminder>"},
			&llm.TextContent{Text: "<system-reminder name=\"context\">\nctx\n</system-reminder>"},
		}},
	}

	messages = RemoveSystemReminder(messages, "skills")

	assert.Equal(t, 2, len(messages[0].Content))
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
	assert.Contains(t, messages[0].Content[1].(*llm.TextContent).Text, "ctx")
}

func TestRemoveSystemReminder_NoBlock(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = RemoveSystemReminder(messages, "skills")
	assert.Equal(t, 1, len(messages[0].Content))
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
}

func TestHasSystemReminder(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.User, Content: []llm.Content{
			&llm.TextContent{Text: "Hello"},
			&llm.TextContent{Text: "<system-reminder name=\"skills\">\ncatalog\n</system-reminder>"},
		}},
	}

	assert.True(t, HasSystemReminder(messages, "skills"))
	assert.False(t, HasSystemReminder(messages, "context"))
}

func TestHasSystemReminder_NoMessages(t *testing.T) {
	assert.False(t, HasSystemReminder(nil, "skills"))
}
