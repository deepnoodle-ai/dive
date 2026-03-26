package dive

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSetSystemReminder_InsertsIntoFirstUserMessage(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = SetSystemReminder(messages, "skills", "Available skills:\n- reviewer")

	text := messages[0].Content[0].(*llm.TextContent).Text
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, `<system-reminder name="skills">`)
	assert.Contains(t, text, "Available skills:")
	assert.Contains(t, text, "</system-reminder>")
}

func TestSetSystemReminder_ReplacesExistingBlock(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello\n<system-reminder name=\"skills\">\nold content\n</system-reminder>"),
	}

	messages = SetSystemReminder(messages, "skills", "new content")

	text := messages[0].Content[0].(*llm.TextContent).Text
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "new content")
	assert.NotContains(t, text, "old content")
	// Should only have one block
	assert.Equal(t, 1, strings.Count(text, `<system-reminder name="skills">`))
}

func TestSetSystemReminder_MultipleNamedBlocks(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = SetSystemReminder(messages, "skills", "skill catalog")
	messages = SetSystemReminder(messages, "context", "CLAUDE.md contents")

	text := messages[0].Content[0].(*llm.TextContent).Text
	assert.Contains(t, text, `<system-reminder name="skills">`)
	assert.Contains(t, text, "skill catalog")
	assert.Contains(t, text, `<system-reminder name="context">`)
	assert.Contains(t, text, "CLAUDE.md contents")
}

func TestSetSystemReminder_ReplaceOnlyTargetedBlock(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = SetSystemReminder(messages, "skills", "original skills")
	messages = SetSystemReminder(messages, "context", "context info")
	messages = SetSystemReminder(messages, "skills", "updated skills")

	text := messages[0].Content[0].(*llm.TextContent).Text
	assert.Contains(t, text, "updated skills")
	assert.NotContains(t, text, "original skills")
	assert.Contains(t, text, "context info") // untouched
}

func TestSetSystemReminder_NoUserMessage(t *testing.T) {
	messages := []*llm.Message{
		{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hi"}}},
	}

	messages = SetSystemReminder(messages, "skills", "skill catalog")

	// Should have created a user message at the front
	assert.Equal(t, 2, len(messages))
	assert.Equal(t, llm.User, messages[0].Role)
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

	// System and assistant messages should be unchanged
	assert.Equal(t, "System", messages[0].Content[0].(*llm.TextContent).Text)
	assert.Equal(t, "Assistant", messages[1].Content[0].(*llm.TextContent).Text)
	// Catalog should be in the user message
	assert.Contains(t, messages[2].Content[0].(*llm.TextContent).Text, "catalog")
}

func TestRemoveSystemReminder(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello\n<system-reminder name=\"skills\">\ncatalog\n</system-reminder>"),
	}

	messages = RemoveSystemReminder(messages, "skills")

	text := messages[0].Content[0].(*llm.TextContent).Text
	assert.NotContains(t, text, "system-reminder")
	assert.Contains(t, text, "Hello")
}

func TestRemoveSystemReminder_LeavesOtherBlocks(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello\n<system-reminder name=\"skills\">\ncatalog\n</system-reminder>\n<system-reminder name=\"context\">\nctx\n</system-reminder>"),
	}

	messages = RemoveSystemReminder(messages, "skills")

	text := messages[0].Content[0].(*llm.TextContent).Text
	assert.NotContains(t, text, "catalog")
	assert.Contains(t, text, `<system-reminder name="context">`)
	assert.Contains(t, text, "ctx")
}

func TestRemoveSystemReminder_NoBlock(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
	}

	messages = RemoveSystemReminder(messages, "skills")
	assert.Equal(t, "Hello", messages[0].Content[0].(*llm.TextContent).Text)
}

func TestHasSystemReminder(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello\n<system-reminder name=\"skills\">\ncatalog\n</system-reminder>"),
	}

	assert.True(t, HasSystemReminder(messages, "skills"))
	assert.False(t, HasSystemReminder(messages, "context"))
}

func TestHasSystemReminder_NoMessages(t *testing.T) {
	assert.False(t, HasSystemReminder(nil, "skills"))
}

