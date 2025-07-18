# Deterministic Workflow Execution in Dive

## Overview

This document describes the design and implementation of deterministic workflow execution in Dive, using a checkpoint-based architecture. The key insight is separating deterministic workflow logic from non-deterministic operations, enabling reliable state recovery, operation tracking, and debugging.

## The Problem

When a workflow execution crashes or needs to be resumed, we need to reconstruct its exact state. There are two approaches:

1. **State Snapshots**: Save the current state periodically
   - Benefit: Simple implementation, fast recovery
   - Challenge: Must ensure state consistency

2. **Event Sourcing**: Record immutable events and replay them
   - Benefit: Complete history, deterministic replay, perfect audit trail
   - Challenge: Complex implementation, requires separating deterministic and non-deterministic code

**Dive's Approach**: We use **checkpoint-based state persistence** combined with **operation tracking**. This provides the reliability benefits of event sourcing while maintaining the simplicity of state snapshots.

## Core Concepts

### Deterministic vs Non-Deterministic

**Deterministic** (Can be replayed identically):
- Workflow orchestration logic
- Condition evaluation
- Variable assignments
- Path management

**Non-Deterministic** (Must be tracked for debugging):
- LLM/Agent responses
- External API calls
- File/document operations
- Time/random value access
- User interactions

### The Operation Boundary

In Dive, we introduce **Operations** as the boundary between deterministic workflow logic and non-deterministic side effects:

```
Workflow Logic (Deterministic)
    ↓
[Operation Boundary]
    ↓
Side Effects (Non-Deterministic)
```

## Architecture

### Operation Definition

```go
type Operation struct {
    ID         OperationID            // Unique operation identifier
    Type       string                 // Operation type (e.g., "agent_response")
    StepName   string                 // Associated workflow step
    PathID     string                 // Execution path identifier
    Parameters map[string]interface{} // Operation parameters
}
```

### Checkpoint-Based Execution

Instead of event sourcing, Dive uses a simplified checkpoint-based approach:

1. **Operation Tracking**: All non-deterministic operations are logged with their parameters, results, and timing
2. **Automatic Checkpointing**: Complete execution state is saved after every operation
3. **State Recovery**: Executions automatically resume from the latest checkpoint
4. **Debugging Support**: Operation logs provide detailed execution history

### Execution State Management

```go
type Execution struct {
    // Core state
    id          string
    workflow    *workflow.Workflow
    status      ExecutionStatus
    inputs      map[string]interface{}
    outputs     map[string]interface{}
    
    // Checkpoint-based persistence
    operationLogger OperationLogger
    checkpointer    ExecutionCheckpointer
    
    // Runtime state
    state       *WorkflowState
    paths       map[string]*PathState
    totalUsage  llm.Usage
}
```

## Implementation Patterns

### Operation Execution Pattern

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

### State Management Pattern

```go
// Deterministic state updates
state.Set("variable_name", value)

// State is automatically included in checkpoints
checkpoint := &ExecutionCheckpoint{
    State: state.Copy(), // Complete state snapshot
    // ... other execution data
}
```

### Recovery Pattern

```go
// Automatic recovery on execution start
execution, err := NewExecution(opts)
if err != nil {
    return err
}

// Automatically attempts to load from latest checkpoint
err = execution.Run(ctx)
```

## Benefits of Checkpoint-Based Approach

### Simplicity
- **Easy to Understand**: Straightforward state snapshots instead of complex event replay
- **Simple Debugging**: Direct state inspection without event reconstruction
- **Easier Testing**: Test against actual state rather than event sequences

### Reliability
- **Automatic Recovery**: Executions resume seamlessly from checkpoints
- **State Consistency**: Complete state captured in each checkpoint
- **Operation Tracking**: Detailed operation logs for debugging and analysis

### Performance
- **Fast Recovery**: Direct state loading instead of event replay
- **Predictable Memory**: State size is bounded and predictable
- **Simple Storage**: Straightforward checkpoint serialization

## Migration from Event Sourcing

The system previously used event sourcing but has migrated to the checkpoint-based approach for the following reasons:

1. **Reduced Complexity**: Checkpoint-based execution is significantly simpler to implement and maintain
2. **Sufficient Recovery**: Checkpoints provide adequate recovery capabilities for workflow execution
3. **Better Debugging**: Direct state inspection is more intuitive than event replay
4. **Operational Simplicity**: Checkpoint management is more straightforward than event store management

### Preserved Benefits
- **Operation Tracking**: Non-deterministic operations are still logged for debugging
- **State Recovery**: Executions can still be resumed from failure points
- **Debugging Support**: Comprehensive logging provides execution visibility
- **Deterministic Design**: Clear separation between deterministic and non-deterministic code

### Simplified Implementation
- **No Event Replay**: Direct state loading eliminates replay complexity
- **Unified State Model**: Single state representation instead of event sequences
- **Streamlined Recovery**: Simple checkpoint loading instead of event processing

## Best Practices

### Operation Design
1. **Keep Operations Atomic**: Each operation should be a single, complete unit of work
2. **Parameterize Properly**: Include all necessary parameters for operation identification
3. **Handle Errors Gracefully**: Operations should be designed to handle and report errors cleanly

### State Management
1. **Minimize State Size**: Keep workflow state focused on essential data
2. **Use Immutable Data**: Prefer immutable data structures where possible
3. **Clear Variable Names**: Use descriptive names for state variables

### Debugging
1. **Use Operation Logs**: Leverage operation logs for debugging non-deterministic behavior
2. **Inspect Checkpoints**: Use checkpoint data to understand execution state
3. **Monitor Performance**: Track operation timing and resource usage

## Future Considerations

While the current checkpoint-based approach meets our needs, future enhancements could include:

- **Configurable Checkpoint Frequency**: Allow tuning checkpoint frequency based on workload
- **Checkpoint Compression**: Compress large state for storage efficiency
- **Selective State Snapshots**: Checkpoint only changed state portions
- **Distributed Checkpointing**: Support for distributed execution environments
