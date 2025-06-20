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

The event system provides comprehensive visibility into workflow execution through a rich set of event types that capture every aspect of execution flow, state changes, and operations. Events enable deterministic replay, debugging, monitoring, and audit trails.

### Event Categories

#### 1. Execution Lifecycle Events

These events track the overall execution lifecycle of a workflow.

**`execution_started`** - Emitted when a workflow execution begins
```go
type ExecutionStartedData struct {
    WorkflowName string                 `json:"workflow_name"` // Name of the workflow being executed
    Inputs       map[string]interface{} `json:"inputs"`        // Initial inputs to the workflow
}
```
*Occurs*: At the very beginning of workflow execution, before any paths or steps are started.
*Purpose*: Establishes the execution context and captures initial state for replay.

**`execution_completed`** - Emitted when a workflow execution completes successfully
```go
type ExecutionCompletedData struct {
    Outputs map[string]interface{} `json:"outputs"` // Final outputs produced by the workflow
}
```
*Occurs*: When all execution paths have completed successfully and final outputs are collected.
*Purpose*: Records successful completion and final results.

**`execution_failed`** - Emitted when a workflow execution fails
```go
type ExecutionFailedData struct {
    Error string `json:"error"` // Error message describing the failure
}
```
*Occurs*: When execution encounters an unrecoverable error that prevents completion.
*Purpose*: Records failure reason for debugging and monitoring.

**`execution_continue_as_new`** - Emitted for workflow continuation patterns
*Purpose*: Supports long-running workflows that need to restart with new state.

#### 2. Path Management Events

These events track the execution paths within a workflow, including parallel execution and branching.

**`path_started`** - Emitted when a new execution path begins
```go
type PathStartedData struct {
    CurrentStep string `json:"current_step"` // Name of the first step in this path
}
```
*Occurs*: When a new execution path is created, either at workflow start or due to branching.
*Purpose*: Tracks the beginning of independent execution paths for parallel processing.

**`path_completed`** - Emitted when an execution path completes successfully
```go
type PathCompletedData struct {
    FinalStep string `json:"final_step"` // Name of the last step executed in this path
}
```
*Occurs*: When all steps in a path have completed successfully.
*Purpose*: Records successful path completion for synchronization and monitoring.

**`path_failed`** - Emitted when an execution path fails
```go
type PathFailedData struct {
    Error string `json:"error"` // Error message describing the path failure
}
```
*Occurs*: When a path encounters an error that prevents further execution.
*Purpose*: Records path-specific failures for debugging while other paths may continue.

**`path_branched`** - Emitted when execution creates multiple parallel paths
```go
type PathBranchedData struct {
    NewPaths []PathBranchInfo `json:"new_paths"` // Information about newly created paths
}

type PathBranchInfo struct {
    ID             string `json:"id"`              // Unique identifier for the new path
    CurrentStep    string `json:"current_step"`    // First step in the new path
    InheritOutputs bool   `json:"inherit_outputs"` // Whether the path inherits parent outputs
}
```
*Occurs*: When conditional logic or parallel steps create multiple execution paths.
*Purpose*: Records branching decisions for replay and tracks parallel execution structure.

#### 3. Step Execution Events

These events track individual workflow step execution, providing detailed visibility into step-by-step progress.

**`step_started`** - Emitted when a workflow step begins execution
```go
type StepStartedData struct {
    StepType   string                 `json:"step_type"`   // Type of step (e.g., "agent", "script", "condition")
    StepParams map[string]interface{} `json:"step_params"` // Parameters configured for this step
}
```
*Occurs*: Just before a step's execution logic is invoked.
*Purpose*: Records step initiation with configuration for debugging and replay.

**`step_completed`** - Emitted when a workflow step completes successfully
```go
type StepCompletedData struct {
    Output         string `json:"output"`                    // Result output from the step
    StoredVariable string `json:"stored_variable,omitempty"` // Variable name if output was stored
}
```
*Occurs*: When a step finishes execution successfully and produces output.
*Purpose*: Records step results and variable assignments for state tracking.

**`step_failed`** - Emitted when a workflow step fails
```go
type StepFailedData struct {
    Error string `json:"error"` // Error message describing the step failure
}
```
*Occurs*: When a step encounters an error during execution.
*Purpose*: Records step-level failures with detailed error information.

#### 4. Operation Events

These events track deterministic operations that need to be recorded for replay, such as LLM calls, file I/O, and external API calls.

**`operation_started`** - Emitted when a deterministic operation begins
```go
type OperationStartedData struct {
    OperationID   string                 `json:"operation_id"`   // Unique identifier for this operation
    OperationType string                 `json:"operation_type"` // Type of operation (e.g., "agent_response", "file_read")
    Parameters    map[string]interface{} `json:"parameters"`     // Operation parameters for deterministic replay
}
```
*Occurs*: Before executing any non-deterministic operation that affects execution flow.
*Purpose*: Records operation parameters for deterministic replay and caching.

**`operation_completed`** - Emitted when a deterministic operation completes successfully
```go
type OperationCompletedData struct {
    OperationID   string        `json:"operation_id"`   // Unique identifier matching the started event
    OperationType string        `json:"operation_type"` // Type of operation completed
    Duration      time.Duration `json:"duration"`       // Time taken to complete the operation
    Result        interface{}   `json:"result"`         // Result data from the operation
}
```
*Occurs*: When a deterministic operation finishes successfully.
*Purpose*: Records operation results for replay and performance monitoring.

