package extended

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/google/uuid"
)

const (
	monitorDefaultTimeoutMs = 300_000
	monitorMaxTimeoutMs     = 3_600_000
	monitorBatchWindow      = 200 * time.Millisecond
)

// MonitorToolInput is the input for the MonitorTool
type MonitorToolInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	TimeoutMs   int    `json:"timeout_ms,omitempty"`
}

// MonitorToolOptions configures a new MonitorTool
type MonitorToolOptions struct {
	Registry *TaskRegistry

	// NotifyCallback is called (from a background goroutine) for each batch of
	// stdout lines. Must be safe for concurrent use.
	NotifyCallback func(description string, lines []string)
}

// MonitorTool starts a background watcher that streams events from a shell command.
type MonitorTool struct {
	registry       *TaskRegistry
	notifyCallback func(description string, lines []string)
}

var _ dive.TypedTool[*MonitorToolInput] = &MonitorTool{}

// NewMonitorTool creates a new MonitorTool
func NewMonitorTool(opts MonitorToolOptions) *MonitorTool {
	return &MonitorTool{
		registry:       opts.Registry,
		notifyCallback: opts.NotifyCallback,
	}
}

func (t *MonitorTool) Name() string { return "Monitor" }

func (t *MonitorTool) Description() string {
	return `Start a background watcher that streams events from a long-running shell command. Each stdout line becomes a notification delivered to the chat while you continue working.

Usage notes:
- Use for ongoing monitoring: log tailing, process watching, event streams
- Lines arriving within 200ms are batched into one notification
- Only stdout triggers notifications; stderr is discarded
- Always use grep --line-buffered in pipes — without it, buffering delays events
- For a single one-time notification, prefer Bash with run_in_background instead
- Do NOT call TaskOutput for monitors; notifications arrive automatically`
}

func (t *MonitorTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"command", "description"},
		Properties: map[string]*schema.Property{
			"command": {
				Type:        "string",
				Description: "Shell command to run. Each stdout line triggers a notification.",
			},
			"description": {
				Type:        "string",
				Description: "Short label shown in each notification (e.g. \"errors in deploy.log\").",
			},
			"timeout_ms": {
				Type:        "number",
				Description: "Kill the monitor after this many milliseconds. Default 300000 (5 min), max 3600000 (1 hour).",
			},
		},
	}
}

func (t *MonitorTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:         "Monitor",
		OpenWorldHint: true,
	}
}

func (t *MonitorTool) ShouldReturnResult() bool { return true }

func (t *MonitorTool) Call(ctx context.Context, input *MonitorToolInput) (*dive.ToolResult, error) {
	if input.Command == "" {
		return dive.NewToolResultError("command is required"), nil
	}
	if input.Description == "" {
		return dive.NewToolResultError("description is required"), nil
	}

	timeoutMs := input.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = monitorDefaultTimeoutMs
	}
	if timeoutMs > monitorMaxTimeoutMs {
		timeoutMs = monitorMaxTimeoutMs
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	taskID := fmt.Sprintf("monitor_%s", uuid.New().String()[:8])
	record := &TaskRecord{
		ID:          taskID,
		Description: input.Description,
		Status:      TaskStatusRunning,
		StartTime:   time.Now(),
		done:        make(chan struct{}),
	}
	if t.registry != nil {
		t.registry.Register(record)
	}

	notifyCallback := t.notifyCallback

	return dive.NewBackgroundResultFull(context.Background(), input.Description, func(_ context.Context) *dive.ToolResult {
		defer close(record.done)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		cmd := exec.CommandContext(timeoutCtx, "sh", "-c", input.Command)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			record.setResult(TaskStatusFailed, fmt.Sprintf("failed to create stdout pipe: %s", err), err, time.Now())
			return dive.NewToolResultError(fmt.Sprintf("Monitor failed to start: %s", err)).
				WithDisplay(fmt.Sprintf("Monitor failed: %s", input.Description))
		}
		cmd.Stderr = nil // discard stderr

		if err := cmd.Start(); err != nil {
			record.setResult(TaskStatusFailed, fmt.Sprintf("failed to start: %s", err), err, time.Now())
			return dive.NewToolResultError(fmt.Sprintf("Monitor failed to start: %s", err)).
				WithDisplay(fmt.Sprintf("Monitor failed: %s", input.Description))
		}

		linesCh := make(chan string, 100)
		scanDone := make(chan struct{})
		go func() {
			defer close(scanDone)
			defer close(linesCh)
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				select {
				case linesCh <- scanner.Text():
				case <-timeoutCtx.Done():
					return
				}
			}
		}()

		var totalLines int
		monitorBatchLines(timeoutCtx, linesCh, monitorBatchWindow, func(batch []string) {
			totalLines += len(batch)
			if notifyCallback != nil {
				notifyCallback(input.Description, batch)
			}
		})

		<-scanDone
		cmd.Wait()

		endTime := time.Now()
		summary := fmt.Sprintf("Monitor finished: %s\nLines delivered: %d", input.Description, totalLines)
		record.setResult(TaskStatusCompleted, summary, nil, endTime)

		return dive.NewToolResultText(summary).
			WithDisplay(fmt.Sprintf("Monitor done: %s", input.Description))
	}), nil
}

// monitorBatchLines reads from linesCh, batches lines that arrive within
// window, and calls emit for each batch. Returns when linesCh is closed or
// ctx is cancelled.
func monitorBatchLines(ctx context.Context, linesCh <-chan string, window time.Duration, emit func([]string)) {
	var batch []string
	timer := time.NewTimer(window)
	defer timer.Stop()

	flush := func() {
		if len(batch) > 0 {
			emit(append([]string(nil), batch...))
			batch = batch[:0]
		}
	}

	for {
		select {
		case line, ok := <-linesCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, line)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(window)
		case <-timer.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}
