# Core Interfaces API Reference

This document covers the core interfaces and types that form the foundation of the Dive framework. These interfaces define the contracts for agents, environments, tools, and other essential components.

## 📋 Table of Contents

- [Agent Interface](#agent-interface)
- [Tool Interface](#tool-interface)
- [Response Types](#response-types)
- [Event System](#event-system)
- [Options and Configuration](#options-and-configuration)

## Agent Interface

The `Agent` interface defines the core contract for AI agents in Dive.

### Interface Definition

```go
type Agent interface {
    // Name returns the agent's identifier
    Name() string

    // IsSupervisor indicates if the agent can assign work to others
    IsSupervisor() bool

    // CreateResponse generates a complete response synchronously
    CreateResponse(ctx context.Context, opts ...Option) (*Response, error)

    // StreamResponse generates a response with real-time streaming
    StreamResponse(ctx context.Context, opts ...Option) (ResponseStream, error)
}
```

### Usage Examples

```go
// Basic agent interaction
response, err := agent.CreateResponse(
    context.Background(),
    dive.WithInput("Hello, how can you help me?"),
)
if err != nil {
    return err
}
fmt.Println(response.Text())

// Streaming response
stream, err := agent.StreamResponse(
    context.Background(),
    dive.WithInput("Write a story about AI"),
    dive.WithEventCallback(func(ctx context.Context, event *dive.ResponseEvent) error {
        if event.Type == dive.EventTypeLLMEvent {
            fmt.Print(event.Item.Event.Delta.Text)
        }
        return nil
    }),
)
```

## Tool Interface

Tools extend agent capabilities by providing access to external functionality.

### Tool Interface

```go
type Tool interface {
    // Name returns the tool identifier
    Name() string

    // Description returns human-readable tool description
    Description() string

    // Schema returns the JSON schema for tool parameters
    Schema() *schema.Schema

    // Annotations provides hints about tool behavior
    Annotations() *ToolAnnotations

    // Call executes the tool with given input
    Call(ctx context.Context, input json.RawMessage) (*ToolResult, error)
}
```

### TypedTool Interface

For type-safe tool development:

```go
type TypedTool[T any] interface {
    Name() string
    Description() string
    Schema() *schema.Schema
    Annotations() *ToolAnnotations
    Call(ctx context.Context, input *T) (*ToolResult, error)
}
```

### Tool Annotations

```go
type ToolAnnotations struct {
    Title           string         // Human-readable tool name
    ReadOnlyHint    bool           // Tool only reads, doesn't modify data
    DestructiveHint bool           // Tool may delete or overwrite data
    IdempotentHint  bool           // Safe to call multiple times with same input
    OpenWorldHint   bool           // Tool accesses external resources (network, APIs)
    Extra           map[string]any // Additional custom annotations
}
```

### Custom Tool Example

```go
import (
    "context"
    "fmt"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/schema"
)

type CalculatorTool struct{}

type CalculatorInput struct {
    Expression string `json:"expression" description:"Mathematical expression to evaluate"`
}

func (t *CalculatorTool) Name() string {
    return "calculate"
}

func (t *CalculatorTool) Description() string {
    return "Evaluate mathematical expressions"
}

func (t *CalculatorTool) Schema() *schema.Schema {
    return &schema.Schema{
        Type: "object",
        Properties: map[string]*schema.Property{
            "expression": {
                Type:        "string",
                Description: "Mathematical expression to evaluate",
            },
        },
        Required: []string{"expression"},
    }
}

func (t *CalculatorTool) Annotations() *dive.ToolAnnotations {
    return &dive.ToolAnnotations{
        Title:          "Calculator",
        ReadOnlyHint:   true,
        IdempotentHint: true,
    }
}

func (t *CalculatorTool) Call(ctx context.Context, input *CalculatorInput) (*dive.ToolResult, error) {
    result, err := evaluateExpression(input.Expression)
    if err != nil {
        return dive.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
    }

    return dive.NewToolResultText(fmt.Sprintf("Result: %v", result)), nil
}

// Use with ToolAdapter for type safety
tool := dive.ToolAdapter(&CalculatorTool{})
```

## Response Types

### Response

The main response type returned by agents:

```go
type Response struct {
    ID         string           `json:"id"`
    Model      string           `json:"model"`
    CreatedAt  time.Time        `json:"created_at"`
    FinishedAt *time.Time       `json:"finished_at,omitempty"`
    Usage      *llm.Usage       `json:"usage,omitempty"`
    Items      []*ResponseItem  `json:"items"`
}

// Helper methods
func (r *Response) Text() string                    // Extract text content
func (r *Response) Messages() []*llm.Message        // Get all messages
func (r *Response) ToolCalls() []*llm.ToolUseContent // Get tool calls
func (r *Response) ToolResults() []*ToolCallResult   // Get tool results
```

### ResponseItem

Individual items within a response:

```go
type ResponseItem struct {
    Type           ResponseItemType    `json:"type"`
    Message        *llm.Message        `json:"message,omitempty"`
    ToolCall       *llm.ToolUseContent `json:"tool_call,omitempty"`
    ToolCallResult *ToolCallResult     `json:"tool_call_result,omitempty"`
    Usage          *llm.Usage          `json:"usage,omitempty"`
    Event          *llm.ResponseEvent  `json:"event,omitempty"`
}

type ResponseItemType string

const (
    ResponseItemTypeMessage        ResponseItemType = "message"
    ResponseItemTypeToolCall       ResponseItemType = "tool_call"
    ResponseItemTypeToolCallResult ResponseItemType = "tool_call_result"
)
```

### ToolResult

Results returned by tool execution:

```go
type ToolResult struct {
    Content []*ToolResultContent `json:"content"`
    IsError bool                 `json:"is_error,omitempty"`
}

type ToolResultContent struct {
    Type  ToolResultContentType `json:"type"`
    Text  string               `json:"text,omitempty"`
    Image *ImageContent        `json:"image,omitempty"`
}

type ToolResultContentType string

const (
    ToolResultContentTypeText  ToolResultContentType = "text"
    ToolResultContentTypeImage ToolResultContentType = "image"
)
```

## Event System

### ResponseEvent

Events emitted during response generation:

```go
type ResponseEvent struct {
    Type     ResponseEventType `json:"type"`
    Response *Response         `json:"response,omitempty"`
    Item     *ResponseItem     `json:"item,omitempty"`
    Error    error            `json:"error,omitempty"`
}

type ResponseEventType string

const (
    EventTypeResponseCreated     ResponseEventType = "response.created"
    EventTypeResponseInProgress  ResponseEventType = "response.in_progress"
    EventTypeResponseCompleted   ResponseEventType = "response.completed"
    EventTypeResponseFailed      ResponseEventType = "response.failed"
    EventTypeResponseToolCall    ResponseEventType = "response.tool_call"
    EventTypeResponseToolResult  ResponseEventType = "response.tool_result"
    EventTypeLLMEvent           ResponseEventType = "llm.event"
)
```

### EventCallback

Function type for handling streaming events:

```go
type EventCallback func(ctx context.Context, event *ResponseEvent) error
```

### ResponseStream

Interface for streaming responses:

```go
type ResponseStream interface {
    Events() <-chan *ResponseEvent
    Close() error
}
```

## Options and Configuration

### Option Function Pattern

Dive uses the functional options pattern for flexible configuration:

```go
type Option func(*Options)

type Options struct {
    ThreadID      string
    UserID        string
    Messages      []*llm.Message
    EventCallback EventCallback
}
```

### Available Options

```go
// WithThreadID associates a conversation thread
func WithThreadID(threadID string) Option

// WithUserID identifies the user making the request
func WithUserID(userID string) Option

// WithMessage provides a single message
func WithMessage(message *llm.Message) Option

// WithMessages provides multiple messages
func WithMessages(messages ...*llm.Message) Option

// WithInput provides simple text input (creates user message)
func WithInput(input string) Option

// WithEventCallback sets up streaming event handling
func WithEventCallback(callback EventCallback) Option
```

### Usage Examples

```go
// Simple input
response, err := agent.CreateResponse(
    ctx,
    dive.WithInput("Hello!"),
)

// Thread-based conversation
response, err := agent.CreateResponse(
    ctx,
    dive.WithThreadID("user-123"),
    dive.WithUserID("alice"),
    dive.WithInput("Continue our discussion"),
)

// Custom messages with streaming
stream, err := agent.StreamResponse(
    ctx,
    dive.WithMessages(
        llm.NewUserTextMessage("Analyze this data"),
        llm.NewUserMessage(
            &llm.TextContent{Text: "Data:"},
            &llm.ImageContent{Source: imageData},
        ),
    ),
    dive.WithEventCallback(func(ctx context.Context, event *dive.ResponseEvent) error {
        // Handle streaming events
        return nil
    }),
)
```

## Repository Interfaces

### DocumentRepository

For managing documents and files:

```go
type DocumentRepository interface {
    GetDocument(ctx context.Context, path string) (*Document, error)
    PutDocument(ctx context.Context, doc *Document) error
    ListDocuments(ctx context.Context, prefix string) ([]*Document, error)
    DeleteDocument(ctx context.Context, path string) error
}

type Document struct {
    Path        string            `json:"path"`
    Content     string            `json:"content"`
    ContentType string            `json:"content_type"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### ThreadRepository

For managing conversation threads:

```go
type ThreadRepository interface {
    GetThread(ctx context.Context, id string) (*Thread, error)
    PutThread(ctx context.Context, thread *Thread) error
    DeleteThread(ctx context.Context, id string) error
}

type Thread struct {
    ID       string         `json:"id"`
    Messages []*llm.Message `json:"messages"`
}

var ErrThreadNotFound = errors.New("thread not found")
```

## Utility Functions

### ID Generation

```go
// NewID generates a new UUID-based identifier
func NewID() string
```

### Helper Functions

```go
// RandomName generates a random agent name
func RandomName() string

// DateString formats a time for LLM consumption
func DateString(t time.Time) string
```

### Error Types

```go
var (
    ErrAgentNotFound = errors.New("agent not found")
    ErrToolNotFound  = errors.New("tool not found")
    ErrInvalidInput  = errors.New("invalid input")
)
```

## Best Practices

### 1. Interface Implementation

```go
// Always verify interface compliance
var _ dive.Agent = (*MyAgent)(nil)
var _ dive.Tool = (*MyTool)(nil)

type MyAgent struct {
    // implementation
}

func (a *MyAgent) Name() string { return "MyAgent" }
// ... implement other methods
```

### 2. Error Handling

```go
response, err := agent.CreateResponse(ctx, dive.WithInput(input))
if err != nil {
    // Check for specific error types
    if errors.Is(err, dive.ErrThreadNotFound) {
        // Handle thread not found
    }
    return fmt.Errorf("agent response failed: %w", err)
}
```

### 3. Resource Management

```go
// Always use context for cancellation
ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
defer cancel()

// Close streams when done
stream, err := agent.StreamResponse(ctx, opts...)
if err != nil {
    return err
}
defer stream.Close()
```

### 4. Event Handling

```go
callback := func(ctx context.Context, event *dive.ResponseEvent) error {
    switch event.Type {
    case dive.EventTypeResponseToolCall:
        log.Printf("Tool called: %s", event.Item.ToolCall.Name)
    case dive.EventTypeResponseToolResult:
        if event.Item.ToolCallResult.Result.IsError {
            log.Printf("Tool error: %s", event.Item.ToolCallResult.Result.Content[0].Text)
        }
    }
    return nil
}
```

## Next Steps

- [Agent API Reference](agent.md) - Detailed agent implementation
- [LLM API Reference](llm.md) - LLM provider interfaces
- [Workflow API Reference](workflow.md) - Workflow engine APIs
- [Agent Guide](../guides/agents.md) - Practical agent usage