**`operation_failed`** - Emitted when a deterministic operation fails
```go
type OperationFailedData struct {
    OperationID   string        `json:"operation_id"`   // Unique identifier matching the started event
    OperationType string        `json:"operation_type"` // Type of operation that failed
    Duration      time.Duration `json:"duration"`       // Time taken before failure
    Error         string        `json:"error"`          // Error message describing the failure
}
```
*Occurs*: When a deterministic operation encounters an error.
*Purpose*: Records operation failures for debugging and retry logic.

#### 5. State Management Events

These events track changes to workflow state and variable assignments.

**`state_mutated`** - Emitted when workflow state is modified
```go
type StateMutatedData struct {
    Mutations []StateMutation `json:"mutations"` // List of state changes made
}

type StateMutation struct {
    Type  StateMutationType `json:"type"`            // "set" or "delete"
    Key   string            `json:"key,omitempty"`   // Variable name being modified
    Value interface{}       `json:"value,omitempty"` // New value (for set operations)
}
```
*Occurs*: When workflow variables are set, updated, or deleted.
*Purpose*: Tracks state changes for replay and debugging variable flow.

#### 6. Deterministic Access Events

These events ensure deterministic behavior by recording access to non-deterministic system resources.

**`time_accessed`** - Emitted when current time is accessed during execution
```go
type TimeAccessedData struct {
    AccessedAt time.Time `json:"accessed_at"` // When the time access occurred
    Value      time.Time `json:"value"`       // The time value that was returned
}
```
*Occurs*: When workflow logic accesses current time (e.g., for timestamps, delays).
*Purpose*: Ensures deterministic replay by recording exact time values used.

**`random_generated`** - Emitted when random values are generated during execution
```go
type RandomGeneratedData struct {
    Seed   int64       `json:"seed"`   // Seed used for random generation
    Value  interface{} `json:"value"`  // The random value that was generated
    Method string      `json:"method"` // Method used ("int", "float", "string", etc.)
}
```
*Occurs*: When workflow logic generates random numbers, strings, or other random data.
*Purpose*: Ensures deterministic replay by recording exact random values used.

#### 7. Script State Management Events

These events track scripting operations within workflow steps, providing visibility into iteration loops.

**`iteration_started`** - Emitted when a loop iteration begins
```go
type IterationStartedData struct {
    IterationIndex int         `json:"iteration_index"`     // Zero-based index of this iteration
    Item           interface{} `json:"item"`                // The item being processed in this iteration
    ItemKey        string      `json:"item_key,omitempty"`  // Key for the item (if iterating over a map)
}
```
*Occurs*: At the start of each iteration in loops (for, while, each).
*Purpose*: Tracks loop execution progress for debugging and replay.

**`iteration_completed`** - Emitted when a loop iteration completes
```go
type IterationCompletedData struct {
    IterationIndex int         `json:"iteration_index"`     // Zero-based index of the completed iteration
    Item           interface{} `json:"item"`                // The item that was processed
    ItemKey        string      `json:"item_key,omitempty"`  // Key for the item (if iterating over a map)
    Result         interface{} `json:"result"`              // Result produced by this iteration
}
```
*Occurs*: When each iteration in loops completes successfully.
*Purpose*: Records iteration results for debugging and state tracking.

#### 8. Control Flow Events

**`signal_received`** - Emitted when external signals are received during execution
*Purpose*: Tracks external control signals that may affect execution flow.

**`version_decision`** - Emitted when workflow versioning decisions are made
*Purpose*: Records workflow version selection for compatibility tracking.

### Event Structure

All events share a common structure with typed data:

```go
type ExecutionEvent struct {
    ID          string             `json:"id"`          // Unique event identifier
    ExecutionID string             `json:"execution_id"` // Execution this event belongs to
    Sequence    int64              `json:"sequence"`     // Sequential event number within execution
    Timestamp   time.Time          `json:"timestamp"`    // When the event occurred
    EventType   ExecutionEventType `json:"event_type"`   // Type of event (see above)
    Path        string             `json:"path,omitempty"` // Execution path identifier (if applicable)
    Step        string             `json:"step,omitempty"` // Step name (if applicable)
    
    // Legacy field for backward compatibility
    Data map[string]interface{} `json:"data,omitempty"`
    
    // Strongly typed event data
    TypedData ExecutionEventData `json:"typed_data,omitempty"`
}
```

### Event Flow Example

A typical workflow execution generates events in this sequence:

1. `execution_started` - Workflow begins
2. `path_started` - Initial execution path starts
3. `step_started` - First step begins
4. `operation_started` - LLM call initiated
5. `operation_completed` - LLM call completes
6. `step_completed` - First step completes
7. `condition_evaluated` - Conditional logic evaluated
8. `path_branched` - Multiple paths created based on condition
9. `path_started` - New parallel paths begin
10. `step_started` - Steps in parallel paths begin
11. ... (more step and operation events)
12. `path_completed` - Parallel paths complete
13. `execution_completed` - Workflow completes

This event stream provides complete visibility into execution flow and enables perfect replay of workflow behavior.

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
