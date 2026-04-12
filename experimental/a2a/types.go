package a2a

import (
	"encoding/json"
	"fmt"
	"time"
)

// TaskState is the lifecycle state of an A2A Task. Values are the
// hyphenated lowercase strings used on the wire by A2A v0.2.
type TaskState string

const (
	TaskStateSubmitted     TaskState = "submitted"
	TaskStateWorking       TaskState = "working"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateAuthRequired  TaskState = "auth-required"
	TaskStateCompleted     TaskState = "completed"
	TaskStateCanceled      TaskState = "canceled"
	TaskStateFailed        TaskState = "failed"
	TaskStateRejected      TaskState = "rejected"
	TaskStateUnknown       TaskState = "unknown"
)

// IsTerminal returns true if the state is one of the A2A terminal states
// (completed, canceled, failed, rejected). A task in a terminal state will
// not emit further updates.
func (s TaskState) IsTerminal() bool {
	switch s {
	case TaskStateCompleted, TaskStateCanceled, TaskStateFailed, TaskStateRejected:
		return true
	}
	return false
}

// Role names a message's author. A2A uses "user" for client-originated
// messages and "agent" for server-originated messages.
type Role string

const (
	RoleUser  Role = "user"
	RoleAgent Role = "agent"
)

// PartKind is the discriminator for a Part's content type.
type PartKind string

const (
	PartKindText PartKind = "text"
	PartKindFile PartKind = "file"
	PartKindData PartKind = "data"
)

