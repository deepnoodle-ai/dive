package environment

import (
	"context"
	"crypto/sha256"
	"fmt"
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

// generateOperationID creates a deterministic operation ID based on operation properties
func (op *Operation) GenerateID() OperationID {
	// Create a deterministic hash from operation properties
	hash := sha256.New()
	hash.Write([]byte(op.Type))
	hash.Write([]byte(op.StepName))
	hash.Write([]byte(op.PathID))

	// Include parameters in hash in deterministic order
	if op.Parameters != nil {
		// Sort keys to ensure deterministic order
		keys := make([]string, 0, len(op.Parameters))
		for k := range op.Parameters {
			keys = append(keys, k)
		}
		// Sort keys for deterministic iteration
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[i] > keys[j] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}

		for _, k := range keys {
			hash.Write([]byte(k))
			hash.Write([]byte(fmt.Sprintf("%v", op.Parameters[k])))
		}
	}

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
