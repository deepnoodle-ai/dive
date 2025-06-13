# Execution V2: Deterministic Operation-Based Workflow Execution

This directory contains the new deterministic execution engine (`execution_v2.go`) that implements the operation-based approach described in the PRDs. This is a parallel implementation to the existing `EventBasedExecution`.

## Key Features

### 🔄 Deterministic Execution
- Clear separation between deterministic workflow logic and non-deterministic operations
- All side effects (LLM calls, actions, time access) wrapped in Operations
- Reproducible execution through operation recording and replay

### 📝 Event-Driven Recording
- Complete audit trail of all execution events
- Operation lifecycle tracking (started, completed, failed)
- State mutations recorded through WorkflowState

### 🔧 Simple Architecture
- Minimal but functional implementation
- Supports prompt and action steps
- Template evaluation for dynamic workflows
- Single-path execution (no branching yet)

## Core Components

### Execution Struct
```go
type Execution struct {
    id               string
    workflow         *workflow.Workflow
    environment      *Environment
    status           ExecutionStatus
    operationResults map[OperationID]*OperationResult
    state            *WorkflowState
    recorder         ExecutionRecorder
    // ... other fields
}
```

### Operation Types
- **`agent_response`**: LLM/Agent calls
- **`action_execution`**: Environment action execution
- **`test_operation`**: For testing operation replay

### Supported Step Types
- **`prompt`**: Execute agent with a prompt template
- **`action`**: Execute environment action with parameters

## Usage Example

```go
// Create execution
execution, err := environment.NewExecution(environment.ExecutionV2Options{
    Workflow:    workflow,
    Environment: env,
    Inputs: map[string]interface{}{
        "name": "World",
    },
    EventStore:  eventStore,
    Logger:      logger,
    ReplayMode:  false,
})

// Run workflow
err = execution.Run(ctx)
```

## Event Recording

The execution records these event types:
- `execution_started` / `execution_completed` / `execution_failed`
- `path_started` / `path_completed` 
- `step_started` / `step_completed` / `step_failed`
- `operation_started` / `operation_completed` / `operation_failed`
- `state_set` / `state_deleted` (via WorkflowState)

## Template Evaluation

Simple template evaluation supports `${variable}` syntax:
```yaml
prompt: "Say hello to ${name}"
parameters:
  Message: "Result: ${previous_step}"
```

Variables are resolved from the WorkflowState.

## Testing

Run tests with:
```bash
go test -v ./environment -run TestExecution
```

Tests cover:
- Basic workflow execution with prompt and action steps
- Operation recording and replay
- Event store integration
- Template evaluation

## Example Program

See `examples/programs/execution_v2_example/` for a complete working example.

## Future Enhancements

This is a minimal implementation. Future enhancements will include:
- ✅ Path branching and parallel execution
- ✅ Advanced condition evaluation
- ✅ Enhanced replay with debugging tools
- ✅ Workflow versioning support
- ✅ Performance optimizations
- ✅ Integration with existing EventBasedExecution features

## Architecture Benefits

1. **Predictable Replay**: Operations enable exact replay of execution
2. **Clear Separation**: Deterministic logic separated from side effects
3. **Complete Audit Trail**: Every operation and state change recorded
4. **Testable**: Easy to test with mock operations
5. **Debuggable**: Step-by-step execution analysis possible

## Migration Path

This implementation runs in parallel with `EventBasedExecution`. The plan is to:
1. ✅ Validate the approach with this minimal implementation
2. 📋 Gradually add missing features (branching, conditions, etc.)
3. 📋 Migrate existing workflows to use the new approach
4. 📋 Eventually replace EventBasedExecution

The new approach maintains backward compatibility while providing the foundation for reliable, deterministic workflow execution. 