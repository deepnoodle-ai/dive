package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Role indicates the role of a message in a conversation.
// Either "user", "assistant", or "system".
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	System    Role = "system"
	Developer Role = "developer"
)

func (r Role) String() string {
	return string(r)
}

// Messages is shorthand for a slice of messages.
type Messages []*Message

// Message containing content passed to or from an LLM.
type Message struct {
	ID      string    `json:"id,omitempty"`
	Role    Role      `json:"role"`
	Content []Content `json:"content"`
	// Effort, on a system message with no content, changes the reasoning
	// effort from the next user turn onward without restarting the prompt
	// cache. See NewEffortMessage.
	Effort ReasoningEffort `json:"effort,omitempty"`
}

// LastText returns the text of the last non-empty text block in the message,
// or "" when there is none. Empty text blocks, such as the empty Gemini part
// that only carries a thought signature, are skipped.
//
// A provider can split one reply into several text blocks (Anthropic splits
// text at citation boundaries). LastText then returns only the last fragment.
// To get the whole answer of a model-written message, use AnswerText.
func (m *Message) LastText() string {
	for i := len(m.Content) - 1; i >= 0; i-- {
		if content, ok := m.Content[i].(*TextContent); ok && content.Text != "" {
			return content.Text
		}
	}
	return ""
}

// Text returns the text of all non-empty text blocks in the message, in
// order, separated by two newlines. Empty text blocks, such as the empty
// Gemini part that only carries a thought signature, are skipped and add no
// separator. Non-text content is ignored.
//
// The separator suits messages built from separate passages, such as a
// system reminder followed by a user prompt. For a model answer that a
// provider split into fragments (Anthropic citations), use AnswerText, which
// joins adjacent fragments with no separator.
func (m *Message) Text() string {
	var sb strings.Builder
	for _, content := range m.Content {
		text, ok := content.(*TextContent)
		if !ok || text.Text == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(text.Text)
	}
	return sb.String()
}

// TextPhaseMetadataKey is the TextContent metadata key under which a provider
// records the phase of the output message a text block came from. The OpenAI
// Responses provider sets it to "commentary" for an intermediate update or
// "final_answer" for the answer, and must get it back unchanged when the
// history is replayed. AnswerText treats text blocks with different phases as
// separate passages.
//
// The value is "openai.phase" because OpenAI is the only provider that labels
// phases, and stored sessions already carry the key under that name.
const TextPhaseMetadataKey = "openai.phase"

// AnswerText returns the answer text of a model-written message: the text of
// all its non-empty text blocks, in order, or "" when it has none. This is
// what dive.Response.OutputText returns for the final message of a turn.
//
// Providers can split one passage into several text blocks (Anthropic splits
// text at citation boundaries; Gemini gives a part carrying a thought
// signature its own block). Text blocks that directly follow one another are
// therefore concatenated with no separator, which reproduces the text as the
// model wrote it. A new passage starts, and is joined with a blank line
// ("\n\n"), when
//   - other content, such as reasoning or a tool call, lies between two text
//     blocks, or
//   - two adjacent text blocks have different phases (TextPhaseMetadataKey),
//     as when an OpenAI commentary message is followed by the final answer.
//
// Empty text blocks are skipped and neither join nor separate passages.
//
// For a message built from separate passages, such as a user message with a
// reminder, use Text.
func (m *Message) AnswerText() string {
	var sb strings.Builder
	separated := false
	var phase string
	for _, content := range m.Content {
		text, ok := content.(*TextContent)
		if !ok {
			separated = true
			continue
		}
		if text.Text == "" {
			continue
		}
		textPhase := text.Metadata[TextPhaseMetadataKey]
		if sb.Len() > 0 && (separated || textPhase != phase) {
			sb.WriteString("\n\n")
		}
		sb.WriteString(text.Text)
		separated = false
		phase = textPhase
	}
	return sb.String()
}

// WithText appends text content block(s) to the message.
func (m *Message) WithText(text ...string) *Message {
	for _, t := range text {
		m.Content = append(m.Content, &TextContent{Text: t})
	}
	return m
}

