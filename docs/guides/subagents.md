# Sub-Agents

> Sub-agent orchestration uses the `subagent/` package for definitions and
> `experimental/toolkit/extended/` for the Task tool.

Sub-agents let a Dive agent spawn specialized agents to handle parts of a complex task. Each sub-agent runs in its own context with its own tools and returns its result to the parent agent.

## How It Works

The primary agent can spawn sub-agents using the **Task** tool. Each sub-agent:

- Gets its own isolated context window
- Runs with a filtered set of the parent's tools (the Task tool itself is always excluded)
- Can run synchronously or in the background
- Returns its result to the parent agent when done

The agent decides when to use sub-agents based on the task. You don't need to do anything special to trigger them.

## Synchronous vs Background

Sub-agents can run in two modes:

- **Synchronous** (default): The parent agent waits for the sub-agent to complete before continuing. Has a configurable timeout (default: 10 minutes).
- **Background** (`run_in_background: true`): The sub-agent runs independently. The parent can continue working and retrieve results later with the **TaskOutput** tool.

Background tasks use an independent context, so they survive if the parent's turn is cancelled (e.g., by Ctrl+C).

### Auto-retrieval

When background tasks complete while the agent is idle, the CLI automatically triggers the agent to retrieve the results. Multiple completions arriving close together are batched into a single retrieval turn (500ms debounce).

## Example Prompts

### Parallel research

```
Read through the authentication module and the payment module.
Summarize how each works and identify any shared patterns.
```

The agent may spawn two sub-agents in parallel: one to explore auth, one to explore payments.

### Code exploration at scale

```
Find all uses of the deprecated `legacyAuth` function across the codebase,
check which ones can be safely migrated, and which have edge cases.
```

The agent may spawn a sub-agent to do the deep search while it continues planning.

### Generate alternatives

```
Give me three different approaches to implementing rate limiting
for our API. Use a separate agent for each approach.
```

Explicitly requesting parallel sub-agents to explore different solutions.

## Custom Agent Definitions

You can define specialized sub-agents by creating markdown files in `.dive/agents/`:

```
.dive/
  agents/
    code-reviewer.md
    test-writer.md
    researcher.md
```

Each file uses YAML frontmatter:

```markdown
---
description: Reviews code for bugs, security issues, and style problems
model: sonnet
tools:
  - Read
  - Glob
  - Grep
---

You are a senior code reviewer. Focus on:
1. Logic errors and edge cases
2. Security vulnerabilities (injection, auth bypass, data exposure)
3. Performance issues
4. Code style and readability

Be specific. Reference line numbers. Suggest fixes.
```

### Frontmatter fields

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | When the LLM should use this agent (shown in tool description) |
| `model` | string | Model override: `sonnet`, `opus`, `haiku`, or empty to inherit |
| `tools` | string[] | Allowed tool names. Omit to inherit all parent tools. |

A `general-purpose` agent is always available by default, even without any custom definitions.

## Tips

- **Let the agent decide.** You don't need to say "use sub-agents." For complex multi-part tasks, the agent will spawn them when it makes sense.
- **Be explicit when you want parallelism.** Saying "use separate agents for each" or "do these in parallel" gives a strong hint.
- **Sub-agents are isolated.** They can't see each other's work. The parent agent combines results.
- **Sub-agents can't spawn sub-agents.** The Task tool is excluded from sub-agent tool sets.
- **Cost scales with agents.** Each sub-agent uses its own tokens. For simple tasks, a single agent is more efficient.

## Library Usage

To use sub-agents programmatically in Go:

```go
import (
    "context"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/subagent"
    "github.com/deepnoodle-ai/dive/experimental/toolkit/extended"
)

// Create registries
taskRegistry := extended.NewTaskRegistry()
subagentRegistry := subagent.NewRegistry(true) // includes general-purpose agent

// Define an agent factory
agentFactory := func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
    return dive.NewAgent(dive.AgentOptions{
        Name:         name,
        SystemPrompt: def.Prompt,
        Model:        myModel,
        Tools:        subagent.FilterTools(def, parentTools),
    })
}

// Create the Task and TaskOutput tools
taskTool := extended.NewTaskTool(extended.TaskToolOptions{
    Registry:         taskRegistry,
    AgentFactory:     agentFactory,
    SubagentRegistry: subagentRegistry,
    ParentTools:      parentTools,
    OnEvent: func(taskID string, item *dive.ResponseItem) {
        // Handle streaming events from sub-agents.
        // Called from background goroutines — must be safe for concurrent use.
    },
})
taskOutputTool := extended.NewTaskOutputTool(extended.TaskOutputToolOptions{
    Registry: taskRegistry,
})

// Add to your agent's tools
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model: myModel,
    Tools: []dive.Tool{
        dive.ToolAdapter(taskTool),
        dive.ToolAdapter(taskOutputTool),
        // ... other tools
    },
})
```

### Streaming events

The `OnEvent` callback receives `*dive.ResponseItem` values tagged with a task ID. Event types include:

- `"subagent_status"` — Lifecycle changes (running, completed, failed). The `Extension` field contains `*extended.SubagentStatusEvent`.
- `dive.ResponseItemTypeModelEvent` — Streaming text from the sub-agent's LLM.
- `dive.ResponseItemTypeToolCall` — Tool calls made by the sub-agent.

## Next Steps

- [Tools Guide](../tools.md) — Built-in tools available to agents
- [Custom Tools](../custom-tools.md) — Create your own tools
- [Agent Guide](../agents.md) — Core agent configuration
