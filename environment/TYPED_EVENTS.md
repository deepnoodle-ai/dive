# Typed Events System

## Overview

The Dive framework uses **typed execution events** throughout the system. All events are created as strongly-typed structs and only converted to maps for storage serialization, providing complete type safety and validation.

## Why Typed Events?

### Before: Generic Events
```go
// Old way: using untyped map data
recorder.RecordEvent(EventStepCompleted, pathID, stepName, map[string]interface{}{
    "output": "Hello World",
    "stored_variable": "greeting",
    "some_typo": "value", // No validation, typos go unnoticed
})
```

### After: Typed Events
```go
// New way: using typed data structures
data := &StepCompletedData{
    Output:         "Hello World",
    StoredVariable: "greeting",
    // SomeTypo: "value", // Compilation error - field doesn't exist!
}
recorder.RecordEvent(pathID, stepName, data)

// Or even simpler with convenience methods:
recorder.RecordStepCompleted(pathID, stepName, "Hello World", "greeting")
```

## Key Benefits

1. **Complete Type Safety**: All events are strongly typed from creation to storage
2. **Automatic Validation**: Built-in validation ensures data integrity
3. **Better IDE Support**: Auto-completion, refactoring, and documentation
4. **Self-Documenting**: Event structure is clear from type definitions
5. **Performance**: No runtime type conversions during event processing
6. **Consistency**: Single approach throughout the entire system

## Event Types and Data Structures

### Execution Lifecycle Events

#### ExecutionStartedData
```go
type ExecutionStartedData struct {
    WorkflowName string                 `json:"workflow_name"`
    Inputs       map[string]interface{} `json:"inputs"`
}
```

#### ExecutionCompletedData
```go
type ExecutionCompletedData struct {
    Outputs map[string]interface{} `json:"outputs"`
}
```

#### ExecutionFailedData
```go
type ExecutionFailedData struct {
    Error string `json:"error"`
}
```

### Path Management Events

#### PathStartedData
```go
type PathStartedData struct {
    CurrentStep string `json:"current_step"`
}
```

#### PathCompletedData
```go
type PathCompletedData struct {
    FinalStep string `json:"final_step"`
}
```

#### PathFailedData
```go
type PathFailedData struct {
    Error string `json:"error"`
}
```

#### PathBranchedData
```go
type PathBranchedData struct {
    NewPaths []PathBranchInfo `json:"new_paths"`
}

type PathBranchInfo struct {
    ID             string `json:"id"`
    CurrentStep    string `json:"current_step"`
    InheritOutputs bool   `json:"inherit_outputs"`
}
```

### Step Execution Events

#### StepStartedData
```go
type StepStartedData struct {
    StepType   string                 `json:"step_type"`
    StepParams map[string]interface{} `json:"step_params"`
}
```

#### StepCompletedData
```go
type StepCompletedData struct {
    Output         string `json:"output"`
    StoredVariable string `json:"stored_variable,omitempty"`
}
```

#### StepFailedData
```go
type StepFailedData struct {
    Error string `json:"error"`
}
```

### Operation Events

#### OperationStartedData
```go
type OperationStartedData struct {
    OperationID   string                 `json:"operation_id"`
    OperationType string                 `json:"operation_type"`
    Parameters    map[string]interface{} `json:"parameters"`
}
```

#### OperationCompletedData
```go
type OperationCompletedData struct {
    OperationID   string        `json:"operation_id"`
    OperationType string        `json:"operation_type"`
    Duration      time.Duration `json:"duration"`
    Result        interface{}   `json:"result"`
}
```

#### OperationFailedData
```go
type OperationFailedData struct {
    OperationID   string        `json:"operation_id"`
    OperationType string        `json:"operation_type"`
    Duration      time.Duration `json:"duration"`
    Error         string        `json:"error"`
}
```

### State Management Events

#### StateMutatedData
```go
type StateMutatedData struct {
    Mutations []StateMutation `json:"mutations"`
}

type StateMutation struct {
    Type  StateMutationType `json:"type"`  // "set" or "delete"
    Key   string            `json:"key,omitempty"`
    Value interface{}       `json:"value,omitempty"`
}
```

## Usage Guide

### 1. Recording Events

#### Direct Typed Event Recording
```go
// Create typed event data
data := &OperationCompletedData{
    OperationID:   "op-123",
    OperationType: "agent_response",
    Duration:      2 * time.Second,
    Result:        "Hello, world!",
}

// Record the event
recorder.RecordEvent("path-1", "", data)
```

