package a2a

import (
	"context"
	"fmt"
	"iter"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// TaskResult is the outcome of a SendText, SendTextOnTask, or StreamText call.
// It carries the fields most callers need without requiring an import of the
// underlying a2a-go SDK. Use LastTask() on RemoteAgent to access the full
// SDK task when artifact metadata or message history is required.
type TaskResult struct {
	ID        string
	ContextID string
	State     string
	Text      string // extracted response text
}

// IsCompleted reports whether the task completed successfully.
func (r *TaskResult) IsCompleted() bool {
	return r != nil && r.State == string(a2asdk.TaskStateCompleted)
}

// IsInputRequired reports whether the agent is paused waiting for user input.
func (r *TaskResult) IsInputRequired() bool {
	return r != nil && r.State == string(a2asdk.TaskStateInputRequired)
}

// IsFailed reports whether the task failed.
func (r *TaskResult) IsFailed() bool {
	return r != nil && r.State == string(a2asdk.TaskStateFailed)
}

// IsCanceled reports whether the task was canceled.
func (r *TaskResult) IsCanceled() bool {
	return r != nil && r.State == string(a2asdk.TaskStateCanceled)
}

// RemoteAgent is a higher-level wrapper around a2aclient.Client that
// provides ergonomic methods for calling a remote A2A agent from Dive code.
//
// It handles context ID persistence across calls and provides simple
// text-based send/stream methods so callers don't need to construct
// SDK message objects manually.
type RemoteAgent struct {
	client    *a2aclient.Client
	contextID string
	lastTask  *a2asdk.Task
}

// NewRemoteAgentFromURL creates a RemoteAgent connected to the given URL
// using the default JSON-RPC transport. Use this for the common case where
// you know the server URL directly.
func NewRemoteAgentFromURL(ctx context.Context, url string) (*RemoteAgent, error) {
	card := &a2asdk.AgentCard{
		SupportedInterfaces: []*a2asdk.AgentInterface{{
			URL:             url,
			ProtocolBinding: a2asdk.TransportProtocolJSONRPC,
			ProtocolVersion: a2asdk.Version,
		}},
	}
	return NewRemoteAgentFromCard(ctx, card)
}

// NewRemoteAgentFromCard creates a RemoteAgent from a full agent card,
// resolving the transport from the card's SupportedInterfaces. Use this
// when you have a card from a discovery endpoint.
func NewRemoteAgentFromCard(ctx context.Context, card *a2asdk.AgentCard, opts ...a2aclient.FactoryOption) (*RemoteAgent, error) {
	client, err := a2aclient.NewFromCard(ctx, card, opts...)
	if err != nil {
		return nil, fmt.Errorf("a2a: create client from card: %w", err)
	}
	return &RemoteAgent{client: client}, nil
}

// NewRemoteAgent constructs a RemoteAgent from an existing a2aclient.Client.
func NewRemoteAgent(client *a2aclient.Client) *RemoteAgent {
	return &RemoteAgent{client: client}
}

// ContextID returns the persistent A2A context ID this agent is using.
func (r *RemoteAgent) ContextID() string {
	return r.contextID
}

// SetContextID overrides the context ID for subsequent calls. Useful when
// resuming a prior conversation by ID.
func (r *RemoteAgent) SetContextID(id string) {
	r.contextID = id
}

// Client returns the underlying a2aclient.Client for callers that need
// direct access to the SDK.
func (r *RemoteAgent) Client() *a2aclient.Client {
	return r.client
}

// LastTask returns the raw SDK task from the most recent Send or Stream call.
// Returns nil if no call has been made yet. Use this when you need access to
// artifact metadata, message history, or other fields not exposed by TaskResult.
func (r *RemoteAgent) LastTask() *a2asdk.Task {
	return r.lastTask
}

// SendText sends a plain text prompt to the remote agent and returns the
// result. If the task enters input-required state, use SendTextOnTask to
// continue on the same task.
func (r *RemoteAgent) SendText(ctx context.Context, prompt string) (*TaskResult, error) {
	if prompt == "" {
		return nil, fmt.Errorf("a2a: SendText: empty prompt")
	}
	msg := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart(prompt))
	msg.ContextID = r.contextID
	result, err := r.client.SendMessage(ctx, &a2asdk.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, err
	}
	return r.taskResultFrom(result)
}

// SendTextOnTask continues an existing task with a new text message.
// Typically used to respond to an input-required state.
func (r *RemoteAgent) SendTextOnTask(ctx context.Context, taskID string, prompt string) (*TaskResult, error) {
	if taskID == "" {
		return nil, fmt.Errorf("a2a: SendTextOnTask: empty taskID")
	}
	if prompt == "" {
		return nil, fmt.Errorf("a2a: SendTextOnTask: empty prompt")
	}
	msg := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart(prompt))
	msg.TaskID = a2asdk.TaskID(taskID)
	msg.ContextID = r.contextID
	result, err := r.client.SendMessage(ctx, &a2asdk.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, err
	}
	return r.taskResultFrom(result)
}

