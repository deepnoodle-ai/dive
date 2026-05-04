package dive

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/google/uuid"
)

// backgroundResult is the internal state carried on ToolResult.Background.
// Tool authors do not construct this directly; they use NewBackgroundResult or
// NewBackgroundResultFull.
type backgroundResult struct {
	id          string
	description string
	done        chan *ToolResult // buffered, cap 1 — goroutine sends exactly once
}

// BackgroundTaskHandle is the caller-facing handle for a background task.
// It is returned on Response.BackgroundTasks after any turn in which one or
// more tools returned a BackgroundResult. The Done channel delivers exactly
// one *ToolResult when the background goroutine completes.
type BackgroundTaskHandle struct {
	// TaskID is a unique identifier for this background task.
	TaskID string

	// ToolUseID is the LLM tool_use block ID for the call that started this task.
	ToolUseID string

	// Description is the human-readable description supplied to NewBackgroundResult.
	Description string

	// Done delivers exactly one *ToolResult when the background goroutine
	// finishes. The channel is buffered (cap 1) so the goroutine can always
	// send without blocking even if the caller never reads.
	Done <-chan *ToolResult

	// hookCtx is the hook context from the original tool call execution.
	// Used by PostBackgroundToolUse hooks to close OTel spans and access
	// the original tool/call metadata. Unexported: callers use the hooks API.
	hookCtx *HookContext
}

// NewBackgroundResult is the primary constructor for starting a background task
// from a tool. fn receives a context derived from the tool's execution context
// so cancellation propagates. The goroutine and channel are managed by Dive.
//
// Example:
//
//	func (t *BuildTool) Call(ctx context.Context, input *BuildInput) (*dive.ToolResult, error) {
//	    return dive.NewBackgroundResult(ctx, "building project", func(ctx context.Context) (string, error) {
//	        output, err := runBuild(ctx, input.Target)
//	        if err != nil {
//	            return "", err
//	        }
//	        return output, nil
//	    }), nil
//	}
func NewBackgroundResult(ctx context.Context, description string, fn func(ctx context.Context) (string, error)) *ToolResult {
	return newBackgroundResult(ctx, description, func(ctx context.Context) *ToolResult {
		text, err := fn(ctx)
		if err != nil {
			return NewToolResultError(err.Error())
		}
		return NewToolResultText(text)
	})
}

// NewBackgroundResultFull is the advanced constructor for tools that need full
// control over the result (setting IsError, Display, or structured content).
//
// Example:
//
//	return dive.NewBackgroundResultFull(ctx, "running tests", func(ctx context.Context) *dive.ToolResult {
//	    out, err := runTests(ctx)
//	    if err != nil {
//	        return dive.NewToolResultError(err.Error())
//	    }
//	    return dive.NewToolResultText(out).WithDisplay("# Test Results\n" + out)
//	}), nil
func NewBackgroundResultFull(ctx context.Context, description string, fn func(ctx context.Context) *ToolResult) *ToolResult {
	return newBackgroundResult(ctx, description, fn)
}

func newBackgroundResult(ctx context.Context, description string, fn func(ctx context.Context) *ToolResult) *ToolResult {
	ch := make(chan *ToolResult, 1)
	id := uuid.New().String()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- NewToolResultError(fmt.Sprintf("background task panicked: %v\n%s", r, debug.Stack()))
			}
		}()
		result := fn(ctx)
		if result == nil {
			result = NewToolResultText("")
		}
		ch <- result
	}()

	return &ToolResult{
		Background: &backgroundResult{
			id:          id,
			description: description,
			done:        ch,
		},
	}
}

// AwaitBackgroundTasks blocks until all tasks deliver results or ctx is
// cancelled, then returns a map of task ID → *ToolResult. On cancellation it
// returns partial results plus ctx.Err(). Background goroutines continue
// running after cancellation; their results remain readable on each handle's
// Done channel.
func AwaitBackgroundTasks(ctx context.Context, tasks []*BackgroundTaskHandle) (map[string]*ToolResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	type collected struct {
		taskID string
		result *ToolResult
	}
	ch := make(chan collected, len(tasks))

	for _, t := range tasks {
		t := t
		go func() {
			select {
			case r := <-t.Done:
				ch <- collected{taskID: t.TaskID, result: r}
			case <-ctx.Done():
				ch <- collected{taskID: t.TaskID, result: nil}
			}
		}()
	}

	results := make(map[string]*ToolResult, len(tasks))
	var firstErr error
	for range tasks {
		it := <-ch
		if it.result != nil {
			results[it.taskID] = it.result
		} else if firstErr == nil {
			firstErr = ctx.Err()
		}
	}
	return results, firstErr
}

// WithBackgroundResults injects completed background task results as a
// synthetic user message at the start of the next CreateResponse call. handles
// provides descriptions and ordering; results maps task ID to the completed
// *ToolResult. Obtain both from Response.BackgroundTasks and AwaitBackgroundTasks.
//
// Example:
//
//	results, err := dive.AwaitBackgroundTasks(ctx, resp.BackgroundTasks)
//	if err != nil { ... }
//	nextResp, err := agent.CreateResponse(ctx,
//	    dive.WithBackgroundResults(resp.BackgroundTasks, results),
//	    dive.WithSession(sess),
//	)
func WithBackgroundResults(handles []*BackgroundTaskHandle, results map[string]*ToolResult) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.BackgroundHandles = handles
		opts.BackgroundResults = results
	}
}

