package environment

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash"
	"sort"
	"time"
)

// OperationID is a deterministic, unique identifier for operations
type OperationID string

// Operation represents a non-deterministic operation that can be recorded and replayed
type Operation struct {
	ID         OperationID            // Deterministic, unique identifier
	Type       string                 // "agent_response", "action", "state_mutation", etc.
	StepName   string                 // Workflow step that triggered this
	PathID     string                 // Execution path identifier
	Parameters map[string]interface{} // Input parameters
}

// OperationResult captures the result of an operation execution
type OperationResult struct {
	OperationID OperationID // The operation that produced this result
	Result      interface{} // The actual result
	Error       error       // Error if operation failed
	ExecutedAt  time.Time   // When the operation was executed
}

// OperationExecutor provides the interface for executing operations with automatic recording/replay
type OperationExecutor interface {
	// ExecuteOperation runs an operation with automatic recording/replay behavior
	ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error)

	// FindOperationResult looks up a previously executed operation result
	FindOperationResult(opID OperationID) (*OperationResult, bool)
}

// hashMapToWriter writes a map[string]interface{} to a hash.Hash in deterministic order
func hashMapToWriter(h hash.Hash, m map[string]interface{}) {
	if m == nil {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(fmt.Sprintf("%v", m[k])))
	}
}

// HashMapToString creates a deterministic SHA256 hash string from a map[string]interface{}
func HashMapToString(m map[string]interface{}) string {
	hash := sha256.New()
	hashMapToWriter(hash, m)
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// generateOperationID creates a deterministic operation ID based on operation properties
func (op *Operation) GenerateID() OperationID {
	// Create a deterministic hash from operation properties
	hash := sha256.New()
	hash.Write([]byte(op.Type))
	hash.Write([]byte(op.StepName))
	hash.Write([]byte(op.PathID))

	// Include parameters in hash in deterministic order
	hashMapToWriter(hash, op.Parameters)

	return OperationID(fmt.Sprintf("op_%x", hash.Sum(nil)[:16]))
}

// NewOperation creates a new operation with generated ID
func NewOperation(opType, stepName, pathID string, parameters map[string]interface{}) Operation {
	op := Operation{
		Type:       opType,
		StepName:   stepName,
		PathID:     pathID,
		Parameters: parameters,
	}
	op.ID = op.GenerateID()
	return op
}
