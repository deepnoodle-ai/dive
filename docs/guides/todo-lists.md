# Todo Lists

Track task progress using Dive's `todo` package. The TodoWrite tool lets agents create and update task lists; the package's `Extension` also installs a stale-list reminder hook so the model is gently reminded to update its list when many turns have passed without one.

## Basic Usage

Wire the extension into your agent — it provides the tool, the reminder hook, and (optionally) an `OnUpdate` observer:

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    "github.com/deepnoodle-ai/dive/todo"
)

agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Task Manager",
    SystemPrompt: "Break complex tasks into steps and track progress with the TodoWrite tool.",
    Model:        anthropic.New(),
    Extensions:   []dive.Extension{todo.New()},
})
```

If you only want the tool with no reminder hook, register it directly:

```go
Tools: []dive.Tool{todo.NewTool()},
```

## Stale-List Reminder

`todo.Extension` installs a `PreGenerationHook` that walks the message history before each generation. If the model has not used `TodoWrite` in the last N assistant turns (default 10) and a list exists, the hook injects a `<system-reminder name="todos">` block into the first user message containing the latest list. When the model uses `TodoWrite` again, the next iteration removes the block automatically.

The hook is fully stateless — message history is the source of truth — so a single `Extension` instance is safe to share across agents, sessions, and subagents.

Tune the threshold (set to 0 to disable):

```go
todo.New(todo.WithReminderTurns(6))
todo.New(todo.WithReminderTurns(0)) // tool only, no reminder
```

## Tracking Progress

Two ways to observe updates externally.

### `TodoTracker` event handler (recommended for per-call observation)

```go
tracker := todo.NewTodoTracker()

resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Build a REST API"),
    dive.WithEventCallback(tracker.HandleEvent),
)

tracker.DisplayProgress(os.Stdout)
completed, inProgress, total := tracker.Progress()
status := tracker.FormatProgress() // "Running tests - 2/5"
```

### `OnUpdate` callback (push notification on every write)

```go
ext := todo.New(todo.WithExtensionOnUpdate(func(items []todo.TodoItem) {
    for _, t := range items {
        fmt.Printf("[%s] %s\n", t.Status, t.Content)
    }
}))
```

## Todo States

| Status        | Description               |
| ------------- | ------------------------- |
| `pending`     | Task not yet started      |
| `in_progress` | Currently being worked on |
| `completed`   | Task finished             |

## Todo Item Structure

Each todo has two text forms:

```go
type TodoItem struct {
    Content    string     // Imperative: "Run tests"
    Status     TodoStatus // pending, in_progress, completed
    ActiveForm string     // Present continuous: "Running tests"
}
```

## Best Practices

1. One `in_progress` task at a time
2. Mark a task as `in_progress` before starting work
3. Mark tasks `completed` immediately when done
4. Provide both `Content` and `ActiveForm` for better UX
5. Each `TodoWrite` call replaces the entire list (include all items)
6. Only mark tasks `completed` when truly finished
