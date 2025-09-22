# Agent Guide

Agents are the core building blocks of Dive applications. They represent intelligent AI entities that can understand natural language, use tools to interact with the world, and work together to accomplish complex tasks.

## 📋 Table of Contents

- [What is an Agent?](#what-is-an-agent)
- [Creating Agents](#creating-agents)
- [Agent Configuration](#agent-configuration)
- [Tool Integration](#tool-integration)
- [Supervisor Patterns](#supervisor-patterns)
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
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
    // Create a basic agent
    assistant, err := agent.New(agent.Options{
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
    "github.com/diveagents/dive/toolkit"
)

// Create an agent with file system access
researcher, err := agent.New(agent.Options{
    Name:         "Research Assistant",
    Instructions: "You help research topics and save findings to files.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewReadFileTool()),
        dive.ToolAdapter(toolkit.NewWriteFileTool()),
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
agent, err := agent.New(agent.Options{
    Name:         "Data Analyst",        // Agent identifier
    Goal:         "Analyze data sets",   // High-level purpose
    Instructions: "You are an expert...", // Detailed behavior instructions
    Model:        anthropic.New(),       // LLM provider
})
```

### Advanced Configuration

```go
agent, err := agent.New(agent.Options{
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

### Environment Integration

```go
import "github.com/diveagents/dive/environment"

// Create environment first
env, err := environment.New(environment.Options{
    Name: "Research Lab",
})

// Create agents within the environment
researcher, err := agent.New(agent.Options{
    Name:        "Researcher",
    Environment: env,  // Shared context
})

analyst, err := agent.New(agent.Options{
    Name:        "Data Analyst", 
    Environment: env,
})
```

## Tool Integration

### Built-in Tools

Dive includes several built-in tools:

```go
import "github.com/diveagents/dive/toolkit"

tools := []dive.Tool{
    // File operations
    dive.ToolAdapter(toolkit.NewReadFileTool()),
    dive.ToolAdapter(toolkit.NewWriteFileTool()),
    dive.ToolAdapter(toolkit.NewListDirectoryTool()),
    
    // Web operations
    dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
        Provider: "google",
    })),
    dive.ToolAdapter(toolkit.NewFetchTool()),
    
    // System operations
    dive.ToolAdapter(toolkit.NewCommandTool()),
    
    // Text editing
    dive.ToolAdapter(toolkit.NewTextEditorTool()),
    
    // Image generation
    dive.ToolAdapter(toolkit.NewGenerateImageTool()),
}
```

### Custom Tools

Create custom tools by implementing the `TypedTool` interface:

```go
type WeatherTool struct {
    APIKey string
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
supervisor, err := agent.New(agent.Options{
    Name:         "Project Manager",
    Instructions: "You coordinate work between team members.",
    IsSupervisor: true,
    Subordinates: []string{"Researcher", "Writer", "Reviewer"},
    Model:        anthropic.New(),
    Environment:  env,
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

## Thread Management

### Persistent Conversations

```go

// Set up thread repository
threadRepo := threads.NewMemoryRepository()

agent, err := agent.New(agent.Options{
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
agent, err := agent.New(agent.Options{
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

- [Workflow Guide](workflows.md) - Orchestrate agents in multi-step processes
- [Custom Tools](custom-tools.md) - Build domain-specific agent capabilities
- [Event Streaming](event-streaming.md) - Monitor and respond to agent activities
- [API Reference](../api/agent.md) - Detailed API documentation