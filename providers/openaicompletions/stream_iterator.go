package openaicompletions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

type StreamIterator struct {
	reader            *bufio.Reader
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	toolCalls         map[int]*ToolCallAccumulator
	contentBlocks     map[int]*ContentBlockAccumulator
	responseID        string
	responseModel     string
	usage             Usage
	prefill           string
	prefillClosingTag string
	eventCount        int
	nextBlockIndex    int
	closeOnce         sync.Once
	eventQueue        []*llm.Event
	// thinkingIndex and textIndex track the sequential content block index
	// assigned to each block type, or -1 if not yet started.
	thinkingIndex int
	textIndex     int
	// toolCallIndices maps OpenAI tool call indices to sequential block indices.
	toolCallIndices map[int]int
}

type ToolCallAccumulator struct {
	ID         string
	Type       string
	Name       string
	Arguments  string
	IsComplete bool
}

type ContentBlockAccumulator struct {
	Type       string
	Text       string
	IsComplete bool
}

// Next advances to the next event in the stream. Returns false when the stream
// is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	// If we have events in the queue, use the first one
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	// Try to get more events
	for {
		events, err := s.next()
		if err != nil {
			if err != io.EOF {
				// EOF is expected when stream ends
				s.Close()
				s.err = err
			}
			return false
		}

		// If we got events, use the first one and queue the rest
		if len(events) > 0 {
			s.currentEvent = events[0]
			if len(events) > 1 {
				s.eventQueue = append(s.eventQueue, events[1:]...)
			}
			return true
		}
	}
}

