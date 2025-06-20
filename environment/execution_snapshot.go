package environment

import "time"

// ExecutionSnapshot represents the complete state of an execution
type ExecutionSnapshot struct {
	ID           string    `json:"id"`
	WorkflowName string    `json:"workflow_name"`
	WorkflowPath string    `json:"workflow_path"`
	WorkflowHash string    `json:"workflow_hash"`
	InputsHash   string    `json:"inputs_hash"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastEventSeq int64     `json:"last_event_seq"`

	// Serialized data (for bootstrapping)
	WorkflowData []byte                 `json:"workflow_data"`
	Inputs       map[string]interface{} `json:"inputs"`
	Outputs      map[string]interface{} `json:"outputs"`
	Error        string                 `json:"error,omitempty"`
}
