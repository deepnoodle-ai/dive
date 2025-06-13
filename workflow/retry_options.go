package workflow

// RetryOptions configures how to retry a failed execution
type RetryOptions struct {
	Strategy  RetryStrategy          `json:"strategy"`
	NewInputs map[string]interface{} `json:"new_inputs,omitempty"`
}

// RetryStrategy defines different approaches to retrying executions
type RetryStrategy string

const (
	RetryFromStart     RetryStrategy = "from_start"      // Complete replay
	RetryFromFailure   RetryStrategy = "from_failure"    // Resume from failed step
	RetryWithNewInputs RetryStrategy = "with_new_inputs" // Replay with different inputs
	RetrySkipFailed    RetryStrategy = "skip_failed"     // Continue past failed steps
)