// Event returns the current event. Should only be called after a successful Next().
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// next processes a single line from the stream and returns events if any are ready
func (s *StreamIterator) next() ([]*llm.Event, error) {
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	// Skip empty lines
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, nil
	}
	// Parse the event type from the SSE format
	if bytes.HasPrefix(line, []byte("event: ")) {
		return nil, nil
	}
	// Remove "data: " prefix if present
	line = bytes.TrimPrefix(line, []byte("data: "))
	// Check for stream end
	if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
		return nil, nil
	}
	// Unmarshal the event
	var event StreamResponse
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}
	if event.ID != "" {
		s.responseID = event.ID
	}
	if event.Model != "" {
		s.responseModel = event.Model
	}
	if event.Usage.TotalTokens > 0 {
		s.usage = event.Usage
	}
	if len(event.Choices) == 0 {
		return nil, nil
	}
	choice := event.Choices[0]
	var events []*llm.Event

	// Emit message start event if this is the first event
	if s.eventCount == 0 {
		s.eventCount++
		events = append(events, &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:      s.responseID,
				Type:    "message",
				Role:    llm.Assistant,
				Model:   s.responseModel,
				Content: []llm.Content{},
				Usage: llm.Usage{
					InputTokens:  s.usage.PromptTokens,
					OutputTokens: s.usage.CompletionTokens,
				},
			},
		})
	}

	if choice.Delta.Reasoning != "" {
		if s.thinkingIndex < 0 {
			s.thinkingIndex = s.nextBlockIndex
			s.nextBlockIndex++
			s.contentBlocks[s.thinkingIndex] = &ContentBlockAccumulator{Type: "thinking"}
			events = append(events, &llm.Event{
				Type:         llm.EventTypeContentBlockStart,
				Index:        &s.thinkingIndex,
				ContentBlock: &llm.EventContentBlock{Type: "thinking"},
			})
		}
		events = append(events, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &s.thinkingIndex,
			Delta: &llm.EventDelta{
				Type:     llm.EventDeltaTypeThinking,
				Thinking: choice.Delta.Reasoning,
			},
		})
	}

	// Handle text content
	if choice.Delta.Content != "" {
		// Apply and clear prefill if there is one
		if s.prefill != "" {
			if !strings.HasPrefix(choice.Delta.Content, s.prefill) &&
				!strings.HasPrefix(s.prefill, choice.Delta.Content) {
				choice.Delta.Content = s.prefill + choice.Delta.Content
			}
			s.prefill = ""
		}
		// If this is a new text block, stop any open blocks and start it
		if s.textIndex < 0 {
			// Stop any previous content blocks that are still open
			for prevIndex, prev := range s.contentBlocks {
				if !prev.IsComplete {
					stopIndex := prevIndex
					events = append(events, &llm.Event{
						Type:  llm.EventTypeContentBlockStop,
						Index: &stopIndex,
					})
					prev.IsComplete = true
				}
			}
			// Stop any previous tool calls that are still open
			for prevIndex, prev := range s.toolCalls {
				if !prev.IsComplete {
					stopIndex := prevIndex
					events = append(events, &llm.Event{
						Type:  llm.EventTypeContentBlockStop,
						Index: &stopIndex,
					})
					prev.IsComplete = true
				}
			}
			s.textIndex = s.nextBlockIndex
			s.nextBlockIndex++
			s.contentBlocks[s.textIndex] = &ContentBlockAccumulator{Type: "text"}
			events = append(events, &llm.Event{
				Type:         llm.EventTypeContentBlockStart,
				Index:        &s.textIndex,
				ContentBlock: &llm.EventContentBlock{Type: "text"},
			})
		}
		// Generate a content_block_delta event
		events = append(events, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &s.textIndex,
			Delta: &llm.EventDelta{
				Type: llm.EventDeltaTypeText,
				Text: choice.Delta.Content,
			},
		})
	}

	if len(choice.Delta.ToolCalls) > 0 {
		for _, toolCallDelta := range choice.Delta.ToolCalls {
			// Map OpenAI tool call index to a sequential display index
			index, known := s.toolCallIndices[toolCallDelta.Index]
			if !known {
				// Stop any previous content blocks that are still open
				for prevIndex, prev := range s.contentBlocks {
					if !prev.IsComplete {
						stopIndex := prevIndex
						events = append(events, &llm.Event{
							Type:  llm.EventTypeContentBlockStop,
							Index: &stopIndex,
						})
						prev.IsComplete = true
					}
				}
				// Stop any previous tool calls that are still open
				for prevIndex, prev := range s.toolCalls {
					if !prev.IsComplete {
						stopIndex := prevIndex
						events = append(events, &llm.Event{
							Type:  llm.EventTypeContentBlockStop,
							Index: &stopIndex,
						})
						prev.IsComplete = true
					}
				}
				index = s.nextBlockIndex
				s.nextBlockIndex++
				s.toolCallIndices[toolCallDelta.Index] = index
				s.toolCalls[index] = &ToolCallAccumulator{Type: "function"}
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &index,
					ContentBlock: &llm.EventContentBlock{
						ID:   toolCallDelta.ID,
						Name: toolCallDelta.Function.Name,
						Type: llm.ContentTypeToolUse,
					},
				})
			}
			toolCall := s.toolCalls[index]
			if toolCallDelta.ID != "" {
				toolCall.ID = toolCallDelta.ID
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventTypeContentBlockStart && queuedEvent.Index != nil && *queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.EventContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.ID = toolCallDelta.ID
					}
				}
			}
			if toolCallDelta.Type != "" {
				toolCall.Type = toolCallDelta.Type
			}
			if toolCallDelta.Function.Name != "" {
				toolCall.Name = toolCallDelta.Function.Name
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventTypeContentBlockStart && queuedEvent.Index != nil && *queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.EventContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.Name = toolCallDelta.Function.Name
					}
				}
			}
			if toolCallDelta.Function.Arguments != "" {
				toolCall.Arguments += toolCallDelta.Function.Arguments
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &index,
					Delta: &llm.EventDelta{
						Type:        llm.EventDeltaTypeInputJSON,
						PartialJSON: toolCallDelta.Function.Arguments,
					},
				})
			}
		}
	}

	if choice.FinishReason != "" {
		// Stop any open content blocks
		for index, block := range s.contentBlocks {
			blockIndex := index
			if !block.IsComplete {
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &blockIndex,
				})
				block.IsComplete = true
			}
		}
		// Stop any open tool calls
		for index, toolCall := range s.toolCalls {
			blockIndex := index
			if !toolCall.IsComplete {
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &blockIndex,
				})
				toolCall.IsComplete = true
			}
		}
		// Add message_delta event with stop reason
		stopReason := choice.FinishReason
		if stopReason == "tool_calls" {
			stopReason = "tool_use" // Match Anthropic
		}
		events = append(events, &llm.Event{
			Type:  llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{StopReason: stopReason},
			Usage: &llm.Usage{
				InputTokens:  s.usage.PromptTokens,
				OutputTokens: s.usage.CompletionTokens,
			},
		})
		events = append(events, &llm.Event{
			Type: llm.EventTypeMessageStop,
		})
	}

	return events, nil
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
}
