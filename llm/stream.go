package llm

// EventType represents the type of streaming event
type EventType string

func (e EventType) String() string {
	return string(e)
}

const (
	EventPing              EventType = "ping"
	EventMessageStart      EventType = "message_start"
	EventMessageDelta      EventType = "message_delta"
	EventMessageStop       EventType = "message_stop"
	EventContentBlockStart EventType = "content_block_start"
	EventContentBlockDelta EventType = "content_block_delta"
	EventContentBlockStop  EventType = "content_block_stop"
)

// Event represents a single streaming event from the LLM. A successfully
// run stream will end with a final message containing the complete Response.
type Event struct {
	Type         EventType          `json:"type"`
	Index        int                `json:"index"`
	Message      *Message           `json:"message,omitempty"`
	ContentBlock *EventContentBlock `json:"content_block,omitempty"`
	Delta        *Delta             `json:"delta,omitempty"`
	Usage        *Usage             `json:"usage,omitempty"`
	Response     *Response          `json:"response,omitempty"`
}

// EventContentBlock carries the start of a content block in an LLM event.
type EventContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// Delta carries a portion of an LLM response.
type Delta struct {
	Type         string  `json:"type,omitempty"`
	Text         string  `json:"text,omitempty"`
	Index        int     `json:"index,omitempty"`
	StopReason   string  `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
	PartialJSON  string  `json:"partial_json,omitempty"`
	Thinking     string  `json:"thinking,omitempty"`
	Signature    string  `json:"signature,omitempty"`
}
