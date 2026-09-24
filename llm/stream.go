package llm

import (
	"encoding/json"
	"errors"
	"fmt"
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
	Type              EventType                  `json:"type"`
	Index             *int                       `json:"index,omitempty"`
	Message           *Response                  `json:"message,omitempty"`
	ContentBlock      *EventContentBlock         `json:"content_block,omitempty"`
	Delta             *EventDelta                `json:"delta,omitempty"`
	Usage             *Usage                     `json:"usage,omitempty"`
	ContextManagement *ContextManagementResponse `json:"context_management,omitempty"`
	// InputTransformations can arrive again on the final message_delta, after
	// a server-side fallback, with the serving model's entries.
	InputTransformations []InputTransformation `json:"input_transformations,omitempty"`
}

// EventContentBlock carries the start of a content block in an LLM event.
type EventContentBlock struct {
	Type        ContentType      `json:"type"`
	Text        string           `json:"text,omitempty"`
	ID          string           `json:"id,omitempty"`
	Name        string           `json:"name,omitempty"`
	ToolsetName string           `json:"toolset_name,omitempty"`
	Input       json.RawMessage  `json:"input,omitempty"`
	Thinking    string           `json:"thinking,omitempty"`
	Signature   string           `json:"signature,omitempty"`
	Metadata    ProviderMetadata `json:"metadata,omitempty"`

	// raw keeps the block's JSON when it was decoded from a provider stream
	// and its type has no field set above (for example Anthropic's
	// server_tool_use or web_search_tool_result). ResponseAccumulator decodes
	// it with UnmarshalContent, so a streamed response holds the same content
	// as the non-streaming one.
	raw json.RawMessage
}

// UnmarshalJSON decodes a content block. For a type that the fields of
// EventContentBlock cannot hold, it also keeps the block's JSON for
// ResponseAccumulator.
func (b *EventContentBlock) UnmarshalJSON(data []byte) error {
	type plain EventContentBlock
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = EventContentBlock(decoded)
	switch b.Type {
	case ContentTypeText, ContentTypeToolUse, ContentTypeThinking, ContentTypeRedactedThinking:
	default:
		b.raw = append(json.RawMessage(nil), data...)
	}
	return nil
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
	EventDeltaTypeMetadata  EventDeltaType = "metadata_delta"
	EventDeltaTypeCitations EventDeltaType = "citations_delta"
)

// EventDelta carries a portion of an LLM response.
type EventDelta struct {
	Type         EventDeltaType   `json:"type,omitempty"`
	Text         string           `json:"text,omitempty"`
	Index        int              `json:"index,omitempty"`
	StopReason   string           `json:"stop_reason,omitempty"`
	StopSequence string           `json:"stop_sequence,omitempty"`
	PartialJSON  string           `json:"partial_json,omitempty"`
	Thinking     string           `json:"thinking,omitempty"`
	Signature    string           `json:"signature,omitempty"`
	Metadata     ProviderMetadata `json:"metadata,omitempty"`
}

// ResponseAccumulator builds up a complete response from a stream of events.
type ResponseAccumulator struct {
	response      *Response
	contentBlocks map[int]Content // Map of content blocks by index
	skippedBlocks map[int]bool    // Indices of unrecognized content block types
	// serverInputs buffers input_json_delta fragments for server-side tool
	// calls (ServerToolUseContent, MCPToolUseContent) until the block stops.
	serverInputs map[int][]byte
	complete     bool
}