// ContinueWithBackground is the convenience wrapper for interactive loops.
// It awaits all background tasks on resp.BackgroundTasks, then calls
// agent.CreateResponse with the results injected. If resp.BackgroundTasks is
// empty, resp is returned unchanged. Additional opts (e.g. WithSession) are
// passed through.
//
// Callers should loop until the returned response has no BackgroundTasks:
//
//	resp, err := agent.CreateResponse(ctx, dive.WithInput("run the test suite"))
//	for err == nil && len(resp.BackgroundTasks) > 0 {
//	    resp, err = dive.ContinueWithBackground(ctx, agent, resp)
//	}
func ContinueWithBackground(ctx context.Context, agent *Agent, resp *Response, opts ...CreateResponseOption) (*Response, error) {
	if len(resp.BackgroundTasks) == 0 {
		return resp, nil
	}
	results, err := AwaitBackgroundTasks(ctx, resp.BackgroundTasks)
	if err != nil {
		return nil, err
	}
	callOpts := append([]CreateResponseOption{
		WithBackgroundResults(resp.BackgroundTasks, results),
	}, opts...)
	return agent.CreateResponse(ctx, callOpts...)
}

// backgroundStartedMessage returns the stable "started" text the agent sends
// to the LLM as the tool result when a tool returns BackgroundResult.
func backgroundStartedMessage(description, taskID string) string {
	return fmt.Sprintf(
		"Background task started: %s\nTask ID: %s\nThe result will be delivered in a follow-up message.",
		description, taskID,
	)
}

// backgroundCompletedMessage formats completed background task results as a
// synthetic user message for the next CreateResponse call.
func backgroundCompletedMessage(handles []*BackgroundTaskHandle, results map[string]*ToolResult) string {
	if len(handles) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(handles) > 1 {
		sb.WriteString("The following background tasks have completed:\n\n")
	}
	for i, h := range handles {
		if i > 0 {
			sb.WriteString("\n")
		}
		result := results[h.TaskID]
		fmt.Fprintf(&sb, "Background task completed: %s\n", h.Description)
		fmt.Fprintf(&sb, "Task ID: %s\n", h.TaskID)
		if result != nil && result.IsError {
			sb.WriteString("Error:\n")
		} else {
			sb.WriteString("Result:\n")
		}
		if result != nil {
			sb.WriteString(toolResultText(result))
		}
	}
	return sb.String()
}

// toolResultText extracts the text from a ToolResult's content blocks.
func toolResultText(r *ToolResult) string {
	if r == nil {
		return ""
	}
	var parts []string
	for _, c := range r.Content {
		if c.Type == ToolResultContentTypeText && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// firePostBackgroundToolUse fires PostBackgroundToolUse hooks for a single
// completed background result, using the HookContext from the original tool
// call. Called from CreateResponse on the main goroutine, never from
// background goroutines.
func (a *Agent) firePostBackgroundToolUse(ctx context.Context, hctx *HookContext, handle *BackgroundTaskHandle, result *ToolResult) error {
	if len(a.hooks.PostBackgroundToolUse) == 0 {
		return nil
	}
	savedCtx := handle.hookCtx
	if savedCtx == nil {
		return nil
	}
	tcr := &ToolCallResult{
		ID:     handle.ToolUseID,
		Name:   savedCtx.Call.Name,
		Result: result,
	}
	bgHctx := &HookContext{
		Agent:   a,
		Session: hctx.Session,
		Values:  hctx.Values,
		Tool:    savedCtx.Tool,
		Call:    savedCtx.Call,
		Result:  tcr,
	}
	for _, hook := range a.hooks.PostBackgroundToolUse {
		if err := hook(ctx, bgHctx); err != nil {
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PostBackgroundToolUse"
				a.logger.Error("post-background-tool-use hook aborted", "error", abortErr)
				return abortErr
			}
			a.logger.Debug("post-background-tool-use hook error", "error", err)
		}
	}
	return nil
}

// injectBackgroundResults prepends a synthetic user message containing the
// completed background task results and fires PostBackgroundToolUse hooks.
// Returns the augmented message slice.
func (a *Agent) injectBackgroundResults(
	ctx context.Context,
	hctx *HookContext,
	messages []*llm.Message,
	handles []*BackgroundTaskHandle,
	results map[string]*ToolResult,
) ([]*llm.Message, error) {
	msg := backgroundCompletedMessage(handles, results)
	if msg == "" {
		return messages, nil
	}
	messages = append(messages, llm.NewUserTextMessage(msg))
	for _, handle := range handles {
		result := results[handle.TaskID]
		if result == nil {
			continue
		}
		if err := a.firePostBackgroundToolUse(ctx, hctx, handle, result); err != nil {
			return nil, err
		}
	}
	return messages, nil
}
