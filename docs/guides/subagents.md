# Sub-Agents

Sub-agents let a Dive agent spawn specialized agents to handle parts of a complex task. Each sub-agent runs in its own context with its own tools and returns a single final message to the parent.

The spawner is the **Agent** tool in `toolkit/orchestration`; the catalog of spawnable personas lives in the `subagent` package.

## How It Works

The parent agent spawns a sub-agent with the **Agent** tool, choosing a persona via `subagent_type`. Each sub-agent:

- Gets its own isolated context window
- Runs with a filtered set of the parent's tools (the Agent tool itself is always excluded, so sub-agents can't spawn sub-agents)
- Can run synchronously or in the background
- Is **single-use**: it returns one final message and is not resumable

The agent decides when to use sub-agents based on the task — you don't need to do anything special to trigger them.

## Synchronous vs Background

- **Synchronous** (`run_in_background: false`): the parent waits for the sub-agent, bounded by a configurable timeout (default: 10 minutes), and the result is returned inline.
- **Background** (`run_in_background: true`, which the tool description recommends): the sub-agent runs on an independent, cancellable context. The parent keeps working, and the result is delivered **automatically** on a later turn — there is no polling tool. A background sub-agent can be cancelled with the **TaskStop** tool using the `task_id` reported when it started.

Background runs use a context independent of the parent's turn, so they survive if the parent's turn is cancelled (e.g., by Ctrl+C).

## Built-in personas

The `subagent` package ships three ready-made `Definition`s:

- `subagent.GeneralPurpose` — general multi-step work; inherits the parent's tools.
- `subagent.Explore` — read-only code search and reading (`Edit`/`Write`/`Bash` stripped via `DisallowedTools`).
- `subagent.Plan` — read-only architectural analysis and planning.

Clone and modify to tweak a built-in — for example, narrowing the tool set the
default factory will allow (assign a fresh slice rather than mutating the
shared one):

```go
myExplore := *subagent.Explore
myExplore.Tools = []string{"Glob", "Grep"} // search only
```

## Custom Agent Definitions

Define specialized sub-agents as markdown files with YAML frontmatter and load them with a `subagent.Loader`:

```
.dive/
  agents/
    code-reviewer.md
```

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
| `description` | string | When the LLM should use this agent (shown in the tool description) |
| `model` | string | Optional, provider-agnostic model identifier a custom `AgentFactory` can route on. The built-in factory ignores it. |
| `tools` | string[] | Allowed tool names. Omit to inherit all parent tools. |

Use `subagent.GeneralPurpose` for a general-purpose agent that inherits the parent's tools, even without custom definitions.

## Library Usage

```go
import (
    "context"
    "maps"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/subagent"
    "github.com/deepnoodle-ai/dive/toolkit/orchestration"
)

// The catalog of spawnable personas, keyed by subagent_type. It's a plain map.
subagents := map[string]*subagent.Definition{
    "GeneralPurpose": subagent.GeneralPurpose,
    "Explore":        subagent.Explore,
    "Plan":           subagent.Plan,
}

// Optionally merge in definitions loaded from .dive/agents/*.md:
loaded, _ := (&subagent.FileLoader{Directories: []string{".dive/agents"}}).Load(ctx)
maps.Copy(subagents, loaded)

// Runs is the shared tracker that lets TaskStop cancel background runs by id.
runs := orchestration.NewRuns()

// Simplest setup: pass a Model and NewAgentTool builds each subagent with a
// built-in factory — the definition's prompt plus the parent's tools filtered by
// the definition's allow/deny lists. For full control, pass an AgentFactory
// instead (see "Custom subagent construction" below).
agentTool := orchestration.NewAgentTool(orchestration.AgentToolOptions{
    Subagents:   subagents,
    Model:       myModel,
    ParentTools: parentTools,
    Runs:        runs,
})
taskStop := orchestration.NewTaskStopTool(orchestration.TaskStopToolOptions{Runs: runs})

agent, _ := dive.NewAgent(dive.AgentOptions{
    Model: myModel,
    Tools: []dive.Tool{agentTool, taskStop /* , ...other tools */},
})
```

The orchestration constructors return `*dive.TypedToolAdapter[T]`, which satisfies `dive.Tool` directly — no manual `dive.ToolAdapter(...)` wrapping needed.

### Custom subagent construction

`Model` is shorthand for the built-in `orchestration.DefaultAgentFactory`. When you need more — worktree isolation, a sandbox, a per-run session, hooks, or per-definition model routing — pass an `AgentFactory` instead. It receives the subagent's name, definition, and the parent's tools, and returns the agent to run:

```go
factory := func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
    return dive.NewAgent(dive.AgentOptions{
        Name:         name,
        SystemPrompt: def.Prompt,
        Model:        myModel,
        Tools:        subagent.FilterTools(def, parentTools), // strips Edit/Write/etc. per def, and the Agent tool
        // ...add hooks, a Session, a sandboxed Bash tool, route on def.Model, etc.
    })
}

agentTool := orchestration.NewAgentTool(orchestration.AgentToolOptions{
    Subagents:    subagents,
    AgentFactory: factory, // takes precedence over Model
    ParentTools:  parentTools,
    Runs:         runs,
})
```

### Background results

When a sub-agent runs in the background, the Agent tool returns immediately and Dive delivers the result on a later turn through its background-task machinery (`Response.BackgroundTasks` + `dive.ContinueWithBackground`). See the [Agents guide](agents.md) for the background-result loop.

## Tips

- **Let the agent decide.** You don't need to say "use sub-agents." For complex multi-part tasks, the agent will spawn them when it makes sense.
- **Be explicit when you want parallelism.** Saying "use separate agents for each" or "do these in parallel" gives a strong hint.
- **Sub-agents are isolated.** They can't see each other's work; the parent agent combines results.
- **Sub-agents can't spawn sub-agents.** The Agent tool is excluded from sub-agent tool sets.
- **Cost scales with agents.** Each sub-agent uses its own tokens; for simple tasks a single agent is more efficient.

## Next Steps

- [Tools Guide](tools.md) — Built-in tools available to agents
- [Custom Tools](custom-tools.md) — Create your own tools
- [Agents Guide](agents.md) — Core agent configuration and the background-result loop
