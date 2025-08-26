# Tools Guide

Tools are the key mechanism for extending agent capabilities in Dive. They allow agents to interact with external systems, perform computations, access files, search the web, and execute custom logic. This guide covers everything you need to know about using and creating tools.

## 📋 Table of Contents

- [What are Tools?](#what-are-tools)
- [Built-in Tools](#built-in-tools)
- [Tool Annotations](#tool-annotations)
- [Using Tools with Agents](#using-tools-with-agents)
- [Creating Custom Tools](#creating-custom-tools)
- [Tool Adapters](#tool-adapters)
- [Advanced Tool Patterns](#advanced-tool-patterns)
- [Error Handling](#error-handling)
- [Best Practices](#best-practices)

## What are Tools?

Tools in Dive are functions that agents can call during conversations to:

- **Access external data** - Web search, API calls, file system access
- **Perform computations** - Mathematical calculations, data processing
- **Take actions** - Send emails, create files, execute commands
- **Interact with services** - Database queries, cloud service operations

Tools are defined with JSON schemas that describe their parameters, making them discoverable and usable by LLMs through function calling.

### Tool Architecture

```
Agent Input → LLM Processing → Tool Call → Tool Execution → Tool Result → LLM Response
```

## Built-in Tools

Dive provides several built-in tools covering common use cases:

### File System Tools

#### Read File Tool
```go
import "github.com/diveagents/dive/toolkit"

readTool := dive.ToolAdapter(toolkit.NewReadFileTool())

// Usage in agent
agent, err := agent.New(agent.Options{
    Name: "File Reader",
    Tools: []dive.Tool{readTool},
})
```

**Capabilities:**
- Read text files from the file system
- Supports various file formats (text, JSON, CSV, etc.)
- Handles encoding detection
- Safe path validation

#### Write File Tool
```go
writeTool := dive.ToolAdapter(toolkit.NewWriteFileTool())
```

**Capabilities:**
- Create new files or overwrite existing ones
- Supports various content types
- Directory creation if needed
- Safe path validation

#### List Directory Tool
```go
listTool := dive.ToolAdapter(toolkit.NewListDirectoryTool())
```

**Capabilities:**
- List files and directories
- Filter by file type or pattern
- Include file metadata (size, modification time)
- Recursive directory traversal option

### Web Tools

#### Web Search Tool
```go
searchTool := dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
    Provider: "google", // or "kagi"
}))
```

**Capabilities:**
- Search the web using Google Custom Search or Kagi
- Configurable result count
- Safe search options
- Result ranking and filtering

**Setup:**
```bash
export GOOGLE_SEARCH_API_KEY="your-google-api-key"
export GOOGLE_SEARCH_CX="your-search-engine-id"
# or
export KAGI_API_KEY="your-kagi-api-key"
```

#### Fetch Tool
```go
fetchTool := dive.ToolAdapter(toolkit.NewFetchTool())
```

**Capabilities:**
- Fetch content from web pages
- Extract clean text content using Firecrawl
- Handle various content types
- Respect robots.txt and rate limits

**Setup:**
```bash
export FIRECRAWL_API_KEY="your-firecrawl-api-key"
```

### System Tools

#### Command Tool
```go
commandTool := dive.ToolAdapter(toolkit.NewCommandTool())
```

**Capabilities:**
- Execute system commands
- Capture stdout and stderr
- Set working directory
- Environment variable control
- Timeout protection

**Security Note:** Use with caution in production environments.

#### Text Editor Tool
```go
editorTool := dive.ToolAdapter(toolkit.NewTextEditorTool())
```

**Capabilities:**
- Advanced file editing operations
- View, create, replace, and insert operations
- Line-based editing with context
- Backup creation before changes

### Creative Tools

#### Generate Image Tool
```go
imageTool := dive.ToolAdapter(toolkit.NewGenerateImageTool())
```

**Capabilities:**
- Generate images using OpenAI's image models
- Configurable image size and quality
- Save images to file system
- Various output formats

**Setup:**
```bash
export OPENAI_API_KEY="your-openai-api-key"
```

## Tool Annotations

Tool annotations provide hints about tool behavior to help agents use them effectively:

```go
type ToolAnnotations struct {
    Title           string  // Human-readable tool name
    ReadOnlyHint    bool    // Tool only reads data
    DestructiveHint bool    // Tool may delete/modify data
    IdempotentHint  bool    // Safe to call multiple times
    OpenWorldHint   bool    // Accesses external resources
}
```

### Annotation Examples

```go
// Read-only web search tool
annotations := dive.ToolAnnotations{
    Title:           "Web Search",
    ReadOnlyHint:    true,
    IdempotentHint:  true,
    OpenWorldHint:   true,
}

// Destructive file operations
annotations := dive.ToolAnnotations{
    Title:           "File Writer",
    ReadOnlyHint:    false,
    DestructiveHint: true,
    IdempotentHint:  false,
    OpenWorldHint:   false,
}

// Safe computational tool
annotations := dive.ToolAnnotations{
    Title:           "Calculator",
    ReadOnlyHint:    true,
    IdempotentHint:  true,
    OpenWorldHint:   false,
}
```

## Using Tools with Agents

### Basic Tool Setup

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

func main() {
    // Create agent with multiple tools
    researcher, err := agent.New(agent.Options{
        Name: "Research Agent",
        Instructions: `You are a research assistant with access to web search,
                      file operations, and content fetching capabilities.
                      Use these tools to help users with their research needs.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewFetchTool()),
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Use the agent with tools
    response, err := researcher.CreateResponse(
        context.Background(),
        dive.WithInput("Research the latest developments in quantum computing and save a summary to quantum-research.txt"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.Text())
    
    // Show which tools were used
    toolCalls := response.ToolCalls()
    fmt.Printf("\nTools used: %d\n", len(toolCalls))
    for _, call := range toolCalls {
        fmt.Printf("- %s\n", call.Name)
    }
}
```

### Tool-Specific Agents

```go
// File management specialist
func createFileManager() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "File Manager",
        Instructions: `You are a file management specialist. You can read, write,
                      organize, and analyze files efficiently.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewListDirectoryTool()),
            dive.ToolAdapter(toolkit.NewTextEditorTool()),
        },
    })
}

// Web researcher specialist
func createWebResearcher() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Web Researcher",
        Instructions: `You are a web research specialist. You can search the web,
                      fetch content from URLs, and analyze web-based information.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewFetchTool()),
        },
    })
}

// System administrator
func createSysAdmin() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "System Administrator",
        Instructions: `You are a system administrator who can execute commands
                      and manage system resources. Be careful with destructive operations.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewCommandTool()),
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewListDirectoryTool()),
        },
    })
}
```

## Creating Custom Tools

### Simple Custom Tool

```go
package main

import (
    "context"
    "fmt"
    "math"
    
    "github.com/diveagents/dive"
)

// Calculator tool implementation
type CalculatorTool struct{}

type CalculatorInput struct {
    Expression string `json:"expression" description:"Mathematical expression to evaluate"`
}

func (t *CalculatorTool) Name() string {
    return "calculate"
}

func (t *CalculatorTool) Description() string {
    return "Evaluate mathematical expressions and perform calculations"
}

func (t *CalculatorTool) Schema() dive.Schema {
    return dive.Schema{
        Type: "object",
        Properties: map[string]dive.Property{
            "expression": {
                Type:        "string",
                Description: "Mathematical expression to evaluate (e.g., '2 + 3 * 4')",
            },
        },
        Required: []string{"expression"},
    }
}

func (t *CalculatorTool) Annotations() dive.ToolAnnotations {
    return dive.ToolAnnotations{
        Title:          "Calculator",
        ReadOnlyHint:   true,
        IdempotentHint: true,
        OpenWorldHint:  false,
    }
}

func (t *CalculatorTool) Call(ctx context.Context, input *CalculatorInput) (*dive.ToolResult, error) {
    // Simple expression evaluator (in practice, use a proper parser)
    result, err := evaluateExpression(input.Expression)
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Error evaluating expression: %v", err),
            }},
            IsError: true,
        }, nil
    }

    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: fmt.Sprintf("Result: %g", result),
        }},
    }, nil
}

