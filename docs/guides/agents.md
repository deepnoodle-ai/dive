# Agent Guide

Agents are the core building blocks of Dive applications. They represent intelligent AI entities that can understand natural language, use tools to interact with the world, and work together to accomplish complex tasks.

## 📋 Table of Contents

- [What is an Agent?](#what-is-an-agent)
- [Creating Agents](#creating-agents)
- [Agent Configuration](#agent-configuration)
- [Tool Integration](#tool-integration)
- [Supervisor Patterns](#supervisor-patterns)
- [Subagents](#subagents)
- [Thread Management](#thread-management)
- [Best Practices](#best-practices)

## What is an Agent?

An agent in Dive is an autonomous AI entity that can:

- **Understand** natural language input and context
- **Reason** about problems and make decisions
- **Act** using tools to interact with external systems
- **Communicate** in natural language responses
- **Collaborate** with other agents through work delegation
- **Remember** conversation history across interactions

Think of agents as AI assistants with specific expertise and capabilities, similar to having specialized team members who can work independently or together.

## Creating Agents

### Basic Agent

```go
package main

import (
    "context"
    "fmt"
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
)

func main() {
    // Create a basic agent
    assistant, err := dive.NewAgent(dive.AgentOptions{
        Name:         "Assistant",
        Instructions: "You are a helpful AI assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        panic(err)
    }

    // Chat with the agent
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithInput("Hello! Can you help me with some questions?"),
    )
    if err != nil {
        panic(err)
    }

    fmt.Println(response.Text())
}
```

### Agent with Tools

```go
import (
    "github.com/deepnoodle-ai/dive/toolkit"
)

// Create an agent with file system access
researcher, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Research Assistant",
    Instructions: "You help research topics and save findings to files.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})),
        dive.ToolAdapter(toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{})),
        dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
            Provider: "google", // or "kagi"
        })),
    },
})
```

### Streaming Responses

```go
// Stream responses for real-time UI updates
stream, err := assistant.StreamResponse(
    context.Background(),
    dive.WithInput("Write a short story about AI"),
)
if err != nil {
    panic(err)
}

for event := range stream.Events() {
    switch event.Type {
    case dive.EventTypeResponseInProgress:
        if event.Item.Message != nil {
            fmt.Print(event.Item.Message.Text())
        }
    case dive.EventTypeResponseCompleted:
        fmt.Println("\n--- Complete ---")
    }
}
```

## Agent Configuration

### Basic Options

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Data Analyst",        // Agent identifier
    Goal:         "Analyze data sets",   // High-level purpose
    Instructions: "You are an expert...", // Detailed behavior instructions
    Model:        anthropic.New(),       // LLM provider
})
```

### Advanced Configuration

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Advanced Assistant",
    Instructions: "You are a specialized assistant...",
    Model:        anthropic.New(),

    // Tool configuration
    Tools: []dive.Tool{
        webSearchTool,
        calculatorTool,
        fileSystemTool,
    },

    // Behavior settings
    ResponseTimeout:      time.Minute * 5,
    ToolIterationLimit:   10,
    DateAwareness:        &[]bool{true}[0],

    // Model fine-tuning
    ModelSettings: &agent.ModelSettings{
        Temperature:       &[]float64{0.7}[0],
        MaxTokens:         &[]int{2048}[0],
        ReasoningBudget:   &[]int{50000}[0],
        ReasoningEffort:   "high",
        ParallelToolCalls: &[]bool{true}[0],
        Caching:           &[]bool{true}[0],
    },

    // Custom system prompt
    SystemPromptTemplate: "You are {{.Name}}. {{.Instructions}}",

    // Additional context
    Context: []llm.Content{
        &llm.TextContent{Text: "Today's priority is data analysis."},
    },
})
```

## Tool Integration

### Built-in Tools

Dive includes several built-in tools:

```go
import "github.com/deepnoodle-ai/dive/toolkit"

tools := []dive.Tool{
    // File operations
    dive.ToolAdapter(toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})),
    dive.ToolAdapter(toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{})),
    dive.ToolAdapter(toolkit.NewListDirectoryTool(toolkit.ListDirectoryToolOptions{})),

    // Web operations
    dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
        Provider: "google",
    })),
    dive.ToolAdapter(toolkit.NewFetchTool(toolkit.FetchToolOptions{})),

    // System operations
    dive.ToolAdapter(toolkit.NewCommandTool(toolkit.CommandToolOptions{})),

    // Text editing
    dive.ToolAdapter(toolkit.NewTextEditorTool(toolkit.TextEditorToolOptions{})),

    // Image generation
    dive.ToolAdapter(toolkit.NewImageGenerationTool(toolkit.ImageGenerationToolOptions{})),
}
```

### Custom Tools

Create custom tools by implementing the `TypedTool` interface:

```go
type WeatherTool struct {
    APIKey string
}

type WeatherInput struct {
    Location string `json:"location"`
}

func (t *WeatherTool) Name() string {
    return "get_weather"
}

func (t *WeatherTool) Description() string {
    return "Get current weather information for a location"
}

func (t *WeatherTool) Schema() schema.Schema {
    return schema.Schema{
        Type: "object",
        Properties: map[string]schema.Property{
            "location": {
                Type:        "string",
                Description: "City name or coordinates",
            },
        },
        Required: []string{"location"},
    }
}

func (t *WeatherTool) Annotations() dive.ToolAnnotations {
    return dive.ToolAnnotations{
        Title:         "Weather Information",
        ReadOnlyHint:  true,
        OpenWorldHint: true,
    }
}

func (t *WeatherTool) Call(ctx context.Context, input *WeatherInput) (*dive.ToolResult, error) {
    // Implementation details...
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: fmt.Sprintf("Weather in %s: %s", input.Location, weatherData),
        }},
    }, nil
}

// Use the tool
weatherTool := dive.ToolAdapter(&WeatherTool{APIKey: "your-key"})
```

### Tool Annotations

Tools can include annotations to help agents understand their behavior:

```go
annotations := dive.ToolAnnotations{
    Title:           "File Writer",        // Human-readable name
    ReadOnlyHint:    false,               // Tool modifies data
    DestructiveHint: true,                // Tool may delete/overwrite
    IdempotentHint:  false,               // Results may vary on repeat
    OpenWorldHint:   false,               // No external network access
}
```

## Supervisor Patterns

Agents can be configured as supervisors to delegate work to other agents:

### Creating a Supervisor

```go
supervisor, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Project Manager",
    Instructions: "You coordinate work between team members.",
    IsSupervisor: true,
    Subordinates: []string{"Researcher", "Writer", "Reviewer"},
    Model:        anthropic.New(),
})
```

### Automatic Work Assignment

Supervisors automatically get an `assign_work` tool:

```go
// The supervisor can now delegate tasks
response, err := supervisor.CreateResponse(
    context.Background(),
    dive.WithInput("Research renewable energy and write a summary report"),
)
```

### Custom Assignment Logic

```go
// Override the default assign_work tool
customAssignTool := &CustomAssignWorkTool{
    // Your custom delegation logic
}

supervisor, err := agent.New(agent.Options{
    Name:         "Smart Manager",
    IsSupervisor: true,
    Tools: []dive.Tool{
        dive.ToolAdapter(customAssignTool), // Custom tool will be used
    },
})
```

## Subagents

Subagents allow an agent to spawn specialized child agents for focused subtasks. Unlike supervisor patterns where agents coordinate pre-existing subordinates, subagents are created on-demand with isolated contexts and restricted tool access.

### Key Concepts

- **Focused Context**: Subagents receive only the prompt provided, not the parent's full conversation history
- **Tool Restrictions**: Subagents can be limited to specific tools
- **No Nesting**: Subagents cannot spawn their own subagents (Task tool is never available to them)
- **Model Override**: Subagents can use different models than their parent

### Programmatic Definition

Define subagents directly in `AgentOptions`:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Main Agent",
    Instructions: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Tools:        allTools,
    Subagents: map[string]*dive.SubagentDefinition{
        "code-reviewer": {
            Description: "Expert code reviewer. Use for security and quality reviews.",
            Prompt:      "You are a code review specialist. Analyze code for bugs, security issues, and style problems.",
            Tools:       []string{"Read", "Grep", "Glob"},
            Model:       "sonnet",
        },
        "test-runner": {
            Description: "Runs and analyzes test suites.",
            Prompt:      "You are a test execution specialist. Run tests and analyze results.",
            Tools:       []string{"Bash", "Read", "Grep"},
        },
    },
})
```

### SubagentDefinition Fields

| Field | Type | Description |
|-------|------|-------------|
| `Description` | `string` | When Claude should use this subagent (shown in Task tool description) |
| `Prompt` | `string` | System prompt for the subagent |
| `Tools` | `[]string` | Tool names allowed (nil = inherit all except Task) |
| `Model` | `string` | Model override: "sonnet", "opus", "haiku", or "" (inherit) |

### Filesystem-Based Loading

Load subagent definitions from markdown files with YAML frontmatter:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Main Agent",
    Instructions: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Tools:        allTools,
    SubagentLoader: &dive.FileSubagentLoader{
        Directories:         []string{".dive/agents"},
        IncludeClaudeAgents: true, // Also load from .claude/agents/
    },
})
```

Subagent files use markdown with YAML frontmatter:

```markdown
<!-- .dive/agents/code-reviewer.md -->
---
description: Expert code reviewer for security and quality reviews
model: sonnet
tools:
  - Read
  - Grep
  - Glob
---

You are a code review specialist.

When reviewing code:
1. Check for security vulnerabilities
2. Identify bugs and logic errors
3. Suggest improvements for readability
4. Ensure proper error handling
```

The filename (without `.md`) becomes the subagent name.

### Custom Loaders

Implement the `SubagentLoader` interface to load definitions from custom sources:

```go
type SubagentLoader interface {
    Load(ctx context.Context) (map[string]*SubagentDefinition, error)
}

// Example: Load from database
type DatabaseSubagentLoader struct {
    DB *sql.DB
}

func (l *DatabaseSubagentLoader) Load(ctx context.Context) (map[string]*SubagentDefinition, error) {
    rows, err := l.DB.QueryContext(ctx, "SELECT name, description, prompt, tools, model FROM subagents")
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    result := make(map[string]*dive.SubagentDefinition)
    for rows.Next() {
        // Parse and add to result...
    }
    return result, nil
}
```

### Combining Multiple Sources

Use `CompositeSubagentLoader` to combine multiple loaders:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:  "Main Agent",
    Model: anthropic.New(),
    SubagentLoader: &dive.CompositeSubagentLoader{
        Loaders: []dive.SubagentLoader{
            &dive.FileSubagentLoader{Directories: []string{".dive/agents"}},
            &dive.MapSubagentLoader{
                Subagents: map[string]*dive.SubagentDefinition{
                    "custom-agent": {
                        Description: "Custom programmatic agent",
                        Prompt:      "You are a custom agent.",
                    },
                },
            },
        },
    },
})
```

Later loaders override earlier ones for definitions with the same name.

### Built-in General-Purpose Subagent

A `general-purpose` subagent is automatically registered unless disabled. It inherits all parent tools (except Task) and can handle any task:

```go
// Access the built-in definition
dive.GeneralPurposeSubagent.Description
// "General-purpose agent for complex, multi-step tasks. Use when no specialized agent matches."
```

### Tool Filtering

Use `FilterTools` to apply a subagent's tool restrictions:

```go
// In your AgentFactory implementation
func createSubagent(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
    // Filter tools based on definition
    filteredTools := dive.FilterTools(def, parentTools)

    return dive.NewAgent(dive.AgentOptions{
        Name:         name,
        Instructions: def.Prompt,
        Model:        selectModel(def.Model),
        Tools:        filteredTools,
    })
}
```

Key behaviors:
- If `def.Tools` is nil or empty, all parent tools are inherited (except Task)
- If `def.Tools` specifies tool names, only those tools are included
- The Task tool is **never** included, preventing nested subagent spawning

## Thread Management

### Persistent Conversations

```go

// Set up thread repository
threadRepo := threads.NewMemoryRepository()

agent, err := dive.NewAgent(dive.AgentOptions{
    Name:             "Assistant",
    ThreadRepository: threadRepo,
})

// First interaction
response1, err := agent.CreateResponse(
    context.Background(),
    dive.WithThreadID("user-123"),
    dive.WithInput("My name is Alice"),
)

// Later interaction - agent remembers previous context
response2, err := agent.CreateResponse(
    context.Background(),
    dive.WithThreadID("user-123"),
    dive.WithInput("What's my name?"),
)
// Agent will respond with "Alice"
```

### Thread Storage Options

```go
// In-memory (development/testing)
threadRepo := threads.NewMemoryRepository()

// File-based (simple persistence)
threadRepo := threads.NewDiskRepository("./threads")

// Database-based (production)
// Implement dive.ThreadRepository interface
```

## Best Practices

### 1. Clear Instructions

```go
// Good: Specific, actionable instructions
Instructions: `You are a code review assistant. When reviewing code:
1. Check for security vulnerabilities
2. Suggest performance improvements
3. Ensure proper error handling
4. Verify documentation completeness
Always explain your reasoning.`

// Avoid: Vague or conflicting instructions
Instructions: "You're helpful and do everything well."
```

### 2. Appropriate Tool Selection

```go
// Good: Include only relevant tools
Tools: []dive.Tool{
    codeAnalysisTool,
    documentationTool,
    securityScanTool,
}

// Avoid: Including too many unrelated tools
Tools: []dive.Tool{
    codeAnalysisTool,
    weatherTool,      // Unrelated
    gamePlayerTool,   // Unrelated
    fileSystemTool,   // Maybe needed?
}
```

### 3. Error Handling

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name: "Assistant",
    ResponseTimeout: time.Minute * 2, // Reasonable timeout
})
if err != nil {
    return fmt.Errorf("failed to create agent: %w", err)
}

response, err := agent.CreateResponse(ctx, dive.WithInput(userInput))
if err != nil {
    // Handle specific error types
    if errors.Is(err, agent.ErrThreadsAreNotEnabled) {
        // Handle thread configuration issue
    }
    return fmt.Errorf("agent response failed: %w", err)
}
```

### 4. Resource Management

```go
// Use contexts for cancellation
ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
defer cancel()

response, err := agent.CreateResponse(ctx, dive.WithInput(input))
```

### 5. Event Handling

```go
response, err := agent.CreateResponse(
    context.Background(),
    dive.WithInput("Analyze this data"),
    dive.WithEventCallback(func(ctx context.Context, event *dive.ResponseEvent) error {
        switch event.Type {
        case dive.EventTypeResponseToolCall:
            log.Printf("Tool called: %s", event.Item.ToolCall.Name)
        case dive.EventTypeResponseToolResult:
            log.Printf("Tool result: %s", event.Item.ToolCallResult.Result)
        }
        return nil
    }),
)
```

## Next Steps

- [Custom Tools](custom-tools.md) - Build domain-specific agent capabilities
- [Tools Guide](tools.md) - Learn about built-in tools and capabilities
- [LLM Guide](llm-guide.md) - Work with different AI models and providers
- [API Reference](../api/agent.md) - Detailed API documentation