#### Using Convenience Methods (Recommended)
```go
// Convenience methods for common events
recorder.RecordExecutionStarted("my-workflow", inputs)
recorder.RecordStepStarted("path-1", "step-1", "prompt", params)
recorder.RecordStepCompleted("path-1", "step-1", "output", "variable")
recorder.RecordOperationCompleted("path-1", "op-123", "agent_response", duration, result)
```

### 2. Reading Typed Events

#### Type-Safe Event Processing
```go
for _, event := range events {
    typedData, err := event.GetTypedData()
    if err != nil {
        log.Printf("Error getting typed data: %v", err)
        continue
    }
    
    switch data := typedData.(type) {
    case *OperationCompletedData:
        log.Printf("Operation %s completed in %v", data.OperationID, data.Duration)
        
    case *StepFailedData:
        log.Printf("Step failed: %s", data.Error)
        
    case *PathBranchedData:
        log.Printf("Path branched into %d new paths", len(data.NewPaths))
        for _, path := range data.NewPaths {
            log.Printf("  - Path %s: %s", path.ID, path.CurrentStep)
        }
        
    default:
        log.Printf("Event type: %T", data)
    }
}
```

### 3. Storage Compatibility

#### Automatic Serialization
Events are automatically converted to/from maps for storage:

```go
// Internal: Typed event is created
data := &StepCompletedData{
    Output:         "Hello",
    StoredVariable: "greeting",
}

// Internal: Stored as map in database
// {
//   "output": "Hello",
//   "stored_variable": "greeting"
// }

// Internal: Retrieved as typed event
retrievedData, _ := event.GetTypedData()
stepData := retrievedData.(*StepCompletedData)
fmt.Printf("Output: %s", stepData.Output) // "Hello"
```

## Validation

All typed event data includes automatic validation:

```go
// This will fail validation
invalidData := &OperationStartedData{
    // Missing required OperationID and OperationType
    Parameters: map[string]interface{}{"param": "value"},
}

if err := invalidData.Validate(); err != nil {
    // err will be: "operation_id is required"
}

// Events are validated when recorded
recorder.RecordEvent("path-1", "", invalidData) // Returns validation error
```

## Custom Event Types

To create custom event types, implement the `ExecutionEventData` interface:

```go
// Define custom event data
type CustomAnalysisData struct {
    AnalysisType string                 `json:"analysis_type"`
    Results      map[string]interface{} `json:"results"`
    Confidence   float64                `json:"confidence"`
}

// Implement ExecutionEventData interface
func (d *CustomAnalysisData) EventType() ExecutionEventType {
    return "custom_analysis" // Define this constant
}

func (d *CustomAnalysisData) Validate() error {
    if d.AnalysisType == "" {
        return fmt.Errorf("analysis_type is required")
    }
    if d.Confidence < 0 || d.Confidence > 1 {
        return fmt.Errorf("confidence must be between 0 and 1")
    }
    return nil
}

// Usage
customData := &CustomAnalysisData{
    AnalysisType: "sentiment",
    Results:      map[string]interface{}{"sentiment": "positive"},
    Confidence:   0.92,
}
recorder.RecordEvent("path-1", "analysis-step", customData)
```

## Testing

The typed event system includes comprehensive test coverage:

```bash
# Run all typed event tests
go test -v ./environment -run "TestTypedEvent"

# Run compatibility tests
go test -v ./environment -run "TestBackwardCompatibility"

# Run validation tests
go test -v ./environment -run "TestValidation"
```

## Best Practices

1. **Use Typed Events for New Code**: Always prefer typed events for new implementations
2. **Leverage Convenience Methods**: Use the provided convenience methods when available
3. **Validate Early**: Take advantage of automatic validation to catch errors early
4. **Document Custom Types**: Well-document any custom event types you create
5. **Gradual Migration**: Migrate existing code gradually to avoid breaking changes

## Future Enhancements

- **JSON Schema Generation**: Automatic schema generation for typed events
- **Event Serialization**: Enhanced serialization with type information
- **IDE Extensions**: Custom IDE extensions for better event development experience
- **Performance Optimizations**: Further performance improvements for high-volume scenarios

---

The typed events system provides a solid foundation for building robust, maintainable workflow execution systems while preserving the flexibility and backward compatibility that existing Dive applications depend on. 