// Simple expression evaluator (replace with proper parser in production)
func evaluateExpression(expr string) (float64, error) {
    // This is a simplified example - use a proper math parser
    switch expr {
    case "2 + 2":
        return 4, nil
    case "10 * 5":
        return 50, nil
    default:
        return 0, fmt.Errorf("unsupported expression: %s", expr)
    }
}

func main() {
    // Use the custom tool
    calculator := dive.ToolAdapter(&CalculatorTool{})
    
    agent, err := agent.New(agent.Options{
        Name:  "Math Assistant",
        Tools: []dive.Tool{calculator},
    })
    if err != nil {
        panic(err)
    }
    
    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("Calculate 2 + 2"),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Println(response.Text())
}
```

### Advanced Custom Tool with Configuration

```go
// Weather service tool with API integration
type WeatherTool struct {
    APIKey  string
    BaseURL string
}

type WeatherInput struct {
    Location string `json:"location" description:"City name or coordinates"`
    Units    string `json:"units,omitempty" description:"Temperature units (celsius, fahrenheit)"`
}

type WeatherResponse struct {
    Location    string  `json:"location"`
    Temperature float64 `json:"temperature"`
    Description string  `json:"description"`
    Humidity    int     `json:"humidity"`
    WindSpeed   float64 `json:"wind_speed"`
}

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{
        APIKey:  apiKey,
        BaseURL: "https://api.weatherservice.com/v1",
    }
}

