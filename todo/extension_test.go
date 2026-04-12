package todo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestExtension_ImplementsDiveExtension(t *testing.T) {
	var _ dive.Extension = New()
}

func TestExtension_TodoWriteToolIsRegistered(t *testing.T) {
	ext := New()
	tools := ext.Tools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "TodoWrite", tools[0].Name())
}

func TestExtension_RulesEmpty(t *testing.T) {
	// Description on the tool itself carries the rules; Extension.Rules()
	// must not duplicate them into the system prompt.
	assert.Equal(t, "", New().Rules())
}

func TestExtension_HooksDisabledWhenThresholdZero(t *testing.T) {
	ext := New(WithReminderTurns(0))
	hooks := ext.Hooks()
	assert.Len(t, hooks.PreGeneration, 0)
}

// helper: build an assistant message with a TodoWrite tool_use containing
// the given list (encoded as JSON).
func todoWriteCall(t *testing.T, items []TodoItem) *llm.Message {
	t.Helper()
	input, err := json.Marshal(WriteInput{Todos: items})
	assert.NoError(t, err)
	return &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ToolUseContent{ID: "tu-1", Name: ToolName, Input: input},
		},
	}
}

func plainAssistant(text string) *llm.Message {
	return &llm.Message{
		Role:    llm.Assistant,
		Content: []llm.Content{&llm.TextContent{Text: text}},
	}
}

func plainUser(text string) *llm.Message {
	return &llm.Message{
		Role:    llm.User,
		Content: []llm.Content{&llm.TextContent{Text: text}},
	}
}

func runHook(t *testing.T, ext *Extension, messages []*llm.Message) []*llm.Message {
	t.Helper()
	hctx := &dive.HookContext{Messages: messages}
	hook := ext.Hooks().PreGeneration[0]
	err := hook(context.Background(), hctx)
	assert.NoError(t, err)
	return hctx.Messages
}

func TestReminderHook_NoTodoWriteEverInjectsNothing(t *testing.T) {
	ext := New(WithReminderTurns(3))
	messages := []*llm.Message{
		plainUser("hi"),
		plainAssistant("hello"),
		plainUser("how are you"),
		plainAssistant("fine"),
	}
	out := runHook(t, ext, messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_RecentWriteInjectsNothing(t *testing.T) {
	ext := New(WithReminderTurns(3))
	items := []TodoItem{
		{Content: "Task A", Status: TodoStatusInProgress, ActiveForm: "Doing A"},
	}
	messages := []*llm.Message{
		plainUser("start"),
		todoWriteCall(t, items),
		plainAssistant("first turn after"),
		plainAssistant("second turn after"),
	}
	out := runHook(t, ext, messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_StaleWriteInjectsBlockWithCurrentList(t *testing.T) {
	ext := New(WithReminderTurns(3))
	items := []TodoItem{
		{Content: "Write tests", Status: TodoStatusInProgress, ActiveForm: "Writing tests"},
		{Content: "Ship feature", Status: TodoStatusPending, ActiveForm: "Shipping feature"},
	}
	messages := []*llm.Message{
		plainUser("start"),
		todoWriteCall(t, items),
		plainAssistant("turn 1"),
		plainAssistant("turn 2"),
		plainAssistant("turn 3"),
		plainAssistant("turn 4"),
	}
	out := runHook(t, ext, messages)
	assert.True(t, dive.HasSystemReminder(out, reminderName))

	// The block contains both the canonical nudge and the current list.
	first := out[0]
	var blockText string
	for _, c := range first.Content {
		if tc, ok := c.(*llm.TextContent); ok {
			blockText = tc.Text
		}
	}
	assert.Contains(t, blockText, "TodoWrite tool hasn't been used recently")
	assert.Contains(t, blockText, "Write tests")
	assert.Contains(t, blockText, "Ship feature")
	assert.Contains(t, blockText, "in_progress")
	assert.Contains(t, blockText, "pending")
}

func TestReminderHook_RemovesStaleBlockWhenWriteIsRecentAgain(t *testing.T) {
	ext := New(WithReminderTurns(3))
	items := []TodoItem{
		{Content: "Task", Status: TodoStatusCompleted, ActiveForm: "Doing task"},
	}
	// Pre-seed the first user message with an existing reminder block.
	messages := []*llm.Message{
		plainUser("start"),
		todoWriteCall(t, items),
		plainAssistant("just used it"),
	}
	messages = dive.SetSystemReminder(messages, reminderName, "old reminder body")
	assert.True(t, dive.HasSystemReminder(messages, reminderName))

	out := runHook(t, ext, messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_RemovesLeftoverBlockWhenNoWriteEver(t *testing.T) {
	ext := New(WithReminderTurns(3))
	messages := []*llm.Message{
		plainUser("fresh conversation"),
	}
	messages = dive.SetSystemReminder(messages, reminderName, "stale leftover")
	assert.True(t, dive.HasSystemReminder(messages, reminderName))

	out := runHook(t, ext, messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestFindLatestTodos_PrefersMostRecentCall(t *testing.T) {
	older := []TodoItem{{Content: "old", Status: TodoStatusPending, ActiveForm: "olding"}}
	newer := []TodoItem{{Content: "new", Status: TodoStatusInProgress, ActiveForm: "newing"}}
	messages := []*llm.Message{
		plainUser("start"),
		todoWriteCall(t, older),
		plainAssistant("turn"),
		todoWriteCall(t, newer),
		plainAssistant("turn after newer"),
	}
	got, turnsSince, found := findLatestTodos(messages)
	assert.True(t, found)
	assert.Equal(t, 1, turnsSince)
	assert.Len(t, got, 1)
	assert.Equal(t, "new", got[0].Content)
}

func TestFindLatestTodos_TurnCountSkipsUserMessages(t *testing.T) {
	items := []TodoItem{{Content: "x", Status: TodoStatusPending, ActiveForm: "xing"}}
	messages := []*llm.Message{
		plainUser("u1"),
		todoWriteCall(t, items),
		plainAssistant("a1"),
		plainUser("u2"),
		plainAssistant("a2"),
		plainUser("u3"),
	}
	_, turnsSince, found := findLatestTodos(messages)
	assert.True(t, found)
	// Two assistant messages after the TodoWrite call. User messages and
	// the call message itself do not count.
	assert.Equal(t, 2, turnsSince)
}
