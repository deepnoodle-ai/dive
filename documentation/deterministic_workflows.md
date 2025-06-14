# Deterministic Workflow Execution in Dive

## Overview

This document describes the design and implementation of deterministic workflow execution in Dive, inspired by Temporal's architecture. The key insight is separating deterministic workflow logic from non-deterministic operations, enabling reliable replay, recovery, and debugging.

## The Problem

When a workflow execution crashes or needs to be resumed, we need to reconstruct its exact state. There are two approaches:

1. **State Snapshots**: Save the current state periodically
   - Problem: Partial updates, inconsistent views, no audit trail
   - Risk: State corruption during crashes

2. **Event Sourcing**: Record immutable events and replay them
   - Benefit: Complete history, deterministic replay, perfect audit trail
   - Challenge: Requires separating deterministic and non-deterministic code

## Core Concepts

### Deterministic vs Non-Deterministic

**Deterministic** (Can be replayed identically):
- Workflow orchestration logic
- Condition evaluation
- Variable assignments
- Path management

**Non-Deterministic** (Must be recorded once):
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
// OperationID uniquely identifies an operation for replay
type OperationID string

// Operation represents a non-deterministic operation that produces side effects
type Operation struct {
    ID         OperationID    // Unique identifier
    Type       string         // "agent_response", "action", "external_call"
    StepName   string         // Which workflow step triggered this
    Parameters interface{}    // Input parameters
}

// OperationResult captures the result of an operation execution
type OperationResult struct {
    OperationID OperationID
    Result      interface{}
    Error       error
    ExecutedAt  time.Time
}
```

### Event Types

New event types for operation tracking:

```go
const (
    // Operation lifecycle events
    EventOperationStarted   ExecutionEventType = "operation_started"
    EventOperationCompleted ExecutionEventType = "operation_completed"
    EventOperationFailed    ExecutionEventType = "operation_failed"
    
    // Deterministic value access events
    EventTimeAccessed       ExecutionEventType = "time_accessed"
    EventRandomGenerated    ExecutionEventType = "random_generated"
    EventVariableSet        ExecutionEventType = "variable_set"
)
```

### Operation Executor

The execution engine provides operation execution with recording/replay:

```go
type OperationExecutor interface {
    // ExecuteOperation runs an operation with automatic recording/replay
    ExecuteOperation(op Operation, fn func() (interface{}, error)) (interface{}, error)
}
```

Implementation in `EventBasedExecution`:

```go
func (e *EventBasedExecution) ExecuteOperation(op Operation, fn func() (interface{}, error)) (interface{}, error) {
    if e.replayMode {
        // During replay: return recorded result
        event := e.findOperationResult(op.ID)
        if event == nil {
            panic(fmt.Sprintf("Operation %s not found in replay", op.ID))
        }
        return event.Data["result"], event.Data["error"].(error)
    }
    
    // First execution: run and record
    e.recordEvent(EventOperationStarted, "", op.StepName, map[string]interface{}{
        "operation_id":   string(op.ID),
        "operation_type": op.Type,
        "parameters":     op.Parameters,
    })
    
    result, err := fn()
    
    e.recordEvent(EventOperationCompleted, "", op.StepName, map[string]interface{}{
        "operation_id": string(op.ID),
        "result":       result,
        "error":        err,
    })
    
    return result, err
}
```

## Implementation Examples

### Agent Response (LLM Call)

```go
func (e *EventBasedExecution) handlePromptStep(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
    // Deterministic: prepare prompt
    prompt, err := e.evalString(ctx, step.Prompt())
    if err != nil {
        return nil, err
    }
    
    // Operation: LLM call
    op := Operation{
        ID:       OperationID(fmt.Sprintf("agent:%s:%d", step.Name(), e.eventSequence)),
        Type:     "agent_response",
        StepName: step.Name(),
        Parameters: map[string]interface{}{
            "prompt": prompt,
            "agent":  agent.Name(),
        },
    }
    
    responseInterface, err := e.ExecuteOperation(op, func() (interface{}, error) {
        return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(prompt)))
    })
    
    if err != nil {
        return nil, err
    }
    
    // Deterministic: process result
    response := responseInterface.(*dive.Response)
    return &dive.StepResult{
        Content: response.OutputText(),
    }, nil
}
```

### Action Execution

```go
func (e *EventBasedExecution) handleActionStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
    // Deterministic: prepare parameters
    params := e.evaluateParams(step.Parameters())
    
    // Operation: execute action
    op := Operation{
        ID:       OperationID(fmt.Sprintf("action:%s:%s:%d", step.Action(), step.Name(), e.eventSequence)),
        Type:     "action",
        StepName: step.Name(),
        Parameters: map[string]interface{}{
            "action": step.Action(),
            "params": params,
        },
    }
    
    result, err := e.ExecuteOperation(op, func() (interface{}, error) {
        action, _ := e.environment.GetAction(step.Action())
        return action.Execute(ctx, params)
    })
    
    if err != nil {
        return nil, err
    }
    
    return &dive.StepResult{Content: fmt.Sprintf("%v", result)}, nil
}
```

### Deterministic Time Access

```go
func (e *EventBasedExecution) Now() time.Time {
    if e.replayMode {
        // Return recorded time
        event := e.findTimeAccessEvent()
        return event.Data["time"].(time.Time)
    }
    
    // Record current time
    now := time.Now()
    e.recordEvent(EventTimeAccessed, "", "", map[string]interface{}{
        "time": now,
    })
    return now
}
```

## Workflow Versioning

Operations enable safe workflow evolution through version decisions:

```go
func (e *EventBasedExecution) GetVersion(changeID string, minVersion, maxVersion int) int {
    // Check if version decision already recorded
    for _, event := range e.eventHistory {
        if event.EventType == EventVersionDecision {
            if event.Data["change_id"] == changeID {
                return event.Data["version"].(int)
            }
        }
    }
    
    // Record new version decision
    version := maxVersion // Use latest for new executions
    e.recordEvent(EventVersionDecision, "", "", map[string]interface{}{
        "change_id":   changeID,
        "version":     version,
        "min_version": minVersion,
        "max_version": maxVersion,
    })
    
    return version
}

