# ✅ COMPLETED: Operations-Based Workflow Execution Migration

> **Status**: This migration has been **completed**. The system now uses a simplified checkpoint-based execution model instead of the full event sourcing approach described in this document.
>
> **Current Implementation**: The environment package implements operation tracking with checkpoint-based state persistence, providing the reliability benefits described here while maintaining simplicity.
>
> **See**: `environment/README.md` and `environment/execution.go` for the current implementation.

---

# Product Requirements Document: Operations-Based Workflow Execution

## Executive Summary

This document outlines the plan to transform Dive's workflow execution engine from its current hybrid approach to a fully deterministic, operation-based system inspired by Temporal's architecture. The migration will enable reliable replay, recovery, and debugging while maintaining backward compatibility with existing workflows.

## Background

### Current State
- **Mixed Determinism**: Workflow execution mixes deterministic logic with non-deterministic operations
- **Limited Replay**: Event recording exists but doesn't capture all side effects
- **State Management**: Script globals and path states are managed inconsistently
- **Recovery Limitations**: Cannot reliably resume failed executions from arbitrary points

### Target State
- **Full Determinism**: Clear separation between deterministic workflow logic and non-deterministic operations
- **Complete Event Sourcing**: All operations recorded and replayable
- **Explicit State Management**: State mutations through controlled interfaces
- **Robust Recovery**: Ability to resume, retry, or skip failed steps with full state reconstruction

## Core Concepts

### Operations
Operations represent the boundary between deterministic workflow logic and non-deterministic side effects:

```go
type Operation struct {
    ID         OperationID    // Deterministic, unique identifier
    Type       string         // "agent_response", "action", "state_mutation", etc.
    StepName   string         // Workflow step that triggered this
    Parameters interface{}    // Input parameters
}
```

### State Management
State mutations will be explicit through a controlled interface:

```go
type WorkflowState interface {
    Set(key string, value interface{}) error
    Get(key string) (interface{}, bool)
    Delete(key string) error
    Keys() []string
}
```

### Event Types
New event types for comprehensive tracking:

```go
const (
    // Operation events
    EventOperationStarted   = "operation_started"
    EventOperationCompleted = "operation_completed"
    EventOperationFailed    = "operation_failed"
    
    // State events
    EventStateSet           = "state_set"
    EventStateDeleted       = "state_deleted"
    
    // Deterministic access events
    EventTimeAccessed       = "time_accessed"
    EventRandomGenerated    = "random_generated"
    EventVersionDecision    = "version_decision"
)
```

## Migration Phases

### Phase 1: Core Operation Framework (Weeks 1-4)

#### Objectives
- Implement the Operation abstraction and execution framework
- Add operation-aware event recording and replay
- Create state management interfaces
- Maintain backward compatibility

#### Deliverables

1. **Operation Types and Interfaces**
   ```go
   // operation.go
   type OperationID string
   
   type Operation struct {
       ID         OperationID
       Type       string
       StepName   string
       PathID     string
       Parameters interface{}
   }
   
   type OperationResult struct {
       OperationID OperationID
       Result      interface{}
       Error       error
       ExecutedAt  time.Time
   }
   
   type OperationExecutor interface {
       ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error)
       FindOperationResult(opID OperationID) (*OperationResult, bool)
   }
   ```

2. **State Management System**
   ```go
   // workflow_state.go
   type WorkflowState struct {
       executionID string
       recorder    ExecutionRecorder
       values      map[string]interface{}
       mutex       sync.RWMutex
   }
   
   func (s *WorkflowState) Set(key string, value interface{}) error {
       s.mutex.Lock()
       defer s.mutex.Unlock()
       
       // Record state mutation
       s.recorder.RecordCustomEvent(EventStateSet, "", "", map[string]interface{}{
           "key":   key,
           "value": value,
       })
       
       s.values[key] = value
       return nil
   }
   ```

3. **Risor Integration**
   ```go
   // Expose state object to Risor scripts
   type RisorStateObject struct {
       state *WorkflowState
   }
   
   func (r *RisorStateObject) Set(key string, value interface{}) error {
       return r.state.Set(key, value)
   }
   
   // In script globals:
   scriptGlobals["state"] = NewRisorStateObject(workflowState)
   ```

