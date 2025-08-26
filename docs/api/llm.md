# LLM API Reference

Complete API reference for the Dive LLM package, covering language model integration, message handling, and provider configuration.

## 📋 Table of Contents

- [Core Interfaces](#core-interfaces)
- [Message System](#message-system)
- [Content Types](#content-types)
- [Configuration Options](#configuration-options)
- [Provider Integration](#provider-integration)
- [Tool Integration](#tool-integration)
- [Response Handling](#response-handling)
- [Streaming](#streaming)
- [Error Handling](#error-handling)
- [Examples](#examples)

## Core Interfaces

### `llm.LLM`

The core LLM interface that all language model providers must implement.

```go
type LLM interface {
    // Name of the LLM provider
    Name() string
    
    // Generate a response from the LLM by passing messages.
    Generate(ctx context.Context, opts ...Option) (*Response, error)
}
```

### `llm.StreamingLLM`

Extended interface for providers that support streaming responses.

```go
type StreamingLLM interface {
    LLM
    
    // Stream starts a streaming response from the LLM by passing messages.
    // The caller should call Close on the returned Stream when done.
    Stream(ctx context.Context, opts ...Option) (StreamIterator, error)
}
```

### `llm.StreamIterator`

Interface for consuming streaming responses.

```go
type StreamIterator interface {
    // Next advances the stream to the next event. Returns false when complete.
    Next() bool
    
    // Event returns the current event in the stream.
    Event() *Event
    
    // Err returns any error that occurred while reading.
    Err() error
    
    // Close closes the stream and releases resources.
    Close() error
}
```

## Message System

### `llm.Message`

Core message structure for conversation flow.

```go
type Message struct {
    ID      string    `json:"id,omitempty"`
    Role    Role      `json:"role"`
    Content []Content `json:"content"`
}
```

### Message Roles

```go
type Role string

const (
    User      Role = "user"      // Messages from the user
    Assistant Role = "assistant" // Messages from the AI
    System    Role = "system"    // System instructions
)
```

### Message Methods

```go
// Content extraction
func (m *Message) Text() string                    // Concatenated text content
func (m *Message) LastText() string                // Last text block
func (m *Message) ImageContent() (*ImageContent, bool) // First image content
func (m *Message) ThinkingContent() (*ThinkingContent, bool) // Thinking content

// Content manipulation
func (m *Message) WithText(text ...string) *Message        // Append text blocks
func (m *Message) WithContent(content ...Content) *Message // Append content blocks

// JSON decoding
func (m *Message) DecodeInto(v any) error          // Decode JSON response
```

### Message Constructors

```go
// Text messages
func NewUserTextMessage(text string) *Message
func NewAssistantTextMessage(text string) *Message
func NewSystemTextMessage(text string) *Message

// Multi-content messages
func NewUserMessage(content ...Content) *Message
func NewAssistantMessage(content ...Content) *Message
func NewSystemMessage(content ...Content) *Message

// Structured messages
func NewUserImageMessage(imageData []byte, mediaType string) *Message
func NewUserFileMessage(filePath string) *Message
```

### Message Collections

```go
type Messages []*Message

// Helper methods for message collections
func (msgs Messages) Text() []string              // Extract all text
func (msgs Messages) FilterByRole(role Role) Messages  // Filter by role
func (msgs Messages) Last(n int) Messages         // Get last n messages
```

## Content Types

### Base Content Interface

```go
type Content interface {
    Type() ContentType
}
```

### Content Types

```go
type ContentType string

const (
    ContentTypeText                    ContentType = "text"
    ContentTypeImage                   ContentType = "image"
    ContentTypeDocument                ContentType = "document"
    ContentTypeFile                    ContentType = "file"
    ContentTypeToolUse                 ContentType = "tool_use"
    ContentTypeToolResult              ContentType = "tool_result"
    ContentTypeThinking                ContentType = "thinking"
    ContentTypeRefusal                 ContentType = "refusal"
    ContentTypeDynamic                 ContentType = "dynamic"
    // ... additional types
)
```

### Text Content

```go
type TextContent struct {
    Type         ContentType    `json:"type"`
    Text         string         `json:"text"`
    CacheControl *CacheControl  `json:"cache_control,omitempty"`
}

func NewTextContent(text string) *TextContent
func (c *TextContent) Type() ContentType { return ContentTypeText }
```

### Image Content

```go
type ImageContent struct {
    Type         ContentType    `json:"type"`
    Source       *ContentSource `json:"source"`
    CacheControl *CacheControl  `json:"cache_control,omitempty"`
}

func NewImageContent(data []byte, mediaType string) *ImageContent
func NewImageContentFromURL(url, mediaType string) *ImageContent
func NewImageContentFromFile(filePath string) (*ImageContent, error)
```

### Document Content

```go
type DocumentContent struct {
    Type         ContentType    `json:"type"`
    Source       *ContentSource `json:"source"`
    CacheControl *CacheControl  `json:"cache_control,omitempty"`
}

func NewDocumentContent(data []byte, mediaType string) *DocumentContent
func NewDocumentContentFromFile(filePath string) (*DocumentContent, error)
```

### Tool Content

```go
type ToolUseContent struct {
    Type         ContentType            `json:"type"`
    ID           string                 `json:"id"`
    Name         string                 `json:"name"`
    Input        map[string]interface{} `json:"input"`
    CacheControl *CacheControl          `json:"cache_control,omitempty"`
}

type ToolResultContent struct {
    Type         ContentType   `json:"type"`
    ToolUseID    string        `json:"tool_use_id"`
    Content      []Content     `json:"content,omitempty"`
    IsError      bool          `json:"is_error,omitempty"`
    CacheControl *CacheControl `json:"cache_control,omitempty"`
}
```

### Content Source

```go
type ContentSource struct {
    Type               ContentSourceType `json:"type"`
    MediaType          string           `json:"media_type,omitempty"`
    Data               string           `json:"data,omitempty"`        // Base64 data
    URL                string           `json:"url,omitempty"`         // URL reference
    FileID             string           `json:"file_id,omitempty"`     // File API ID
    Content            []*ContentChunk  `json:"content,omitempty"`     // Chunked content
    GenerationID       string           `json:"generation_id,omitempty"`
    GenerationStatus   string           `json:"generation_status,omitempty"`
}

type ContentSourceType string

const (
    ContentSourceTypeBase64 ContentSourceType = "base64"
    ContentSourceTypeURL    ContentSourceType = "url"
    ContentSourceTypeText   ContentSourceType = "text"
    ContentSourceTypeFile   ContentSourceType = "file"
)

// Helper methods
func (c *ContentSource) DecodedData() ([]byte, error)  // Decode base64 data
```

## Configuration Options

### `llm.Config`

Core configuration structure for LLM requests.

```go
type Config struct {
    // Model configuration
    Model              string                   `json:"model,omitempty"`
    SystemPrompt       string                   `json:"system_prompt,omitempty"`
    Endpoint           string                   `json:"endpoint,omitempty"`
    APIKey             string                   `json:"api_key,omitempty"`
    
    // Response configuration
    MaxTokens          *int                     `json:"max_tokens,omitempty"`
    Temperature        *float64                 `json:"temperature,omitempty"`
    PresencePenalty    *float64                 `json:"presence_penalty,omitempty"`
    FrequencyPenalty   *float64                 `json:"frequency_penalty,omitempty"`
    
    // Reasoning configuration (o1 models)
    ReasoningBudget    *int                     `json:"reasoning_budget,omitempty"`
    ReasoningEffort    ReasoningEffort          `json:"reasoning_effort,omitempty"`
    ReasoningSummary   ReasoningSummary         `json:"reasoning_summary,omitempty"`
    
    // Tool configuration
    Tools              []Tool                   `json:"tools,omitempty"`
    ToolChoice         *ToolChoice              `json:"tool_choice,omitempty"`
    ParallelToolCalls  *bool                    `json:"parallel_tool_calls,omitempty"`
    
    // Advanced features
    Features           []string                 `json:"features,omitempty"`
    RequestHeaders     http.Header              `json:"request_headers,omitempty"`
    MCPServers         []MCPServerConfig        `json:"mcp_servers,omitempty"`
    Caching            *bool                    `json:"caching,omitempty"`
    ResponseFormat     *ResponseFormat          `json:"response_format,omitempty"`
    
    // Provider-specific
    ServiceTier        string                   `json:"service_tier,omitempty"`
    ProviderOptions    map[string]interface{}   `json:"provider_options,omitempty"`
    
    // Request context
    Messages           Messages                 `json:"messages"`
    PreviousResponseID string                   `json:"previous_response_id,omitempty"`
    
    // Runtime configuration
    Hooks              Hooks                    `json:"-"`
    Client             *http.Client             `json:"-"`
    Logger             slogger.Logger           `json:"-"`
    SSECallback        ServerSentEventsCallback `json:"-"`
}
```

### Configuration Options

```go
// Model and endpoint
func WithModel(model string) Option
func WithEndpoint(endpoint string) Option
func WithAPIKey(apiKey string) Option

// Response control
func WithMaxTokens(maxTokens int) Option
func WithTemperature(temperature float64) Option
func WithPresencePenalty(penalty float64) Option
func WithFrequencyPenalty(penalty float64) Option

// System prompts
func WithSystemPrompt(prompt string) Option
func WithPrefill(text string) Option

// Tool configuration
func WithTools(tools ...Tool) Option
func WithToolChoice(choice *ToolChoice) Option
func WithParallelToolCalls(enabled bool) Option

// Advanced features
func WithFeatures(features ...string) Option
func WithCaching(enabled bool) Option
func WithResponseFormat(format *ResponseFormat) Option

// Request configuration
func WithMessages(messages ...*Message) Option
func WithRequestHeaders(headers http.Header) Option
func WithClient(client *http.Client) Option

// Reasoning (o1 models)
func WithReasoningEffort(effort ReasoningEffort) Option
func WithReasoningBudget(budget int) Option
```

### Reasoning Configuration

```go
type ReasoningEffort string

const (
    ReasoningEffortLow    ReasoningEffort = "low"
    ReasoningEffortMedium ReasoningEffort = "medium"
    ReasoningEffortHigh   ReasoningEffort = "high"
)

type ReasoningSummary string

const (
    ReasoningSummaryEnabled  ReasoningSummary = "enabled"
    ReasoningSummaryDisabled ReasoningSummary = "disabled"
)
```

### Response Format

```go
type ResponseFormat struct {
    Type       string  `json:"type"`       // "text" or "json_object"
    JSONSchema *Schema `json:"json_schema,omitempty"`
}

func TextResponseFormat() *ResponseFormat
func JSONResponseFormat() *ResponseFormat
func JSONSchemaResponseFormat(schema *Schema) *ResponseFormat
```

## Provider Integration

### Built-in Providers

```go
// Anthropic (Claude)
import "github.com/diveagents/dive/llm/providers/anthropic"
model := anthropic.New()
model := anthropic.NewWithOptions(anthropic.Options{
    APIKey:      "your-api-key",
    BaseURL:     "https://api.anthropic.com",
    Model:       "claude-sonnet-4-20250514",
    MaxTokens:   4000,
    Temperature: 0.7,
})

// OpenAI (GPT)
import "github.com/diveagents/dive/llm/providers/openai"
model := openai.New()
model := openai.NewWithOptions(openai.Options{
    APIKey:           "your-api-key",
    BaseURL:          "https://api.openai.com/v1",
    Model:            "gpt-4o",
    MaxTokens:        4000,
    Temperature:      0.7,
    OrganizationID:   "org-id",
})

// Groq
import "github.com/diveagents/dive/llm/providers/groq"
model := groq.New()

// Ollama (Local)
import "github.com/diveagents/dive/llm/providers/ollama"
model := ollama.New()
model := ollama.NewWithOptions(ollama.Options{
    BaseURL: "http://localhost:11434",
    Model:   "llama3",
})
```

### Provider-Specific Features

```go
// Anthropic features
features := []string{
    "prompt_caching",
    "computer_use",
    "code_execution",
}

// OpenAI features  
features := []string{
    "function_calling",
    "vision",
    "dall_e_3",
}
```

## Tool Integration

### Tool Interface

```go
type Tool interface {
    // Tool metadata
    Name() string
    Description() string
    
    // Parameter schema
    Parameters() *Schema
    
    // Execution
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}
```

### Tool Choice Configuration

```go
type ToolChoice struct {
    Type string `json:"type"`            // "auto", "required", "none", "tool"
    Name string `json:"name,omitempty"`  // Specific tool name
}

// Helper constructors
func AutoToolChoice() *ToolChoice         // Let model decide
func RequiredToolChoice() *ToolChoice     // Must use a tool
func NoToolChoice() *ToolChoice           // Disable tools
func SpecificToolChoice(name string) *ToolChoice  // Use specific tool
```

### Tool Registration

```go
// Register tools with LLM
tools := []Tool{
    NewWebSearchTool(),
    NewCalculatorTool(),
    NewFileReadTool(),
}

response, err := llm.Generate(ctx, 
    WithTools(tools...),
    WithToolChoice(AutoToolChoice()),
    WithParallelToolCalls(true),
    WithMessages(messages...),
)
```

## Response Handling

### `llm.Response`

Response structure containing LLM output and metadata.

```go
type Response struct {
    ID               string                 `json:"id"`
    Model            string                 `json:"model"`
    Messages         []*Message             `json:"messages"`
    FinishReason     string                 `json:"finish_reason"`
    Usage            *TokenUsage            `json:"usage"`
    Headers          http.Header            `json:"headers"`
    ResponseMetadata map[string]interface{} `json:"response_metadata"`
}

// Content access
func (r *Response) Text() string                    // Response text
func (r *Response) LastMessage() *Message           // Last message
func (r *Response) ToolCalls() []*ToolCall          // Tool calls made

// Metadata access
func (r *Response) TokensUsed() int                 // Total tokens
func (r *Response) InputTokens() int                // Input tokens
func (r *Response) OutputTokens() int               // Output tokens
func (r *Response) CacheTokens() int                // Cached tokens

// JSON decoding
func (r *Response) DecodeInto(v any) error          // Decode JSON response
```

### Token Usage

```go
type TokenUsage struct {
    InputTokens            int `json:"input_tokens,omitempty"`
    OutputTokens           int `json:"output_tokens,omitempty"`
    TotalTokens            int `json:"total_tokens,omitempty"`
    CacheTokens            int `json:"cache_tokens,omitempty"`
    CacheReadTokens        int `json:"cache_read_tokens,omitempty"`
    CacheWriteTokens       int `json:"cache_write_tokens,omitempty"`
    ReasoningTokens        int `json:"reasoning_tokens,omitempty"`
    AudioTokens            int `json:"audio_tokens,omitempty"`
}
```

### Finish Reasons

```go
const (
    FinishReasonStop         = "stop"           // Natural completion
    FinishReasonMaxTokens    = "max_tokens"     // Token limit reached
    FinishReasonToolCalls    = "tool_calls"     // Tool calls initiated
    FinishReasonContentFilter = "content_filter" // Content filtered
    FinishReasonError        = "error"          // Error occurred
)
```

## Streaming

### Stream Events

```go
type Event struct {
    Type      EventType              `json:"type"`
    Data      map[string]interface{} `json:"data"`
    Message   *Message               `json:"message,omitempty"`
    Usage     *TokenUsage            `json:"usage,omitempty"`
    Error     error                  `json:"error,omitempty"`
}

type EventType string

const (
    EventTypeMessageStart    EventType = "message_start"
    EventTypeMessageDelta    EventType = "message_delta"
    EventTypeMessageStop     EventType = "message_stop"
    EventTypeContentStart    EventType = "content_start"
    EventTypeContentDelta    EventType = "content_delta"
    EventTypeContentStop     EventType = "content_stop"
    EventTypeToolCallStart   EventType = "tool_call_start"
    EventTypeToolCallDelta   EventType = "tool_call_delta"
    EventTypeToolCallStop    EventType = "tool_call_stop"
    EventTypeError           EventType = "error"
)
```

### Consuming Streams

```go
stream, err := llm.Stream(ctx, 
    WithModel("claude-sonnet-4-20250514"),
    WithMessages(messages...),
)
if err != nil {
    return err
}
defer stream.Close()

var responseText strings.Builder

for stream.Next() {
    event := stream.Event()
    
    switch event.Type {
    case EventTypeMessageDelta:
        if delta, ok := event.Data["delta"].(string); ok {
            responseText.WriteString(delta)
            fmt.Print(delta) // Print streaming text
        }
    case EventTypeToolCallStart:
        fmt.Printf("\n[Using tool: %s]\n", event.Data["name"])
    case EventTypeError:
        fmt.Printf("Error: %v\n", event.Error)
    }
}

if err := stream.Err(); err != nil {
    return err
}

fmt.Printf("\nFinal response: %s\n", responseText.String())
```

## Error Handling

### Error Types

```go
// Provider errors
type ProviderError struct {
    Provider string
    Code     string
    Message  string
    Details  map[string]interface{}
}

// Rate limiting errors
type RateLimitError struct {
    Provider     string
    RetryAfter   time.Duration
    LimitType    string // "requests" or "tokens"
    CurrentUsage int
    Limit        int
}

// Authentication errors
type AuthenticationError struct {
    Provider string
    Message  string
}

// Model errors
type ModelError struct {
    Provider    string
    Model       string
    Message     string
    Available   []string // Available models
}

// Tool execution errors
type ToolExecutionError struct {
    ToolName   string
    Parameters map[string]interface{}
    Cause      error
}
```

### Error Checking

```go
response, err := llm.Generate(ctx, options...)
if err != nil {
    switch e := err.(type) {
    case *RateLimitError:
        fmt.Printf("Rate limited, retry after: %v\n", e.RetryAfter)
        time.Sleep(e.RetryAfter)
        // Retry request
        
    case *AuthenticationError:
        return fmt.Errorf("authentication failed: %s", e.Message)
        
    case *ModelError:
        fmt.Printf("Model %s not available. Options: %v\n", e.Model, e.Available)
        
    case *ToolExecutionError:
        fmt.Printf("Tool %s failed: %v\n", e.ToolName, e.Cause)
        
    default:
        return fmt.Errorf("LLM request failed: %w", err)
    }
}
```

## Examples

### Basic Text Generation

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/diveagents/dive/llm"
    "github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
    model := anthropic.New()
    
    response, err := model.Generate(context.Background(),
        llm.WithModel("claude-sonnet-4-20250514"),
        llm.WithMessages(
            llm.NewUserTextMessage("What is machine learning?"),
        ),
        llm.WithMaxTokens(1000),
        llm.WithTemperature(0.7),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Response: %s\n", response.Text())
    fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
}
```

### Multi-Modal Input

```go
func processImageAndText() error {
    model := anthropic.New()
    
    // Load image
    imageData, err := os.ReadFile("diagram.png")
    if err != nil {
        return err
    }
    
    response, err := model.Generate(context.Background(),
        llm.WithMessages(
            llm.NewUserMessage(
                llm.NewTextContent("Analyze this diagram and explain what it shows:"),
                llm.NewImageContent(imageData, "image/png"),
            ),
        ),
        llm.WithMaxTokens(2000),
    )
    if err != nil {
        return err
    }
    
    fmt.Printf("Analysis: %s\n", response.Text())
    return nil
}
```

### Streaming with Tools

```go
func streamingWithTools(tools []llm.Tool) error {
    model := anthropic.New()
    
    stream, err := model.Stream(context.Background(),
        llm.WithModel("claude-sonnet-4-20250514"),
        llm.WithTools(tools...),
        llm.WithToolChoice(llm.AutoToolChoice()),
        llm.WithMessages(
            llm.NewUserTextMessage("Research recent developments in quantum computing"),
        ),
    )
    if err != nil {
        return err
    }
    defer stream.Close()
    
    for stream.Next() {
        event := stream.Event()
        
        switch event.Type {
        case llm.EventTypeMessageDelta:
            if delta := event.Data["delta"]; delta != nil {
                fmt.Print(delta)
            }
        case llm.EventTypeToolCallStart:
            fmt.Printf("\n[🔧 Using %s]\n", event.Data["name"])
        case llm.EventTypeToolCallStop:
            fmt.Printf("[✅ Tool completed]\n")
        }
    }
    
    return stream.Err()
}
```

### JSON Response Format

```go
func structuredResponse() error {
    model := openai.New()
    
    schema := &llm.Schema{
        Type: "object",
        Properties: map[string]*llm.Property{
            "analysis": {
                Type:        "string",
                Description: "Analysis of the text",
            },
            "sentiment": {
                Type: "string",
                Enum: []interface{}{"positive", "negative", "neutral"},
            },
            "confidence": {
                Type: "number",
                Minimum: 0,
                Maximum: 1,
            },
        },
        Required: []string{"analysis", "sentiment", "confidence"},
    }
    
    response, err := model.Generate(context.Background(),
        llm.WithModel("gpt-4o"),
        llm.WithResponseFormat(llm.JSONSchemaResponseFormat(schema)),
        llm.WithMessages(
            llm.NewUserTextMessage("Analyze this text: 'I love this product!'"),
        ),
    )
    if err != nil {
        return err
    }
    
    var result struct {
        Analysis   string  `json:"analysis"`
        Sentiment  string  `json:"sentiment"`
        Confidence float64 `json:"confidence"`
    }
    
    if err := response.DecodeInto(&result); err != nil {
        return err
    }
    
    fmt.Printf("Analysis: %s\n", result.Analysis)
    fmt.Printf("Sentiment: %s (%.2f confidence)\n", result.Sentiment, result.Confidence)
    return nil
}
```

### Provider Comparison

```go
func compareProviders(input string) error {
    providers := map[string]llm.LLM{
        "claude": anthropic.NewWithOptions(anthropic.Options{
            Model: "claude-sonnet-4-20250514",
        }),
        "gpt": openai.NewWithOptions(openai.Options{
            Model: "gpt-4o",
        }),
        "groq": groq.NewWithOptions(groq.Options{
            Model: "mixtral-8x7b-32768",
        }),
    }
    
    message := llm.NewUserTextMessage(input)
    
    for name, provider := range providers {
        response, err := provider.Generate(context.Background(),
            llm.WithMessages(message),
            llm.WithMaxTokens(500),
            llm.WithTemperature(0.3),
        )
        if err != nil {
            fmt.Printf("%s error: %v\n", name, err)
            continue
        }
        
        fmt.Printf("\n%s Response:\n", strings.Title(name))
        fmt.Printf("%s\n", response.Text())
        fmt.Printf("Tokens: %d\n", response.Usage.TotalTokens)
    }
    
    return nil
}
```

This comprehensive API reference covers all aspects of the LLM package, enabling developers to integrate multiple language model providers with advanced features like streaming, tool calling, multi-modal inputs, and structured outputs.