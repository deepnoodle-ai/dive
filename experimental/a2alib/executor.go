package a2alib

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// SessionProvider returns the Dive session to use for a given A2A context ID.
// A nil return means the agent runs stateless for that context.
type SessionProvider func(ctx context.Context, contextID string) (dive.Session, error)

// Executor implements a2asrv.AgentExecutor by wrapping a *dive.Agent.
// It translates between the a2a-go event iterator model and Dive's
// CreateResponse flow.
type Executor struct {
	agent    *dive.Agent
	sessions SessionProvider

	// inflight tracks cancel functions for in-progress Execute runs
	// so Cancel can abort a running CreateResponse.
	inflightMu sync.Mutex
	inflight   map[a2a.TaskID]context.CancelFunc
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithSessionProvider sets a session provider for multi-turn conversations.
func WithSessionProvider(sp SessionProvider) ExecutorOption {
	return func(e *Executor) { e.sessions = sp }
}

// NewExecutor creates an Executor that bridges a Dive agent to a2a-go.
func NewExecutor(agent *dive.Agent, opts ...ExecutorOption) *Executor {
	e := &Executor{
		agent:    agent,
		inflight: make(map[a2a.TaskID]context.CancelFunc),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Execute implements a2asrv.AgentExecutor. It runs a Dive agent turn and
// yields a2a events. The method runs in a dedicated goroutine managed by
// a2a-go's execution framework.
func (e *Executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		msg := execCtx.Message
		if msg == nil {
			yield(nil, fmt.Errorf("a2alib: nil message in executor context"))
			return
		}

		// Emit submitted task if this is a new task (no stored task).
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, msg), nil) {
				return
			}
		}

		// Emit working status.
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		// Convert the a2a message to Dive LLM content.
		llmMsg, err := messageFromA2A(msg)
		if err != nil {
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))), nil)
			return
		}

		// Build CreateResponse options.
		var opts []dive.CreateResponseOption

		if e.sessions != nil {
			sess, err := e.sessions(ctx, execCtx.ContextID)
			if err != nil {
				yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed,
					a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("session error: "+err.Error()))), nil)
				return
			}
			if sess != nil {
				opts = append(opts, dive.WithSession(sess))
			}
		}

		// Check for resume: if the stored task has suspension metadata, set up
		// WithResume instead of WithMessages.
		if execCtx.StoredTask != nil {
			resumeOpts, err := e.buildResumeOpts(execCtx, msg)
			if err != nil {
				yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed,
					a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))), nil)
				return
			}
			if resumeOpts != nil {
				opts = append(opts, resumeOpts...)
			} else {
				// Follow-up message on same task, not a resume.
				opts = append(opts, dive.WithMessages(llmMsg))
			}
		} else {
			opts = append(opts, dive.WithMessages(llmMsg))
		}

		// Set up streaming: run CreateResponse with an event callback that
		// yields intermediate a2a status updates for tool calls and text.
		// Use a derived context so the goroutine is cancelled if yield
		// returns false (consumer disconnected) or Cancel is called.
		execCtx2, cancelExec := context.WithCancel(ctx)
		defer cancelExec()

		// Register so Cancel can abort this run.
		taskID := execCtx.TaskID
		e.trackInflight(taskID, cancelExec)
		defer e.untrackInflight(taskID)

		type runResult struct {
			resp *dive.Response
			err  error
		}
		events := make(chan a2a.Event, 64)
		results := make(chan runResult, 1)

		go func() {
			defer close(events)
			cb := func(_ context.Context, item *dive.ResponseItem) error {
				event := streamEventFromItem(execCtx, item)
				if event != nil {
					select {
					case events <- event:
					case <-execCtx2.Done():
						return execCtx2.Err()
					}
				}
				return nil
			}
			opts = append(opts, dive.WithEventCallback(cb))
			resp, err := e.agent.CreateResponse(execCtx2, opts...)
			results <- runResult{resp: resp, err: err}
		}()

		// Yield intermediate streaming events.
		for event := range events {
			if !yield(event, nil) {
				cancelExec()
				// Wait for the goroutine to finish so its write to
				// the result channel happens-before we return.
				<-results
				return
			}
		}

		// CreateResponse finished. Yield final result.
		result := <-results
		if result.err != nil {
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(result.err.Error()))), nil)
			return
		}

		// Map response to final events.
		e.yieldResponseEvents(execCtx, result.resp, yield)
	}
}

