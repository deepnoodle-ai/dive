package orchestration

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

// MonitorToolInput is the input for the Monitor tool.
type MonitorToolInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	TimeoutMs   int    `json:"timeout_ms,omitempty"`
}

// MonitorToolOptions configures a new Monitor tool.
type MonitorToolOptions struct {
	// Runs, if non-nil, tracks the monitor so TaskStop can cancel it early.
	Runs *Runs

	// NotifyCallback is called (from a background goroutine) for each batch of
	// stdout lines. Must be safe for concurrent use.
	NotifyCallback func(description string, lines []string)
}

// MonitorTool starts a background watcher that streams stdout lines from a shell
// command as chat notifications.
type monitorTool struct {
	runs           *Runs
	notifyCallback func(description string, lines []string)
}

var _ dive.TypedTool[*MonitorToolInput] = &monitorTool{}

// NewMonitorTool creates the Monitor tool.
func NewMonitorTool(opts MonitorToolOptions) *dive.TypedToolAdapter[*MonitorToolInput] {
	return dive.ToolAdapter(&monitorTool{
		runs:           opts.Runs,
		notifyCallback: opts.NotifyCallback,
	})
}

func (t *monitorTool) Name() string { return "Monitor" }

func (t *monitorTool) Description() string {
	return `Start a background watcher that streams events from a long-running shell command. Each stdout line becomes a notification delivered to the chat while you continue working.

Usage notes:
- Use for ongoing monitoring: log tailing, process watching, event streams
- Lines arriving within 200ms are batched into one notification
- Only stdout triggers notifications; stderr is discarded
- Always use grep --line-buffered in pipes — without it, buffering delays events
- For a single one-time notification, prefer Bash with run_in_background instead
- Use TaskStop to cancel a monitor early
- Notifications arrive automatically as the command produces output`
}

func (t *monitorTool) Schema() *schema.Schema {
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

func (t *monitorTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:         "Monitor",
		OpenWorldHint: true,
	}
}

func (t *monitorTool) Call(ctx context.Context, input *MonitorToolInput) (*dive.ToolResult, error) {
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

	// Independent of the parent context so the monitor survives the turn that
	// started it; bounded by its own timeout and cancellable via TaskStop.
	cancelCtx, cancel := context.WithTimeout(context.Background(), timeout)
	if t.runs != nil {
		t.runs.add(taskID, input.Description, cancel)
	}

	notifyCallback := t.notifyCallback

	go func() {
		defer cancel()
		if t.runs != nil {
			defer t.runs.remove(taskID)
		}

		cmd := exec.CommandContext(cancelCtx, "sh", "-c", input.Command)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			if notifyCallback != nil {
				notifyCallback(input.Description, []string{fmt.Sprintf("[Monitor error: %s]", err)})
			}
			return
		}
		cmd.Stderr = nil // discard stderr

		if err := cmd.Start(); err != nil {
			if notifyCallback != nil {
				notifyCallback(input.Description, []string{fmt.Sprintf("[Monitor error: %s]", err)})
			}
			return
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
				case <-cancelCtx.Done():
					return
				}
			}
		}()

		var totalLines int
		monitorBatchLines(cancelCtx, linesCh, monitorBatchWindow, func(batch []string) {
			totalLines += len(batch)
			if notifyCallback != nil {
				notifyCallback(input.Description, batch)
			}
		})

		<-scanDone
		cmd.Wait()

		if notifyCallback != nil {
			notifyCallback(input.Description, []string{
				fmt.Sprintf("[Monitor done — %d lines delivered]", totalLines),
			})
		}
	}()

	return dive.NewToolResultText(fmt.Sprintf(
		"Monitor started\nTask ID: %s\nDescription: %s\nTimeout: %s\n\nNotifications will arrive automatically. Use TaskStop(%s) to cancel early.",
		taskID, input.Description, timeout, taskID,
	)).WithDisplay(fmt.Sprintf("Monitor started: %s", input.Description)), nil
}

// monitorBatchLines reads from linesCh, batches lines that arrive within window,
// and calls emit for each batch. Returns when linesCh is closed or ctx is
// cancelled.
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
