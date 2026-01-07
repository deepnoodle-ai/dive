# Todo Lists

Track and display task progress using Dive's built-in todo functionality. The TodoWrite tool and TodoTracker helper provide structured task management that keeps users informed about agent progress on complex workflows.

## Overview

Dive's todo system provides:

1. A `TodoWrite` tool for agents to create and update task lists
2. Real-time `TodoEvent` emissions through the event callback system
3. A `TodoTracker` helper for consuming and displaying progress
4. Concurrent-safe state management for parallel access

## Basic Usage

### Adding the TodoWrite Tool

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/toolkit"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Task Manager",
    Instructions: "Break complex tasks into steps and track progress with the TodoWrite tool.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewTodoWriteTool(),
    },
})
```

### Tracking Progress with TodoTracker

```go
tracker := dive.NewTodoTracker()

resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Set up a new Go project with testing"),
    dive.WithEventCallback(tracker.HandleEvent),
)

// Display final progress
tracker.DisplayProgress(os.Stdout)
```

## Todo Lifecycle

Todos follow a predictable lifecycle:

1. **Created** as `pending` when tasks are identified
2. **Activated** to `in_progress` when work begins
3. **Completed** when the task finishes successfully
4. **Removed** when no longer relevant

### Todo States

| Status        | Description                                    |
| ------------- | ---------------------------------------------- |
| `pending`     | Task not yet started                           |
| `in_progress` | Currently being worked on (only one at a time) |
| `completed`   | Task finished successfully                     |

### Todo Item Structure

Each todo has two text forms:

```go
type TodoItem struct {
    Content    string     // Imperative: "Run tests", "Fix the bug"
    Status     TodoStatus // pending, in_progress, completed
    ActiveForm string     // Present continuous: "Running tests", "Fixing the bug"
}
```

The `ActiveForm` is displayed while the task is in progress, providing a natural description of ongoing work.

## Real-Time Progress Display

### Using Event Callbacks

Monitor todo updates as they happen:

```go
resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Build a REST API"),
    dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        if item.Type == dive.ResponseItemTypeTodo {
            fmt.Println("\n--- Todo Update ---")
            for i, todo := range item.Todo.Todos {
                status := "❌"
                if todo.Status == dive.TodoStatusCompleted {
                    status = "✅"
                } else if todo.Status == dive.TodoStatusInProgress {
                    status = "🔧"
                }
                fmt.Printf("%d. %s %s\n", i+1, status, todo.Content)
            }
        }
        return nil
    }),
)
```

### Combining TodoTracker with Other Callbacks

Use `ChainCallback` to combine todo tracking with other event handling:

```go
tracker := dive.NewTodoTracker()

resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("..."),
    dive.WithEventCallback(tracker.ChainCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        // Handle other events
        if item.Type == dive.ResponseItemTypeMessage {
            fmt.Println(item.Message.Text())
        }
        return nil
    })),
)
```

## TodoTracker Methods

| Method                   | Description                                   |
| ------------------------ | --------------------------------------------- |
| `HandleEvent(ctx, item)` | EventCallback that tracks todo updates        |
| `Todos()`                | Returns a copy of the current todo list       |
| `CurrentTask()`          | Returns the in-progress task, if any          |
| `Progress()`             | Returns (completed, inProgress, total) counts |
| `DisplayProgress(w)`     | Writes formatted progress to a writer         |
| `FormatProgress()`       | Returns a single-line progress string         |
| `ChainCallback(next)`    | Combines with another EventCallback           |

### Progress Display Example

```go
tracker := dive.NewTodoTracker()

// After agent execution...
tracker.DisplayProgress(os.Stdout)
```

Output:

```
Progress: 2/5 completed
Currently working on: 1 task(s)

