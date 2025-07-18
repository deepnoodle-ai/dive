package environment

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/diveagents/dive/workflow"
)

// WorkflowCompatibility represents the compatibility status between two workflow versions
type WorkflowCompatibility struct {
	IsCompatible      bool     `json:"is_compatible"`
	IncompatibleSteps []string `json:"incompatible_steps"`
	ChangedInputs     []string `json:"changed_inputs"`
	ChangesSummary    string   `json:"changes_summary"`
}

// WorkflowHasher provides methods for generating workflow hashes
type WorkflowHasher interface {
	HashWorkflow(workflow *workflow.Workflow) (string, error)
	HashInputs(inputs map[string]interface{}) (string, error)
	CompareWorkflows(oldHash, newHash string, oldWorkflow, newWorkflow *workflow.Workflow) (*WorkflowCompatibility, error)
}

// BasicWorkflowHasher provides a simple implementation of WorkflowHasher
type BasicWorkflowHasher struct{}

// NewBasicWorkflowHasher creates a new basic workflow hasher
func NewBasicWorkflowHasher() *BasicWorkflowHasher {
	return &BasicWorkflowHasher{}
}

// HashWorkflow generates a deterministic hash of a workflow definition
func (h *BasicWorkflowHasher) HashWorkflow(workflow *workflow.Workflow) (string, error) {
	// Create a normalized representation of the workflow for hashing
	workflowData := map[string]interface{}{
		"name":        workflow.Name(),
		"description": workflow.Description(),
		"inputs":      h.normalizeInputs(workflow.Inputs()),
		"output":      h.normalizeOutput(workflow.Output()),
		"steps":       h.normalizeSteps(workflow.Steps()),
		"graph_edges": h.normalizeGraph(workflow.Graph()),
	}

	// Convert to JSON for consistent hashing
	jsonData, err := json.Marshal(workflowData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow data: %w", err)
	}

	// Generate hash
	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

// HashInputs generates a hash of input parameters
func (h *BasicWorkflowHasher) HashInputs(inputs map[string]interface{}) (string, error) {
	// Normalize inputs for consistent hashing
	normalizedInputs := make(map[string]interface{})
	for key, value := range inputs {
		normalizedInputs[key] = value
	}

	jsonData, err := json.Marshal(normalizedInputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inputs: %w", err)
	}

	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

// CompareWorkflows compares two workflows and determines compatibility
func (h *BasicWorkflowHasher) CompareWorkflows(oldHash, newHash string, oldWorkflow, newWorkflow *workflow.Workflow) (*WorkflowCompatibility, error) {
	if oldHash == newHash {
		return &WorkflowCompatibility{
			IsCompatible:   true,
			ChangesSummary: "No changes detected",
		}, nil
	}

	// Analyze specific changes
	incompatibleSteps := h.findIncompatibleSteps(oldWorkflow, newWorkflow)
	changedInputs := h.findChangedInputs(oldWorkflow, newWorkflow)

	isCompatible := len(incompatibleSteps) == 0

	summary := fmt.Sprintf("Workflow changed: %d incompatible steps, %d input changes",
		len(incompatibleSteps), len(changedInputs))

	return &WorkflowCompatibility{
		IsCompatible:      isCompatible,
		IncompatibleSteps: incompatibleSteps,
		ChangedInputs:     changedInputs,
		ChangesSummary:    summary,
	}, nil
}

// normalizeInputs creates a normalized representation of workflow inputs
func (h *BasicWorkflowHasher) normalizeInputs(inputs []*workflow.Input) []map[string]interface{} {
	var normalized []map[string]interface{}
	for _, input := range inputs {
		normalized = append(normalized, map[string]interface{}{
			"name":        input.Name,
			"type":        input.Type,
			"required":    input.Required,
			"description": input.Description,
			"default":     input.Default,
		})
	}
	return normalized
}

// normalizeOutput creates a normalized representation of workflow output
func (h *BasicWorkflowHasher) normalizeOutput(output *workflow.Output) map[string]interface{} {
	if output == nil {
		return nil
	}
	return map[string]interface{}{
		"name":        output.Name,
		"type":        output.Type,
		"description": output.Description,
		"format":      output.Format,
		"document":    output.Document,
	}
}

// normalizeSteps creates a normalized representation of workflow steps
func (h *BasicWorkflowHasher) normalizeSteps(steps []*workflow.Step) []map[string]interface{} {
	var normalized []map[string]interface{}
	for _, step := range steps {
		normalized = append(normalized, map[string]interface{}{
			"name":       step.Name(),
			"type":       step.Type(),
			"parameters": step.Parameters(),
		})
	}
	return normalized
}

// normalizeGraph creates a normalized representation of the workflow graph
func (h *BasicWorkflowHasher) normalizeGraph(graph *workflow.Graph) map[string]interface{} {
	// Simple representation - in a full implementation this would be more detailed
	stepNames := graph.Names()
	return map[string]interface{}{
		"start_step": graph.Start().Name(),
		"step_count": len(stepNames),
		"step_names": stepNames,
	}
}

// findIncompatibleSteps identifies steps that have changed in incompatible ways
func (h *BasicWorkflowHasher) findIncompatibleSteps(oldWorkflow, newWorkflow *workflow.Workflow) []string {
	var incompatible []string

	oldSteps := make(map[string]*workflow.Step)
	for _, step := range oldWorkflow.Steps() {
		oldSteps[step.Name()] = step
	}

	newSteps := make(map[string]*workflow.Step)
	for _, step := range newWorkflow.Steps() {
		newSteps[step.Name()] = step
	}

	// Check for removed steps
	for name := range oldSteps {
		if _, exists := newSteps[name]; !exists {
			incompatible = append(incompatible, name+" (removed)")
		}
	}

	// Check for changed step types
	for name, newStep := range newSteps {
		if oldStep, exists := oldSteps[name]; exists {
			if oldStep.Type() != newStep.Type() {
				incompatible = append(incompatible, name+" (type changed)")
			}
		}
	}

	return incompatible
}

// findChangedInputs identifies inputs that have changed between workflow versions
func (h *BasicWorkflowHasher) findChangedInputs(oldWorkflow, newWorkflow *workflow.Workflow) []string {
	var changed []string

	oldInputs := make(map[string]*workflow.Input)
	for _, input := range oldWorkflow.Inputs() {
		oldInputs[input.Name] = input
	}

	newInputs := make(map[string]*workflow.Input)
	for _, input := range newWorkflow.Inputs() {
		newInputs[input.Name] = input
	}

	// Check for removed inputs
	for name := range oldInputs {
		if _, exists := newInputs[name]; !exists {
			changed = append(changed, name+" (removed)")
		}
	}

	// Check for new inputs
	for name := range newInputs {
		if _, exists := oldInputs[name]; !exists {
			changed = append(changed, name+" (added)")
		}
	}

	// Check for type changes
	for name, newInput := range newInputs {
		if oldInput, exists := oldInputs[name]; exists {
			if oldInput.Type != newInput.Type || oldInput.Required != newInput.Required {
				changed = append(changed, name+" (modified)")
			}
		}
	}

	return changed
}