// Cancel implements a2asrv.AgentExecutor.
func (e *Executor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		// Abort any in-flight Execute for this task.
		e.cancelInflight(execCtx.TaskID)

		// If the session supports cancellation, clean up suspension state.
		if e.sessions != nil && execCtx.ContextID != "" {
			if sess, err := e.sessions(ctx, execCtx.ContextID); err == nil && sess != nil {
				if suspendable, ok := sess.(dive.SuspendableSession); ok {
					_ = suspendable.CancelSuspension(ctx)
				}
			}
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func (e *Executor) trackInflight(id a2a.TaskID, cancel context.CancelFunc) {
	e.inflightMu.Lock()
	e.inflight[id] = cancel
	e.inflightMu.Unlock()
}

func (e *Executor) untrackInflight(id a2a.TaskID) {
	e.inflightMu.Lock()
	delete(e.inflight, id)
	e.inflightMu.Unlock()
}

func (e *Executor) cancelInflight(id a2a.TaskID) {
	e.inflightMu.Lock()
	cancel, ok := e.inflight[id]
	e.inflightMu.Unlock()
	if ok {
		cancel()
	}
}

// yieldResponseEvents emits artifacts and a final status event from a
// completed or suspended Dive response.
func (e *Executor) yieldResponseEvents(
	execCtx *a2asrv.ExecutorContext,
	resp *dive.Response,
	yield func(a2a.Event, error) bool,
) {
	switch resp.Status {
	case dive.ResponseStatusSuspended:
		state := a2a.TaskStateInputRequired
		if resp.Suspension != nil && len(resp.Suspension.PendingToolCalls) > 0 {
			if resp.Suspension.PendingToolCalls[0].Reason == dive.SuspendReasonAuth {
				state = a2a.TaskStateAuthRequired
			}
		}

		// Build the status message from the suspension prompt.
		var statusMsg *a2a.Message
		if resp.Suspension != nil && len(resp.Suspension.PendingToolCalls) > 0 {
			prompt := resp.Suspension.PendingToolCalls[0].Prompt
			if prompt == "" {
				prompt = "Agent is waiting for input."
			}
			statusMsg = a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx,
				a2a.NewTextPart(prompt))
		}

		event := a2a.NewStatusUpdateEvent(execCtx, state, statusMsg)

		// Stash the suspension state in task metadata so we can resume later.
		// We marshal to JSON then unmarshal to map[string]any because
		// a2a-go's task store only permits basic metadata types.
		if resp.Suspension != nil {
			suspBytes, err := json.Marshal(resp.Suspension)
			if err == nil {
				var suspMap map[string]any
				if json.Unmarshal(suspBytes, &suspMap) == nil {
					event.SetMeta("dive.suspension", suspMap)
				}
			}
		}

		yield(event, nil)

	case "", dive.ResponseStatusCompleted:
		// Emit artifacts from assistant messages.
		for _, out := range resp.OutputMessages {
			if out.Role != llm.Assistant {
				continue
			}
			parts := partsFromContent(out.Content)
			if len(parts) == 0 {
				continue
			}
			artEvent := a2a.NewArtifactEvent(execCtx, parts...)
			artEvent.LastChunk = true
			if !yield(artEvent, nil) {
				return
			}
		}

		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)

	default:
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed,
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("unknown response status: "+string(resp.Status)))), nil)
	}
}

