package environment

import (
	"fmt"

	"github.com/diveagents/dive/workflow"
)

// ChangeDetector analyzes workflow and input changes for replay compatibility
type ChangeDetector interface {
	DetectWorkflowChanges(events []*ExecutionEvent, currentWorkflow *workflow.Workflow) (*WorkflowCompatibility, error)
	DetectInputChanges(oldInputs, newInputs map[string]interface{}) ([]string, error)
	FindAffectedSteps(changedInputs []string, workflow *workflow.Workflow) ([]string, error)
}

// BasicChangeDetector provides a simple implementation of ChangeDetector
type BasicChangeDetector struct {
	hasher WorkflowHasher
}

// NewBasicChangeDetector creates a new basic change detector
func NewBasicChangeDetector(hasher WorkflowHasher) *BasicChangeDetector {
	return &BasicChangeDetector{hasher: hasher}
}

// DetectWorkflowChanges analyzes event history to detect workflow changes
func (d *BasicChangeDetector) DetectWorkflowChanges(events []*ExecutionEvent, currentWorkflow *workflow.Workflow) (*WorkflowCompatibility, error) {
	// Find the workflow hash from execution start event
	var originalHash string
	for _, event := range events {
		if event.EventType == EventExecutionStarted {
			if hash, ok := event.Data["workflow_hash"].(string); ok {
				originalHash = hash
				break
			}
		}
	}

	if originalHash == "" {
		return &WorkflowCompatibility{
			IsCompatible:   false,
			ChangesSummary: "No original workflow hash found in event history",
		}, nil
	}

	// Get current workflow hash
	currentHash, err := d.hasher.HashWorkflow(currentWorkflow)
	if err != nil {
		return nil, fmt.Errorf("failed to hash current workflow: %w", err)
	}

	// For now, we can't reconstruct the original workflow from just the hash
	// In a full implementation, we'd store workflow definitions alongside events
	if originalHash == currentHash {
		return &WorkflowCompatibility{
			IsCompatible:   true,
			ChangesSummary: "Workflow unchanged",
		}, nil
	}

	return &WorkflowCompatibility{
		IsCompatible:   false,
		ChangesSummary: "Workflow hash changed (detailed comparison not available)",
	}, nil
}

// DetectInputChanges compares old and new inputs to find changes
func (d *BasicChangeDetector) DetectInputChanges(oldInputs, newInputs map[string]interface{}) ([]string, error) {
	var changed []string

	// Check for removed inputs
	for key := range oldInputs {
		if _, exists := newInputs[key]; !exists {
			changed = append(changed, key+" (removed)")
		}
	}

	// Check for new inputs
	for key := range newInputs {
		if _, exists := oldInputs[key]; !exists {
			changed = append(changed, key+" (added)")
		}
	}

	// Check for changed values
	for key, newValue := range newInputs {
		if oldValue, exists := oldInputs[key]; exists {
			// Simple comparison - in production this should handle complex types better
			if fmt.Sprintf("%v", oldValue) != fmt.Sprintf("%v", newValue) {
				changed = append(changed, key+" (value changed)")
			}
		}
	}

	return changed, nil
}

// FindAffectedSteps determines which steps might be affected by input changes
func (d *BasicChangeDetector) FindAffectedSteps(changedInputs []string, workflow *workflow.Workflow) ([]string, error) {
	// Conservative approach - assume all steps might be affected by any input change
	// In a more sophisticated implementation, this would analyze step dependencies
	if len(changedInputs) == 0 {
		return []string{}, nil
	}

	var allSteps []string
	for _, step := range workflow.Steps() {
		allSteps = append(allSteps, step.Name())
	}

	return allSteps, nil
}