4. **Operation Recording in EventBasedExecution**
   ```go
   func (e *EventBasedExecution) ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error) {
       // Generate deterministic operation ID
       op.ID = e.generateOperationID(op)
       
       if e.replayMode {
           // During replay: return recorded result
           if result, found := e.operationResults[op.ID]; found {
               return result.Result, result.Error
           }
           panic(fmt.Sprintf("Operation %s not found during replay", op.ID))
       }
       
       // Record operation start
       e.recordEvent(EventOperationStarted, op.PathID, op.StepName, map[string]interface{}{
           "operation_id":   string(op.ID),
           "operation_type": op.Type,
           "parameters":     op.Parameters,
       })
       
       // Execute the operation
       result, err := fn()
       
       // Record operation completion
       e.recordEvent(EventOperationCompleted, op.PathID, op.StepName, map[string]interface{}{
           "operation_id": string(op.ID),
           "result":       result,
           "error":        err,
       })
       
       // Cache for potential replay
       e.operationResults[op.ID] = &OperationResult{
           OperationID: op.ID,
           Result:      result,
           Error:       err,
           ExecutedAt:  time.Now(),
       }
       
       return result, err
   }
   ```

#### Testing Strategy
- Unit tests for Operation execution and recording
- Integration tests for state management
- Replay tests ensuring deterministic behavior
- Backward compatibility tests with existing workflows

### Phase 2: Refactor Non-Deterministic Operations (Weeks 5-8)

#### Objectives
- Wrap all LLM/Agent calls in operations
- Convert action executions to use operations
- Handle time/random access deterministically
- Migrate existing workflows incrementally

#### Deliverables

1. **Agent Response Operations**
   ```go
   func (e *EventBasedExecution) handlePromptStep(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
       // Deterministic: prepare prompt
       promptTemplate := step.Prompt()
       prompt, err := e.evalString(ctx, promptTemplate)
       if err != nil {
           return nil, err
       }
       
       // Deterministic: prepare content
       content, err := e.prepareStepContent(ctx, step, agent)
       if err != nil {
           return nil, err
       }
       
       // Operation: LLM call
       op := Operation{
           Type:     "agent_response",
           StepName: step.Name(),
           PathID:   e.currentPathID,
           Parameters: map[string]interface{}{
               "agent":   agent.Name(),
               "prompt":  prompt,
               "content": content,
           },
       }
       
       responseInterface, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
           return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(content...)))
       })
       
       if err != nil {
           return nil, err
       }
       
       // Deterministic: process result
       response := responseInterface.(*dive.Response)
       return &dive.StepResult{
           Content: response.OutputText(),
           Usage:   response.Usage(),
       }, nil
   }
   ```

2. **Action Execution Operations**
   ```go
   func (e *EventBasedExecution) handleActionStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
       actionName := step.Action()
       
       // Deterministic: prepare parameters
       params, err := e.evaluateActionParams(ctx, step.Parameters())
       if err != nil {
           return nil, err
       }
       
       // Operation: execute action
       op := Operation{
           Type:     "action_execution",
           StepName: step.Name(),
           PathID:   e.currentPathID,
           Parameters: map[string]interface{}{
               "action_name": actionName,
               "params":      params,
           },
       }
       
       result, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
           action, exists := e.environment.GetAction(actionName)
           if !exists {
               return nil, fmt.Errorf("action %q not found", actionName)
           }
           return action.Execute(ctx, params)
       })
       
       if err != nil {
           return nil, err
       }
       
       return &dive.StepResult{
           Content: fmt.Sprintf("%v", result),
       }, nil
   }
   ```

3. **Deterministic Time and Random Access**
   ```go
   type DeterministicRuntime struct {
       execution *EventBasedExecution
   }
   
   func (d *DeterministicRuntime) Now() time.Time {
       op := Operation{
           Type: "time_access",
           Parameters: map[string]interface{}{
               "access_type": "now",
           },
       }
       
       timeInterface, _ := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
           return time.Now(), nil
       })
       
       return timeInterface.(time.Time)
   }
   
   func (d *DeterministicRuntime) Random() float64 {
       op := Operation{
           Type: "random_generation",
           Parameters: map[string]interface{}{
               "type": "float64",
           },
       }
       
       randInterface, _ := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
           return rand.Float64(), nil
       })
       
       return randInterface.(float64)
   }
   ```

