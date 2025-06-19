# Environment Package Documentation

The `environment` package provides a comprehensive runtime container for executing AI-powered workflows with deterministic execution, event streaming, and replay capabilities. It orchestrates agents, workflows, and operations while maintaining full execution history for debugging and replay.

## Table of Contents

- [Overview](#overview)
- [Core Concepts](#core-concepts)
- [Key Interfaces & Structs](#key-interfaces--structs)
- [Execution System](#execution-system)
- [Event System](#event-system)
- [Operations & Determinism](#operations--determinism)
- [Path Management](#path-management)
- [State Management](#state-management)
- [Examples](#examples)
- [Advanced Features](#advanced-features)

## Overview

The environment package serves as the orchestration layer for the Dive framework, providing:

- **Deterministic Execution**: Reproducible workflow runs with full replay capabilities
- **Event Streaming**: Real-time execution events for monitoring and debugging
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

An `Execution` represents a single deterministic run of a workflow, featuring:
- **Deterministic Operations**: Non-deterministic calls (LLM, I/O) are tracked and replayable
- **Path Branching**: Support for parallel execution paths
- **Event Recording**: Complete execution history
- **State Management**: Safe scripting environment with Risor
- **Content Fingerprinting**: Deterministic tracking of dynamic content

### Operations

Operations represent non-deterministic function calls that need to be tracked for replay:
- LLM agent responses
- File I/O operations  
- External API calls
- Script executions

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
    
    // Deterministic execution
    operationResults map[OperationID]*OperationResult
    replayMode       bool
    
    // Path management
    paths       map[string]*PathState
    activePaths map[string]*executionPath
    
    // Event recording
    recorder ExecutionRecorder
    state    *WorkflowState
}
```

**Key Methods:**
- `NewExecution(opts ExecutionOptions) (*Execution, error)` - Create execution
- `Run(ctx context.Context) error` - Execute workflow to completion
- `ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error)` - Execute tracked operation
- `LoadFromEvents(ctx context.Context) error` - Replay from recorded events

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
    Name:    "MyEnvironment",
    Agents:  []dive.Agent{myAgent},
    Workflows: []*workflow.Workflow{myWorkflow},
    Logger:  slogger.DefaultLogger,
})
if err != nil {
    return err
}

// Create execution
execution, err := environment.NewExecution(environment.ExecutionOptions{
    Workflow:    myWorkflow,
    Environment: env,
    Inputs:      map[string]interface{}{"query": "Hello"},
    EventStore:  myEventStore,
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

### Replay Mode

```go
// Create execution in replay mode
execution, err := environment.NewExecutionFromReplay(environment.ExecutionOptions{
    Workflow:    myWorkflow,
    Environment: env,
    Inputs:      originalInputs,
    EventStore:  myEventStore,
    ReplayMode:  true,
})
```

## Event System

### Event Types

```go
const (
    // Execution lifecycle
    EventExecutionStarted   ExecutionEventType = "execution_started"
    EventExecutionCompleted ExecutionEventType = "execution_completed"
    EventExecutionFailed    ExecutionEventType = "execution_failed"
    
    // Path management
    EventPathStarted   ExecutionEventType = "path_started"
    EventPathCompleted ExecutionEventType = "path_completed"
    EventPathBranched  ExecutionEventType = "path_branched"
    
    // Step execution
    EventStepStarted   ExecutionEventType = "step_started"
    EventStepCompleted ExecutionEventType = "step_completed"
    EventStepFailed    ExecutionEventType = "step_failed"
    
    // Operations
    EventOperationStarted   ExecutionEventType = "operation_started"
    EventOperationCompleted ExecutionEventType = "operation_completed"
    EventOperationFailed    ExecutionEventType = "operation_failed"
    
    // State changes
    EventStateMutated ExecutionEventType = "state_mutated"
)
```

### Event Structure

```go
type ExecutionEvent struct {
    ID          string                 `json:"id"`
    ExecutionID string                 `json:"execution_id"`
    Sequence    int64                  `json:"sequence"`
    Timestamp   time.Time              `json:"timestamp"`
    EventType   ExecutionEventType     `json:"event_type"`
    Path        string                 `json:"path,omitempty"`
    Step        string                 `json:"step,omitempty"`
    Data        map[string]interface{} `json:"data,omitempty"`
}
```

## Operations & Determinism

### Deterministic Operation Execution

Operations ensure deterministic execution by:

1. **ID Generation**: Deterministic IDs based on operation parameters
2. **Result Caching**: Results cached for replay
3. **Content Fingerprinting**: Dynamic content tracked by hash
4. **Event Recording**: All operations recorded with full context

```go
// Execute a deterministic operation
op := Operation{
    Type:     "agent_response",
    StepName: step.Name(),
    PathID:   pathID,
    Parameters: map[string]interface{}{
        "agent":           agent.Name(),
        "prompt":          prompt,
        "content_hash":    contentSnapshot.Fingerprint.Hash,
    },
}

result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
    return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(content...)))
})
```

### Content Fingerprinting

```go
type ContentFingerprint struct {
    Hash    string `json:"hash"`    // SHA256 hash of content
    Source  string `json:"source"`  // Content source identifier
    Size    int64  `json:"size"`    // Content size in bytes
    ModTime string `json:"modtime"` // Modification time for files
}

type ContentSnapshot struct {
    Fingerprint ContentFingerprint `json:"fingerprint"`
    Content     []llm.Content      `json:"content"`
    Raw         []byte             `json:"raw,omitempty"`
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
    CurrentStep *workflow.Step    `json:"current_step,omitempty"`
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
    // Record branching event
    execution.recorder.RecordEvent(EventPathBranched, pathID, stepName, map[string]interface{}{
        "new_paths": pathData,
    })
    
    // Start new paths concurrently
    for _, newPath := range newPaths {
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

### Event Streaming

```go
// Implement custom event store
type MyEventStore struct{}

func (s *MyEventStore) RecordEvent(event *environment.ExecutionEvent) error {
    // Store event in your preferred backend
    log.Printf("Event: %s - %s", event.EventType, event.Step)
    return nil
}

func (s *MyEventStore) GetEvents(ctx context.Context, executionID string) ([]*environment.ExecutionEvent, error) {
    // Retrieve events from storage  
    return events, nil
}

// Use with execution
execution, err := environment.NewExecution(environment.ExecutionOptions{
    Workflow:   myWorkflow,
    Environment: env,
    EventStore: &MyEventStore{},
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

The environment package provides a robust foundation for building deterministic, observable, and replayable AI workflow systems. Its event-driven architecture and operation tracking enable sophisticated debugging, testing, and production monitoring capabilities.