1. ✅ Create project structure
2. ✅ Initialize go.mod
3. 🔧 Writing main.go
4. ❌ Add unit tests
5. ❌ Run tests and verify
```

### Status Line Format

```go
status := tracker.FormatProgress()
// "Writing main.go • 2/5"
```

## When Todos Are Used

The TodoWrite tool is designed for proactive use in these scenarios:

1. Complex multi-step tasks requiring 3 or more distinct steps or actions
2. Non-trivial operations that require careful planning or multiple operations
3. User explicitly requests a todo list
4. User-provided task lists when multiple items are mentioned (numbered or comma-separated)
5. After receiving new instructions - immediately capture requirements as todos
6. When starting a task - mark as `in_progress` BEFORE beginning work
7. After completing a task - mark complete and add any follow-up tasks discovered

Skip using todos when:

- There is only a single, straightforward task
- The task is trivial and completable in under 3 simple steps
- The task is purely conversational or informational

If there is only one trivial task, agents should just do it directly without the todo overhead.

### Instructing Agents to Use Todos

Include guidance in your agent's instructions:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name: "Developer Assistant",
    Instructions: `You are a software development assistant.

When given complex tasks:
1. Break them into smaller steps using the TodoWrite tool
2. Mark each step as in_progress before starting
3. Mark steps complete immediately after finishing
4. Only have one task in_progress at a time

Use todos for tasks with 3+ steps to show your progress.`,
    Model: anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewTodoWriteTool(),
    },
})
```

## Using OnUpdate Callback

The tool also supports a direct callback for updates:

```go
tool := toolkit.NewTodoWriteTool(toolkit.TodoWriteToolOptions{
    OnUpdate: func(todos []toolkit.TodoItem) {
        fmt.Printf("Todos updated: %d items\n", len(todos))
    },
})
```

This is useful when you need direct access to todo updates without going through the event callback system.

## Accessing the Tool's State

If you need direct access to the tool's internal state:

```go
adapter := toolkit.NewTodoWriteTool()

// After agent execution, get the underlying tool
tool := adapter.Unwrap().(*toolkit.TodoWriteTool)

// Get current todos
todos := tool.GetTodos()

// Get current in-progress task
current := tool.GetCurrentTask()
if current != nil {
    fmt.Printf("Currently: %s\n", current.ActiveForm)
}
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    "github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
    ctx := context.Background()

    // Create a tracker
    tracker := dive.NewTodoTracker()

    // Create agent with TodoWrite tool
    agent, err := dive.NewAgent(dive.AgentOptions{
        Name: "Project Setup Assistant",
        Instructions: `You help set up new projects. When given a setup task:
1. Create a todo list with the TodoWrite tool
2. Update status as you work through each step
3. Mark tasks complete when done`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            toolkit.NewTodoWriteTool(),
        },
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    // Execute with real-time progress display
    _, err = agent.CreateResponse(ctx,
        dive.WithInput("Set up a new Go project with testing and CI"),
        dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
            if err := tracker.HandleEvent(ctx, item); err != nil {
                return err
            }

            // Display progress on todo updates
            if item.Type == dive.ResponseItemTypeTodo {
                fmt.Print("\033[H\033[2J") // Clear screen
                tracker.DisplayProgress(os.Stdout)
            }
            return nil
        }),
    )
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    // Final summary
    completed, _, total := tracker.Progress()
    fmt.Printf("\nCompleted %d/%d tasks\n", completed, total)
}
```

## Best Practices

1. One `in_progress` at a time - Agents should only have one task in progress
2. Mark before starting - Set a task to `in_progress` before beginning work on it
3. Mark complete immediately - Don't batch completions; mark done when finished
4. Use both forms - Provide both `content` and `activeForm` for better UX
5. Break down complex tasks - Smaller steps provide better visibility
6. Add discovered work - If follow-up tasks are discovered during implementation, add them
7. Remove irrelevant tasks - Clean up todos that are no longer needed
8. Clear instructions - Tell agents when and how to use the todo tool
9. Full replacement - Each TodoWrite call replaces the entire list; include all items in every call
10. Honest completion - Only mark tasks as `completed` when truly finished, not if blocked or partial

## Limitations

- No TodoRead tool - Agents track state through conversation context, not by querying the list
- User visibility only - The list is visible to users in the UI but not programmatically queryable by agents
- Memory-based tracking - In long conversations, agents rely on their memory of what was written

## Integration with Other Features

### With Compaction

Todo state persists across compaction. The summary will include task progress, and agents can continue from where they left off.

### With Session Persistence

Todo updates are tracked in the event stream. While the tool state itself isn't persisted, the agent's todo usage is recorded in session messages.

### With Skills

Skills can include the TodoWrite tool in their `allowed-tools` list to enable task tracking within skill execution.
