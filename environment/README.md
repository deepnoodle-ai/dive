# Environment Package Documentation

The `environment` package provides a comprehensive runtime container for executing AI-powered workflows with **checkpoint-based execution**, **operation tracking**, and **state management**. It orchestrates agents, workflows, and operations while maintaining execution state through simple, reliable checkpointing for recovery and debugging.

## Table of Contents

- [Overview](#overview)
- [Core Concepts](#core-concepts)
- [Key Interfaces & Structs](#key-interfaces--structs)
- [Execution System](#execution-system)
- [Checkpoint System](#checkpoint-system)
- [Operations & Determinism](#operations--determinism)
- [Path Management](#path-management)
- [State Management](#state-management)
- [Examples](#examples)
- [Advanced Features](#advanced-features)

## Overview

The environment package serves as the orchestration layer for the Dive framework, providing:

- **Checkpoint-Based Execution**: Simple, reliable state persistence with automatic recovery
- **Operation Tracking**: Deterministic handling of non-deterministic operations (LLM calls, file I/O)
- **Path Management**: Support for parallel execution paths and conditional branching
- **State Management**: Safe, scriptable state with Risor integration
- **MCP Integration**: Model Context Protocol server management
- **Action System**: Pluggable action handlers for workflow steps

## Core Concepts

### Environment

An `Environment` is a container that manages:
- **Agents**: AI agents with LLM capabilities
- **Workflows**: Declarative workflow definitions
- **Executions**: Active workflow runs
- **Actions**: Pluggable step handlers
- **MCP Servers**: External tool providers

### Execution

An `Execution` represents a single run of a workflow, featuring:
- **Checkpoint-Based State**: Simple state persistence with automatic recovery
- **Operation Tracking**: Non-deterministic calls (LLM, I/O) are logged for debugging
- **Path Branching**: Support for parallel execution paths
- **State Management**: Safe scripting environment with Risor
- **Token Usage Tracking**: Comprehensive LLM usage monitoring

### Operations

Operations represent non-deterministic function calls that need to be tracked:
- LLM agent responses
- File I/O operations  
- External API calls
- Script executions

Each operation is logged with its parameters, results, and timing for debugging and analysis.

## Key Interfaces & Structs

### Environment

```go
type Environment struct {
    // Core identification
    id              string
    name            string
    description     string
    
    // Component management
    agents          map[string]dive.Agent
    workflows       map[string]*workflow.Workflow
    executions      map[string]*Execution
    actions         map[string]Action
    
    // External integrations
    documentRepo    dive.DocumentRepository
    threadRepo      dive.ThreadRepository
    mcpManager      *mcp.Manager
    mcpServers      []*mcp.ServerConfig
    
    // Runtime state
    started         bool
    logger          slogger.Logger
    formatter       WorkflowFormatter
}
```

**Key Methods:**
- `New(opts Options) (*Environment, error)` - Create new environment
- `Start(ctx context.Context) error` - Initialize and start services
- `GetAgent(name string) (dive.Agent, error)` - Retrieve agent by name
- `GetWorkflow(name string) (*workflow.Workflow, error)` - Retrieve workflow
- `GetMCPTools() map[string]dive.Tool` - Get all MCP tools

### Execution

```go
type Execution struct {
    // Core identification
    id          string
    workflow    *workflow.Workflow
    environment *Environment
    
    // Execution state
    status      ExecutionStatus
    startTime   time.Time
    endTime     time.Time
    inputs      map[string]interface{}
    outputs     map[string]interface{}
    
    // Checkpoint-based persistence
    operationLogger OperationLogger
    checkpointer    ExecutionCheckpointer
    
    // Path management
    paths       map[string]*PathState
    activePaths map[string]*executionPath
    
    // State management
    state    *WorkflowState
    totalUsage llm.Usage
}
```

**Key Methods:**
- `NewExecution(opts ExecutionOptions) (*Execution, error)` - Create execution
- `Run(ctx context.Context) error` - Execute workflow to completion
- `ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error)` - Execute tracked operation
- `LoadFromCheckpoint(ctx context.Context) error` - Load state from latest checkpoint

### ExecutionStatus

```go
type ExecutionStatus string

const (
    ExecutionStatusPending   ExecutionStatus = "pending"
    ExecutionStatusRunning   ExecutionStatus = "running"
    ExecutionStatusCompleted ExecutionStatus = "completed"
    ExecutionStatusFailed    ExecutionStatus = "failed"
)
```

### Operation

```go
type Operation struct {
    ID         OperationID            // Unique operation identifier
    Type       string                 // Operation type (e.g., "agent_response")
    StepName   string                 // Associated workflow step
    PathID     string                 // Execution path identifier
    Parameters map[string]interface{} // Operation parameters for deterministic ID generation
}
```

## Execution System

### Creating and Running Executions

```go
// Create environment
env, err := environment.New(environment.Options{
    Name:      "MyEnvironment",
    Agents:    []dive.Agent{myAgent},
    Workflows: []*workflow.Workflow{myWorkflow},
    Logger:    slogger.DefaultLogger,
})
if err != nil {
    return err
}

// Create execution
execution, err := environment.NewExecution(environment.ExecutionOptions{
    Workflow:    myWorkflow,
    Environment: env,
    Inputs:      map[string]interface{}{"query": "Hello"},
    Logger:      env.logger,
})
if err != nil {
    return err
}

// Run workflow
ctx := context.Background()
if err := execution.Run(ctx); err != nil {
    return fmt.Errorf("execution failed: %w", err)
}
```

### Execution Recovery

```go
// Execution automatically attempts to load from latest checkpoint
execution, err := environment.NewExecution(environment.ExecutionOptions{
    Workflow:     myWorkflow,
    Environment:  env,
    Inputs:       originalInputs,
    ExecutionID:  existingExecutionID, // Resume specific execution
})

// Run will automatically continue from last checkpoint
if err := execution.Run(ctx); err != nil {
    return fmt.Errorf("execution failed: %w", err)
}
```

## Checkpoint System

The checkpoint system provides simple, reliable state persistence for workflow executions.

### Checkpoint Components

**ExecutionCheckpoint** - Complete execution state snapshot
```go
type ExecutionCheckpoint struct {
    ID           string                     `json:"id"`
    ExecutionID  string                     `json:"execution_id"`
    WorkflowName string                     `json:"workflow_name"`
    Status       string                     `json:"status"`
    Inputs       map[string]interface{}     `json:"inputs"`
    Outputs      map[string]interface{}     `json:"outputs"`
    State        map[string]interface{}     `json:"state"`
    PathStates   map[string]*PathState      `json:"path_states"`
    TotalUsage   *llm.Usage                 `json:"total_usage"`
    StartTime    time.Time                  `json:"start_time"`
    EndTime      time.Time                  `json:"end_time"`
    CheckpointAt time.Time                  `json:"checkpoint_at"`
    Error        string                     `json:"error,omitempty"`
}
```

**ExecutionCheckpointer** - Interface for checkpoint persistence
```go
type ExecutionCheckpointer interface {
    SaveCheckpoint(ctx context.Context, checkpoint *ExecutionCheckpoint) error
    LoadCheckpoint(ctx context.Context, executionID string) (*ExecutionCheckpoint, error)
}
```

### Checkpoint Behavior

- **Automatic Checkpointing**: Checkpoint saved after every operation for maximum reliability
- **Recovery on Start**: Executions automatically attempt to load from latest checkpoint
- **Idempotent Operations**: Operations are designed to be safely retried from checkpoints
- **State Consistency**: Complete execution state captured in each checkpoint

## Operations & Determinism

### Operation Execution

Operations ensure reliable execution tracking by:

1. **Unique ID Generation**: Deterministic IDs based on operation parameters
2. **Comprehensive Logging**: All operations logged with parameters, results, and timing
3. **Checkpoint Integration**: State saved after each operation
4. **Error Tracking**: Failed operations logged with detailed error information

```go
// Execute a tracked operation
op := Operation{
    Type:     "agent_response",
    StepName: step.Name(),
    PathID:   pathID,
    Parameters: map[string]interface{}{
        "agent":  agent.Name(),
        "prompt": prompt,
    },
}

result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
    return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(content...)))
})
```

### Operation Logging

```go
type OperationLogEntry struct {
    ID            string                 `json:"id"`
    ExecutionID   string                 `json:"execution_id"`
    StepName      string                 `json:"step_name"`
    PathID        string                 `json:"path_id"`
    OperationType string                 `json:"operation_type"`
    Parameters    map[string]interface{} `json:"parameters"`
    Result        interface{}            `json:"result"`
    StartTime     time.Time              `json:"start_time"`
    Duration      time.Duration          `json:"duration"`
    Error         string                 `json:"error,omitempty"`
}
```

## Path Management

### Path States

```go
type PathStatus string

const (
    PathStatusPending   PathStatus = "pending"
    PathStatusRunning   PathStatus = "running"
    PathStatusCompleted PathStatus = "completed"
    PathStatusFailed    PathStatus = "failed"
)

type PathState struct {
    ID          string            `json:"id"`
    Status      PathStatus        `json:"status"`
    CurrentStep string            `json:"current_step"`
    StartTime   time.Time         `json:"start_time"`
    EndTime     time.Time         `json:"end_time,omitempty"`
    StepOutputs map[string]string `json:"step_outputs"`
    Error       error             `json:"error,omitempty"`
}
```

### Path Branching

Paths can branch when workflow conditions create multiple execution routes:

```go
// Handle path branching during execution
newPaths, err := execution.handlePathBranching(ctx, currentStep, pathID)
if err != nil {
    return err
}

// Multiple paths indicate branching
if len(newPaths) > 1 {
    // Start new paths concurrently
    for _, newPath := range newPaths {
        execution.addPath(newPath)
        go execution.runPath(ctx, newPath)
    }
}
```

## State Management

### Script Contexts

The environment provides different scripting contexts with varying capabilities:

```go
type ScriptContext int

const (
    ScriptContextCondition ScriptContext = iota // Read-only, safe functions only  
    ScriptContextTemplate                      // Read-only, safe functions only
    ScriptContextEach                          // Read-only, safe functions only
    ScriptContextActivity                      // Full function access
)
```

### State Access

```go
// Get workflow state
state := execution.state

// Set state variables
state.Set("myVariable", value)

// Get state variables  
value, exists := state.Get("myVariable")

// Delete state variables
state.Delete("myVariable")

// Get state snapshot for scripting
stateSnapshot := state.Copy()
```

## Examples

### Basic Environment Setup

```go
package main

import (
    "context"
    "log"
    
    "github.com/diveagents/dive/environment"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/workflow"
)

func main() {
    // Create agent
    myAgent, err := agent.New(agent.Options{
        Name: "assistant",
        // ... agent configuration
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create workflow
    myWorkflow, err := workflow.New(workflow.Options{
        Name: "greeting",
        Steps: []*workflow.Step{
            // ... workflow steps  
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create environment
    env, err := environment.New(environment.Options{
        Name:      "MyEnvironment",
        Agents:    []dive.Agent{myAgent},
        Workflows: []*workflow.Workflow{myWorkflow},
        AutoStart: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer env.Stop(context.Background())
    
    // Create and run execution
    execution, err := environment.NewExecution(environment.ExecutionOptions{
        Workflow:    myWorkflow,
        Environment: env,
        Inputs:      map[string]interface{}{"name": "World"},
    })
    if err != nil {
        log.Fatal(err)
    }
    
    if err := execution.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Execution completed with status: %s", execution.Status())
}
```

### Custom Action Registration

```go
// Define custom action
type MyAction struct{}

func (a *MyAction) Name() string {
    return "my_custom_action"
}

func (a *MyAction) Description() string {
    return "Performs a custom operation"
}

func (a *MyAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Custom logic here
    return "Action completed", nil
}

// Register action with environment
env, err := environment.New(environment.Options{
    Name:    "MyEnvironment",
    Actions: []environment.Action{&MyAction{}},
})
```

### Custom Checkpointing

```go
// Implement custom checkpointer
type MyCheckpointer struct{}

func (c *MyCheckpointer) SaveCheckpoint(ctx context.Context, checkpoint *environment.ExecutionCheckpoint) error {
    // Store checkpoint in your preferred backend
    log.Printf("Saving checkpoint: %s", checkpoint.ID)
    return nil
}

func (c *MyCheckpointer) LoadCheckpoint(ctx context.Context, executionID string) (*environment.ExecutionCheckpoint, error) {
    // Load checkpoint from storage  
    return checkpoint, nil
}

// Use with execution
execution, err := environment.NewExecution(environment.ExecutionOptions{
    Workflow:     myWorkflow,
    Environment:  env,
    Checkpointer: &MyCheckpointer{},
})
```

### Operation Logging

```go
// Implement custom operation logger
type MyOperationLogger struct{}

func (l *MyOperationLogger) LogOperation(ctx context.Context, entry *environment.OperationLogEntry) error {
    // Log operation to your preferred system
    log.Printf("Operation: %s - %s took %v", entry.OperationType, entry.StepName, entry.Duration)
    return nil
}

// Use with execution
execution, err := environment.NewExecution(environment.ExecutionOptions{
    Workflow:        myWorkflow,
    Environment:     env,
    OperationLogger: &MyOperationLogger{},
})
```

## Advanced Features

### MCP Server Integration

```go
// Configure MCP servers
mcpServers := []*mcp.ServerConfig{
    {
        Name:    "github",
        Command: "mcp-server-github",
        Args:    []string{"--token", "your-token"},
    },
}

env, err := environment.New(environment.Options{
    Name:       "MCPEnvironment", 
    MCPServers: mcpServers,
})

// Access MCP tools
tools := env.GetMCPTools()
githubTool := env.GetMCPTool("github/create_issue")
```

### Workflow Formatters

```go
// Implement custom formatter
type MyFormatter struct{}

func (f *MyFormatter) PrintStepStart(stepName, stepType string) {
    fmt.Printf("🚀 Starting %s step: %s\n", stepType, stepName)
}

func (f *MyFormatter) PrintStepOutput(stepName, output string) {
    fmt.Printf("✅ %s completed: %s\n", stepName, output)
}

func (f *MyFormatter) PrintStepError(stepName string, err error) {
    fmt.Printf("❌ %s failed: %v\n", stepName, err)
}

// Use with environment
env, err := environment.New(environment.Options{
    Name:      "FormattedEnvironment",
    Formatter: &MyFormatter{},
})
```

### Document Repository Integration

```go
// Environment automatically registers document actions when a repository is provided
env, err := environment.New(environment.Options{
    Name:               "DocumentEnvironment",
    DocumentRepository: myDocumentRepo,
})

// Document read/write actions are automatically available in workflows
// Actions: "document.read", "document.write"
```

## Key Design Principles

### Simplicity

The checkpoint-based design prioritizes simplicity and reliability over complex event sourcing patterns. Checkpoints provide sufficient state recovery capabilities while being much easier to understand, debug, and maintain.

### Reliability

- **Automatic Recovery**: Executions automatically resume from the latest checkpoint
- **Operation Tracking**: All non-deterministic operations are logged for debugging
- **State Consistency**: Complete execution state is captured in each checkpoint
- **Error Handling**: Comprehensive error tracking and recovery mechanisms

### Observability

- **Operation Logging**: Detailed logging of all operations with timing and results
- **State Snapshots**: Complete state visibility through checkpoints
- **Path Tracking**: Visibility into parallel execution paths and their states
- **Token Usage**: Comprehensive LLM usage tracking across all operations

The environment package provides a robust foundation for building reliable, observable, and recoverable AI workflow systems. Its checkpoint-based architecture enables sophisticated debugging, testing, and production monitoring capabilities while maintaining simplicity and ease of use.
