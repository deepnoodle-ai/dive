# Custom Tools Guide

Learn how to create powerful custom tools that extend your agents' capabilities beyond the built-in toolkit.

## 📋 Table of Contents

- [Understanding Tools](#understanding-tools)
- [TypedTool Interface](#typedtool-interface)
- [Simple Tools](#simple-tools)
- [Advanced Tools](#advanced-tools)
- [Tool Parameters](#tool-parameters)
- [Error Handling](#error-handling)
- [Async Operations](#async-operations)
- [Composite Tools](#composite-tools)
- [Tool Testing](#tool-testing)
- [Best Practices](#best-practices)

## Understanding Tools

Tools are functions that agents can call to perform specific actions. They bridge the gap between AI reasoning and real-world operations.

### Tool Lifecycle

```go
// 1. Tool is registered with agent
agent.New(agent.Options{
    Tools: []dive.Tool{
        dive.ToolAdapter(customTool),
    },
})

// 2. LLM decides to use tool based on description
// 3. Tool is called with parsed parameters
// 4. Tool executes and returns result
// 5. Result is provided to LLM for reasoning
```

### Core Concepts

- **Deterministic**: Tools should be predictable and repeatable
- **Focused**: Each tool should have a single, well-defined purpose  
- **Robust**: Handle errors gracefully and provide useful feedback
- **Documented**: Clear descriptions help LLMs use tools effectively

## TypedTool Interface

All custom tools implement the `TypedTool` interface:

```go
type TypedTool interface {
    Name() string
    Description() string
    Parameters() ToolParameters
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}
```

### Basic Implementation

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/toolkit"
)

type TimestampTool struct{}

func (t *TimestampTool) Name() string {
    return "get_timestamp"
}

func (t *TimestampTool) Description() string {
    return "Get the current timestamp in various formats"
}

func (t *TimestampTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "format": {
                Type:        "string",
                Description: "Timestamp format (unix, iso, readable)",
                Enum:        []interface{}{"unix", "iso", "readable"},
            },
        },
        Required: []string{"format"},
    }
}

func (t *TimestampTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    format, ok := params["format"].(string)
    if !ok {
        return nil, fmt.Errorf("format parameter is required and must be a string")
    }
    
    now := time.Now()
    
    switch format {
    case "unix":
        return now.Unix(), nil
    case "iso":
        return now.Format(time.RFC3339), nil
    case "readable":
        return now.Format("January 2, 2006 at 3:04 PM MST"), nil
    default:
        return nil, fmt.Errorf("unsupported format: %s", format)
    }
}
```

## Simple Tools

### Math Calculator Tool

```go
type CalculatorTool struct{}

func (c *CalculatorTool) Name() string {
    return "calculate"
}

func (c *CalculatorTool) Description() string {
    return "Perform basic mathematical calculations"
}

func (c *CalculatorTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "expression": {
                Type:        "string",
                Description: "Mathematical expression to evaluate (e.g., '2 + 3 * 4')",
            },
        },
        Required: []string{"expression"},
    }
}

func (c *CalculatorTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    expr, ok := params["expression"].(string)
    if !ok {
        return nil, fmt.Errorf("expression parameter is required")
    }
    
    // Simple evaluation (in practice, use a proper math parser)
    result, err := evaluateExpression(expr)
    if err != nil {
        return nil, fmt.Errorf("calculation error: %w", err)
    }
    
    return map[string]interface{}{
        "expression": expr,
        "result":     result,
    }, nil
}
```

### Text Processing Tool

```go
type TextProcessorTool struct{}

func (t *TextProcessorTool) Name() string {
    return "process_text"
}

func (t *TextProcessorTool) Description() string {
    return "Process text with various operations (uppercase, lowercase, word count, etc.)"
}

func (t *TextProcessorTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "text": {
                Type:        "string",
                Description: "Text to process",
            },
            "operation": {
                Type:        "string",
                Description: "Operation to perform",
                Enum:        []interface{}{"uppercase", "lowercase", "word_count", "reverse"},
            },
        },
        Required: []string{"text", "operation"},
    }
}

func (t *TextProcessorTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    text, _ := params["text"].(string)
    operation, _ := params["operation"].(string)
    
    switch operation {
    case "uppercase":
        return strings.ToUpper(text), nil
    case "lowercase":
        return strings.ToLower(text), nil
    case "word_count":
        words := strings.Fields(text)
        return len(words), nil
    case "reverse":
        runes := []rune(text)
        for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
            runes[i], runes[j] = runes[j], runes[i]
        }
        return string(runes), nil
    default:
        return nil, fmt.Errorf("unsupported operation: %s", operation)
    }
}
```

## Advanced Tools

### Database Query Tool

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
)

type DatabaseQueryTool struct {
    db *sql.DB
}

func NewDatabaseQueryTool(connectionString string) (*DatabaseQueryTool, error) {
    db, err := sql.Open("postgres", connectionString)
    if err != nil {
        return nil, err
    }
    
    return &DatabaseQueryTool{db: db}, nil
}

func (d *DatabaseQueryTool) Name() string {
    return "query_database"
}

func (d *DatabaseQueryTool) Description() string {
    return "Execute SQL queries against the database. Use for data retrieval and analysis."
}

func (d *DatabaseQueryTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "query": {
                Type:        "string",
                Description: "SQL query to execute (SELECT statements only for safety)",
            },
            "limit": {
                Type:        "integer",
                Description: "Maximum number of rows to return (default: 100)",
                Default:     100,
            },
        },
        Required: []string{"query"},
    }
}

func (d *DatabaseQueryTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    query, _ := params["query"].(string)
    limit, _ := params["limit"].(float64)
    
    // Safety check - only allow SELECT statements
    if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SELECT") {
        return nil, fmt.Errorf("only SELECT queries are allowed for safety")
    }
    
    // Add limit if not present
    if limit > 0 && !strings.Contains(strings.ToUpper(query), "LIMIT") {
        query = fmt.Sprintf("%s LIMIT %d", query, int(limit))
    }
    
    rows, err := d.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("query execution failed: %w", err)
    }
    defer rows.Close()
    
    // Get column names
    columns, err := rows.Columns()
    if err != nil {
        return nil, err
    }
    
    var results []map[string]interface{}
    
    for rows.Next() {
        // Create a slice to hold column values
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range values {
            valuePtrs[i] = &values[i]
        }
        
        if err := rows.Scan(valuePtrs...); err != nil {
            return nil, err
        }
        
        // Convert to map
        row := make(map[string]interface{})
        for i, col := range columns {
            row[col] = values[i]
        }
        results = append(results, row)
    }
    
    return map[string]interface{}{
        "query":   query,
        "columns": columns,
        "rows":    results,
        "count":   len(results),
    }, nil
}
```

### HTTP API Tool

```go
import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
)

type HTTPRequestTool struct {
    client *http.Client
}

func NewHTTPRequestTool() *HTTPRequestTool {
    return &HTTPRequestTool{
        client: &http.Client{
            Timeout: time.Second * 30,
        },
    }
}

func (h *HTTPRequestTool) Name() string {
    return "http_request"
}

func (h *HTTPRequestTool) Description() string {
    return "Make HTTP requests to web APIs. Supports GET, POST, PUT, DELETE methods."
}

func (h *HTTPRequestTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "url": {
                Type:        "string",
                Description: "URL to request",
            },
            "method": {
                Type:        "string",
                Description: "HTTP method",
                Enum:        []interface{}{"GET", "POST", "PUT", "DELETE"},
                Default:     "GET",
            },
            "headers": {
                Type:        "object",
                Description: "HTTP headers to include",
            },
            "body": {
                Type:        "object",
                Description: "Request body (for POST/PUT requests)",
            },
        },
        Required: []string{"url"},
    }
}

func (h *HTTPRequestTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    url, _ := params["url"].(string)
    method, _ := params["method"].(string)
    if method == "" {
        method = "GET"
    }
    
    var reqBody io.Reader
    if body, ok := params["body"]; ok {
        bodyBytes, err := json.Marshal(body)
        if err != nil {
            return nil, fmt.Errorf("failed to serialize request body: %w", err)
        }
        reqBody = bytes.NewBuffer(bodyBytes)
    }
    
    req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    // Add headers
    if headers, ok := params["headers"].(map[string]interface{}); ok {
        for key, value := range headers {
            if strValue, ok := value.(string); ok {
                req.Header.Set(key, strValue)
            }
        }
    }
    
    // Set content type for JSON body
    if reqBody != nil {
        req.Header.Set("Content-Type", "application/json")
    }
    
    resp, err := h.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response body: %w", err)
    }
    
    // Try to parse as JSON
    var jsonBody interface{}
    if err := json.Unmarshal(respBody, &jsonBody); err != nil {
        // If not JSON, return as string
        jsonBody = string(respBody)
    }
    
    return map[string]interface{}{
        "status_code": resp.StatusCode,
        "headers":     resp.Header,
        "body":        jsonBody,
        "url":         url,
        "method":      method,
    }, nil
}
```

## Tool Parameters

### Parameter Types

```go
// String parameter
"name": {
    Type:        "string",
    Description: "User's name",
    MinLength:   1,
    MaxLength:   100,
}

// Number parameter
"age": {
    Type:        "number",
    Description: "User's age",
    Minimum:     0,
    Maximum:     150,
}

// Integer parameter
"count": {
    Type:        "integer",
    Description: "Number of items",
    Default:     10,
}

// Boolean parameter
"enabled": {
    Type:        "boolean",
    Description: "Whether feature is enabled",
    Default:     true,
}

// Enum parameter
"level": {
    Type:        "string",
    Description: "Log level",
    Enum:        []interface{}{"debug", "info", "warn", "error"},
}

// Array parameter
"tags": {
    Type:        "array",
    Description: "List of tags",
    Items: &ToolParameter{
        Type: "string",
    },
}

// Object parameter
"config": {
    Type:        "object",
    Description: "Configuration settings",
    Properties: map[string]ToolParameter{
        "timeout": {
            Type:        "integer",
            Description: "Timeout in seconds",
        },
    },
}
```

### Parameter Validation

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Type assertion with validation
    name, ok := params["name"].(string)
    if !ok || name == "" {
        return nil, fmt.Errorf("name parameter is required and must be non-empty string")
    }
    
    // Optional parameter with default
    timeout := 30
    if t, ok := params["timeout"].(float64); ok {
        timeout = int(t)
    }
    
    // Array parameter
    var tags []string
    if tagsInterface, ok := params["tags"].([]interface{}); ok {
        for _, tag := range tagsInterface {
            if str, ok := tag.(string); ok {
                tags = append(tags, str)
            }
        }
    }
    
    // Validation logic
    if timeout < 1 || timeout > 300 {
        return nil, fmt.Errorf("timeout must be between 1 and 300 seconds")
    }
    
    return executeLogic(name, timeout, tags)
}
```

## Error Handling

### Structured Error Responses

```go
type ToolError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details interface{} `json:"details,omitempty"`
}

