package environment

// RetryOptions configures how to retry a failed execution
type RetryOptions struct {
	Strategy RetryStrategy `json:"strategy"`
}

// RetryStrategy defines different approaches to retrying executions
type RetryStrategy string

const (
	RetryFromStart   RetryStrategy = "from_start"   // Complete replay
	RetryFromFailure RetryStrategy = "from_failure" // Resume from failed step
	RetrySkipFailed  RetryStrategy = "skip_failed"  // Continue past failed steps
)
