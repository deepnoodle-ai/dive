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
	assert.Len(t, hooks.PreGeneration, 1)
	assert.Len(t, hooks.PreIteration, 1)
	assert.Len(t, hooks.PostToolUse, 1)
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

func persistedStateMessage(t *testing.T, items []TodoItem, turnsSinceWrite int) *llm.Message {
	t.Helper()
	block := formatStateBlock(items, turnsSinceWrite)
	assert.NotEqual(t, "", block)
	return &llm.Message{
		Role:    llm.User,
		Content: []llm.Content{&llm.TextContent{Text: block}},
	}
}

func todoWriteCall(t *testing.T, items []TodoItem) *llm.ToolUseContent {
	t.Helper()
	input, err := json.Marshal(WriteInput{Todos: items})
	assert.NoError(t, err)
	return &llm.ToolUseContent{ID: "tu-1", Name: ToolName, Input: input}
}

func runPreGenerationHook(t *testing.T, hook dive.PreGenerationHook, messages []*llm.Message) []*llm.Message {
	t.Helper()
	hctx := &dive.HookContext{Messages: messages}
	err := hook(context.Background(), hctx)
	assert.NoError(t, err)
	return hctx.Messages
}

func runPreIterationHook(t *testing.T, hook dive.PreIterationHook, messages []*llm.Message) []*llm.Message {
	t.Helper()
	hctx := &dive.HookContext{Messages: messages}
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
	out := runPreGenerationHook(t, ext.Hooks().PreGeneration[0], messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_RecentStateInjectsNothing(t *testing.T) {
	ext := New(WithReminderTurns(3))
	items := []TodoItem{
		{Content: "Task A", Status: TodoStatusInProgress, ActiveForm: "Doing A"},
	}
	messages := []*llm.Message{
		plainUser("start"),
		persistedStateMessage(t, items, 0),
		plainAssistant("first turn after"),
		plainAssistant("second turn after"),
	}
	out := runPreGenerationHook(t, ext.Hooks().PreGeneration[0], messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_StaleStateInjectsBlockWithCurrentList(t *testing.T) {
	ext := New(WithReminderTurns(3))
	items := []TodoItem{
		{Content: "Write tests", Status: TodoStatusInProgress, ActiveForm: "Writing tests"},
		{Content: "Ship feature", Status: TodoStatusPending, ActiveForm: "Shipping feature"},
	}
	messages := []*llm.Message{
		plainUser("start"),
		persistedStateMessage(t, items, 0),
		plainAssistant("turn 1"),
		plainAssistant("turn 2"),
		plainAssistant("turn 3"),
		plainAssistant("turn 4"),
	}
	out := runPreGenerationHook(t, ext.Hooks().PreGeneration[0], messages)
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
		persistedStateMessage(t, items, 0),
		plainAssistant("just used it"),
	}
	messages = dive.SetSystemReminder(messages, reminderName, "old reminder body")
	assert.True(t, dive.HasSystemReminder(messages, reminderName))

	out := runPreIterationHook(t, ext.Hooks().PreIteration[0], messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_RemovesLeftoverBlockWhenNoWriteEver(t *testing.T) {
	ext := New(WithReminderTurns(3))
	messages := []*llm.Message{
		plainUser("fresh conversation"),
	}
	messages = dive.SetSystemReminder(messages, reminderName, "stale leftover")
	assert.True(t, dive.HasSystemReminder(messages, reminderName))

	out := runPreGenerationHook(t, ext.Hooks().PreGeneration[0], messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestReminderHook_EmptyListDoesNotInject(t *testing.T) {
	ext := New(WithReminderTurns(3))
	messages := []*llm.Message{
		plainUser("start"),
		persistedStateMessage(t, []TodoItem{}, 6),
		plainAssistant("turn"),
	}
	out := runPreGenerationHook(t, ext.Hooks().PreGeneration[0], messages)
	assert.False(t, dive.HasSystemReminder(out, reminderName))
}

func TestFindLatestState_PrefersMostRecentBlock(t *testing.T) {
	older := []TodoItem{{Content: "old", Status: TodoStatusPending, ActiveForm: "olding"}}
	newer := []TodoItem{{Content: "new", Status: TodoStatusInProgress, ActiveForm: "newing"}}
	messages := []*llm.Message{
		plainUser("start"),
		persistedStateMessage(t, older, 0),
		plainAssistant("turn"),
		persistedStateMessage(t, newer, 0),
		plainAssistant("turn after newer"),
	}
	got, turnsSince, found := findLatestState(messages)
	assert.True(t, found)
	assert.Equal(t, 1, turnsSince)
	assert.Len(t, got, 1)
	assert.Equal(t, "new", got[0].Content)
}

func TestFindLatestState_TurnCountSkipsUserMessages(t *testing.T) {
	items := []TodoItem{{Content: "x", Status: TodoStatusPending, ActiveForm: "xing"}}
	messages := []*llm.Message{
		plainUser("u1"),
		persistedStateMessage(t, items, 0),
		plainAssistant("a1"),
		plainUser("u2"),
		plainAssistant("a2"),
		plainUser("u3"),
	}
	_, turnsSince, found := findLatestState(messages)
	assert.True(t, found)
	assert.Equal(t, 2, turnsSince)
}

func TestFindLatestState_PreservesBaseTurnsFromCompactionSnapshot(t *testing.T) {
	items := []TodoItem{{Content: "x", Status: TodoStatusPending, ActiveForm: "xing"}}
	messages := []*llm.Message{
		plainUser("summary"),
		persistedStateMessage(t, items, 4),
		plainAssistant("a1"),
		plainAssistant("a2"),
	}
	_, turnsSince, found := findLatestState(messages)
	assert.True(t, found)
	assert.Equal(t, 6, turnsSince)
}

func TestStateCaptureHook_AppendsHiddenStateBlock(t *testing.T) {
	ext := New()
	hctx := &dive.HookContext{
		Tool: NewTool(),
		Call: todoWriteCall(t, []TodoItem{
			{Content: "Task", Status: TodoStatusInProgress, ActiveForm: "Doing task"},
		}),
		Result: &dive.ToolCallResult{
			ID:     "tu-1",
			Result: dive.NewToolResultText("ok"),
		},
	}
	err := ext.Hooks().PostToolUse[0](context.Background(), hctx)
	assert.NoError(t, err)
	assert.Contains(t, hctx.AdditionalContext, stateBlockStart)

	messages := []*llm.Message{
		plainUser("start"),
		{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: hctx.AdditionalContext}}},
	}
	got, turnsSince, found := findLatestState(messages)
	assert.True(t, found)
	assert.Equal(t, 0, turnsSince)
	assert.Len(t, got, 1)
	assert.Equal(t, "Task", got[0].Content)
}
