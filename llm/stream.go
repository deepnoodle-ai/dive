package llm

import (
	"encoding/json"
	"errors"
	"sort"
)

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
	Type      ContentType     `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
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
	EventDeltaTypeCitations EventDeltaType = "citations_delta"
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
	response      *Response
	contentBlocks map[int]Content // Map of content blocks by index
	complete      bool
}

// NewResponseAccumulator creates a new ResponseAccumulator.
func NewResponseAccumulator() *ResponseAccumulator {
	return &ResponseAccumulator{
		contentBlocks: make(map[int]Content),
	}
}

// AddEvent adds an event to the ResponseAccumulator.
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
		var content Content
		switch event.ContentBlock.Type {
		case ContentTypeText:
			content = &TextContent{
				Text: event.ContentBlock.Text,
			}
		case ContentTypeToolUse:
			content = &ToolUseContent{
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
		case ContentTypeThinking:
			content = &ThinkingContent{
				Thinking:  event.ContentBlock.Thinking,
				Signature: event.ContentBlock.Signature,
			}
		case ContentTypeRedactedThinking:
			content = &RedactedThinkingContent{}
		}

		if event.Index != nil {
			// Store content by index in map
			r.contentBlocks[*event.Index] = content
		} else {
			// If no index provided, use the next available index
			nextIndex := len(r.contentBlocks)
			r.contentBlocks[nextIndex] = content
		}

	case EventTypeContentBlockDelta:
		if r.response == nil || event.Delta == nil || event.Index == nil {
			return errors.New("invalid content block delta event")
		}

		content, exists := r.contentBlocks[*event.Index]
		if !exists {
			return errors.New("content block not found for index")
		}

		switch event.Delta.Type {
		case EventDeltaTypeText:
			if textContent, ok := content.(*TextContent); ok {
				textContent.Text += event.Delta.Text
			} else {
				return errors.New("in-progress block is not a text content")
			}
		case EventDeltaTypeInputJSON:
			if toolUseContent, ok := content.(*ToolUseContent); ok {
				toolUseContent.Input = append(toolUseContent.Input, []byte(event.Delta.PartialJSON)...)
			} else {
				return errors.New("in-progress block is not a tool use content")
			}
		case EventDeltaTypeThinking, EventDeltaTypeSignature:
			if thinkingContent, ok := content.(*ThinkingContent); ok {
				thinkingContent.Thinking += event.Delta.Thinking
				thinkingContent.Signature += event.Delta.Signature
			} else {
				return errors.New("in-progress block is not a thinking content")
			}
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
		// Convert map to sorted slice when complete
		r.finalizeContent()
	}

	// Update usage information if provided
	if event.Usage != nil {
		r.response.Usage.InputTokens += event.Usage.InputTokens
		r.response.Usage.OutputTokens += event.Usage.OutputTokens
		r.response.Usage.CacheReadInputTokens += event.Usage.CacheReadInputTokens
		r.response.Usage.CacheCreationInputTokens += event.Usage.CacheCreationInputTokens
	}
	return nil
}

// finalizeContent converts the content blocks map to a sorted slice
func (r *ResponseAccumulator) finalizeContent() {
	if r.response == nil || len(r.contentBlocks) == 0 {
		return
	}

	// Get sorted indices
	indices := make([]int, 0, len(r.contentBlocks))
	for index := range r.contentBlocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	// Create content slice in sorted order
	content := make([]Content, len(indices))
	for i, index := range indices {
		content[i] = r.contentBlocks[index]
	}

	r.response.Content = content
}

func (r *ResponseAccumulator) IsComplete() bool {
	return r.complete
}

func (r *ResponseAccumulator) Response() *Response {
	// Ensure content is finalized even if called before completion
	if r.response != nil && len(r.contentBlocks) > 0 && len(r.response.Content) == 0 {
		r.finalizeContent()
	}
	return r.response
}

func (r *ResponseAccumulator) Usage() *Usage {
	return &r.response.Usage
}
