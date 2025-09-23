# Agent API Reference

Complete API reference for Dive agent types (now in the main dive package), covering agent creation, configuration, and interaction patterns.

## 📋 Table of Contents

- [Core Interface](#core-interface)
- [Agent Implementation](#agent-implementation)
- [Configuration](#configuration)
- [Model Settings](#model-settings)
- [Response Handling](#response-handling)
- [Tool Integration](#tool-integration)
- [Thread Management](#thread-management)
- [Error Handling](#error-handling)
- [Examples](#examples)

## Core Interface

### `dive.Agent`

The core agent interface that all agent implementations must satisfy.

```go
type Agent interface {
    // Name of the Agent
    Name() string

    // IsSupervisor indicates whether the Agent can assign work to other Agents
    IsSupervisor() bool

    // CreateResponse creates a new Response from the Agent
    CreateResponse(ctx context.Context, opts ...Option) (*Response, error)

    // StreamResponse streams a new Response from the Agent
    StreamResponse(ctx context.Context, opts ...Option) (ResponseStream, error)
}
```

### Response Generation Options

Options that can be passed to `CreateResponse` and `StreamResponse`:

```go
// WithThreadID associates a conversation thread ID with generation
func WithThreadID(threadID string) Option

// WithUserID associates a user ID with generation
func WithUserID(userID string) Option

// WithMessage specifies a single message for generation
func WithMessage(message *llm.Message) Option

// WithMessages specifies multiple messages for generation
func WithMessages(messages ...*llm.Message) Option

// WithInput specifies simple text input (convenience wrapper)
func WithInput(input string) Option

// WithEventCallback specifies callback for streaming events
func WithEventCallback(callback EventCallback) Option
```

## Agent Implementation

### `dive.StandardAgent`

The standard implementation of the Agent interface.

```go
type StandardAgent struct {
    name                 string
    goal                 string
    instructions         string
    model                llm.LLM
    tools                []dive.Tool
    toolsByName          map[string]dive.Tool
    isSupervisor         bool
    subordinates         []string
    responseTimeout      time.Duration
    hooks                llm.Hooks
    logger               log.Logger
    toolIterationLimit   int
    modelSettings        *ModelSettings
    dateAwareness        *bool
    threadRepository     dive.ThreadRepository
    confirmer            dive.Confirmer
    systemPromptTemplate *template.Template
    context              []llm.Content
}
```

### Constructor

```go
func NewAgent(opts AgentOptions) (Agent, error)
```

Creates a new Agent with the specified configuration options.

**Parameters:**

- `opts` - Configuration options for the agent

**Returns:**

- `Agent` - Configured agent instance
- `error` - Error if configuration is invalid

**Example:**

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
    "github.com/deepnoodle-ai/dive/toolkit"
)

agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Assistant",
    Instructions: "You are a helpful AI assistant.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
            Provider: "google",
        })),
    },
})
```

## Configuration

### `dive.AgentOptions`

Configuration structure for creating new agents.

```go
type Options struct {
    // Basic configuration
    Name                 string                    // Agent name (required)
    Goal                 string                    // High-level goal description
    Instructions         string                    // Detailed behavior instructions

    // Hierarchy and collaboration
    IsSupervisor         bool                      // Can assign work to others
    Subordinates         []string                  // Names of subordinate agents

    // LLM configuration
    Model                llm.LLM                   // LLM provider (required)
    ModelSettings        *ModelSettings            // Model-specific settings

    // Capabilities
    Tools                []dive.Tool               // Available tools
    DateAwareness        *bool                     // Include current date in context
    Context              []llm.Content             // Additional context content

    // Behavior configuration
    ResponseTimeout      time.Duration             // Max response time
    ToolIterationLimit   int                       // Max tool usage iterations
    SystemPromptTemplate string                    // Custom system prompt template

    // Integration
    ThreadRepository     dive.ThreadRepository     // Conversation storage
    Confirmer            dive.Confirmer            // User confirmation handler

    // Advanced
    Hooks                llm.Hooks                 // LLM lifecycle hooks
    Logger               log.Logger            // Logging interface
}
```

### Required Fields

The following fields are required when creating an agent:

- **`Name`** - Unique identifier for the agent
- **`Model`** - LLM provider instance
- **`Instructions`** - Behavior and personality description

### Default Values

```go
const (
    DefaultResponseTimeout    = time.Minute * 10
    DefaultToolIterationLimit = 16
)
```

## Model Settings

### `dive.ModelSettings`

Fine-tune LLM behavior for specific use cases.

```go
type ModelSettings struct {
    // Response creativity and randomness
    Temperature       *float64              // 0.0-1.0, higher = more creative

    // Content repetition control
    PresencePenalty   *float64              // -2.0 to 2.0, reduce repetition
    FrequencyPenalty  *float64              // -2.0 to 2.0, reduce frequency

    // Tool usage control
    ParallelToolCalls *bool                 // Allow concurrent tool calls
    ToolChoice        *llm.ToolChoice       // Control tool selection behavior

    // Token and reasoning limits
    MaxTokens         *int                  // Maximum response tokens
    ReasoningBudget   *int                  // Reasoning token limit (o1 models)
    ReasoningEffort   llm.ReasoningEffort   // Reasoning intensity level

    // Performance optimization
    Caching           *bool                 // Enable prompt caching

    // Provider-specific features
    Features          []string              // Enable specific capabilities
    RequestHeaders    http.Header           // Custom HTTP headers
    MCPServers        []llm.MCPServerConfig // MCP server configurations
}
```

### Tool Choice Configuration

```go
type ToolChoice struct {
    Type string `json:"type"`            // "auto", "required", "none", "tool"
    Name string `json:"name,omitempty"`  // Specific tool name (when type="tool")
}

// Helper constructors
func AutoToolChoice() *ToolChoice         // Let model decide
func RequiredToolChoice() *ToolChoice     // Must use a tool
func NoToolChoice() *ToolChoice           // Disable tools
func SpecificToolChoice(name string) *ToolChoice  // Use specific tool
```

### Reasoning Effort Levels

```go
type ReasoningEffort string

const (
    ReasoningEffortLow    ReasoningEffort = "low"
    ReasoningEffortMedium ReasoningEffort = "medium"
    ReasoningEffortHigh   ReasoningEffort = "high"
)
```

## Response Handling

### Response Types

```go
type Response interface {
    // Content access
    Text() string                    // Response text content
    ToolCalls() []ToolCall          // Tool calls made during generation
    Messages() []*llm.Message       // All messages in conversation

    // Metadata
    ID() string                     // Unique response identifier
    ThreadID() string               // Associated thread identifier
    Usage() *TokenUsage             // Token usage statistics

    // Event access
    Events() []*ResponseEvent       // All events generated
    FirstEvent() *ResponseEvent     // First event (if any)
    LastEvent() *ResponseEvent      // Last event (if any)
}
```

### Streaming Responses

```go
type ResponseStream interface {
    // Stream control
    Events() <-chan *ResponseEvent  // Channel of streaming events
    Close() error                   // Close the stream

    // Result access (after stream completion)
    Response() (*Response, error)   // Final response after streaming
}
```

### Response Events

```go
type ResponseEvent struct {
    Type      EventType             // Event type identifier
    Data      map[string]interface{} // Event-specific data
    Timestamp time.Time             // When event occurred

    // Specific event data (populated based on Type)
    Message   *llm.Message          // For message events
    ToolCall  *ToolCall             // For tool call events
    Error     error                 // For error events
}

// Event types
const (
    EventTypeResponseStarted     EventType = "response_started"
    EventTypeResponseCompleted   EventType = "response_completed"
    EventTypeMessageDelta        EventType = "message_delta"
    EventTypeToolCallStarted     EventType = "tool_call_started"
    EventTypeToolCallCompleted   EventType = "tool_call_completed"
    EventTypeError               EventType = "error"
)
```

## Tool Integration

### Tool Interface

```go
type Tool interface {
    // Metadata
    Name() string
    Description() string

    // Execution
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}
```

### Tool Adapter

Convert any tool to be compatible with Dive agents:

```go
func ToolAdapter(tool interface{}) dive.Tool
```

**Supported Tool Types:**

- `dive.Tool` - Native Dive tools
- `toolkit.TypedTool` - Toolkit tools with type information
- Custom tools implementing the Tool interface

### Tool Configuration

```go
type ToolConfig struct {
    Name       string                 `json:"name"`
    Enabled    *bool                  `json:"enabled,omitempty"`
    Parameters map[string]interface{} `json:"parameters,omitempty"`
}
```

### Tool Iteration Control

```go
const (
    DefaultToolIterationLimit = 16  // Maximum tool calls per response
    FinishNow = "Do not use any more tools. You must respond with your final answer now."
)
```

## Thread Management

### Thread Repository

```go
type ThreadRepository interface {
    // Thread lifecycle
    CreateThread(ctx context.Context, thread *Thread) error
    GetThread(ctx context.Context, threadID string) (*Thread, error)
    UpdateThread(ctx context.Context, thread *Thread) error
    DeleteThread(ctx context.Context, threadID string) error

    // Message management
    AddMessage(ctx context.Context, threadID string, message *Message) error
    GetMessages(ctx context.Context, threadID string, limit int, offset int) ([]*Message, error)

    // Search and filtering
    ListThreads(ctx context.Context, userID string, limit int, offset int) ([]*Thread, error)
    SearchThreads(ctx context.Context, query string, userID string) ([]*Thread, error)
}
```

### Thread Structure

```go
type Thread struct {
    ID        string                 `json:"id"`
    UserID    string                 `json:"user_id,omitempty"`
    CreatedAt time.Time              `json:"created_at"`
    UpdatedAt time.Time              `json:"updated_at"`
    Messages  []*Message             `json:"messages"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type Message struct {
    ID        string                 `json:"id"`
    ThreadID  string                 `json:"thread_id"`
    Role      string                 `json:"role"`      // "user", "assistant", "system"
    Content   string                 `json:"content"`
    ToolCalls []*ToolCall            `json:"tool_calls,omitempty"`
    CreatedAt time.Time              `json:"created_at"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
```

### Built-in Repositories

```go
// In-memory repository (not persistent)
func NewInMemoryThreadRepository() ThreadRepository

// File-based repository
func NewFileThreadRepository(basePath string) ThreadRepository

// PostgreSQL repository
func NewPostgresThreadRepository(connectionString string) (ThreadRepository, error)
```

## Error Handling

### Common Errors

```go
var (
    ErrThreadsAreNotEnabled = errors.New("threads are not enabled")
    ErrLLMNoResponse       = errors.New("llm did not return a response")
    ErrNoInstructions      = errors.New("no instructions provided")
    ErrNoLLM               = errors.New("no llm provided")
)
```

### Error Types

```go
// Configuration errors
type ConfigurationError struct {
    Field   string
    Message string
}

// Runtime errors
type ResponseError struct {
    AgentName string
    ThreadID  string
    Cause     error
}

// Tool execution errors
type ToolError struct {
    ToolName   string
    Parameters map[string]interface{}
    Cause      error
}
```

### Error Recovery Patterns

```go
// Retry with exponential backoff
response, err := agent.CreateResponse(ctx, opts...)
if err != nil {
    if isRetryable(err) {
        time.Sleep(backoffDuration)
        response, err = agent.CreateResponse(ctx, opts...)
    }
}

// Graceful degradation
response, err := agent.CreateResponse(ctx,
    dive.WithInput("Complex query requiring tools"))
if isToolError(err) {
    // Fallback to simpler approach
    response, err = agent.CreateResponse(ctx,
        dive.WithInput("Simple query without tools"))
}
```

## Examples

### Basic Agent Creation

```go
package main

import (
    "context"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
)

func main() {
    // Create basic agent
    assistant, err := dive.NewAgent(dive.AgentOptions{
        Name:         "Assistant",
        Instructions: "You are a helpful AI assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal(err)
    }

    // Generate response
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithInput("What is machine learning?"),
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Response: %s", response.Text())
}
```

### Agent with Tools and Memory

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/threads"
    "github.com/deepnoodle-ai/dive/toolkit"
)

func createAdvancedAgent() (dive.Agent, error) {
    // Create thread repository for memory
    threadRepo := threads.NewMemoryRepository()

    // Create agent with tools and memory
    return dive.NewAgent(dive.AgentOptions{
        Name: "Research Assistant",
        Instructions: `You are a research assistant who can search the web,
                      read files, and maintain conversation history.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
        ThreadRepository: threadRepo,
        ModelSettings: &dive.ModelSettings{
            Temperature:       &[]float64{0.7}[0],
            MaxTokens:         &[]int{4000}[0],
            ParallelToolCalls: &[]bool{true}[0],
        },
        ResponseTimeout:    time.Minute * 5,
        ToolIterationLimit: 10,
        DateAwareness:      &[]bool{true}[0],
    })
}
```

### Streaming Response

```go
func streamingExample(agent dive.Agent) error {
    stream, err := agent.StreamResponse(
        context.Background(),
        dive.WithInput("Tell me about quantum computing"),
        dive.WithEventCallback(func(ctx context.Context, event *dive.ResponseEvent) error {
            switch event.Type {
            case dive.EventTypeMessageDelta:
                // Print streaming text
                if delta := event.Data["delta"]; delta != nil {
                    fmt.Print(delta)
                }
            case dive.EventTypeToolCallStarted:
                fmt.Printf("\n[Using tool: %s]\n", event.ToolCall.Name)
            case dive.EventTypeError:
                fmt.Printf("\n[Error: %s]\n", event.Error)
            }
            return nil
        }),
    )
    if err != nil {
        return err
    }
    defer stream.Close()

    // Wait for completion
    for event := range stream.Events() {
        // Events are handled by callback
        _ = event
    }

    // Get final response
    response, err := stream.Response()
    if err != nil {
        return err
    }

    fmt.Printf("\nFinal response: %s\n", response.Text())
    return nil
}
```

### Custom Model Settings

```go
func createSpecializedAgent() (dive.Agent, error) {
    return dive.NewAgent(dive.AgentOptions{
        Name: "Code Reviewer",
        Instructions: "You are a thorough code reviewer focused on quality and security.",
        Model: anthropic.New(),
        ModelSettings: &dive.ModelSettings{
            // Low temperature for consistency
            Temperature: &[]float64{0.2}[0],

            // Longer responses for detailed reviews
            MaxTokens: &[]int{8000}[0],

            // Require tool usage for analysis
            ToolChoice: llm.RequiredToolChoice(),

            // Enable caching for better performance
            Caching: &[]bool{true}[0],

            // Disable parallel tools for sequential analysis
            ParallelToolCalls: &[]bool{false}[0],
        },
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(NewCodeAnalysisTool()),
            dive.ToolAdapter(NewSecurityScanTool()),
        },
    })
}
```

### Supervisor Agent

```go
func createSupervisorAgent(subordinates []string) (dive.Agent, error) {
    return dive.NewAgent(dive.AgentOptions{
        Name:         "Project Manager",
        IsSupervisor: true,
        Subordinates: subordinates, // ["Developer", "Designer", "QA Tester"]
        Instructions: `You coordinate work between team members, assign tasks,
                      and ensure project milestones are met.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(NewAssignWorkTool()),
            dive.ToolAdapter(NewProjectTrackingTool()),
        },
        ModelSettings: &dive.ModelSettings{
            Temperature: &[]float64{0.4}[0], // Balanced for planning
        },
    })
}
```

This API reference provides comprehensive coverage of the agent package, enabling developers to create sophisticated AI agents with custom behaviors, tool integration, and memory management.