4. **Script Evaluation with State Tracking**
   ```go
   func (e *EventBasedExecution) evaluateRisorCondition(ctx context.Context, codeStr string, pathID string) (bool, error) {
       // Create isolated state for this evaluation
       evalState := &WorkflowState{
           executionID: e.id,
           recorder:    e.recorder,
           values:      make(map[string]interface{}),
       }
       
       // Copy current state
       e.mutex.RLock()
       for k, v := range e.scriptGlobals {
           evalState.values[k] = v
       }
       e.mutex.RUnlock()
       
       // Add state object
       globals := map[string]interface{}{
           "state": NewRisorStateObject(evalState),
       }
       for k, v := range evalState.values {
           globals[k] = v
       }
       
       // Compile and evaluate
       compiledCode, err := compileScript(ctx, codeStr, globals)
       if err != nil {
           return false, err
       }
       
       result, err := risor.EvalCode(ctx, compiledCode, risor.WithGlobals(globals))
       if err != nil {
           return false, err
       }
       
       // Apply state changes back to global state
       for k, v := range evalState.values {
           if existing, exists := e.scriptGlobals[k]; !exists || existing != v {
               e.state.Set(k, v)
           }
       }
       
       return result.IsTruthy(), nil
   }
   ```

#### Migration Tools
- Workflow analyzer to identify non-deterministic operations
- Automated refactoring tool for common patterns
- Compatibility checker to ensure workflows still function
- Performance benchmarking tools

### Phase 3: Enhanced Replay and Workflow Versioning (Weeks 9-12)

#### Objectives
- Implement full replay capabilities with validation
- Add workflow versioning support
- Create debugging and inspection tools
- Optimize performance for large event histories

#### Deliverables

1. **Enhanced Replay System**
   ```go
   type ReplayController struct {
       execution   *EventBasedExecution
       events      []*ExecutionEvent
       position    int
       breakpoints map[string]bool
   }
   
   func (r *ReplayController) StepForward() error {
       if r.position >= len(r.events) {
           return fmt.Errorf("no more events")
       }
       
       event := r.events[r.position]
       if err := r.execution.applyEvent(event); err != nil {
           return fmt.Errorf("replay divergence at event %d: %w", r.position, err)
       }
       
       r.position++
       return nil
   }
   
   func (r *ReplayController) RunToBreakpoint() error {
       for r.position < len(r.events) {
           event := r.events[r.position]
           
           if r.breakpoints[event.StepName] {
               return nil
           }
           
           if err := r.StepForward(); err != nil {
               return err
           }
       }
       return nil
   }
   ```

2. **Workflow Versioning**
   ```go
   func (e *EventBasedExecution) GetVersion(changeID string, minVersion, maxVersion int) int {
       op := Operation{
           Type: "version_decision",
           Parameters: map[string]interface{}{
               "change_id":   changeID,
               "min_version": minVersion,
               "max_version": maxVersion,
           },
       }
       
       versionInterface, _ := e.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
           // For new executions, use max version
           // For replays, this will return the recorded decision
           return maxVersion, nil
       })
       
       return versionInterface.(int)
   }
   
   // Usage in workflow steps
   func (e *EventBasedExecution) executeWithVersioning(ctx context.Context, step *workflow.Step) error {
       version := e.GetVersion("add_validation_v2", 1, 2)
       
       if version >= 2 {
           // New logic with validation
           if err := e.validateInputs(step); err != nil {
               return err
           }
       }
       
       return e.executeStepCore(ctx, step)
   }
   ```

3. **Debugging Tools**
   ```go
   type ExecutionInspector struct {
       execution *EventBasedExecution
       store     ExecutionEventStore
   }
   
   func (i *ExecutionInspector) GetOperationHistory() []OperationSummary {
       var operations []OperationSummary
       
       events, _ := i.store.GetEventHistory(context.Background(), i.execution.ID())
       for _, event := range events {
           if event.EventType == EventOperationCompleted {
               operations = append(operations, OperationSummary{
                   ID:        event.Data["operation_id"].(string),
                   Type:      event.Data["operation_type"].(string),
                   StepName:  event.StepName,
                   Timestamp: event.Timestamp,
                   Duration:  event.Data["duration"].(time.Duration),
               })
           }
       }
       
       return operations
   }
   
   func (i *ExecutionInspector) ValidateDeterminism() error {
       // Run execution twice with same inputs
       // Compare operation sequences
       // Flag any divergence
       return nil
   }
   ```

