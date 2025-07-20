package environment

// WorkflowFormatter interface for pretty output
type WorkflowFormatter interface {
	PrintStepStart(stepName, stepType string)
	PrintStepOutput(stepName, content string)
	PrintStepError(stepName string, err error)
}
