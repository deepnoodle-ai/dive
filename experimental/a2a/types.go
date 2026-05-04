package a2a

import (
	"encoding/json"
	"fmt"
	"time"
)

// TaskState is the lifecycle state of an A2A Task. Values are the
// SCREAMING_SNAKE_CASE strings used on the wire by A2A v1.0.
type TaskState string

const (
	TaskStateSubmitted     TaskState = "TASK_STATE_SUBMITTED"
	TaskStateWorking       TaskState = "TASK_STATE_WORKING"
	TaskStateInputRequired TaskState = "TASK_STATE_INPUT_REQUIRED"
	TaskStateAuthRequired  TaskState = "TASK_STATE_AUTH_REQUIRED"
	TaskStateCompleted     TaskState = "TASK_STATE_COMPLETED"
	TaskStateCanceled      TaskState = "TASK_STATE_CANCELED"
	TaskStateFailed        TaskState = "TASK_STATE_FAILED"
	TaskStateRejected      TaskState = "TASK_STATE_REJECTED"
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

// Role names a message's author. A2A v1.0 uses ROLE_USER for
// client-originated messages and ROLE_AGENT for server-originated messages.
type Role string

const (
	RoleUser  Role = "ROLE_USER"
	RoleAgent Role = "ROLE_AGENT"
)

// Part is one piece of a Message or Artifact. Exactly one of Text, Raw,
// Data, or URL is populated. The presence of that field serves as the
// discriminator (A2A v1.0 content-based discrimination).
type Part struct {
	Text      string         `json:"text,omitempty"`
	Raw       string         `json:"raw,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	URL       string         `json:"url,omitempty"`
	Filename  string         `json:"filename,omitempty"`
	MediaType string         `json:"mediaType,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// NewTextPart returns a Part containing text content.
func NewTextPart(text string) Part {
	return Part{Text: text}
}

// NewDataPart returns a Part containing structured data.
func NewDataPart(data map[string]any) Part {
	return Part{Data: data}
}

// NewRawPart returns a Part containing base64-encoded raw bytes.
func NewRawPart(raw string, mediaType string) Part {
	return Part{Raw: raw, MediaType: mediaType}
}

// NewURLPart returns a Part containing a URL reference.
func NewURLPart(url string, mediaType string) Part {
	return Part{URL: url, MediaType: mediaType}
}

// IsText returns true if this is a text part.
func (p Part) IsText() bool { return p.Text != "" }

// IsRaw returns true if this is a raw bytes part.
func (p Part) IsRaw() bool { return p.Raw != "" }

// IsData returns true if this is a structured data part.
func (p Part) IsData() bool { return len(p.Data) > 0 }

// IsURL returns true if this is a URL part.
func (p Part) IsURL() bool { return p.URL != "" }

// Validate enforces the A2A v1.0 invariant that exactly one of Text,
// Raw, Data, or URL is populated on a Part.
func (p Part) Validate() error {
	count := 0
	if p.IsText() {
		count++
	}
	if p.IsRaw() {
		count++
	}
	if p.IsData() {
		count++
	}
	if p.IsURL() {
		count++
	}
	switch count {
	case 0:
		return fmt.Errorf("a2a: part has no content (one of text, raw, data, url required)")
	case 1:
		return nil
	default:
		return fmt.Errorf("a2a: part must populate exactly one of text, raw, data, url")
	}
}

// Message is a unit of conversation exchanged between an A2A client and
// agent. It matches the A2A v1.0 Message shape.
type Message struct {
	MessageID string         `json:"messageId"`
	Role      Role           `json:"role"`
	Parts     []Part         `json:"parts"`
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
		if p.IsText() {
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
	State     TaskState  `json:"state"`
	Message   *Message   `json:"message,omitempty"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
}

// Artifact is a named output produced by the agent during a task.
type Artifact struct {
	ArtifactID  string         `json:"artifactId"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parts       []Part         `json:"parts"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Task is the top-level A2A task object returned by message/send and
// tasks/get.
type Task struct {
	ID        string         `json:"id"`
	ContextID string         `json:"contextId"`
	Status    TaskStatus     `json:"status"`
	History   []*Message     `json:"history,omitempty"`
	Artifacts []*Artifact    `json:"artifacts,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TaskStatusUpdateEvent is a streaming update announcing a new TaskStatus.
type TaskStatusUpdateEvent struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Status    TaskStatus     `json:"status"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TaskArtifactUpdateEvent is a streaming update announcing a new or updated
// artifact.
type TaskArtifactUpdateEvent struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Artifact  *Artifact      `json:"artifact"`
	Append    bool           `json:"append,omitempty"`
	LastChunk bool           `json:"lastChunk,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// StreamResponse wraps an A2A v1.0 streaming event with a field-name
// discriminator. Exactly one field is populated.
type StreamResponse struct {
	StatusUpdate   *TaskStatusUpdateEvent   `json:"statusUpdate,omitempty"`
	ArtifactUpdate *TaskArtifactUpdateEvent `json:"artifactUpdate,omitempty"`
	Task           *Task                    `json:"task,omitempty"`
	Message        *Message                 `json:"message,omitempty"`
}

// ---- Agent Card ----

// AgentInterface describes one transport endpoint.
type AgentInterface struct {
	URL             string `json:"url"`
	ProtocolBinding string `json:"protocolBinding"`
	ProtocolVersion string `json:"protocolVersion,omitempty"`
}

// AgentCard describes an A2A agent: identity, endpoint, supported skills,
// and capability flags. It is served at /.well-known/agent-card.json (and,
// for backwards compatibility, /.well-known/agent.json).
type AgentCard struct {
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	SupportedInterfaces []AgentInterface      `json:"supportedInterfaces"`
	Version             string                `json:"version"`
	DocumentationURL    string                `json:"documentationUrl,omitempty"`
	IconURL             string                `json:"iconUrl,omitempty"`
	Provider            *AgentProvider        `json:"provider,omitempty"`
	Capabilities        AgentCapabilities     `json:"capabilities"`
	DefaultInputModes   []string              `json:"defaultInputModes"`
	DefaultOutputModes  []string              `json:"defaultOutputModes"`
	Skills              []AgentSkill          `json:"skills"`
	SecuritySchemes     map[string]any        `json:"securitySchemes,omitempty"`
	Security            []map[string][]string `json:"security,omitempty"`
}

// AgentProvider identifies the organization providing the agent.
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// AgentCapabilities enumerates optional features the A2A server supports.
type AgentCapabilities struct {
	Streaming         bool `json:"streaming,omitempty"`
	PushNotifications bool `json:"pushNotifications,omitempty"`
	ExtendedAgentCard bool `json:"extendedAgentCard,omitempty"`
}

// AgentSkill is one coarse capability the agent advertises.
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
	ReturnImmediately      bool     `json:"returnImmediately,omitempty"`
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
	for i, part := range p.Message.Parts {
		if err := part.Validate(); err != nil {
			return fmt.Errorf("a2a: parts[%d]: %w", i, err)
		}
	}
	return nil
}

// MarshalJSON ensures Parts serializes to "[]" rather than null.
func (m Message) MarshalJSON() ([]byte, error) {
	type alias Message
	clone := alias(m)
	if clone.Parts == nil {
		clone.Parts = []Part{}
	}
	return json.Marshal(clone)
}

// MarshalJSON ensures the slice fields the A2A schema marks as required
// (DefaultInputModes, DefaultOutputModes, Skills, SupportedInterfaces)
// serialize to empty arrays rather than null when the caller has not set
// them.
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
	if clone.SupportedInterfaces == nil {
		clone.SupportedInterfaces = []AgentInterface{}
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
