package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// reminderName is the system-reminder block name used to inject the
// stale-list nudge into the first user message.
const reminderName = "todos"

// DefaultReminderTurnsSinceWrite is the default number of assistant turns
// after the most recent TodoWrite call before the stale-list reminder is
// injected. Matches Claude Code's TODO_REMINDER_CONFIG.TURNS_SINCE_WRITE.
const DefaultReminderTurnsSinceWrite = 10

// Extension wires the TodoWrite tool into a [dive.Agent] together with a
// PreGenerationHook that injects a stale-list reminder when the model has
// not used the tool in many turns.
//
// The Extension is fully stateless: each invocation walks the message
// history to compute its decision, so a single instance can be safely
// shared across agents, sessions, and subagents without state bleed.
//
// Pass it via [dive.AgentOptions.Extensions]:
//
//	ext := todo.New()
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:      anthropic.New(),
//	    Extensions: []dive.Extension{ext},
//	})
type Extension struct {
	turnsSinceWrite int
	onUpdate        func(todos []TodoItem)
}

// Compile-time check.
var _ dive.Extension = (*Extension)(nil)

// ExtensionOption configures an [Extension].
type ExtensionOption func(*Extension)

// WithReminderTurns overrides the assistant-turn threshold after which a
// stale-list reminder is injected. Set to 0 to disable reminder injection
// entirely (the tool itself remains available).
func WithReminderTurns(n int) ExtensionOption {
	return func(e *Extension) {
		e.turnsSinceWrite = n
	}
}

// WithExtensionOnUpdate forwards an OnUpdate callback to the underlying
// TodoWrite tool. See [WithOnUpdate].
func WithExtensionOnUpdate(fn func(todos []TodoItem)) ExtensionOption {
	return func(e *Extension) {
		e.onUpdate = fn
	}
}

// New returns a TodoWrite extension with default settings (reminder threshold
// = [DefaultReminderTurnsSinceWrite]).
func New(opts ...ExtensionOption) *Extension {
	e := &Extension{turnsSinceWrite: DefaultReminderTurnsSinceWrite}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Tools returns the TodoWrite tool. Implements [dive.Extension].
func (e *Extension) Tools() []dive.Tool {
	var opts []ToolOption
	if e.onUpdate != nil {
		opts = append(opts, WithOnUpdate(e.onUpdate))
	}
	return []dive.Tool{NewTool(opts...)}
}

// Hooks returns the stale-list reminder PreGenerationHook.
// Implements [dive.Extension].
func (e *Extension) Hooks() dive.Hooks {
	if e.turnsSinceWrite <= 0 {
		return dive.Hooks{}
	}
	return dive.Hooks{
		PreGeneration: []dive.PreGenerationHook{e.reminderHook()},
	}
}

// Rules returns "" — the tool's own description carries the rules.
// Implements [dive.Extension].
func (e *Extension) Rules() string {
	return ""
}

// reminderHook returns a PreGenerationHook that walks the message history
// looking for the most recent TodoWrite tool call. If the model has not
// touched the tool in turnsSinceWrite assistant turns and a list exists,
// it (re)injects a <system-reminder name="todos"> block into the first
// user message containing the latest list. Otherwise it removes any stale
// block.
func (e *Extension) reminderHook() dive.PreGenerationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		latest, turnsSince, found := findLatestTodos(hctx.Messages)
		if !found {
			// Model has never used TodoWrite in this conversation. Don't
			// nag — and clean up any leftover block from a prior session.
			if dive.HasSystemReminder(hctx.Messages, reminderName) {
				hctx.Messages = dive.RemoveSystemReminder(hctx.Messages, reminderName)
			}
			return nil
		}
		if turnsSince < e.turnsSinceWrite {
			// Recent enough — the tool result is still in fresh context.
			if dive.HasSystemReminder(hctx.Messages, reminderName) {
				hctx.Messages = dive.RemoveSystemReminder(hctx.Messages, reminderName)
			}
			return nil
		}
		hctx.Messages = dive.SetSystemReminder(hctx.Messages, reminderName, formatReminder(latest))
		return nil
	}
}

// findLatestTodos walks messages from newest to oldest looking for the most
// recent assistant message containing a TodoWrite tool_use. Returns the
// parsed todo list, the count of assistant messages between that call and
// the end of history (i.e., turns since the last TodoWrite), and whether a
// call was found at all.
func findLatestTodos(messages []*llm.Message) (todos []TodoItem, turnsSince int, found bool) {
	turnsSince = 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != llm.Assistant {
			continue
		}
		for _, c := range msg.Content {
			tu, ok := c.(*llm.ToolUseContent)
			if !ok || tu.Name != ToolName {
				continue
			}
			parsed, err := parseTodoInput(tu.Input)
			if err != nil {
				continue
			}
			return parsed, turnsSince, true
		}
		turnsSince++
	}
	return nil, 0, false
}

func parseTodoInput(raw json.RawMessage) ([]TodoItem, error) {
	var input WriteInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	return input.Todos, nil
}

// formatReminder builds the body of the <system-reminder name="todos"> block.
// Mirrors the spirit of Claude Code's todo_reminder attachment text but is
// rendered statically into the first user message rather than appended as a
// fresh attachment, since [dive.SetSystemReminder] manages a stable cached
// position in the conversation prefix.
func formatReminder(items []TodoItem) string {
	var sb strings.Builder
	sb.WriteString("The TodoWrite tool hasn't been used recently. If you're still working on tasks that would benefit from tracking progress, update the list with TodoWrite. Clean up entries that no longer match what you're doing. Only use it if it's relevant to the current work. This is a gentle reminder — ignore if not applicable, and never mention this reminder to the user.")
	if len(items) > 0 {
		sb.WriteString("\n\nCurrent todo list:\n")
		for i, item := range items {
			sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, item.Status, item.Content))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