func (t *WeatherTool) Name() string {
    return "get_weather"
}

func (t *WeatherTool) Description() string {
    return "Get current weather information for any location"
}

func (t *WeatherTool) Schema() dive.Schema {
    return dive.Schema{
        Type: "object",
        Properties: map[string]dive.Property{
            "location": {
                Type:        "string",
                Description: "City name, country, or coordinates (e.g., 'London, UK' or '40.7128,-74.0060')",
            },
            "units": {
                Type:        "string",
                Description: "Temperature units: 'celsius' or 'fahrenheit'",
                Enum:        []string{"celsius", "fahrenheit"},
            },
        },
        Required: []string{"location"},
    }
}

func (t *WeatherTool) Annotations() dive.ToolAnnotations {
    return dive.ToolAnnotations{
        Title:         "Weather Information",
        ReadOnlyHint:  true,
        IdempotentHint: true,
        OpenWorldHint: true, // Makes external API calls
    }
}

func (t *WeatherTool) Call(ctx context.Context, input *WeatherInput) (*dive.ToolResult, error) {
    // Set default units
    units := input.Units
    if units == "" {
        units = "celsius"
    }
    
    // Make API request
    weather, err := t.fetchWeather(ctx, input.Location, units)
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Failed to fetch weather data: %v", err),
            }},
            IsError: true,
        }, nil
    }
    
    // Format response
    unitsSymbol := "°C"
    if units == "fahrenheit" {
        unitsSymbol = "°F"
    }
    
    response := fmt.Sprintf(`Current weather in %s:
Temperature: %.1f%s
Conditions: %s
Humidity: %d%%
Wind Speed: %.1f m/s`,
        weather.Location,
        weather.Temperature,
        unitsSymbol,
        weather.Description,
        weather.Humidity,
        weather.WindSpeed,
    )
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: response,
        }},
    }, nil
}

func (t *WeatherTool) fetchWeather(ctx context.Context, location, units string) (*WeatherResponse, error) {
    // Implementation would make actual HTTP request to weather API
    // This is a mock response for demonstration
    return &WeatherResponse{
        Location:    location,
        Temperature: 22.5,
        Description: "Partly cloudy",
        Humidity:    65,
        WindSpeed:   3.2,
    }, nil
}

