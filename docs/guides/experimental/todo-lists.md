# Todo Lists

> **Experimental**: The TodoWrite tool is in `experimental/toolkit/`. The API may change.

Track task progress using Dive's todo functionality. The TodoWrite tool lets agents create and update task lists, with real-time `TodoEvent` emissions through the event callback system.

## Basic Usage

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/experimental/toolkit"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Task Manager",
    SystemPrompt: "Break complex tasks into steps and track progress with the TodoWrite tool.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewTodoWriteTool(),
    },
})
```

## Tracking Progress

Monitor todo updates via event callbacks:

```go
response, _ := agent.CreateResponse(ctx,
    dive.WithInput("Set up a new Go project with testing"),
    dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        if item.Type == dive.ResponseItemTypeTodo {
            for _, todo := range item.Todo.Todos {
                status := "pending"
                if todo.Status == dive.TodoStatusCompleted {
                    status = "done"
                } else if todo.Status == dive.TodoStatusInProgress {
                    status = "working"
                }
                fmt.Printf("[%s] %s\n", status, todo.Content)
            }
        }
        return nil
    }),
)
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

## TodoTracker Helper

The `TodoTracker` helper (in the core `dive` package) consumes todo events and provides progress tracking:

```go
tracker := dive.NewTodoTracker()

resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Build a REST API"),
    dive.WithEventCallback(tracker.HandleEvent),
)

// Display progress
tracker.DisplayProgress(os.Stdout)

// Get counts
completed, inProgress, total := tracker.Progress()

// Get status line
status := tracker.FormatProgress() // "Running tests - 2/5"
```

## Best Practices

1. One `in_progress` task at a time
2. Mark a task as `in_progress` before starting work
3. Mark tasks `completed` immediately when done
4. Provide both `Content` and `ActiveForm` for better UX
5. Each TodoWrite call replaces the entire list (include all items)
6. Only mark tasks `completed` when truly finished
