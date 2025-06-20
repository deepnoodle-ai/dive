package environment

import (
	"time"

	"github.com/diveagents/dive/workflow"
)

// PathStatus represents the current state of an execution path
type PathStatus string

const (
	PathStatusPending   PathStatus = "pending"
	PathStatusRunning   PathStatus = "running"
	PathStatusCompleted PathStatus = "completed"
	PathStatusFailed    PathStatus = "failed"
)

// PathState tracks the state of an execution path
type PathState struct {
	ID          string
	Status      PathStatus
	CurrentStep *workflow.Step
	StartTime   time.Time
	EndTime     time.Time
	Error       error
	StepOutputs map[string]string
}

// Copy returns a shallow copy of the path state.
func (p *PathState) Copy() *PathState {
	return &PathState{
		ID:          p.ID,
		Status:      p.Status,
		CurrentStep: p.CurrentStep,
		StartTime:   p.StartTime,
		EndTime:     p.EndTime,
		Error:       p.Error,
		StepOutputs: p.StepOutputs,
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