// NewResponseAccumulator creates a new ResponseAccumulator.
func NewResponseAccumulator() *ResponseAccumulator {
	return &ResponseAccumulator{
		contentBlocks: make(map[int]Content),
		skippedBlocks: make(map[int]bool),
		serverInputs:  make(map[int][]byte),
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
				Text:     event.ContentBlock.Text,
				Metadata: event.ContentBlock.Metadata.Clone(),
			}
		case ContentTypeToolUse:
			content = &ToolUseContent{
				ID:          event.ContentBlock.ID,
				Name:        event.ContentBlock.Name,
				ToolsetName: event.ContentBlock.ToolsetName,
				Metadata:    event.ContentBlock.Metadata.Clone(),
			}
		case ContentTypeThinking:
			content = &ThinkingContent{
				ID:        event.ContentBlock.ID,
				Thinking:  event.ContentBlock.Thinking,
				Signature: event.ContentBlock.Signature,
				Metadata:  event.ContentBlock.Metadata.Clone(),
			}
		case ContentTypeRedactedThinking:
			content = &RedactedThinkingContent{}
		default:
			// Blocks such as Anthropic's server_tool_use and
			// web_search_tool_result arrive whole in the start event. Decode
			// them the way a non-streaming response is decoded, so streaming
			// and non-streaming calls produce the same message.
			if len(event.ContentBlock.raw) > 0 {
				if decoded, err := UnmarshalContent(event.ContentBlock.raw); err == nil {
					content = decoded
				}
			}
		}
		if content == nil {
			// Unrecognized content block type (for example one that
			// UnmarshalContent does not support). Skip it rather than
			// storing a nil entry, and remember the index so subsequent delta
			// events for this block are ignored.
			if event.Index != nil {
				r.skippedBlocks[*event.Index] = true
			}
			return nil
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
			if r.skippedBlocks[*event.Index] {
				// Delta for a skipped (unrecognized) content block; ignore it.
				return nil
			}
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
			switch toolUse := content.(type) {
			case *ToolUseContent:
				toolUse.Input = append(toolUse.Input, []byte(event.Delta.PartialJSON)...)
			case *ServerToolUseContent, *MCPToolUseContent:
				// The start event carried a placeholder input ({}). Buffer
				// the fragments and replace it when the block stops.
				r.serverInputs[*event.Index] = append(r.serverInputs[*event.Index], event.Delta.PartialJSON...)
			default:
				return errors.New("in-progress block is not a tool use content")
			}
		case EventDeltaTypeThinking, EventDeltaTypeSignature:
			if thinkingContent, ok := content.(*ThinkingContent); ok {
				thinkingContent.Thinking += event.Delta.Thinking
				thinkingContent.Signature += event.Delta.Signature
			} else {
				return errors.New("in-progress block is not a thinking content")
			}
		case EventDeltaTypeMetadata:
			if err := mergeContentMetadata(content, event.Delta.Metadata); err != nil {
				return err
			}
		}

	case EventTypeContentBlockStop:
		// A call with no arguments can stream no input deltas at all. Complete
		// it as the empty object Generate would have returned.
		if event.Index != nil {
			if toolUse, ok := r.contentBlocks[*event.Index].(*ToolUseContent); ok && len(toolUse.Input) == 0 {
				toolUse.Input = json.RawMessage("{}")
			}
			if input, ok := r.serverInputs[*event.Index]; ok {
				delete(r.serverInputs, *event.Index)
				if err := setServerToolInput(r.contentBlocks[*event.Index], input); err != nil {
					return err
				}
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

	// Update usage information if provided. Streaming usage frames carry
	// cumulative totals for the whole message, not increments, so merge by
	// supersession rather than summing.
	if event.Usage != nil && r.response != nil {
		r.response.Usage.Absorb(event.Usage)
	}

	// Update context management information if provided
	if event.ContextManagement != nil && r.response != nil {
		r.response.ContextManagement = event.ContextManagement
	}
	if event.InputTransformations != nil && r.response != nil {
		r.response.InputTransformations = event.InputTransformations
	}

	// Once the message is complete, attach an estimated cost from resolved
	// model pricing. No-op when no resolver/pricing is registered. Done here,
	// after usage accumulation, so the final token counts are reflected.
	if r.complete && r.response != nil {
		PopulateCost(r.response.Model, r.response.Usage.Speed == string(SpeedFast), &r.response.Usage)
	}
	return nil
}

// setServerToolInput replaces the input of a streamed server-side tool call
// with the JSON gathered from its input_json_delta events.
func setServerToolInput(content Content, input []byte) error {
	if len(input) == 0 {
		return nil
	}
	switch toolUse := content.(type) {
	case *ServerToolUseContent:
		var parsed map[string]any
		if err := json.Unmarshal(input, &parsed); err != nil {
			return fmt.Errorf("invalid server tool input for %s: %w", toolUse.ID, err)
		}
		toolUse.Input = parsed
	case *MCPToolUseContent:
		toolUse.Input = json.RawMessage(input)
	}
	return nil
}

func mergeContentMetadata(content Content, metadata ProviderMetadata) error {
	if len(metadata) == 0 {
		return nil
	}
	var target *ProviderMetadata
	switch typed := content.(type) {
	case *TextContent:
		target = &typed.Metadata
	case *ThinkingContent:
		target = &typed.Metadata
	case *ToolUseContent:
		target = &typed.Metadata
	default:
		return errors.New("in-progress block does not support provider metadata")
	}
	if *target == nil {
		*target = make(ProviderMetadata, len(metadata))
	}
	for key, value := range metadata {
		(*target)[key] = value
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