// StreamText sends a text prompt and calls onChunk for each text chunk as it
// arrives. Returns the final TaskResult when the stream ends. Cancellation is
// handled via the context.
func (r *RemoteAgent) StreamText(ctx context.Context, prompt string, onChunk func(string)) (*TaskResult, error) {
	if prompt == "" {
		return nil, fmt.Errorf("a2a: StreamText: empty prompt")
	}
	msg := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart(prompt))
	msg.ContextID = r.contextID
	stream := r.client.SendStreamingMessage(ctx, &a2asdk.SendMessageRequest{Message: msg})

	var lastTask *a2asdk.Task
	for event, err := range stream {
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}
		if cid := event.TaskInfo().ContextID; cid != "" {
			r.contextID = cid
		}
		switch v := event.(type) {
		case *a2asdk.TaskArtifactUpdateEvent:
			if onChunk != nil {
				for _, p := range v.Artifact.Parts {
					if t := p.Text(); t != "" {
						onChunk(t)
					}
				}
			}
		case *a2asdk.Task:
			lastTask = v
		case *a2asdk.TaskStatusUpdateEvent:
			if v.Status.State == a2asdk.TaskStateCompleted ||
				v.Status.State == a2asdk.TaskStateInputRequired ||
				v.Status.State == a2asdk.TaskStateFailed ||
				v.Status.State == a2asdk.TaskStateCanceled {
				t, fetchErr := r.client.GetTask(ctx, &a2asdk.GetTaskRequest{ID: v.TaskID})
				if fetchErr == nil {
					lastTask = t
				}
			}
		}
	}

	if lastTask == nil {
		return nil, fmt.Errorf("a2a: StreamText: stream ended without a terminal task")
	}
	r.lastTask = lastTask
	return toTaskResult(lastTask), nil
}

// StreamEvents returns the raw SDK event iterator for callers that need full
// control over streaming. Context ID is updated from each event automatically.
func (r *RemoteAgent) StreamEvents(ctx context.Context, prompt string) iter.Seq2[a2asdk.Event, error] {
	msg := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart(prompt))
	msg.ContextID = r.contextID
	inner := r.client.SendStreamingMessage(ctx, &a2asdk.SendMessageRequest{Message: msg})
	return func(yield func(a2asdk.Event, error) bool) {
		for event, err := range inner {
			if err == nil && event != nil {
				if cid := event.TaskInfo().ContextID; cid != "" {
					r.contextID = cid
				}
			}
			if !yield(event, err) {
				return
			}
		}
	}
}

// taskResultFrom converts a SendMessageResult to a TaskResult and updates
// the stored context ID and lastTask.
func (r *RemoteAgent) taskResultFrom(result a2asdk.SendMessageResult) (*TaskResult, error) {
	switch v := result.(type) {
	case *a2asdk.Task:
		if v.ContextID != "" {
			r.contextID = v.ContextID
		}
		r.lastTask = v
		return toTaskResult(v), nil
	case *a2asdk.Message:
		if v.ContextID != "" {
			r.contextID = v.ContextID
		}
		task := &a2asdk.Task{
			ID:        a2asdk.TaskID(v.ID),
			ContextID: v.ContextID,
			Status:    a2asdk.TaskStatus{State: a2asdk.TaskStateCompleted},
			History:   []*a2asdk.Message{v},
		}
		r.lastTask = task
		return toTaskResult(task), nil
	default:
		return nil, fmt.Errorf("a2a: unexpected result type: %T", result)
	}
}

// toTaskResult converts a raw SDK task to a TaskResult.
func toTaskResult(task *a2asdk.Task) *TaskResult {
	return &TaskResult{
		ID:        string(task.ID),
		ContextID: task.ContextID,
		State:     string(task.Status.State),
		Text:      extractText(task),
	}
}

// extractText returns the most useful text from a task: prefers artifact
// text parts, falls back to the last agent message in history.
func extractText(task *a2asdk.Task) string {
	if task == nil {
		return ""
	}
	for _, art := range task.Artifacts {
		for _, p := range art.Parts {
			if t := p.Text(); t != "" {
				return t
			}
		}
	}
	for i := len(task.History) - 1; i >= 0; i-- {
		msg := task.History[i]
		if msg.Role != a2asdk.MessageRoleAgent {
			continue
		}
		for _, p := range msg.Parts {
			if t := p.Text(); t != "" {
				return t
			}
		}
	}
	return ""
}
