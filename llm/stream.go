package llm

import "errors"

// EventType represents the type of streaming event
type EventType string

func (e EventType) String() string {
	return string(e)
}

const (
	EventTypePing              EventType = "ping"
	EventTypeMessageStart      EventType = "message_start"
	EventTypeMessageDelta      EventType = "message_delta"
	EventTypeMessageStop       EventType = "message_stop"
	EventTypeContentBlockStart EventType = "content_block_start"
	EventTypeContentBlockDelta EventType = "content_block_delta"
	EventTypeContentBlockStop  EventType = "content_block_stop"
)

// Event represents a single streaming event from the LLM. A successfully
// run stream will end with a final message containing the complete Response.
type Event struct {
	Type         EventType          `json:"type"`
	Index        *int               `json:"index,omitempty"`
	Message      *Response          `json:"message,omitempty"`
	ContentBlock *EventContentBlock `json:"content_block,omitempty"`
	Delta        *EventDelta        `json:"delta,omitempty"`
	Usage        *Usage             `json:"usage,omitempty"`
}

// EventContentBlock carries the start of a content block in an LLM event.
type EventContentBlock struct {
	Type      ContentType `json:"type"`
	Text      string      `json:"text,omitempty"`
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Thinking  string      `json:"thinking,omitempty"`
	Signature string      `json:"signature,omitempty"`
}

// EventDeltaType indicates the type of delta in an LLM event.
type EventDeltaType string

func (e EventDeltaType) String() string {
	return string(e)
}

const (
	EventDeltaTypeText      EventDeltaType = "text_delta"
	EventDeltaTypeInputJSON EventDeltaType = "input_json_delta"
	EventDeltaTypeThinking  EventDeltaType = "thinking_delta"
	EventDeltaTypeSignature EventDeltaType = "signature_delta"
)

// EventDelta carries a portion of an LLM response.
type EventDelta struct {
	Type         EventDeltaType `json:"type,omitempty"`
	Text         string         `json:"text,omitempty"`
	Index        int            `json:"index,omitempty"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	PartialJSON  string         `json:"partial_json,omitempty"`
	Thinking     string         `json:"thinking,omitempty"`
	Signature    string         `json:"signature,omitempty"`
}

// ResponseAccumulator builds up a complete response from a stream of events.
type ResponseAccumulator struct {
	response *Response
	usage    *Usage
	complete bool
}

func NewResponseAccumulator() *ResponseAccumulator {
	return &ResponseAccumulator{}
}

func (r *ResponseAccumulator) AddEvent(event *Event) error {
	switch event.Type {
	case EventTypeMessageStart:
		if event.Message == nil {
			return errors.New("invalid message start event")
		}
		r.response = event.Message
		return nil

	case EventTypeContentBlockStart:
		if r.response == nil {
			return errors.New("no message start event found")
		}
		if event.ContentBlock == nil {
			return errors.New("no content block found in event")
		}
		content := &Content{
			Type:      event.ContentBlock.Type,
			Text:      event.ContentBlock.Text,
			ID:        event.ContentBlock.ID,
			Name:      event.ContentBlock.Name,
			Thinking:  event.ContentBlock.Thinking,
			Signature: event.ContentBlock.Signature,
		}
		if event.Index != nil {
			// Ensure slice has enough capacity
			for len(r.response.Content) <= *event.Index {
				r.response.Content = append(r.response.Content, nil)
			}
			r.response.Content[*event.Index] = content
		} else {
			r.response.Content = append(r.response.Content, content)
		}

	case EventTypeContentBlockDelta:
		if r.response == nil || event.Delta == nil || event.Index == nil {
			return errors.New("invalid content block delta event")
		}
		if *event.Index >= len(r.response.Content) {
			return errors.New("content block index out of bounds")
		}
		content := r.response.Content[*event.Index]
		if content == nil {
			return errors.New("content block not found")
		}
		switch event.Delta.Type {
		case EventDeltaTypeText:
			content.Text += event.Delta.Text
		case EventDeltaTypeInputJSON:
			if content.Input == nil {
				content.Input = []byte(event.Delta.PartialJSON)
			} else {
				content.Input = append(content.Input, []byte(event.Delta.PartialJSON)...)
			}
		case EventDeltaTypeThinking:
			content.Thinking += event.Delta.Thinking
		case EventDeltaTypeSignature:
			content.Signature += event.Delta.Signature
		}

	case EventTypeMessageDelta:
		if r.response == nil || event.Delta == nil {
			return errors.New("invalid message delta event")
		}
		if event.Delta.StopReason != "" {
			r.response.StopReason = event.Delta.StopReason
		}
		if event.Delta.StopSequence != "" {
			r.response.StopSequence = &event.Delta.StopSequence
		}

	case EventTypeMessageStop:
		r.complete = true
	}

	// Update usage information if provided
	if event.Usage != nil {
		r.usage = event.Usage
	}
	return nil
}

func (r *ResponseAccumulator) IsComplete() bool {
	return r.complete
}

func (r *ResponseAccumulator) Response() *Response {
	return r.response
}

func (r *ResponseAccumulator) Usage() *Usage {
	return r.usage
}