func (e ToolError) Error() string {
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Parameter validation error
    if name == "" {
        return nil, ToolError{
            Code:    "INVALID_PARAMETER",
            Message: "name parameter is required",
            Details: map[string]string{"parameter": "name"},
        }
    }
    
    // External service error
    if err := callExternalAPI(); err != nil {
        return nil, ToolError{
            Code:    "EXTERNAL_SERVICE_ERROR",
            Message: "failed to call external API",
            Details: err.Error(),
        }
    }
    
    // Business logic error
    if !isValidUser(name) {
        return nil, ToolError{
            Code:    "BUSINESS_RULE_VIOLATION",
            Message: "user does not meet requirements",
            Details: map[string]interface{}{
                "user": name,
                "requirements": []string{"active", "verified"},
            },
        }
    }
    
    return result, nil
}
```

### Graceful Degradation

```go
func (t *WeatherTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    location, _ := params["location"].(string)
    
    // Try primary weather service
    weather, err := t.primaryService.GetWeather(ctx, location)
    if err == nil {
        return weather, nil
    }
    
    // Fallback to secondary service
    log.Printf("Primary weather service failed: %v, trying fallback", err)
    weather, err = t.fallbackService.GetWeather(ctx, location)
    if err == nil {
        return weather, nil
    }
    
    // Return limited information if all services fail
    return map[string]interface{}{
        "location": location,
        "error": "Weather services temporarily unavailable",
        "suggestion": "Please try again later or check weather manually",
    }, nil
}
```

## Async Operations

### Long-Running Tasks

```go
type FileProcessingTool struct {
    jobStore map[string]*ProcessingJob
    mutex    sync.RWMutex
}

