package environment

import (
	"context"
	"fmt"
	"sync"
)

// ScriptStateManager manages script execution state and variable tracking
type ScriptStateManager interface {
	// SetVariable sets a variable and emits an event
	SetVariable(ctx context.Context, name string, value interface{}) error

	// GetVariable retrieves a variable value
	GetVariable(name string) (interface{}, bool)

	// EvaluateTemplate evaluates a template and records the result
	EvaluateTemplate(ctx context.Context, template string) (string, error)

	// EvaluateCondition evaluates a condition and records the result
	EvaluateCondition(ctx context.Context, condition string) (bool, error)

	// Snapshot returns a copy of all variables
	Snapshot() map[string]interface{}

	// RestoreFromSnapshot restores state from a snapshot
	RestoreFromSnapshot(snapshot map[string]interface{})
}

// EventEmitter is used by ScriptStateManager to emit state change events
type EventEmitter interface {
	EmitEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{})
}

// TrackedScriptStateManager implements ScriptStateManager with event tracking
type TrackedScriptStateManager struct {
	globals   map[string]interface{}
	emitter   EventEmitter
	evaluator interface{} // Will be the actual eval implementation
	mutex     sync.RWMutex
}

// NewTrackedScriptStateManager creates a new tracked script state manager
func NewTrackedScriptStateManager(emitter EventEmitter, initialGlobals map[string]interface{}) *TrackedScriptStateManager {
	globals := make(map[string]interface{})
	for k, v := range initialGlobals {
		globals[k] = v
	}

	return &TrackedScriptStateManager{
		globals: globals,
		emitter: emitter,
	}
}

// SetVariable sets a variable and emits a state change event
func (m *TrackedScriptStateManager) SetVariable(ctx context.Context, name string, value interface{}) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	oldValue, existed := m.globals[name]
	m.globals[name] = value

	// Emit variable change event
	m.emitter.EmitEvent(EventVariableChanged, "", "", map[string]interface{}{
		"variable_name": name,
		"old_value":     oldValue,
		"new_value":     value,
		"existed":       existed,
	})

	return nil
}

// GetVariable retrieves a variable value
func (m *TrackedScriptStateManager) GetVariable(name string) (interface{}, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	value, exists := m.globals[name]
	return value, exists
}

// EvaluateTemplate evaluates a template and records the result
func (m *TrackedScriptStateManager) EvaluateTemplate(ctx context.Context, template string) (string, error) {
	// TODO: Implement actual template evaluation
	// For now, just emit the event
	result := fmt.Sprintf("evaluated: %s", template)

	m.emitter.EmitEvent(EventTemplateEvaluated, "", "", map[string]interface{}{
		"template": template,
		"result":   result,
	})

	return result, nil
}

// EvaluateCondition evaluates a condition and records the result
func (m *TrackedScriptStateManager) EvaluateCondition(ctx context.Context, condition string) (bool, error) {
	// TODO: Implement actual condition evaluation
	// For now, just emit the event
	result := true

	m.emitter.EmitEvent(EventConditionEvaluated, "", "", map[string]interface{}{
		"condition": condition,
		"result":    result,
	})

	return result, nil
}

// Snapshot returns a copy of all variables
func (m *TrackedScriptStateManager) Snapshot() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	snapshot := make(map[string]interface{})
	for k, v := range m.globals {
		snapshot[k] = v
	}
	return snapshot
}

// RestoreFromSnapshot restores state from a snapshot
func (m *TrackedScriptStateManager) RestoreFromSnapshot(snapshot map[string]interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.globals = make(map[string]interface{})
	for k, v := range snapshot {
		m.globals[k] = v
	}
}

// Additional event types for script state tracking
const (
	EventVariableChanged    ExecutionEventType = "variable_changed"
	EventTemplateEvaluated  ExecutionEventType = "template_evaluated"
	EventConditionEvaluated ExecutionEventType = "condition_evaluated"
	EventIterationStarted   ExecutionEventType = "iteration_started"
	EventIterationCompleted ExecutionEventType = "iteration_completed"
)
