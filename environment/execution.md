# EventBasedExecution Internal State Analysis

## Overview

The `EventBasedExecution` manages the complete state of a workflow execution, including path tracking, event recording, and synchronization across multiple concurrent execution paths. This document describes the internal state maintained during execution and the interactions between state components and concurrent execution patterns.

## Core State Components

### 1. Execution Metadata
```go
type EventBasedExecution struct {
    id            string                    // Unique execution identifier
    environment   *Environment              // Runtime environment reference
    workflow      *workflow.Workflow        // Workflow definition being executed
    status        Status                    // Current execution status
    startTime     time.Time                 // Execution start timestamp
    endTime       time.Time                 // Execution completion timestamp
    inputs        map[string]interface{}    // Initial workflow inputs
    outputs       map[string]interface{}    // Final workflow outputs
    err           error                     // Top-level execution error
}
```

**State Transitions:**
- `status`: `StatusPending` → `StatusRunning` → `StatusCompleted|StatusFailed`
- `startTime`: Set during `Run()` initialization
- `endTime`: Set when execution completes or fails
- `err`: Set if execution fails at top level

### 2. Path Management State
```go
paths         map[string]*PathState        // Active execution paths
```

Each `PathState` tracks:
```go
type PathState struct {
    ID          string                     // Unique path identifier
    Status      PathStatus                 // Path execution status
    CurrentStep *workflow.Step             // Currently executing step
    StartTime   time.Time                  // Path start time
    EndTime     time.Time                  // Path completion time
    Error       error                      // Path-specific error
    StepOutputs map[string]string          // Step name → output mapping
}
```

**Path Lifecycle:**
1. **Creation**: New paths created via `addPath()` when execution starts or branches
2. **Updates**: Path state updated via `updatePathState()` as steps execute
3. **Branching**: Multiple new paths created when conditional logic splits execution
4. **Completion**: Paths removed from active set when they complete or fail

### 3. Script Execution Context
```go
scriptGlobals map[string]any              // Global variables for script evaluation
```

**Contains:**
- Initial workflow inputs (`inputs`)
- Step outputs stored in variables (via `store` parameter)
- Built-in Risor language functions
- Document repository interface (if available)
- Each-block iteration variables
- Custom action results

**State Evolution:**
- Variables added as steps complete and store results
- Each-block variables updated during iteration
- Condition evaluation results cached
- Action execution results stored

### 4. Event Recording System
```go
eventStore    ExecutionEventStore         // Persistent event storage
eventBuffer   []*ExecutionEvent           // Buffered events for batch writes  
eventSequence int64                       // Atomic event sequence counter
replayMode    bool                        // Flag to disable event recording during replay
batchSize     int                         // Event buffer flush threshold
```

**Event Types Recorded:**
- `EventExecutionStarted`: Workflow execution begins
- `EventPathStarted`: New execution path created
- `EventStepStarted`: Step execution begins
- `EventStepCompleted`: Step execution completes successfully
- `EventStepFailed`: Step execution fails
- `EventPathBranched`: Path splits into multiple paths
- `EventPathCompleted`: Path execution completes
- `EventPathFailed`: Path execution fails
- `EventExecutionCompleted`: Workflow execution completes
- `EventExecutionFailed`: Workflow execution fails

### 5. Synchronization Primitives
```go
mutex         sync.RWMutex                // Protects shared state access
doneWg        sync.WaitGroup              // Signals execution completion
bufferMutex   sync.Mutex                  // Protects event buffer
```

## Concurrency Architecture

### Main Execution Flow
```
Run() [Main Goroutine]
├── Initialize state
├── Start initial path goroutine
└── Event processing loop
    ├── Receive path updates
    ├── Update path states  
    ├── Start new path goroutines
    └── Check for completion
```

### Path Execution Pattern
Each execution path runs in its own goroutine:
```
runPath() [Per-Path Goroutine]
├── Record EventPathStarted
├── Step execution loop
│   ├── Record EventStepStarted
│   ├── Execute step (prompt/action)
│   ├── Record EventStepCompleted/Failed
│   └── Handle path branching
├── Send pathUpdate to main goroutine
└── Record EventPathCompleted
```

### Communication Patterns
- **Path Updates**: Goroutines communicate via `pathUpdate` channel
- **Event Recording**: Thread-safe via `bufferMutex` and atomic sequence counter
- **State Access**: Protected by `mutex` (read/write lock)

## State Persistence Strategy

### Snapshot System
Periodic snapshots capture execution state:
```go
type ExecutionSnapshot struct {
    ID           string                     // Execution ID
    WorkflowName string                     // Workflow being executed
    Status       string                     // Current status
    StartTime    time.Time                  // Execution start
    EndTime      time.Time                  // Execution end
    LastEventSeq int64                      // Last recorded event sequence
    Inputs       map[string]interface{}     // Initial inputs
    Outputs      map[string]interface{}     // Final outputs
    Error        string                     // Error message if failed
}
```

### Event Sourcing
Complete execution history preserved as event stream:
- Events written in batches to improve performance
- Sequence numbers ensure ordering
- Events contain enough data to reconstruct state

## State Reconstruction Requirements

To successfully replay execution state from events, the following must be captured:

### 1. Path State Reconstruction
- Path creation and branching events
- Step execution order and results
- Path completion status and timing
- Error conditions and failure points

### 2. Script Context Reconstruction
- Variable assignments from step outputs
- Each-block iteration state
- Conditional evaluation results
- Action execution results

### 3. Execution Flow Reconstruction
- Step execution dependencies
- Branching conditions and results
- Path synchronization points
- Error propagation paths

### 4. Timing and Ordering
- Event sequence preservation
- Concurrent path execution timing
- Step execution duration tracking
- Path lifecycle timing

## Critical State Interactions

### 1. Path Branching
When a step has multiple conditional next steps:
```go
// Current path may split into multiple new paths
newPaths := e.handlePathBranching(...)
if len(newPaths) > 1 {
    // Record branching event
    e.recordEvent(EventPathBranched, ...)
    // Start new goroutines for each new path
    for _, newPath := range newPaths {
        go e.runPath(ctx, newPath, updates)
    }
}
```

### 2. Variable Scoping
Script variables are shared across all paths:
```go
// Step output stored in global scope
if varName := step.Store(); varName != "" {
    e.scriptGlobals[varName] = result.Content
}
```

### 3. Error Propagation
Errors can occur at multiple levels:
- Step-level errors (recorded but may not fail path)
- Path-level errors (fail entire path)
- Execution-level errors (fail entire workflow)

## State Consistency Guarantees

1. **Event Ordering**: Atomic sequence counter ensures global event ordering
2. **Path Isolation**: Each path maintains independent step outputs
3. **Script Context**: Global variables synchronized via mutex
4. **Event Persistence**: Buffered writes with failure handling
5. **State Snapshots**: Periodic snapshots provide recovery points

## Implementation Considerations for Replay

1. **Event Completeness**: All state changes must be captured in events
2. **Deterministic Replay**: Event replay must produce identical state
3. **Concurrency Handling**: Path creation and completion must be properly sequenced
4. **Variable Management**: Script globals must be reconstructed in correct order
5. **Error Recovery**: Failed replays must be detectable and reportable