type ProcessingJob struct {
    ID       string
    Status   string
    Progress float64
    Result   interface{}
    Error    error
    Started  time.Time
    Finished *time.Time
}

func (f *FileProcessingTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    async, _ := params["async"].(bool)
    
    if !async {
        // Synchronous execution
        return f.processFile(ctx, params)
    }
    
    // Asynchronous execution
    jobID := generateJobID()
    job := &ProcessingJob{
        ID:      jobID,
        Status:  "started",
        Started: time.Now(),
    }
    
    f.mutex.Lock()
    f.jobStore[jobID] = job
    f.mutex.Unlock()
    
    // Start processing in background
    go func() {
        result, err := f.processFile(context.Background(), params)
        
        f.mutex.Lock()
        defer f.mutex.Unlock()
        
        job.Status = "completed"
        job.Result = result
        job.Error = err
        now := time.Now()
        job.Finished = &now
        
        if err != nil {
            job.Status = "failed"
        }
    }()
    
    return map[string]interface{}{
        "job_id": jobID,
        "status": "started",
        "check_status_with": "get_job_status",
    }, nil
}

// Companion tool to check job status
type JobStatusTool struct {
    processor *FileProcessingTool
}

func (j *JobStatusTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    jobID, _ := params["job_id"].(string)
    
    j.processor.mutex.RLock()
    job, exists := j.processor.jobStore[jobID]
    j.processor.mutex.RUnlock()
    
    if !exists {
        return nil, fmt.Errorf("job not found: %s", jobID)
    }
    
    response := map[string]interface{}{
        "job_id":   job.ID,
        "status":   job.Status,
        "progress": job.Progress,
        "started":  job.Started,
    }
    
    if job.Finished != nil {
        response["finished"] = job.Finished
        response["duration"] = job.Finished.Sub(job.Started).String()
    }
    
    if job.Status == "completed" {
        response["result"] = job.Result
    } else if job.Status == "failed" {
        response["error"] = job.Error.Error()
    }
    
    return response, nil
}
```

## Composite Tools

### Multi-Step Workflow Tool

```go
type WorkflowTool struct {
    steps []WorkflowStep
}