// Part is one piece of a Message or Artifact. Exactly one of Text, File, or
// Data is populated according to Kind.
type Part struct {
	Kind     PartKind       `json:"kind"`
	Text     string         `json:"text,omitempty"`
	File     *FileContent   `json:"file,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewTextPart returns a Part containing text content.
func NewTextPart(text string) Part {
	return Part{Kind: PartKindText, Text: text}
}

// NewDataPart returns a Part containing structured data.
func NewDataPart(data map[string]any) Part {
	return Part{Kind: PartKindData, Data: data}
}

// FileContent carries a file referenced by a Part, either inline as base64
// bytes or by URI.
type FileContent struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Bytes    string `json:"bytes,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// Message is a unit of conversation exchanged between an A2A client and
// agent. It matches the A2A Message shape.
type Message struct {
	MessageID string         `json:"messageId"`
	Role      Role           `json:"role"`
	Parts     []Part         `json:"parts"`
	Kind      string         `json:"kind"`
	TaskID    string         `json:"taskId,omitempty"`
	ContextID string         `json:"contextId,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TextContent returns the concatenated text from all text parts in the
// message. Non-text parts are ignored.
func (m *Message) TextContent() string {
	if m == nil {
		return ""
	}
	var out string
	for _, p := range m.Parts {
		if p.Kind == PartKindText {
			if out != "" {
				out += "\n\n"
			}
			out += p.Text
		}
	}
	return out
}

// TaskStatus is the status of an A2A task at a point in time.
type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Artifact is a named output produced by the agent during a task. The
// prototype treats the final assistant message as a single "response"
// artifact with text parts.
type Artifact struct {
	ArtifactID string         `json:"artifactId"`
	Name       string         `json:"name,omitempty"`
	Parts      []Part         `json:"parts"`
	Index      int            `json:"index,omitempty"`
	Append     bool           `json:"append,omitempty"`
	LastChunk  bool           `json:"lastChunk,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Task is the top-level A2A task object returned by message/send and
// tasks/get. The wire schema requires Kind to be the literal string
// "task"; the marshaler fills it in if it is empty.
type Task struct {
	ID        string         `json:"id"`
	ContextID string         `json:"contextId"`
	Kind      string         `json:"kind"`
	Status    TaskStatus     `json:"status"`
	History   []*Message     `json:"history,omitempty"`
	Artifacts []*Artifact    `json:"artifacts,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TaskStatusUpdateEvent is a streaming update announcing a new TaskStatus.
// The wire schema marks Kind, TaskID, ContextID, Status, and Final as
// required fields, so they are always emitted (with zero defaults filled
// in by the marshaler).
type TaskStatusUpdateEvent struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Kind      string         `json:"kind"`
	Status    TaskStatus     `json:"status"`
	Final     bool           `json:"final"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TaskArtifactUpdateEvent is a streaming update announcing a new or updated
// artifact. The wire schema marks Kind, TaskID, ContextID, and Artifact as
// required fields, so they are always emitted.
type TaskArtifactUpdateEvent struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Kind      string         `json:"kind"`
	Artifact  *Artifact      `json:"artifact"`
	Append    bool           `json:"append,omitempty"`
	LastChunk bool           `json:"lastChunk,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ---- Agent Card ----

// AgentCard describes an A2A agent: identity, endpoint, supported skills,
// and capability flags. It is served at /.well-known/agent-card.json (and,
// for backwards compatibility, /.well-known/agent.json).
//
// The A2A schema marks Name, Description, URL, Version, Capabilities,
// DefaultInputModes, DefaultOutputModes, and Skills as required. The
// custom MarshalJSON below fills in safe defaults when those fields are
// empty so that a partially configured card still validates against
// strict A2A clients.
type AgentCard struct {
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	URL                string                `json:"url"`
	Version            string                `json:"version"`
	ProtocolVersion    string                `json:"protocolVersion,omitempty"`
	PreferredTransport string                `json:"preferredTransport,omitempty"`
	DocumentationURL   string                `json:"documentationUrl,omitempty"`
	Provider           *AgentProvider        `json:"provider,omitempty"`
	Capabilities       AgentCapabilities     `json:"capabilities"`
	DefaultInputModes  []string              `json:"defaultInputModes"`
	DefaultOutputModes []string              `json:"defaultOutputModes"`
	Skills             []AgentSkill          `json:"skills"`
	SecuritySchemes    map[string]any        `json:"securitySchemes,omitempty"`
	Security           []map[string][]string `json:"security,omitempty"`
}

// AgentProvider identifies the organization providing the agent.
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// AgentCapabilities enumerates optional features the A2A server supports.
type AgentCapabilities struct {
	Streaming              bool `json:"streaming,omitempty"`
	PushNotifications      bool `json:"pushNotifications,omitempty"`
	StateTransitionHistory bool `json:"stateTransitionHistory,omitempty"`
}

// AgentSkill is one coarse capability the agent advertises. A Dive agent
// that exposes a single conversational surface typically publishes one
// skill matching its name. The A2A schema marks ID, Name, Description, and
// Tags as required; Tags is always emitted (as an empty array if unset).
type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

// ---- Request params (JSON-RPC) ----

// SendMessageParams is the params object for the message/send and
// message/stream methods.
type SendMessageParams struct {
	Message       *Message              `json:"message"`
	Configuration *MessageConfiguration `json:"configuration,omitempty"`
	Metadata      map[string]any        `json:"metadata,omitempty"`
}

// MessageConfiguration tunes server-side behavior for a single send call.
type MessageConfiguration struct {
	AcceptedOutputModes    []string `json:"acceptedOutputModes,omitempty"`
	Blocking               bool     `json:"blocking,omitempty"`
	HistoryLength          int      `json:"historyLength,omitempty"`
	PushNotificationConfig any      `json:"pushNotificationConfig,omitempty"`
}

// TaskIDParams is the params object for tasks/get, tasks/cancel, and
// tasks/resubscribe.
type TaskIDParams struct {
	ID            string `json:"id"`
	HistoryLength int    `json:"historyLength,omitempty"`
}

// Validate returns an error if p is missing required fields.
func (p *SendMessageParams) Validate() error {
	if p == nil || p.Message == nil {
		return fmt.Errorf("a2a: send params missing message")
	}
	if p.Message.MessageID == "" {
		return fmt.Errorf("a2a: message is missing messageId")
	}
	if p.Message.Role != RoleUser {
		return fmt.Errorf("a2a: message role must be %q, got %q", RoleUser, p.Message.Role)
	}
	if len(p.Message.Parts) == 0 {
		return fmt.Errorf("a2a: message has no parts")
	}
	return nil
}

// MarshalJSON ensures Parts serializes to "[]" rather than null and that
// Kind defaults to "message" so the wire payload satisfies strict A2A
// validators.
func (m Message) MarshalJSON() ([]byte, error) {
	type alias Message
	clone := alias(m)
	if clone.Parts == nil {
		clone.Parts = []Part{}
	}
	if clone.Kind == "" {
		clone.Kind = "message"
	}
	return json.Marshal(clone)
}

// MarshalJSON defaults Kind to "task" so Task always carries the
// discriminator the A2A schema requires.
func (t Task) MarshalJSON() ([]byte, error) {
	type alias Task
	clone := alias(t)
	if clone.Kind == "" {
		clone.Kind = "task"
	}
	return json.Marshal(clone)
}

// MarshalJSON defaults Kind to "status-update" and ensures the
// discriminator is always present on streamed status events.
func (e TaskStatusUpdateEvent) MarshalJSON() ([]byte, error) {
	type alias TaskStatusUpdateEvent
	clone := alias(e)
	if clone.Kind == "" {
		clone.Kind = "status-update"
	}
	return json.Marshal(clone)
}

// MarshalJSON defaults Kind to "artifact-update" and ensures the
// discriminator is always present on streamed artifact events.
func (e TaskArtifactUpdateEvent) MarshalJSON() ([]byte, error) {
	type alias TaskArtifactUpdateEvent
	clone := alias(e)
	if clone.Kind == "" {
		clone.Kind = "artifact-update"
	}
	return json.Marshal(clone)
}

// MarshalJSON ensures the slice fields the A2A schema marks as required
// (DefaultInputModes, DefaultOutputModes, Skills) serialize to empty
// arrays rather than null when the caller has not set them.
func (c AgentCard) MarshalJSON() ([]byte, error) {
	type alias AgentCard
	clone := alias(c)
	if clone.DefaultInputModes == nil {
		clone.DefaultInputModes = []string{}
	}
	if clone.DefaultOutputModes == nil {
		clone.DefaultOutputModes = []string{}
	}
	if clone.Skills == nil {
		clone.Skills = []AgentSkill{}
	}
	return json.Marshal(clone)
}

// MarshalJSON ensures Tags serializes to "[]" instead of null so the
// resulting skill validates against strict A2A clients.
func (s AgentSkill) MarshalJSON() ([]byte, error) {
	type alias AgentSkill
	clone := alias(s)
	if clone.Tags == nil {
		clone.Tags = []string{}
	}
	return json.Marshal(clone)
}