// buildResumeOpts checks if the stored task is suspended and builds the
// Dive options to resume the turn. Returns nil opts if no suspension.
func (e *Executor) buildResumeOpts(
	execCtx *a2asrv.ExecutorContext,
	msg *a2a.Message,
) ([]dive.CreateResponseOption, error) {
	if execCtx.StoredTask == nil || execCtx.StoredTask.Metadata == nil {
		return nil, nil
	}

	raw, ok := execCtx.StoredTask.Metadata["dive.suspension"]
	if !ok {
		return nil, nil
	}

	// Deserialize the suspension state. The metadata value may be a
	// json.RawMessage or a map[string]any depending on how the task store
	// round-tripped it.
	suspBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("a2alib: marshal suspension metadata: %w", err)
	}

	var suspension dive.SuspensionState
	if err := json.Unmarshal(suspBytes, &suspension); err != nil {
		return nil, fmt.Errorf("a2alib: unmarshal suspension state: %w", err)
	}

	if len(suspension.PendingToolCalls) == 0 {
		return nil, nil
	}

	// Extract tool results from the inbound message.
	results, err := resumeToolResults(&suspension, msg)
	if err != nil {
		return nil, err
	}

	return []dive.CreateResponseOption{dive.WithResume(&suspension, results)}, nil
}

// ---------------------------------------------------------------------------
// Content conversion: a2a -> Dive
// ---------------------------------------------------------------------------

// messageFromA2A converts an a2a Message to a Dive LLM message.
func messageFromA2A(msg *a2a.Message) (*llm.Message, error) {
	if msg == nil || len(msg.Parts) == 0 {
		return nil, fmt.Errorf("a2alib: message has no parts")
	}
	out := &llm.Message{Role: llm.User}
	for _, p := range msg.Parts {
		if p == nil || p.Content == nil {
			continue
		}
		c := contentFromPart(p)
		if c != nil {
			out.Content = append(out.Content, c)
		}
	}
	if len(out.Content) == 0 {
		return nil, fmt.Errorf("a2alib: message has no renderable content")
	}
	return out, nil
}

func contentFromPart(p *a2a.Part) llm.Content {
	switch v := p.Content.(type) {
	case a2a.Text:
		if string(v) == "" {
			return nil
		}
		return &llm.TextContent{Text: string(v)}

	case a2a.Data:
		encoded, err := json.Marshal(v.Value)
		if err != nil {
			return nil
		}
		return &llm.TextContent{Text: "```json\n" + string(encoded) + "\n```"}

	case a2a.Raw:
		if isImageMIME(p.MediaType) {
			return &llm.ImageContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: p.MediaType,
					Data:      base64.StdEncoding.EncodeToString([]byte(v)),
				},
			}
		}
		return &llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: p.MediaType,
				Data:      base64.StdEncoding.EncodeToString([]byte(v)),
			},
			Title: p.Filename,
		}

	case a2a.URL:
		if isImageMIME(p.MediaType) {
			return &llm.ImageContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeURL,
					MediaType: p.MediaType,
					URL:       string(v),
				},
			}
		}
		return &llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeURL,
				MediaType: p.MediaType,
				URL:       string(v),
			},
			Title: p.Filename,
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Content conversion: Dive -> a2a
// ---------------------------------------------------------------------------

// partsFromContent converts Dive LLM content to a2a parts. Internal content
// types (tool use, tool result, thinking) are skipped.
func partsFromContent(content []llm.Content) []*a2a.Part {
	var parts []*a2a.Part
	for _, c := range content {
		switch v := c.(type) {
		case *llm.TextContent:
			if v.Text != "" {
				parts = append(parts, a2a.NewTextPart(v.Text))
			}
		case *llm.ImageContent:
			if v.Source != nil {
				p := partFromSource(v.Source, "")
				if p != nil {
					parts = append(parts, p)
				}
			}
		case *llm.DocumentContent:
			if v.Source != nil {
				p := partFromSource(v.Source, v.Title)
				if p != nil {
					parts = append(parts, p)
				}
			}
		case *llm.RefusalContent:
			if v.Text != "" {
				parts = append(parts, a2a.NewTextPart(v.Text))
			}
		}
	}
	return parts
}