// Usage in workflow
func (e *EventBasedExecution) executeStepWithVersioning(step *workflow.Step) error {
    version := e.GetVersion("add_fraud_check_v2", 1, 2)
    
    if version >= 2 {
        // New version: includes fraud check
        if err := e.executeFraudCheck(step); err != nil {
            return err
        }
    }
    // Version 1: skip fraud check
    
    return e.executeMainLogic(step)
}
```

## Benefits

1. **Deterministic Replay**: Workflow logic replays identically every time
2. **Failure Recovery**: Resume from any point using event history
3. **Perfect Debugging**: Step through execution deterministically
4. **Testability**: Test workflows with mocked operation results
5. **Observability**: Complete audit trail of all decisions and operations
6. **Safe Evolution**: Workflows can evolve without breaking running instances

## Implementation Phases

### Phase 1: Core Operation Framework (Week 1-2)
- [ ] Implement Operation and OperationResult types
- [ ] Add ExecuteOperation to EventBasedExecution
- [ ] Create operation event types
- [ ] Update event recording/replay

### Phase 2: Refactor Existing Code (Week 3-4)
- [ ] Wrap agent.CreateResponse calls in operations
- [ ] Wrap action.Execute calls in operations
- [ ] Add deterministic time/random access
- [ ] Update script variable mutations to record events

### Phase 3: Enhanced Replay (Week 5-6)
- [ ] Implement operation-aware replay
- [ ] Add replay validation
- [ ] Create debugging tools
- [ ] Performance optimization

### Phase 4: Workflow Versioning (Week 7-8)
- [ ] Implement GetVersion mechanism
- [ ] Add version decision events
- [ ] Create migration patterns
- [ ] Document versioning best practices

### Phase 5: Testing & Validation (Week 9-10)
- [ ] Comprehensive test suite
- [ ] Determinism validators
- [ ] Performance benchmarks
- [ ] Documentation and examples

## Design Principles

1. **Never call non-deterministic functions directly in workflow code**
2. **Always wrap external calls in operations**
3. **Record all operation results in event history**
4. **During replay, use recorded results instead of re-executing**
5. **Keep workflow logic pure and side-effect free**
6. **Make operation IDs deterministic and unique**
7. **Fail fast if replay diverges from recorded history**

## Testing Strategy

### Determinism Tests

```go
func TestDeterministicReplay(t *testing.T) {
    workflow := loadWorkflow("test_workflow")
    inputs := map[string]interface{}{"value": 42}
    
    // Run twice with same inputs
    events1 := runExecution(workflow, inputs)
    events2 := runExecution(workflow, inputs)
    
    // Operation results will differ, but the sequence must be identical
    require.Equal(t, extractOperationSequence(events1), extractOperationSequence(events2))
}
```

### Replay Tests

```go
func TestReplayRecovery(t *testing.T) {
    // Run partial execution
    events := runExecutionUntilStep(workflow, inputs, "step3")
    
    // Replay from history
    execution := replayFromEvents(events)
    
    // Continue execution
    execution.Resume()
    
    // Verify completion
    require.Equal(t, "completed", execution.Status())
}
```

## Migration Path

For existing Dive workflows:

1. **Audit**: Identify all non-deterministic operations
2. **Wrap**: Convert to use ExecuteOperation
3. **Test**: Verify deterministic replay
4. **Deploy**: Enable event-based execution
5. **Monitor**: Track replay success rates

## Future Enhancements

- **Operation Batching**: Execute multiple operations in parallel
- **Operation Caching**: Cache frequently used operation results
- **Distributed Execution**: Run operations on different workers
- **Operation Timeouts**: Configurable timeouts per operation type
- **Operation Retries**: Built-in retry logic with exponential backoff