type WorkflowStep struct {
    Name string
    Tool dive.Tool
}

func NewWorkflowTool(steps []WorkflowStep) *WorkflowTool {
    return &WorkflowTool{steps: steps}
}

func (w *WorkflowTool) Name() string {
    return "workflow"
}

func (w *WorkflowTool) Description() string {
    return "Execute a multi-step workflow with defined steps"
}

func (w *WorkflowTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "inputs": {
                Type:        "object",
                Description: "Input parameters for the workflow",
            },
            "skip_steps": {
                Type:        "array",
                Description: "Steps to skip (by name)",
                Items: &toolkit.ToolParameter{
                    Type: "string",
                },
            },
        },
    }
}

func (w *WorkflowTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    inputs, _ := params["inputs"].(map[string]interface{})
    skipSteps := make(map[string]bool)
    
    if skip, ok := params["skip_steps"].([]interface{}); ok {
        for _, step := range skip {
            if stepName, ok := step.(string); ok {
                skipSteps[stepName] = true
            }
        }
    }
    
    results := make(map[string]interface{})
    currentInputs := inputs
    
    for i, step := range w.steps {
        if skipSteps[step.Name] {
            log.Printf("Skipping step: %s", step.Name)
            continue
        }
        
        log.Printf("Executing step %d: %s", i+1, step.Name)
        
        result, err := step.Tool.Execute(ctx, currentInputs)
        if err != nil {
            return map[string]interface{}{
                "status":      "failed",
                "failed_step": step.Name,
                "error":       err.Error(),
                "completed_steps": results,
            }, nil
        }
        
        results[step.Name] = result
        
        // Use result as input for next step
        if resultMap, ok := result.(map[string]interface{}); ok {
            currentInputs = resultMap
        }
    }
    
    return map[string]interface{}{
        "status":  "completed",
        "results": results,
    }, nil
}
```

## Tool Testing

### Unit Tests

```go
package main

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestCalculatorTool(t *testing.T) {
    tool := &CalculatorTool{}
    ctx := context.Background()
    
    t.Run("basic addition", func(t *testing.T) {
        params := map[string]interface{}{
            "expression": "2 + 3",
        }
        
        result, err := tool.Execute(ctx, params)
        require.NoError(t, err)
        
        resultMap, ok := result.(map[string]interface{})
        require.True(t, ok)
        assert.Equal(t, "2 + 3", resultMap["expression"])
        assert.Equal(t, 5.0, resultMap["result"])
    })
    
    t.Run("missing parameter", func(t *testing.T) {
        params := map[string]interface{}{}
        
        _, err := tool.Execute(ctx, params)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "expression parameter is required")
    })
    
    t.Run("invalid expression", func(t *testing.T) {
        params := map[string]interface{}{
            "expression": "invalid",
        }
        
        _, err := tool.Execute(ctx, params)
        assert.Error(t, err)
    })
}
```

### Integration Tests

```go
func TestDatabaseQueryToolIntegration(t *testing.T) {
    // Set up test database
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)
    
    tool, err := NewDatabaseQueryTool(getTestConnectionString())
    require.NoError(t, err)
    
    ctx := context.Background()
    
    t.Run("simple select query", func(t *testing.T) {
        params := map[string]interface{}{
            "query": "SELECT * FROM users WHERE active = true",
            "limit": 10,
        }
        
        result, err := tool.Execute(ctx, params)
        require.NoError(t, err)
        
        resultMap := result.(map[string]interface{})
        rows := resultMap["rows"].([]map[string]interface{})
        assert.LessOrEqual(t, len(rows), 10)
    })
    
    t.Run("blocked unsafe query", func(t *testing.T) {
        params := map[string]interface{}{
            "query": "DELETE FROM users",
        }
        
        _, err := tool.Execute(ctx, params)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "only SELECT queries are allowed")
    })
}
```

### Mock Tools for Testing

```go
type MockTool struct {
    name        string
    description string
    responses   map[string]interface{}
    callLog     []map[string]interface{}
}

