# Environment API Reference

Complete API reference for the Dive environment package, covering environment management, execution control, and multi-agent coordination.

## 📋 Table of Contents

- [Core Interface](#core-interface)
- [Environment Implementation](#environment-implementation)
- [Configuration](#configuration)
- [Agent Management](#agent-management)
- [Workflow Execution](#workflow-execution)
- [Repository Management](#repository-management)
- [Action System](#action-system)
- [Event Management](#event-management)
- [MCP Integration](#mcp-integration)
- [Examples](#examples)

## Core Interface

### `dive.Environment`

The core environment interface that all environment implementations must satisfy.

```go
type Environment interface {
    // Name of the Environment
    Name() string
    
    // Agents returns the list of all Agents belonging to this Environment
    Agents() []Agent
    
    // AddAgent adds an Agent to this Environment
    AddAgent(agent Agent) error
    
    // GetAgent returns the Agent with the given name, if found
    GetAgent(name string) (Agent, error)
    
    // DocumentRepository returns the DocumentRepository for this Environment
    DocumentRepository() DocumentRepository
    
    // ThreadRepository returns the ThreadRepository for this Environment
    ThreadRepository() ThreadRepository
    
    // Confirmer returns the Confirmer for this Environment
    Confirmer() Confirmer
}
```

## Environment Implementation

### `environment.Environment`

The standard implementation of the Environment interface.

```go
type Environment struct {
    id              string
    name            string
    description     string
    agents          map[string]dive.Agent
    workflows       map[string]*workflow.Workflow
    triggers        []*Trigger
    executions      map[string]*Execution
    logger          slogger.Logger
    defaultWorkflow string
    documentRepo    dive.DocumentRepository
    threadRepo      dive.ThreadRepository
    actions         map[string]Action
    started         bool
    confirmer       dive.Confirmer
    mcpManager      *mcp.Manager
    mcpServers      []*mcp.ServerConfig
    formatter       WorkflowFormatter
}
```

### Constructor

```go
func New(opts Options) (*Environment, error)
```

Creates a new Environment with the specified configuration options.

**Parameters:**
- `opts` - Configuration options for the environment

**Returns:**
- `*Environment` - Configured environment instance
- `error` - Error if configuration is invalid

## Configuration

### `environment.Options`

Configuration structure for creating new environments.

```go
type Options struct {
    // Basic configuration
    ID                 string                    // Unique identifier
    Name               string                    // Environment name (required)
    Description        string                    // Human-readable description
    
    // Components
    Agents             []dive.Agent              // Pre-configured agents
    Workflows          []*workflow.Workflow      // Available workflows
    Triggers           []*Trigger               // Event triggers
    Executions         []*Execution             // Running executions
    Actions            []Action                 // Custom actions
    
    // Storage and persistence
    DocumentRepository dive.DocumentRepository   // Document storage
    ThreadRepository   dive.ThreadRepository     // Conversation storage
    
    // Runtime configuration
    DefaultWorkflow    string                    // Default workflow name
    AutoStart          bool                      // Auto-start on creation
    Logger             slogger.Logger           // Logging interface
    Confirmer          dive.Confirmer           // User confirmation handler
    
    // MCP integration
    MCPServers         []*mcp.ServerConfig      // MCP server configurations
    MCPManager         *mcp.Manager             // MCP connection manager
    
    // Formatting
    Formatter          WorkflowFormatter        // Output formatter
}
```

### Required Fields

- **`Name`** - Unique identifier for the environment

### Environment Methods

```go
// Metadata access
func (e *Environment) ID() string
func (e *Environment) Name() string
func (e *Environment) Description() string

// Lifecycle management
func (e *Environment) Start(ctx context.Context) error
func (e *Environment) Stop(ctx context.Context) error
func (e *Environment) IsStarted() bool

// Agent management
func (e *Environment) Agents() []dive.Agent
func (e *Environment) AddAgent(agent dive.Agent) error
func (e *Environment) GetAgent(name string) (dive.Agent, error)
func (e *Environment) RemoveAgent(name string) error

// Workflow management
func (e *Environment) Workflows() []*workflow.Workflow
func (e *Environment) AddWorkflow(workflow *workflow.Workflow) error
func (e *Environment) GetWorkflow(name string) (*workflow.Workflow, error)
func (e *Environment) RunWorkflow(ctx context.Context, name string, inputs map[string]interface{}) (*Execution, error)

// Storage access
func (e *Environment) DocumentRepository() dive.DocumentRepository
func (e *Environment) ThreadRepository() dive.ThreadRepository
func (e *Environment) Confirmer() dive.Confirmer

// Execution management
func (e *Environment) GetExecution(id string) (*Execution, error)
func (e *Environment) ListExecutions() []*Execution
func (e *Environment) CancelExecution(ctx context.Context, id string) error

// Action system
func (e *Environment) RegisterAction(action Action) error
func (e *Environment) GetAction(name string) (Action, error)
func (e *Environment) ExecuteAction(ctx context.Context, name string, params map[string]interface{}) (interface{}, error)
```

## Agent Management

### Adding Agents

```go
// Add single agent
err := env.AddAgent(agent)

// Add multiple agents
agents := []dive.Agent{agent1, agent2, agent3}
for _, agent := range agents {
    if err := env.AddAgent(agent); err != nil {
        return err
    }
}

// Agents are automatically configured with environment context
agent.SetEnvironment(env)
```

### Agent Discovery

```go
// Get specific agent
agent, err := env.GetAgent("Assistant")
if err != nil {
    return fmt.Errorf("agent not found: %w", err)
}

// List all agents
agents := env.Agents()
for _, agent := range agents {
    fmt.Printf("Agent: %s (Supervisor: %t)\n", 
        agent.Name(), agent.IsSupervisor())
}

// Filter agents by capability
var supervisors []dive.Agent
for _, agent := range env.Agents() {
    if agent.IsSupervisor() {
        supervisors = append(supervisors, agent)
    }
}
```

## Workflow Execution

### `environment.Execution`

Represents a running workflow execution.

```go
type Execution struct {
    id             string
    workflowName   string
    status         ExecutionStatus
    inputs         map[string]interface{}
    outputs        map[string]interface{}
    variables      map[string]interface{}
    currentStep    string
    stepHistory    []string
    startTime      time.Time
    endTime        *time.Time
    environment    *Environment
    context        context.Context
    cancel         context.CancelFunc
    events         chan *Event
    errors         []error
    checkpoints    []*Checkpoint
    retryOptions   *RetryOptions
}
```

### Execution Status

```go
type ExecutionStatus string

const (
    ExecutionStatusPending    ExecutionStatus = "pending"
    ExecutionStatusRunning    ExecutionStatus = "running"
    ExecutionStatusPaused     ExecutionStatus = "paused"
    ExecutionStatusCompleted  ExecutionStatus = "completed"
    ExecutionStatusFailed     ExecutionStatus = "failed"
    ExecutionStatusCancelled  ExecutionStatus = "cancelled"
)
```

### Execution Methods

```go
// Metadata access
func (e *Execution) ID() string
func (e *Execution) WorkflowName() string
func (e *Execution) Status() ExecutionStatus
func (e *Execution) StartTime() time.Time
func (e *Execution) EndTime() *time.Time

// State access
func (e *Execution) Inputs() map[string]interface{}
func (e *Execution) Outputs() map[string]interface{}
func (e *Execution) Variables() map[string]interface{}
func (e *Execution) CurrentStep() string
func (e *Execution) StepHistory() []string

// Control operations
func (e *Execution) Pause(ctx context.Context) error
func (e *Execution) Resume(ctx context.Context) error
func (e *Execution) Cancel(ctx context.Context) error
func (e *Execution) Wait(ctx context.Context) (*ExecutionResult, error)

// Event streaming
func (e *Execution) Events() <-chan *Event
func (e *Execution) AddEventListener(listener EventListener) error

// Checkpointing
func (e *Execution) CreateCheckpoint(ctx context.Context, name string) (*Checkpoint, error)
func (e *Execution) RestoreCheckpoint(ctx context.Context, checkpointID string) error
func (e *Execution) ListCheckpoints() []*Checkpoint
```

### Running Workflows

```go
// Simple workflow execution
inputs := map[string]interface{}{
    "message": "Hello, world!",
    "priority": 5,
}

execution, err := env.RunWorkflow(ctx, "MessageProcessor", inputs)
if err != nil {
    return err
}

// Wait for completion
result, err := execution.Wait(ctx)
if err != nil {
    return err
}

fmt.Printf("Result: %v\n", result.Outputs)
```

### Monitoring Execution

```go
// Monitor execution with events
go func() {
    for event := range execution.Events() {
        switch event.Type {
        case EventTypeStepStarted:
            fmt.Printf("Step started: %s\n", event.StepName)
        case EventTypeStepCompleted:
            fmt.Printf("Step completed: %s\n", event.StepName)
        case EventTypeStepFailed:
            fmt.Printf("Step failed: %s - %s\n", event.StepName, event.Error)
        case EventTypeExecutionCompleted:
            fmt.Printf("Workflow completed: %s\n", event.ExecutionID)
        }
    }
}()

// Check status periodically
ticker := time.NewTicker(5 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        fmt.Printf("Status: %s, Step: %s\n", 
            execution.Status(), execution.CurrentStep())
    case <-ctx.Done():
        return
    }
}
```

## Repository Management

### Document Repository

```go
type DocumentRepository interface {
    // CRUD operations
    CreateDocument(ctx context.Context, doc *Document) error
    GetDocument(ctx context.Context, id string) (*Document, error)
    UpdateDocument(ctx context.Context, doc *Document) error  
    DeleteDocument(ctx context.Context, id string) error
    
    // Search and listing
    ListDocuments(ctx context.Context, limit, offset int) ([]*Document, error)
    SearchDocuments(ctx context.Context, query string) ([]*Document, error)
    
    // Content operations
    GetDocumentContent(ctx context.Context, id string) ([]byte, error)
    SetDocumentContent(ctx context.Context, id string, content []byte) error
}
```

### Thread Repository

```go
type ThreadRepository interface {
    // Thread management
    CreateThread(ctx context.Context, thread *Thread) error
    GetThread(ctx context.Context, threadID string) (*Thread, error)
    UpdateThread(ctx context.Context, thread *Thread) error
    DeleteThread(ctx context.Context, threadID string) error
    
    // Message operations
    AddMessage(ctx context.Context, threadID string, message *Message) error
    GetMessages(ctx context.Context, threadID string, limit, offset int) ([]*Message, error)
    
    // Search operations
    ListThreads(ctx context.Context, userID string, limit, offset int) ([]*Thread, error)
    SearchThreads(ctx context.Context, query, userID string) ([]*Thread, error)
}
```

### Built-in Repositories

```go
// In-memory repositories (not persistent)
documentRepo := objects.NewInMemoryDocumentRepository()
threadRepo := objects.NewInMemoryThreadRepository()

// File-based repositories
documentRepo := objects.NewFileDocumentRepository("./data/documents")
threadRepo := objects.NewFileThreadRepository("./data/threads")

// PostgreSQL repositories
documentRepo, err := objects.NewPostgresDocumentRepository(connectionString)
threadRepo, err := objects.NewPostgresThreadRepository(connectionString)
```

## Action System

### `environment.Action`

Interface for custom actions that can be executed within workflows.

```go
type Action interface {
    // Metadata
    Name() string
    Description() string
    
    // Execution
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}
```

### Built-in Actions

```go
// Document operations
type DocumentWriteAction struct {
    repo dive.DocumentRepository
}

func (a *DocumentWriteAction) Name() string { 
    return "Document.Write" 
}

func (a *DocumentWriteAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    path := params["Path"].(string)
    content := params["Content"].(string)
    
    doc := &dive.Document{
        ID:      dive.NewID(),
        Name:    filepath.Base(path),
        Path:    path,
        Content: content,
    }
    
    return nil, a.repo.CreateDocument(ctx, doc)
}

// Logging actions
type LogAction struct {
    level string
}

func (a *LogAction) Name() string { 
    return "Log." + strings.Title(a.level)
}

// HTTP request actions
type HTTPRequestAction struct{}

func (a *HTTPRequestAction) Name() string { 
    return "HTTP.Request" 
}
```

### Custom Actions

```go
type SlackNotificationAction struct {
    token   string
    channel string
}

func (a *SlackNotificationAction) Name() string {
    return "Slack.Notify"
}

func (a *SlackNotificationAction) Description() string {
    return "Send notification to Slack channel"
}

func (a *SlackNotificationAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    message := params["Message"].(string)
    channel := params["Channel"].(string)
    if channel == "" {
        channel = a.channel
    }
    
    // Send to Slack API
    return a.sendSlackMessage(channel, message)
}

// Register custom action
env.RegisterAction(&SlackNotificationAction{
    token:   os.Getenv("SLACK_TOKEN"),
    channel: "#notifications",
})
```

## Event Management

### Event Types

```go
type EventType string

const (
    // Environment events
    EventTypeEnvironmentStarted EventType = "environment_started"
    EventTypeEnvironmentStopped EventType = "environment_stopped"
    
    // Agent events
    EventTypeAgentAdded         EventType = "agent_added"
    EventTypeAgentRemoved       EventType = "agent_removed"
    
    // Execution events
    EventTypeExecutionStarted   EventType = "execution_started"
    EventTypeExecutionPaused    EventType = "execution_paused"
    EventTypeExecutionResumed   EventType = "execution_resumed"
    EventTypeExecutionCompleted EventType = "execution_completed"
    EventTypeExecutionFailed    EventType = "execution_failed"
    EventTypeExecutionCancelled EventType = "execution_cancelled"
    
    // Step events
    EventTypeStepStarted        EventType = "step_started"
    EventTypeStepCompleted      EventType = "step_completed" 
    EventTypeStepFailed         EventType = "step_failed"
    EventTypeStepSkipped        EventType = "step_skipped"
)
```

### Event Structure

```go
type Event struct {
    ID           string                 `json:"id"`
    Type         EventType              `json:"type"`
    Timestamp    time.Time              `json:"timestamp"`
    
    // Context information
    EnvironmentID string                `json:"environment_id,omitempty"`
    ExecutionID   string                `json:"execution_id,omitempty"`
    WorkflowName  string                `json:"workflow_name,omitempty"`
    StepName      string                `json:"step_name,omitempty"`
    AgentName     string                `json:"agent_name,omitempty"`
    
    // Event data
    Data          map[string]interface{} `json:"data,omitempty"`
    Error         string                 `json:"error,omitempty"`
    
    // Metadata
    UserID        string                 `json:"user_id,omitempty"`
    Tags          []string               `json:"tags,omitempty"`
}
```

### Event Listeners

```go
type EventListener interface {
    OnEvent(ctx context.Context, event *Event) error
}

// Custom event listener
type ExecutionMonitor struct {
    logger slogger.Logger
}

func (m *ExecutionMonitor) OnEvent(ctx context.Context, event *Event) error {
    switch event.Type {
    case EventTypeExecutionStarted:
        m.logger.Info("Execution started", 
            "execution_id", event.ExecutionID,
            "workflow", event.WorkflowName)
    case EventTypeExecutionFailed:
        m.logger.Error("Execution failed",
            "execution_id", event.ExecutionID,
            "error", event.Error)
    }
    return nil
}

// Add listener to environment
monitor := &ExecutionMonitor{logger: slogger.DefaultLogger}
env.AddEventListener(monitor)
```

## MCP Integration

### MCP Server Configuration

```go
type ServerConfig struct {
    Type               string            `json:"type"`               // "stdio" or "url"
    Name               string            `json:"name"`
    Command            string            `json:"command,omitempty"`
    URL                string            `json:"url,omitempty"`
    Env                map[string]string `json:"env,omitempty"`
    Args               []string          `json:"args,omitempty"`
    AuthorizationToken string            `json:"authorization_token,omitempty"`
    Headers            map[string]string `json:"headers,omitempty"`
    OAuth              *OAuthConfig      `json:"oauth,omitempty"`
    ToolConfiguration  *ToolConfiguration `json:"tool_configuration,omitempty"`
}
```

### MCP Manager

```go
type Manager struct {
    servers    map[string]*Client
    logger     slogger.Logger
    registry   *ToolRegistry
}

func NewManager(logger slogger.Logger) *Manager

// Server management
func (m *Manager) AddServer(config *ServerConfig) error
func (m *Manager) RemoveServer(name string) error
func (m *Manager) GetServer(name string) (*Client, error)
func (m *Manager) ListServers() []*ServerConfig

// Tool access
func (m *Manager) GetTools() []dive.Tool
func (m *Manager) GetTool(name string) (dive.Tool, error)

// Resource access
func (m *Manager) ListResources(ctx context.Context) ([]*Resource, error)
func (m *Manager) GetResource(ctx context.Context, uri string) (*ResourceContent, error)
```

### Environment MCP Integration

```go
// Configure MCP servers
mcpServers := []*mcp.ServerConfig{
    {
        Type:    "stdio",
        Name:    "filesystem",
        Command: "npx",
        Args:    []string{"@modelcontextprotocol/server-filesystem", "./workspace"},
    },
    {
        Type: "url",
        Name: "github",
        URL:  "https://mcp.github.com/sse",
        Headers: map[string]string{
            "Authorization": "Bearer " + os.Getenv("GITHUB_TOKEN"),
        },
    },
}

// Create environment with MCP
env, err := environment.New(environment.Options{
    Name:       "MCP Environment",
    MCPServers: mcpServers,
    Agents:     agents,
})
```

## Examples

### Basic Environment Setup

```go
package main

import (
    "context"
    "log"
    
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/environment"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/objects"
)

func main() {
    // Create repositories
    docRepo := objects.NewInMemoryDocumentRepository()
    threadRepo := objects.NewInMemoryThreadRepository()
    
    // Create agent
    assistant, err := agent.New(agent.Options{
        Name:         "Assistant",
        Instructions: "You are a helpful assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create environment
    env, err := environment.New(environment.Options{
        Name:               "Basic Environment",
        Agents:             []dive.Agent{assistant},
        DocumentRepository: docRepo,
        ThreadRepository:   threadRepo,
        AutoStart:          true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer env.Stop(context.Background())
    
    log.Printf("Environment %s started with %d agents", 
        env.Name(), len(env.Agents()))
}
```

### Multi-Agent Environment

```go
func createMultiAgentEnvironment() (*environment.Environment, error) {
    // Create specialized agents
    researcher, _ := agent.New(agent.Options{
        Name:         "Researcher",
        Instructions: "You conduct thorough research on topics.",
        Model:        anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool()),
        },
    })
    
    analyst, _ := agent.New(agent.Options{
        Name:         "Analyst", 
        Instructions: "You analyze data and provide insights.",
        Model:        anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewDataAnalysisTool()),
        },
    })
    
    supervisor, _ := agent.New(agent.Options{
        Name:         "Supervisor",
        Instructions: "You coordinate work between agents.",
        IsSupervisor: true,
        Subordinates: []string{"Researcher", "Analyst"},
        Model:        anthropic.New(),
    })
    
    return environment.New(environment.Options{
        Name:   "Research Team",
        Agents: []dive.Agent{researcher, analyst, supervisor},
        DocumentRepository: objects.NewFileDocumentRepository("./data"),
        ThreadRepository:   objects.NewFileThreadRepository("./threads"),
    })
}
```

### Environment with Custom Actions

```go
type EmailAction struct {
    smtpConfig SMTPConfig
}

func (a *EmailAction) Name() string { return "Email.Send" }

func (a *EmailAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    to := params["To"].(string)
    subject := params["Subject"].(string)
    body := params["Body"].(string)
    
    return nil, a.sendEmail(to, subject, body)
}

func setupEnvironmentWithActions() (*environment.Environment, error) {
    // Custom actions
    emailAction := &EmailAction{smtpConfig: loadSMTPConfig()}
    slackAction := &SlackNotificationAction{token: os.Getenv("SLACK_TOKEN")}
    
    return environment.New(environment.Options{
        Name:   "Notification Environment",
        Agents: agents,
        Actions: []environment.Action{emailAction, slackAction},
    })
}
```

### Production Environment

```go
func createProductionEnvironment() (*environment.Environment, error) {
    // PostgreSQL repositories
    docRepo, err := objects.NewPostgresDocumentRepository(
        os.Getenv("DATABASE_URL"))
    if err != nil {
        return nil, err
    }
    
    threadRepo, err := objects.NewPostgresThreadRepository(
        os.Getenv("DATABASE_URL"))
    if err != nil {
        return nil, err
    }
    
    // Production logger
    logger := slogger.New(slogger.Options{
        Level:  slogger.LevelInfo,
        Format: slogger.FormatJSON,
    })
    
    // MCP servers
    mcpServers := []*mcp.ServerConfig{
        {
            Type:    "stdio",
            Name:    "database",
            Command: "npx",
            Args:    []string{"@modelcontextprotocol/server-postgres"},
            Env: map[string]string{
                "DATABASE_URL": os.Getenv("DATABASE_URL"),
            },
        },
    }
    
    return environment.New(environment.Options{
        Name:               "Production Environment",
        Description:        "Production-ready environment with persistence",
        Agents:             loadProductionAgents(),
        Workflows:          loadWorkflows(),
        DocumentRepository: docRepo,
        ThreadRepository:   threadRepo,
        MCPServers:         mcpServers,
        Logger:             logger,
        AutoStart:          true,
    })
}
```

This comprehensive API reference covers all aspects of the environment package, enabling developers to create sophisticated multi-agent environments with workflow orchestration, persistent storage, and extensible action systems.