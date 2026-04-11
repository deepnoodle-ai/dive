package a2a

import (
	"context"
	"fmt"
)

// RemoteAgent is a higher-level wrapper around Client that lets Dive code
// call a remote A2A agent without building Message objects by hand.
//
// It holds onto the remote agent card (fetched lazily) plus any
// persistent contextId so follow-up calls on the same conversation go
// through as multi-turn interactions rather than fresh tasks.
type RemoteAgent struct {
	client    *Client
	card      *AgentCard
	contextID string
}

// NewRemoteAgent constructs a RemoteAgent from an already-built Client.
func NewRemoteAgent(client *Client) *RemoteAgent {
	return &RemoteAgent{client: client}
}

// Card returns the cached remote agent card, fetching it if it has not
// been loaded yet.
func (r *RemoteAgent) Card(ctx context.Context) (*AgentCard, error) {
	if r.card != nil {
		return r.card, nil
	}
	card, err := r.client.FetchCard(ctx)
	if err != nil {
		return nil, err
	}
	r.card = card
	return card, nil
}

// ContextID returns the persistent A2A context ID this agent is using,
// if any.
func (r *RemoteAgent) ContextID() string {
	return r.contextID
}

// SetContextID overrides the context ID used for subsequent calls.
// Useful when the caller wants to resume a prior A2A context by ID
// instead of the one the server assigned on the first message.
func (r *RemoteAgent) SetContextID(id string) {
	r.contextID = id
}

// SendText is the most common entry point: send a plain text prompt to
// the remote agent and return the resulting Task. If the remote task
// completes, the Task carries the response text in its first Artifact
// (or in the final assistant history entry). If it enters
// input-required, the caller should inspect task.Status.Message and
// follow up with another SendText call on the same RemoteAgent.
func (r *RemoteAgent) SendText(ctx context.Context, prompt string) (*Task, error) {
	if prompt == "" {
		return nil, fmt.Errorf("a2a: SendText: empty prompt")
	}
	msg := &Message{
		Role:      RoleUser,
		Parts:     []Part{NewTextPart(prompt)},
		ContextID: r.contextID,
	}
	task, err := r.client.SendMessage(ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	if task != nil && task.ContextID != "" {
		r.contextID = task.ContextID
	}
	return task, nil
}

// SendTextOnTask continues an existing task with a new text message.
// Equivalent to SendText but also stamps the task ID so the server
// routes the message into the existing task (typically used to respond
// to an input-required state).
func (r *RemoteAgent) SendTextOnTask(ctx context.Context, taskID, prompt string) (*Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("a2a: SendTextOnTask: empty taskID")
	}
	if prompt == "" {
		return nil, fmt.Errorf("a2a: SendTextOnTask: empty prompt")
	}
	msg := &Message{
		Role:      RoleUser,
		Parts:     []Part{NewTextPart(prompt)},
		TaskID:    taskID,
		ContextID: r.contextID,
	}
	task, err := r.client.SendMessage(ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	if task != nil && task.ContextID != "" {
		r.contextID = task.ContextID
	}
	return task, nil
}

// StreamText sends a text prompt via message/stream and delivers stream
// events to the callback. Returns when the stream ends.
func (r *RemoteAgent) StreamText(ctx context.Context, prompt string, onEvent func(*StreamEvent) error) error {
	msg := &Message{
		Role:      RoleUser,
		Parts:     []Part{NewTextPart(prompt)},
		ContextID: r.contextID,
	}
	return r.client.StreamMessage(ctx, msg, nil, func(ev *StreamEvent) error {
		if ev.Task != nil && ev.Task.ContextID != "" {
			r.contextID = ev.Task.ContextID
		} else if ev.StatusUpdate != nil && ev.StatusUpdate.ContextID != "" {
			r.contextID = ev.StatusUpdate.ContextID
		}
		return onEvent(ev)
	})
}

// ResponseText returns the most useful text response from a Task. It
// prefers the first artifact's text parts and falls back to the last
// agent message in the task history.
func ResponseText(task *Task) string {
	if task == nil {
		return ""
	}
	for _, art := range task.Artifacts {
		for _, p := range art.Parts {
			if p.Kind == PartKindText && p.Text != "" {
				return p.Text
			}
		}
	}
	for i := len(task.History) - 1; i >= 0; i-- {
		msg := task.History[i]
		if msg.Role != RoleAgent {
			continue
		}
		if text := msg.TextContent(); text != "" {
			return text
		}
	}
	return ""
}