// Usage
func createWeatherAgent() (*agent.Agent, error) {
    weatherTool := dive.ToolAdapter(NewWeatherTool(os.Getenv("WEATHER_API_KEY")))
    
    return agent.New(agent.Options{
        Name: "Weather Assistant",
        Instructions: `You are a weather assistant who can provide current weather
                      information for any location worldwide.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{weatherTool},
    })
}
```

### Database Tool Example

```go
import (
    "database/sql"
    "encoding/json"
    _ "github.com/lib/pq"
)

// Database query tool
type DatabaseTool struct {
    db *sql.DB
}

type DatabaseInput struct {
    Query      string                 `json:"query" description:"SQL query to execute"`
    Parameters []interface{}         `json:"parameters,omitempty" description:"Query parameters"`
}

func NewDatabaseTool(connectionString string) (*DatabaseTool, error) {
    db, err := sql.Open("postgres", connectionString)
    if err != nil {
        return nil, err
    }
    
    return &DatabaseTool{db: db}, nil
}

func (t *DatabaseTool) Name() string {
    return "database_query"
}

func (t *DatabaseTool) Description() string {
    return "Execute SQL queries against the database"
}

func (t *DatabaseTool) Schema() dive.Schema {
    return dive.Schema{
        Type: "object",
        Properties: map[string]dive.Property{
            "query": {
                Type:        "string",
                Description: "SQL query to execute (SELECT statements only for safety)",
            },
            "parameters": {
                Type:        "array",
                Description: "Parameters for parameterized queries",
                Items: &dive.Property{
                    Type: "string",
                },
            },
        },
        Required: []string{"query"},
    }
}

func (t *DatabaseTool) Annotations() dive.ToolAnnotations {
    return dive.ToolAnnotations{
        Title:           "Database Query",
        ReadOnlyHint:    true, // Only allow SELECT queries for safety
        DestructiveHint: false,
        IdempotentHint:  true,
        OpenWorldHint:   false,
    }
}

func (t *DatabaseTool) Call(ctx context.Context, input *DatabaseInput) (*dive.ToolResult, error) {
    // Safety check - only allow SELECT queries
    query := strings.TrimSpace(strings.ToUpper(input.Query))
    if !strings.HasPrefix(query, "SELECT") {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "Only SELECT queries are allowed for safety",
            }},
            IsError: true,
        }, nil
    }
    
    // Execute query
    rows, err := t.db.QueryContext(ctx, input.Query, input.Parameters...)
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Query failed: %v", err),
            }},
            IsError: true,
        }, nil
    }
    defer rows.Close()
    
    // Get column names
    columns, err := rows.Columns()
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Failed to get columns: %v", err),
            }},
            IsError: true,
        }, nil
    }
    
    // Read results
    var results []map[string]interface{}
    for rows.Next() {
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range values {
            valuePtrs[i] = &values[i]
        }
        
        err := rows.Scan(valuePtrs...)
        if err != nil {
            return &dive.ToolResult{
                Content: []*dive.ToolResultContent{{
                    Type: dive.ToolResultContentTypeText,
                    Text: fmt.Sprintf("Failed to scan row: %v", err),
                }},
                IsError: true,
            }, nil
        }
        
        row := make(map[string]interface{})
        for i, col := range columns {
            row[col] = values[i]
        }
        results = append(results, row)
    }
    
    // Format results as JSON
    jsonData, err := json.MarshalIndent(results, "", "  ")
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Failed to format results: %v", err),
            }},
            IsError: true,
        }, nil
    }
    
    response := fmt.Sprintf("Query returned %d rows:\n\n%s", len(results), string(jsonData))
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: response,
        }},
    }, nil
}
```

## Tool Adapters

The `ToolAdapter` function converts typed tools into the generic `Tool` interface:

```go
// Convert typed tool to generic tool interface
type TypedTool[TInput any] interface {
    Name() string
    Description() string
    Schema() dive.Schema
    Annotations() dive.ToolAnnotations
    Call(ctx context.Context, input *TInput) (*dive.ToolResult, error)
}

// Usage
calculator := &CalculatorTool{}
genericTool := dive.ToolAdapter(calculator)

// Use in agent
agent, err := agent.New(agent.Options{
    Tools: []dive.Tool{genericTool},
})
```

### Benefits of Tool Adapters

1. **Type Safety** - Compile-time type checking for tool inputs
2. **Automatic Serialization** - JSON marshaling/unmarshaling handled automatically
3. **Schema Generation** - Automatic schema generation from struct tags
4. **Error Handling** - Consistent error handling patterns

## Advanced Tool Patterns

### Composite Tools

```go
// Tool that combines multiple operations
type FileAnalysisTool struct {
    readTool  *ReadFileTool
    writeTool *WriteFileTool
}

func NewFileAnalysisTool() *FileAnalysisTool {
    return &FileAnalysisTool{
        readTool:  &ReadFileTool{},
        writeTool: &WriteFileTool{},
    }
}

func (t *FileAnalysisTool) Call(ctx context.Context, input *FileAnalysisInput) (*dive.ToolResult, error) {
    // Read file
    content, err := t.readTool.readFile(input.FilePath)
    if err != nil {
        return nil, err
    }
    
    // Analyze content
    analysis := analyzeFileContent(content)
    
    // Write analysis report
    if input.OutputPath != "" {
        err = t.writeTool.writeFile(input.OutputPath, analysis)
        if err != nil {
            return nil, err
        }
    }
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: analysis,
        }},
    }, nil
}
```

### Stateful Tools

```go
// Tool that maintains state between calls
type SessionTool struct {
    sessions map[string]*Session
    mutex    sync.RWMutex
}

type Session struct {
    ID       string
    Variables map[string]interface{}
    CreatedAt time.Time
}

func (t *SessionTool) Call(ctx context.Context, input *SessionInput) (*dive.ToolResult, error) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    session, exists := t.sessions[input.SessionID]
    if !exists {
        session = &Session{
            ID:        input.SessionID,
            Variables: make(map[string]interface{}),
            CreatedAt: time.Now(),
        }
        t.sessions[input.SessionID] = session
    }
    
    // Perform operation on session
    switch input.Operation {
    case "set":
        session.Variables[input.Key] = input.Value
    case "get":
        value, exists := session.Variables[input.Key]
        if !exists {
            return &dive.ToolResult{
                Content: []*dive.ToolResultContent{{
                    Type: dive.ToolResultContentTypeText,
                    Text: fmt.Sprintf("Variable %s not found", input.Key),
                }},
            }, nil
        }
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("%s = %v", input.Key, value),
            }},
        }, nil
    }
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: "Operation completed",
        }},
    }, nil
}
```

### Async Tools

```go
// Tool that performs long-running operations
type AsyncProcessingTool struct {
    jobs map[string]*Job
    mutex sync.RWMutex
}

type Job struct {
    ID        string
    Status    string
    Progress  float64
    Result    interface{}
    Error     error
    StartedAt time.Time
}

func (t *AsyncProcessingTool) Call(ctx context.Context, input *AsyncInput) (*dive.ToolResult, error) {
    if input.JobID != "" {
        // Check job status
        return t.getJobStatus(input.JobID)
    }
    
    // Start new job
    jobID := generateJobID()
    job := &Job{
        ID:        jobID,
        Status:    "running",
        StartedAt: time.Now(),
    }
    
    t.mutex.Lock()
    t.jobs[jobID] = job
    t.mutex.Unlock()
    
    // Start processing in background
    go t.processJob(ctx, job, input)
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: fmt.Sprintf("Job started with ID: %s", jobID),
        }},
    }, nil
}

func (t *AsyncProcessingTool) processJob(ctx context.Context, job *Job, input *AsyncInput) {
    defer func() {
        t.mutex.Lock()
        defer t.mutex.Unlock()
        
        if r := recover(); r != nil {
            job.Status = "failed"
            job.Error = fmt.Errorf("panic: %v", r)
        }
    }()
    
    // Simulate long-running process
    for i := 0; i < 100; i++ {
        select {
        case <-ctx.Done():
            job.Status = "cancelled"
            return
        default:
            // Update progress
            t.mutex.Lock()
            job.Progress = float64(i) / 100.0
            t.mutex.Unlock()
            
            time.Sleep(100 * time.Millisecond)
        }
    }
    
    t.mutex.Lock()
    job.Status = "completed"
    job.Progress = 1.0
    job.Result = "Processing completed successfully"
    t.mutex.Unlock()
}
```

## Error Handling

### Tool-Level Error Handling

```go
func (t *MyTool) Call(ctx context.Context, input *MyInput) (*dive.ToolResult, error) {
    // Validate input
    if input.RequiredField == "" {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "RequiredField is required",
            }},
            IsError: true,
        }, nil
    }
    
    // Handle timeouts
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    // Perform operation with error handling
    result, err := performOperation(ctx, input)
    if err != nil {
        // Return user-friendly error without failing the tool call
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Operation failed: %v", err),
            }},
            IsError: true,
        }, nil
    }
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: result,
        }},
    }, nil
}
```

### Retry Logic in Tools

```go
func (t *RetryableTool) Call(ctx context.Context, input *MyInput) (*dive.ToolResult, error) {
    maxRetries := 3
    backoff := time.Second
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        result, err := t.performOperation(ctx, input)
        if err == nil {
            return result, nil
        }
        
        // Check if error is retryable
        if !isRetryableError(err) {
            return &dive.ToolResult{
                Content: []*dive.ToolResultContent{{
                    Type: dive.ToolResultContentTypeText,
                    Text: fmt.Sprintf("Non-retryable error: %v", err),
                }},
                IsError: true,
            }, nil
        }
        
        if attempt < maxRetries-1 {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(backoff * time.Duration(attempt+1)):
                // Continue to next attempt
            }
        }
    }
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: "Operation failed after maximum retries",
        }},
        IsError: true,
    }, nil
}

func isRetryableError(err error) bool {
    // Check for network errors, temporary failures, etc.
    if netErr, ok := err.(net.Error); ok {
        return netErr.Temporary()
    }
    
    // Check for specific error patterns
    errStr := err.Error()
    return strings.Contains(errStr, "timeout") ||
           strings.Contains(errStr, "connection refused") ||
           strings.Contains(errStr, "temporary failure")
}
```

## Best Practices

### 1. Tool Design Principles

```go
// Good: Focused, single-purpose tool
type EmailSenderTool struct {
    SMTPConfig SMTPConfig
}

// Avoid: Overly broad, multi-purpose tool
type CommunicationTool struct {
    // Handles email, SMS, push notifications, etc.
}

// Good: Clear, descriptive input structure
type EmailInput struct {
    To      string `json:"to" description:"Recipient email address"`
    Subject string `json:"subject" description:"Email subject line"`
    Body    string `json:"body" description:"Email body content"`
    HTML    bool   `json:"html,omitempty" description:"Whether body is HTML formatted"`
}

// Avoid: Vague or unclear input structure
type EmailInput struct {
    Data map[string]interface{} `json:"data"`
}
```

### 2. Security Considerations

```go
// Validate and sanitize inputs
func (t *FileTool) Call(ctx context.Context, input *FileInput) (*dive.ToolResult, error) {
    // Validate file path
    if !isValidPath(input.Path) {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "Invalid file path",
            }},
            IsError: true,
        }, nil
    }
    
    // Check path traversal attacks
    if strings.Contains(input.Path, "..") {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "Path traversal not allowed",
            }},
            IsError: true,
        }, nil
    }
    
    // Restrict to allowed directories
    if !strings.HasPrefix(input.Path, "/allowed/directory/") {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "Access denied to this directory",
            }},
            IsError: true,
        }, nil
    }
    
    // Continue with operation...
}

// Implement proper authentication for external API tools
func (t *APITool) Call(ctx context.Context, input *APIInput) (*dive.ToolResult, error) {
    // Use secure credential management
    apiKey := os.Getenv("API_KEY") // Better: use secret management service
    if apiKey == "" {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "API key not configured",
            }},
            IsError: true,
        }, nil
    }
    
    // Make authenticated request...
}
```

### 3. Performance Optimization

```go
// Use connection pooling for database tools
type DatabaseTool struct {
    db *sql.DB // Connection pool
}

// Cache results for expensive operations
type CachingTool struct {
    cache   map[string]*CacheEntry
    mutex   sync.RWMutex
    ttl     time.Duration
}

type CacheEntry struct {
    Value     *dive.ToolResult
    ExpiresAt time.Time
}

func (t *CachingTool) Call(ctx context.Context, input *CacheInput) (*dive.ToolResult, error) {
    cacheKey := generateCacheKey(input)
    
    // Check cache first
    t.mutex.RLock()
    if entry, exists := t.cache[cacheKey]; exists && time.Now().Before(entry.ExpiresAt) {
        t.mutex.RUnlock()
        return entry.Value, nil
    }
    t.mutex.RUnlock()
    
    // Perform expensive operation
    result, err := t.performExpensiveOperation(ctx, input)
    if err != nil {
        return nil, err
    }
    
    // Cache result
    t.mutex.Lock()
    t.cache[cacheKey] = &CacheEntry{
        Value:     result,
        ExpiresAt: time.Now().Add(t.ttl),
    }
    t.mutex.Unlock()
    
    return result, nil
}
```

### 4. Testing Tools

```go
func TestCalculatorTool(t *testing.T) {
    tool := &CalculatorTool{}
    
    tests := []struct {
        name     string
        input    *CalculatorInput
        expected string
        hasError bool
    }{
        {
            name:     "simple addition",
            input:    &CalculatorInput{Expression: "2 + 2"},
            expected: "Result: 4",
            hasError: false,
        },
        {
            name:     "invalid expression",
            input:    &CalculatorInput{Expression: "invalid"},
            expected: "Error evaluating expression",
            hasError: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := tool.Call(context.Background(), tt.input)
            
            require.NoError(t, err) // Tool calls shouldn't return Go errors
            require.NotNil(t, result)
            require.Len(t, result.Content, 1)
            
            content := result.Content[0].Text
            assert.Contains(t, content, tt.expected)
            assert.Equal(t, tt.hasError, result.IsError)
        })
    }
}

// Integration test with agent
func TestToolWithAgent(t *testing.T) {
    calculator := dive.ToolAdapter(&CalculatorTool{})
    
    agent, err := agent.New(agent.Options{
        Name:  "Test Agent",
        Model: &MockLLM{}, // Use mock LLM for testing
        Tools: []dive.Tool{calculator},
    })
    require.NoError(t, err)
    
    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("Calculate 2 + 2"),
    )
    require.NoError(t, err)
    
    // Verify tool was called
    toolCalls := response.ToolCalls()
    assert.Len(t, toolCalls, 1)
    assert.Equal(t, "calculate", toolCalls[0].Name)
}
```

### 5. Documentation and Discoverability

```go
// Good: Comprehensive tool documentation
func (t *WeatherTool) Description() string {
    return `Get current weather information for any location worldwide.
    
    This tool provides:
    - Current temperature and conditions
    - Humidity and wind speed
    - Support for multiple temperature units
    - Worldwide location coverage
    
    Examples:
    - "London, UK"
    - "New York, NY, USA"  
    - "40.7128,-74.0060" (coordinates)`
}

func (t *WeatherTool) Schema() dive.Schema {
    return dive.Schema{
        Type: "object",
        Properties: map[string]dive.Property{
            "location": {
                Type:        "string",
                Description: "City name with country (e.g., 'Paris, France') or coordinates (e.g., '40.7128,-74.0060')",
                Examples:    []string{"London, UK", "Tokyo, Japan", "40.7128,-74.0060"},
            },
            "units": {
                Type:        "string", 
                Description: "Temperature units: 'celsius' (default) or 'fahrenheit'",
                Enum:        []string{"celsius", "fahrenheit"},
                Default:     "celsius",
            },
        },
        Required: []string{"location"},
    }
}
```

## Next Steps

- [Custom Tools Guide](custom-tools.md) - Deep dive into building custom tools
- [Agent Guide](agents.md) - Learn how agents use tools
- [MCP Integration](mcp-integration.md) - Connect external tool servers
- [API Reference](../api/core.md) - Tool interface documentation