# YAML Workflow Engine Design Decisions

## Overview

This document captures the key architectural decisions for designing a YAML-defined workflow engine that uses LLM interactions as activities, with a **checkpoint-based execution model** for reliability and simplicity.

## Core Architecture

### Checkpoint-Based Design with Operation Tracking

**Decision**: Use checkpoint-based state persistence combined with operation tracking for non-deterministic activities (LLM calls, external operations).

**Rationale**: 
- Provides fault tolerance and recovery capabilities
- Much simpler than event sourcing while maintaining reliability
- Clear separation between orchestration logic and external interactions
- Easy to understand and debug

**Implementation**:
- Each workflow step execution is tracked and logged
- Complete execution state is checkpointed after every operation
- Activities (LLM calls, actions) are tracked for debugging and analysis
- Automatic recovery from checkpoints on execution restart

## Script Usage Categories

### Three Distinct Categories

**1. Conditional Scripts** - Must be deterministic
```yaml
- Name: Check Progress
  Condition: |
    len(state.completed_items) >= state.target_count && 
    state.quality_score > 0.8
```

**2. Template Scripts** - Must be deterministic  
```yaml
- Name: Generate Prompt
  Prompt: |
    Analyze ${format_item(state.current_item)}
    Previous results: ${build_context(state.results)}
```

**3. Activity Scripts** - Can be non-deterministic
```yaml
- Name: Fetch External Data
  Type: script
  Script: |
    response = http.get("https://api.example.com/data")
    return process_response(response)
  Store: external_data
```

**Rationale**: 
- Conditional and template scripts affect workflow decisions and must be replayable
- Activity scripts are isolated and their results are stored in checkpoints
- Clear boundaries prevent accidental non-determinism in critical paths

### Script Enforcement Strategy

**Implementation**: Restrict available functions based on script context
- Conditional/Template: Only deterministic functions (no time, random, HTTP, etc.)
- Activity: All functions allowed including non-deterministic ones

## State Management

### Checkpoint-Based Store Pattern

**Decision**: Use `Store:` syntax that updates workflow state with automatic checkpointing

**User Interface**:
```yaml
- Name: Analyze Language
  Agent: Analyst  
  Prompt: "Analyze ${state.language}"
  Store: language_analysis  # Simple variable assignment
```

**Internal Implementation**:
```go
// State update tracked in checkpoint
state.Set("language_analysis", "Python is a high-level...")

// Automatic checkpoint after operation
checkpoint := &ExecutionCheckpoint{
    State: state.Copy(),
    // ... complete execution state
}
checkpointer.SaveCheckpoint(ctx, checkpoint)
```

**Benefits**:
- Users get simple mental model (set variable)
- Engine gets reliability benefits (recovery, audit trail, fault tolerance)
- Clean abstraction hides complexity while preserving power

### Script Activity State Updates

**Decision**: Script activities use the same `Store:` pattern as other steps

**Rationale**:
- Consistent user experience across all step types
- Maintains activity pattern (input → output → store)
- State updates are automatically checkpointed

### Operation Tracking

**Decision**: Track all non-deterministic operations for debugging and analysis

**Implementation**:
```go
type OperationLogEntry struct {
    ID            string
    ExecutionID   string
    StepName      string
    OperationType string
    Parameters    map[string]interface{}
    Result        interface{}
    StartTime     time.Time
    Duration      time.Duration
    Error         string
}
```

**Benefits**:
- Complete visibility into execution behavior
- Detailed debugging information
- Performance monitoring
- Audit trail for compliance

## Execution Model

### Checkpoint-Based Recovery

**Decision**: Save complete execution state after every operation

**Rationale**:
- Simple and reliable recovery mechanism
- No complex event replay logic needed
- Fast recovery with direct state loading
- Predictable memory and storage requirements

**Implementation**:
```go
type ExecutionCheckpoint struct {
    ID           string
    ExecutionID  string
    Status       string
    Inputs       map[string]interface{}
    Outputs      map[string]interface{}
    State        map[string]interface{}  // Complete workflow state
    PathStates   map[string]*PathState   // Parallel execution paths
    TotalUsage   *llm.Usage             // LLM usage tracking
    CheckpointAt time.Time
}
```

### Path Management

**Decision**: Support parallel execution paths with individual state tracking

**Implementation**:
- Each path maintains its own state and execution context
- Paths can branch based on conditional logic
- All paths are tracked in the checkpoint
- Failed paths don't affect other paths

### Deterministic Design

**Decision**: Maintain clear separation between deterministic and non-deterministic code

**Deterministic Components**:
- Workflow orchestration logic
- Condition evaluation
- State variable assignments
- Path management decisions

**Non-Deterministic Components** (Tracked via Operations):
- LLM/Agent responses
- External API calls
- File I/O operations
- Time-dependent operations
- Random value generation

## Benefits of Current Architecture

### Simplicity
- **Easy to understand**: Checkpoint-based model is intuitive
- **Simple debugging**: Direct state inspection
- **Straightforward testing**: Test against actual state snapshots

### Reliability
- **Automatic recovery**: Executions resume from latest checkpoint
- **Complete state capture**: No partial or inconsistent state
- **Operation tracking**: Full visibility into non-deterministic operations

### Performance
- **Fast recovery**: Direct state loading vs. event replay
- **Bounded memory**: Predictable state size
- **Efficient storage**: Simple checkpoint serialization

### Operational Benefits
- **Easy monitoring**: Clear execution state visibility
- **Simple backup**: Checkpoint-based backup strategies
- **Debugging support**: Rich operation logs and state snapshots

## Design Principles

### State Management
1. **Explicit State Updates**: All state changes go through the state interface
2. **Automatic Checkpointing**: State persisted after every operation
3. **Immutable Operations**: Operations are logged but not replayed

### Operation Tracking
1. **Track All Side Effects**: Every non-deterministic operation is logged
2. **Complete Parameter Capture**: All operation inputs are recorded
3. **Result and Error Tracking**: Both success and failure cases are logged

### Recovery Model
1. **Checkpoint-First**: Always attempt recovery from latest checkpoint
2. **Graceful Degradation**: Handle checkpoint corruption gracefully
3. **Operation Awareness**: Use operation logs for debugging recovery issues

## Future Enhancements

### Enhanced Checkpointing
- **Configurable Frequency**: Allow tuning checkpoint frequency
- **Compression**: Compress large state for storage efficiency
- **Incremental Checkpoints**: Only save changed state portions

### Advanced Operation Tracking
- **Operation Metrics**: Track performance and resource usage
- **Operation Grouping**: Batch related operations for efficiency
- **Retry Logic**: Built-in retry mechanisms for failed operations

### Distributed Execution
- **Distributed Checkpoints**: Support for distributed execution environments
- **Path Partitioning**: Execute different paths on different workers
- **Shared State Management**: Coordinated state updates across workers

This checkpoint-based architecture provides a robust foundation for reliable workflow execution while maintaining simplicity and ease of use.