func partFromSource(src *llm.ContentSource, title string) *a2a.Part {
	if src == nil {
		return nil
	}
	switch src.Type {
	case llm.ContentSourceTypeBase64:
		data, _ := base64.StdEncoding.DecodeString(src.Data)
		p := a2a.NewRawPart(data)
		p.MediaType = src.MediaType
		p.Filename = title
		return p
	case llm.ContentSourceTypeURL:
		p := a2a.NewFileURLPart(a2a.URL(src.URL), src.MediaType)
		p.Filename = title
		return p
	}
	return nil
}

// ---------------------------------------------------------------------------
// Streaming event conversion
// ---------------------------------------------------------------------------

// streamEventFromItem converts a Dive ResponseItem into an a2a status update
// for streaming progress. Returns nil for items that don't map to user-visible
// progress.
func streamEventFromItem(execCtx *a2asrv.ExecutorContext, item *dive.ResponseItem) a2a.Event {
	switch item.Type {
	case dive.ResponseItemTypeToolCall:
		if item.ToolCall == nil {
			return nil
		}
		return a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking,
			a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx,
				a2a.NewTextPart(fmt.Sprintf("Calling tool: %s", item.ToolCall.Name))))
	case dive.ResponseItemTypeMessage:
		if item.Message == nil || item.Message.Role != llm.Assistant {
			return nil
		}
		text := item.Message.LastText()
		if text == "" {
			return nil
		}
		return a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking,
			a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx,
				a2a.NewTextPart(text)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Resume helpers
// ---------------------------------------------------------------------------

// resumeToolResults translates an inbound a2a message into Dive ToolResults
// for suspended pending tool calls.
func resumeToolResults(state *dive.SuspensionState, msg *a2a.Message) (map[string]*dive.ToolResult, error) {
	if len(state.PendingToolCalls) == 0 {
		return nil, nil
	}

	// Check for a structured toolResults DataPart.
	mapped, found, err := extractToolResultsMap(msg)
	if err != nil {
		return nil, err
	}
	if found {
		results := make(map[string]*dive.ToolResult, len(state.PendingToolCalls))
		for _, call := range state.PendingToolCalls {
			text, ok := mapped[call.ID]
			if !ok {
				return nil, fmt.Errorf("a2alib: toolResults map missing pending call ID %q", call.ID)
			}
			results[call.ID] = dive.NewToolResultText(text)
		}
		return results, nil
	}

	// Fall back to text: broadcast for all pending calls.
	text := strings.TrimSpace(textFromMessage(msg))
	if text == "" {
		return nil, fmt.Errorf("a2alib: resume message has no text and no structured toolResults")
	}
	results := make(map[string]*dive.ToolResult, len(state.PendingToolCalls))
	for _, call := range state.PendingToolCalls {
		results[call.ID] = dive.NewToolResultText(text)
	}
	return results, nil
}

// extractToolResultsMap looks for a DataPart with a "toolResults" key.
// Returns (map, true, nil) on success, (nil, false, nil) when no DataPart
// has a toolResults key, and (nil, true, err) when a toolResults key is
// present but malformed (not a map of strings).
func extractToolResultsMap(msg *a2a.Message) (map[string]string, bool, error) {
	if msg == nil {
		return nil, false, nil
	}
	for _, p := range msg.Parts {
		data := p.Data()
		if data == nil {
			continue
		}
		m, ok := data.(map[string]any)
		if !ok {
			continue
		}
		raw, ok := m["toolResults"]
		if !ok {
			continue
		}
		// toolResults key is present — must be a valid map[string]string.
		tm, ok := raw.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("a2alib: toolResults must be an object, got %T", raw)
		}
		out := make(map[string]string, len(tm))
		for k, v := range tm {
			s, ok := v.(string)
			if !ok {
				return nil, true, fmt.Errorf("a2alib: toolResults value for %q must be a string, got %T", k, v)
			}
			out[k] = s
		}
		return out, true, nil
	}
	return nil, false, nil
}

func textFromMessage(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range msg.Parts {
		if t := p.Text(); t != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(t)
		}
	}
	return sb.String()
}

func isImageMIME(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}
