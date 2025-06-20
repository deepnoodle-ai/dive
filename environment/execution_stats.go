package environment

import (
	"time"

	"github.com/diveagents/dive/llm"
)

// ExecutionStats provides statistics about the execution
type ExecutionStats struct {
	TotalPaths     int           `json:"total_paths"`
	ActivePaths    int           `json:"active_paths"`
	CompletedPaths int           `json:"completed_paths"`
	FailedPaths    int           `json:"failed_paths"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`

	// Token usage information
	TotalUsage *llm.Usage `json:"total_usage,omitempty"`
}

// GetStats returns current execution statistics
func (e *Execution) GetStats() ExecutionStats {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	stats := ExecutionStats{
		TotalPaths: len(e.paths),
		StartTime:  e.startTime,
		EndTime:    e.endTime,
		TotalUsage: &e.totalUsage,
	}
	for _, state := range e.paths {
		switch state.Status {
		case PathStatusRunning:
			stats.ActivePaths++
		case PathStatusCompleted:
			stats.CompletedPaths++
		case PathStatusFailed:
			stats.FailedPaths++
		}
	}
	if !e.endTime.IsZero() {
		stats.Duration = e.endTime.Sub(e.startTime)
	} else if !e.startTime.IsZero() {
		stats.Duration = time.Since(e.startTime)
	}
	return stats
}
