# LLM API Reference

API reference for the Dive LLM package covering language model integration and message handling.

## Core Interfaces

### LLM Interface

```go
type LLM interface {
    Name() string
    Generate(ctx context.Context, opts ...Option) (*Response, error)
}
```

### StreamingLLM Interface

```go
type StreamingLLM interface {
    LLM
    Stream(ctx context.Context, opts ...Option) (StreamIterator, error)
}
```

## Message System

### Message Types

```go
type Message struct {
    Role    MessageRole `json:"role"`
    Content []Content   `json:"content"`
}

type MessageRole string
const (
    MessageRoleSystem    MessageRole = "system"
    MessageRoleUser      MessageRole = "user"
    MessageRoleAssistant MessageRole = "assistant"
    MessageRoleTool      MessageRole = "tool"
)
```

### Content Types

```go
type Content interface {
    Type() string
}

type TextContent struct {
    Text string `json:"text"`
}

type ImageContent struct {
    Source ImageSource `json:"source"`
}
```

## Configuration Options

### Generation Options

```go
func WithMessages(messages ...*Message) Option
func WithMaxTokens(maxTokens int) Option
func WithTemperature(temperature float64) Option
func WithTools(tools []Tool) Option
func WithSystemPrompt(systemPrompt string) Option
```

### Model Settings

```go
type ModelSettings struct {
    Temperature       *float64
    MaxTokens         int
    ReasoningBudget   *int
    ReasoningEffort   string
    ParallelToolCalls *bool
    Caching          *bool
}
```

## Response Handling

### Response Structure

```go
type Response struct {
    Message     *Message `json:"message"`
    Usage       Usage    `json:"usage"`
    FinishReason string   `json:"finish_reason"`
}

func (r *Response) Text() string
func (r *Response) ToolCalls() []ToolCall
```

### Usage Information

```go
type Usage struct {
    InputTokens     int `json:"input_tokens"`
    OutputTokens    int `json:"output_tokens"`
    CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
    CacheReadTokens     *int `json:"cache_read_tokens,omitempty"`
}
```

## Tool Integration

### Tool Call Structure

```go
type ToolCall struct {
    ID    string          `json:"id"`
    Name  string          `json:"name"`
    Input json.RawMessage `json:"input"`
}
```

### Tool Results

```go
type ToolResult struct {
    ToolCallID string    `json:"tool_call_id"`
    Content    []Content `json:"content"`
    IsError    bool      `json:"is_error,omitempty"`
}
```

## Streaming

### Stream Iterator

```go
type StreamIterator interface {
    Next() bool
    Event() *StreamEvent
    Err() error
    Close() error
}

type StreamEvent struct {
    Type    string      `json:"type"`
    Content interface{} `json:"content"`
}
```

## Error Handling

### Common Error Types

```go
type APIError struct {
    Type    string `json:"type"`
    Message string `json:"message"`
    Code    int    `json:"code"`
}

type RateLimitError struct {
    RetryAfter time.Duration
}
```

## Usage Examples

### Basic Generation

```go
model := anthropic.New()
response, err := model.Generate(
    context.Background(),
    llm.WithMessages(llm.NewUserTextMessage("Hello")),
    llm.WithMaxTokens(1000),
)
```

### Streaming

```go
stream, err := model.(llm.StreamingLLM).Stream(
    context.Background(),
    llm.WithMessages(llm.NewUserTextMessage("Tell me a story")),
)
defer stream.Close()

for stream.Next() {
    event := stream.Event()
    // Process event
}
```

### Tool Usage

```go
response, err := model.Generate(
    context.Background(),
    llm.WithMessages(llm.NewUserTextMessage("Search for Go tutorials")),
    llm.WithTools([]Tool{searchTool}),
)
```