// WithContent appends content block(s) to the message.
func (m *Message) WithContent(content ...Content) *Message {
	m.Content = append(m.Content, content...)
	return m
}

// ImageContent returns the first image content in the message, if any.
func (m *Message) ImageContent() (*ImageContent, bool) {
	for _, content := range m.Content {
		if image, ok := content.(*ImageContent); ok {
			return image, true
		}
	}
	return nil, false
}

// ThinkingContent returns the first thinking content in the message, if any.
func (m *Message) ThinkingContent() (*ThinkingContent, bool) {
	for _, content := range m.Content {
		if thinking, ok := content.(*ThinkingContent); ok {
			return thinking, true
		}
	}
	return nil, false
}

// DecodeInto decodes the last text content in the message as JSON into a given
// Go object. This pairs with the WithResponseFormat request option.
func (m *Message) DecodeInto(v any) error {
	for i := len(m.Content) - 1; i >= 0; i-- {
		switch content := m.Content[i].(type) {
		case *TextContent:
			return json.Unmarshal([]byte(content.Text), v)
		}
	}
	return fmt.Errorf("no text content found")
}

// Copy creates a deep copy of the message.
//
// This method uses JSON marshaling/unmarshaling to create a fully independent
// copy of the message including all content blocks. The copied message can be
// modified without affecting the original.
//
// This is primarily used by ThreadRepository.ForkThread to ensure that forked
// conversation threads have independent message histories.
//
// If marshaling fails (which should be rare), falls back to a shallow copy
// of the content slice.
func (m *Message) Copy() *Message {
	data, err := json.Marshal(m)
	if err != nil {
		// Fall back to shallow copy if marshaling fails
		contentCopy := make([]Content, len(m.Content))
		copy(contentCopy, m.Content)
		return &Message{
			ID:      m.ID,
			Role:    m.Role,
			Content: contentCopy,
			Effort:  m.Effort,
		}
	}
	var messageCopy Message
	if err := json.Unmarshal(data, &messageCopy); err != nil {
		// Fall back to shallow copy if unmarshaling fails
		contentCopy := make([]Content, len(m.Content))
		copy(contentCopy, m.Content)
		return &Message{
			ID:      m.ID,
			Role:    m.Role,
			Content: contentCopy,
			Effort:  m.Effort,
		}
	}
	return &messageCopy
}

// MarshalJSON implements custom marshaling for Message to properly handle
// the polymorphic Content field.
func (m *Message) MarshalJSON() ([]byte, error) {
	type tempMessage struct {
		ID      string            `json:"id,omitempty"`
		Role    Role              `json:"role"`
		Content []json.RawMessage `json:"content"`
		Effort  ReasoningEffort   `json:"effort,omitempty"`
	}

	// Marshal each content item individually
	var contentRaw []json.RawMessage
	for _, content := range m.Content {
		contentBytes, err := json.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %v", err)
		}
		contentRaw = append(contentRaw, contentBytes)
	}

	tmp := tempMessage{
		ID:      m.ID,
		Role:    m.Role,
		Content: contentRaw,
		Effort:  m.Effort,
	}

	return json.Marshal(tmp)
}

// UnmarshalJSON implements custom unmarshaling for Message to properly handle
// the polymorphic Content field.
func (m *Message) UnmarshalJSON(data []byte) error {
	type tempMessage struct {
		ID      string            `json:"id,omitempty"`
		Role    Role              `json:"role"`
		Content []json.RawMessage `json:"content"`
		Effort  ReasoningEffort   `json:"effort,omitempty"`
	}

	// Unmarshal JSON into the temporary struct
	var tmp tempMessage
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// Copy all fields except Content
	m.ID = tmp.ID
	m.Role = tmp.Role
	m.Effort = tmp.Effort

	// Process each content item
	m.Content = make([]Content, 0, len(tmp.Content))
	for _, rawContent := range tmp.Content {
		content, err := UnmarshalContent(rawContent)
		if err != nil {
			return fmt.Errorf("failed to unmarshal content: %v", err)
		}
		m.Content = append(m.Content, content)
	}
	return nil
}
