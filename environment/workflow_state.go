package environment

import (
	"sync"
)

// WorkflowState provides controlled access to workflow state
type WorkflowState struct {
	executionID string
	values      map[string]interface{}
	mutex       sync.RWMutex
}

// NewWorkflowState creates a new workflow state instance
func NewWorkflowState(executionID string, recorder interface{}) *WorkflowState {
	return &WorkflowState{
		executionID: executionID,
		values:      make(map[string]interface{}),
	}
}

// Set sets a value in the workflow state
func (s *WorkflowState) Set(key string, value interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.values[key] = value
	return nil
}

// Get retrieves a value from the workflow state
func (s *WorkflowState) Get(key string) (interface{}, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	value, exists := s.values[key]
	return value, exists
}

// Delete removes a key from the workflow state
func (s *WorkflowState) Delete(key string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.values, key)
	return nil
}

// Keys returns all keys in the workflow state
func (s *WorkflowState) Keys() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		keys = append(keys, key)
	}
	return keys
}

// Copy creates a copy of the current state values
func (s *WorkflowState) Copy() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	copy := make(map[string]interface{}, len(s.values))
	for k, v := range s.values {
		copy[k] = v
	}
	return copy
}

// LoadFromMap loads state from a map (used during recovery)
func (s *WorkflowState) LoadFromMap(values map[string]interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.values = make(map[string]interface{}, len(values))
	for k, v := range values {
		s.values[k] = v
	}
}

// RisorStateObject wraps WorkflowState for Risor script access
type RisorStateObject struct {
	state *WorkflowState
}

// NewRisorStateObject creates a new Risor-compatible state object
func NewRisorStateObject(state *WorkflowState) *RisorStateObject {
	return &RisorStateObject{state: state}
}

// Set sets a value in the state (Risor-compatible)
func (r *RisorStateObject) Set(key string, value interface{}) error {
	return r.state.Set(key, value)
}

// Get gets a value from the state (Risor-compatible)
func (r *RisorStateObject) Get(key string) interface{} {
	value, _ := r.state.Get(key)
	return value
}

// Delete deletes a key from the state (Risor-compatible)
func (r *RisorStateObject) Delete(key string) error {
	return r.state.Delete(key)
}

// Has checks if a key exists in the state (Risor-compatible)
func (r *RisorStateObject) Has(key string) bool {
	_, exists := r.state.Get(key)
	return exists
}
