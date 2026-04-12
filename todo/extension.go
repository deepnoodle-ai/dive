package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
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

// Hooks returns the stale-list reminder hooks and the state-capture hook.
// Implements [dive.Extension].
func (e *Extension) Hooks() dive.Hooks {
	stateHook := e.reminderHook()
	return dive.Hooks{
		PreGeneration: []dive.PreGenerationHook{stateHook},
		PreIteration:  []dive.PreIterationHook{stateHook},
		PostToolUse:   []dive.PostToolUseHook{e.stateCaptureHook()},
	}
}

// Rules returns "" — the tool's own description carries the rules.
// Implements [dive.Extension].
func (e *Extension) Rules() string {
	return ""
}

// reminderHook returns a hook that walks the message history looking for the
// most recent successful TodoWrite state block. If the model has not touched
// the tool in turnsSinceWrite assistant turns and a non-empty list exists, it
// injects a <system-reminder name="todos"> block into the first user message.
// Otherwise it removes any stale block.
func (e *Extension) reminderHook() func(context.Context, *dive.HookContext) error {
	return func(_ context.Context, hctx *dive.HookContext) error {
		latest, turnsSince, found := findLatestState(hctx.Messages)
		if !found || len(latest) == 0 {
			if dive.HasSystemReminder(hctx.Messages, reminderName) {
				hctx.Messages = dive.RemoveSystemReminder(hctx.Messages, reminderName)
			}
			return nil
		}
		if e.turnsSinceWrite <= 0 || turnsSince < e.turnsSinceWrite {
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

func (e *Extension) stateCaptureHook() dive.PostToolUseHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		if hctx.Tool == nil || hctx.Tool.Name() != ToolName || hctx.Result == nil || hctx.Result.Result == nil || hctx.Call == nil {
			return nil
		}
		// Defensive: the agent dispatches failed tool results to
		// PostToolUseFailure hooks rather than PostToolUse, so in practice
		// this branch will not fire. Guard anyway so the state block is
		// never persisted for an invocation the LLM sees as an error —
		// otherwise a bad write (e.g. validation failure) would silently
		// overwrite the last good state.
		if hctx.Result.Error != nil || hctx.Result.Result.IsError {
			return nil
		}
		var input WriteInput
		if err := json.Unmarshal(hctx.Call.Input, &input); err != nil {
			return nil
		}
		block := formatStateBlock(input.Todos, 0)
		if block == "" {
			return nil
		}
		if hctx.AdditionalContext != "" {
			hctx.AdditionalContext += "\n"
		}
		hctx.AdditionalContext += block
		return nil
	}
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