func NewMockTool(name, description string) *MockTool {
    return &MockTool{
        name:        name,
        description: description,
        responses:   make(map[string]interface{}),
        callLog:     make([]map[string]interface{}, 0),
    }
}

func (m *MockTool) Name() string { return m.name }
func (m *MockTool) Description() string { return m.description }

func (m *MockTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{Type: "object"}
}

func (m *MockTool) SetResponse(key string, response interface{}) {
    m.responses[key] = response
}

func (m *MockTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    m.callLog = append(m.callLog, params)
    
    // Return predefined response based on parameters
    if response, exists := m.responses["default"]; exists {
        return response, nil
    }
    
    return "mock response", nil
}

func (m *MockTool) GetCallLog() []map[string]interface{} {
    return m.callLog
}
```

## Best Practices

### 1. Clear Naming and Documentation

```go
// Good: Clear, descriptive name
func (t *EmailValidatorTool) Name() string {
    return "validate_email"
}

func (t *EmailValidatorTool) Description() string {
    return "Validate email address format and check if domain exists. Returns validation status and details."
}

// Bad: Vague name
func (t *ProcessorTool) Name() string {
    return "process"
}
```

### 2. Robust Parameter Handling

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Always validate required parameters
    email, ok := params["email"].(string)
    if !ok || email == "" {
        return nil, fmt.Errorf("email parameter is required and must be non-empty")
    }
    
    // Provide sensible defaults
    timeout := 30
    if t, ok := params["timeout"].(float64); ok && t > 0 {
        timeout = int(t)
    }
    
    // Validate parameter values
    if timeout > 300 {
        return nil, fmt.Errorf("timeout cannot exceed 300 seconds")
    }
    
    return processEmail(email, timeout)
}
```

