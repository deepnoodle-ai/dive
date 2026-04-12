package a2alib

import (
	"context"
	"fmt"
	"iter"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// RemoteAgent is a higher-level wrapper around a2aclient.Client that
// provides ergonomic methods for calling a remote A2A agent from Dive code.
//
// It handles context ID persistence across calls and provides simple
// text-based send/stream methods so callers don't need to construct
// a2a.Message objects manually.
type RemoteAgent struct {
	client    *a2aclient.Client
	card      *a2a.AgentCard
	contextID string
}

// NewRemoteAgent constructs a RemoteAgent from an a2a-go client.
func NewRemoteAgent(client *a2aclient.Client) *RemoteAgent {
	return &RemoteAgent{client: client}
}

// NewRemoteAgentFromCard resolves a remote agent from its card and creates
// a RemoteAgent ready for use.
func NewRemoteAgentFromCard(ctx context.Context, card *a2a.AgentCard, opts ...a2aclient.FactoryOption) (*RemoteAgent, error) {
	client, err := a2aclient.NewFromCard(ctx, card, opts...)
	if err != nil {
		return nil, fmt.Errorf("a2alib: create client from card: %w", err)
	}
	return &RemoteAgent{client: client, card: card}, nil
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

// Client returns the underlying a2aclient.Client.
func (r *RemoteAgent) Client() *a2aclient.Client {
	return r.client
}

// SendText sends a plain text prompt to the remote agent and returns the
// resulting task. If the task completes, the response text can be extracted
// with ResponseText. If it enters input-required, the caller should inspect
// the status message and follow up with SendTextOnTask.
func (r *RemoteAgent) SendText(ctx context.Context, prompt string) (*a2a.Task, error) {
	if prompt == "" {
		return nil, fmt.Errorf("a2alib: SendText: empty prompt")
	}
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(prompt))
	msg.ContextID = r.contextID
	result, err := r.client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, err
	}
	return r.extractTask(result)
}

// SendTextOnTask continues an existing task with a new text message.
// Typically used to respond to an input-required state.
func (r *RemoteAgent) SendTextOnTask(ctx context.Context, taskID a2a.TaskID, prompt string) (*a2a.Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("a2alib: SendTextOnTask: empty taskID")
	}
	if prompt == "" {
		return nil, fmt.Errorf("a2alib: SendTextOnTask: empty prompt")
	}
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(prompt))
	msg.TaskID = taskID
	msg.ContextID = r.contextID
	result, err := r.client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, err
	}
	return r.extractTask(result)
}

// StreamText sends a text prompt via streaming and delivers events to the
// caller as an iterator. It captures the context ID from the first event
// that carries one so ContextID() stays current. Returns when the stream
// ends.
func (r *RemoteAgent) StreamText(ctx context.Context, prompt string) iter.Seq2[a2a.Event, error] {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(prompt))
	msg.ContextID = r.contextID
	inner := r.client.SendStreamingMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	return func(yield func(a2a.Event, error) bool) {
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

// extractTask converts a SendMessageResult to a *Task and updates the
// stored context ID.
func (r *RemoteAgent) extractTask(result a2a.SendMessageResult) (*a2a.Task, error) {
	switch v := result.(type) {
	case *a2a.Task:
		if v.ContextID != "" {
			r.contextID = v.ContextID
		}
		return v, nil
	case *a2a.Message:
		// Bare message response — wrap as completed task.
		if v.ContextID != "" {
			r.contextID = v.ContextID
		}
		return &a2a.Task{
			ID:        a2a.TaskID(v.ID),
			ContextID: v.ContextID,
			Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
			History:   []*a2a.Message{v},
			Metadata:  map[string]any{"a2a.syntheticFromMessage": true},
		}, nil
	default:
		return nil, fmt.Errorf("a2alib: unexpected result type: %T", result)
	}
}

// ResponseText returns the most useful text from a task's response.
// It prefers the first artifact's text parts and falls back to the last
// agent message in the task history.
func ResponseText(task *a2a.Task) string {
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
		if msg.Role != a2a.MessageRoleAgent {
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