4. **Performance Optimizations**
   ```go
   type EventCache struct {
       executionID string
       events      []*ExecutionEvent
       operations  map[OperationID]*OperationResult
       checkpoints map[int64]*ExecutionCheckpoint
   }
   
   type ExecutionCheckpoint struct {
       Sequence      int64
       State         map[string]interface{}
       OperationIDs  []OperationID
       ActivePaths   []string
   }
   
   func (c *EventCache) CreateCheckpoint(sequence int64, state *EventBasedExecution) {
       checkpoint := &ExecutionCheckpoint{
           Sequence:     sequence,
           State:        c.snapshotState(state),
           OperationIDs: c.getOperationsSince(c.lastCheckpoint()),
           ActivePaths:  c.getActivePaths(state),
       }
       c.checkpoints[sequence] = checkpoint
   }
   ```

## Success Criteria

### Phase 1
- [ ] Operation framework implemented and tested
- [ ] State management system functional
- [ ] Existing workflows continue to work
- [ ] Event recording captures all operations

### Phase 2
- [ ] All non-deterministic operations wrapped
- [ ] 100% of existing workflows migrated
- [ ] Replay produces identical results
- [ ] Performance impact < 10%

### Phase 3
- [ ] Full replay debugging available
- [ ] Workflow versioning in production
- [ ] Recovery success rate > 99%
- [ ] Event history queryable and analyzable

## Risk Mitigation

### Technical Risks
1. **Performance Impact**
   - Mitigation: Event batching, async recording, checkpointing
   
2. **Storage Requirements**
   - Mitigation: Event compression, retention policies, archival strategies
   
3. **Backward Compatibility**
   - Mitigation: Gradual migration, compatibility layer, extensive testing

### Operational Risks
1. **Migration Complexity**
   - Mitigation: Automated tools, phased rollout, rollback procedures
   
2. **Learning Curve**
   - Mitigation: Documentation, examples, training materials

## Timeline

### Phase 1: Weeks 1-4
- Week 1-2: Core operation framework
- Week 3: State management and Risor integration
- Week 4: Testing and documentation

### Phase 2: Weeks 5-8
- Week 5-6: Refactor agent and action operations
- Week 7: Migrate time/random access
- Week 8: Migration tools and validation

### Phase 3: Weeks 9-12
- Week 9-10: Replay system and debugging tools
- Week 11: Workflow versioning
- Week 12: Performance optimization and final testing

## Appendix

### Example: Migrated Workflow Execution

```go
// Before: Mixed deterministic/non-deterministic
func (e *EventBasedExecution) handlePromptStep(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
    prompt, _ := e.evalString(ctx, step.Prompt())
    
    // Direct LLM call - non-deterministic!
    response, err := agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(prompt)))
    if err != nil {
        return nil, err
    }
    
    return &dive.StepResult{Content: response.OutputText()}, nil
}

// After: Clear separation with operations
func (e *EventBasedExecution) handlePromptStep(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
    // Deterministic preparation
    prompt, _ := e.evalString(ctx, step.Prompt())
    
    // Non-deterministic operation
    op := Operation{
        Type:     "agent_response",
        StepName: step.Name(),
        PathID:   e.currentPathID,
        Parameters: map[string]interface{}{
            "agent":  agent.Name(),
            "prompt": prompt,
        },
    }
    
    responseInterface, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
        return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(prompt)))
    })
    
    if err != nil {
        return nil, err
    }
    
    // Deterministic processing
    response := responseInterface.(*dive.Response)
    return &dive.StepResult{Content: response.OutputText()}, nil
}
```

### Example: State Management in Risor

```javascript
// Before: Direct mutation of globals
inputs.processed = true
results.push(response)

// After: Explicit state management
state.set("inputs.processed", true)
var results = state.get("results") || []
results.push(response)
state.set("results", results)
``` 