package environment

import (
	"time"

	"github.com/deepnoodle-ai/dive/workflow"
)

// PathStatus represents the current state of an execution path
type PathStatus string

const (
	PathStatusPending   PathStatus = "pending"
	PathStatusRunning   PathStatus = "running"
	PathStatusCompleted PathStatus = "completed"
	PathStatusFailed    PathStatus = "failed"
)

// PathState tracks the state of an execution path. This struct is designed to
// be fully JSON serializable.
type PathState struct {
	ID           string            `json:"id"`
	Status       PathStatus        `json:"status"`
	CurrentStep  string            `json:"current_step"`
	StartTime    time.Time         `json:"start_time,omitzero"`
	EndTime      time.Time         `json:"end_time,omitzero"`
	ErrorMessage string            `json:"error_message,omitempty"`
	StepOutputs  map[string]string `json:"step_outputs"`
}

// Copy returns a shallow copy of the path state.
func (p *PathState) Copy() *PathState {
	// Create a copy of step outputs map
	stepOutputs := make(map[string]string)
	for k, v := range p.StepOutputs {
		stepOutputs[k] = v
	}
	return &PathState{
		ID:           p.ID,
		Status:       p.Status,
		CurrentStep:  p.CurrentStep,
		StartTime:    p.StartTime,
		EndTime:      p.EndTime,
		ErrorMessage: p.ErrorMessage,
		StepOutputs:  stepOutputs,
	}
}

type executionPath struct {
	id          string
	currentStep *workflow.Step
}

type pathUpdate struct {
	pathID     string
	stepName   string
	stepOutput string
	newPaths   []*executionPath
	err        error
	isDone     bool // true if this path should be removed from active paths
}