### 3. Consistent Return Formats

```go
// Good: Structured response
return map[string]interface{}{
    "status": "success",
    "result": processedData,
    "metadata": map[string]interface{}{
        "processed_at": time.Now(),
        "items_count":  len(items),
    },
}, nil

// Good: Error response with details
return nil, ToolError{
    Code:    "PROCESSING_FAILED",
    Message: "failed to process data",
    Details: map[string]interface{}{
        "step": "validation",
        "issues": validationErrors,
    },
}
```

### 4. Context Awareness

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Respect context cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // Pass context to downstream calls
    result, err := t.externalService.Process(ctx, data)
    if err != nil {
        return nil, err
    }
    
    // Check context again for long operations
    for i, item := range items {
        select {
        case <-ctx.Done():
            return map[string]interface{}{
                "status": "cancelled",
                "processed_count": i,
            }, ctx.Err()
        default:
            processItem(item)
        }
    }
    
    return result, nil
}
```

### 5. Tool Composition

```go
// Create reusable tool components
type ToolRegistry struct {
    tools map[string]dive.Tool
}

func (r *ToolRegistry) CreateCompositeAgent(tools []string) (*agent.Agent, error) {
    var selectedTools []dive.Tool
    
    for _, toolName := range tools {
        if tool, exists := r.tools[toolName]; exists {
            selectedTools = append(selectedTools, tool)
        }
    }
    
    return agent.New(agent.Options{
        Tools: selectedTools,
        // ... other options
    })
}
```

### 6. Performance Optimization

```go
type CachingTool struct {
    cache  map[string]CacheEntry
    mutex  sync.RWMutex
    ttl    time.Duration
}

type CacheEntry struct {
    Result    interface{}
    Timestamp time.Time
}

func (c *CachingTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Create cache key from parameters
    key := createCacheKey(params)
    
    // Check cache first
    c.mutex.RLock()
    if entry, exists := c.cache[key]; exists && time.Since(entry.Timestamp) < c.ttl {
        c.mutex.RUnlock()
        return entry.Result, nil
    }
    c.mutex.RUnlock()
    
    // Execute actual logic
    result, err := c.executeLogic(ctx, params)
    if err != nil {
        return nil, err
    }
    
    // Cache result
    c.mutex.Lock()
    c.cache[key] = CacheEntry{
        Result:    result,
        Timestamp: time.Now(),
    }
    c.mutex.Unlock()
    
    return result, nil
}
```

Custom tools are the key to extending Dive agents beyond their base capabilities. Focus on creating focused, well-documented, and robust tools that handle errors gracefully and provide clear feedback to the AI models using them.