package openai

// // mockEventSource simulates an SSE stream by providing events one by one
// type mockEventSource struct {
// 	events []string
// 	index  int
// 	closed bool
// }

// func newMockEventSource(events []string) *mockEventSource {
// 	return &mockEventSource{
// 		events: events,
// 		index:  0,
// 		closed: false,
// 	}
// }

// func (m *mockEventSource) next() (string, bool) {
// 	if m.closed || m.index >= len(m.events) {
// 		return "", false
// 	}
// 	event := m.events[m.index]
// 	m.index++
// 	return event, true
// }

// func (m *mockEventSource) close() {
// 	m.closed = true
// }

// // mockSSEStream simulates the ssestream.Stream interface for testing
// type mockSSEStream struct {
// 	eventSource *mockEventSource
// }

// func newMockSSEStream(events []string) *mockSSEStream {
// 	return &mockSSEStream{
// 		eventSource: newMockEventSource(events),
// 	}
// }

// func (m *mockSSEStream) Next() bool {
// 	_, hasNext := m.eventSource.next()
// 	return hasNext
// }

// func (m *mockSSEStream) Current() responses.ResponseStreamEventUnion {
// 	// Get the current event (we need to back up one since Next() advances)
// 	if m.eventSource.index > 0 {
// 		eventData := m.eventSource.events[m.eventSource.index-1]

// 		// Parse the SSE format to extract the JSON data
// 		lines := strings.Split(eventData, "\n")
// 		var jsonData string
// 		var eventType string
// 		for _, line := range lines {
// 			if strings.HasPrefix(line, "event: ") {
// 				eventType = strings.TrimPrefix(line, "event: ")
// 			} else if strings.HasPrefix(line, "data: ") {
// 				jsonData = strings.TrimPrefix(line, "data: ")
// 			}
// 		}

// 		if jsonData != "" && eventType != "" {
// 			// Create the appropriate event type based on the event type
// 			switch eventType {
// 			case "response.created":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					if response, ok := data["response"].(map[string]interface{}); ok {
// 						return responses.ResponseStreamEventUnion{
// 							Type:           eventType,
// 							SequenceNumber: int64(data["sequence_number"].(float64)),
// 							Response: responses.Response{
// 								ID:     response["id"].(string),
// 								Model:  response["model"].(string),
// 								Status: response["status"].(string),
// 								Output: []responses.ResponseOutputItemUnion{},
// 							},
// 						}
// 					}
// 				}

// 			case "response.output_item.added":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					outputIndex := int64(data["output_index"].(float64))
// 					if item, ok := data["item"].(map[string]interface{}); ok {
// 						return responses.ResponseStreamEventUnion{
// 							Type:           eventType,
// 							SequenceNumber: int64(data["sequence_number"].(float64)),
// 							OutputIndex:    outputIndex,
// 							Item: responses.ResponseOutputItemUnion{
// 								ResponseOutputItemMessage: &responses.ResponseOutputItemMessage{
// 									ID:      item["id"].(string),
// 									Type:    "message",
// 									Role:    "assistant",
// 									Content: []responses.ResponseOutputContentUnion{},
// 								},
// 							},
// 						}
// 					}
// 				}

// 			case "response.content_part.added":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					outputIndex := int64(data["output_index"].(float64))
// 					contentIndex := int64(data["content_index"].(float64))
// 					if part, ok := data["part"].(map[string]interface{}); ok {
// 						return responses.ResponseStreamEventUnion{
// 							Type:           eventType,
// 							SequenceNumber: int64(data["sequence_number"].(float64)),
// 							OutputIndex:    outputIndex,
// 							ContentIndex:   contentIndex,
// 							ItemID:         data["item_id"].(string),
// 							Part: responses.ResponseStreamEventUnionPart{
// 								ResponseContentPartAddedEventPart: &responses.ResponseContentPartAddedEventPart{
// 									Type: part["type"].(string),
// 									Text: part["text"].(string),
// 								},
// 							},
// 						}
// 					}
// 				}

// 			case "response.output_text.delta":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					outputIndex := int64(data["output_index"].(float64))
// 					contentIndex := int64(data["content_index"].(float64))
// 					delta := data["delta"].(string)
// 					return responses.ResponseStreamEventUnion{
// 						Type:           eventType,
// 						SequenceNumber: int64(data["sequence_number"].(float64)),
// 						OutputIndex:    outputIndex,
// 						ContentIndex:   contentIndex,
// 						ItemID:         data["item_id"].(string),
// 						Delta: responses.ResponseStreamEventUnionDelta{
// 							String: &delta,
// 						},
// 					}
// 				}

// 			case "response.output_text.done":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					outputIndex := int64(data["output_index"].(float64))
// 					contentIndex := int64(data["content_index"].(float64))
// 					text := data["text"].(string)
// 					return responses.ResponseStreamEventUnion{
// 						Type:           eventType,
// 						SequenceNumber: int64(data["sequence_number"].(float64)),
// 						OutputIndex:    outputIndex,
// 						ContentIndex:   contentIndex,
// 						ItemID:         data["item_id"].(string),
// 						Text:           text,
// 					}
// 				}

// 			case "response.content_part.done":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					outputIndex := int64(data["output_index"].(float64))
// 					contentIndex := int64(data["content_index"].(float64))
// 					if part, ok := data["part"].(map[string]interface{}); ok {
// 						return responses.ResponseStreamEventUnion{
// 							Type:           eventType,
// 							SequenceNumber: int64(data["sequence_number"].(float64)),
// 							OutputIndex:    outputIndex,
// 							ContentIndex:   contentIndex,
// 							ItemID:         data["item_id"].(string),
// 							Part: responses.ResponseStreamEventUnionPart{
// 								ResponseContentPartDoneEventPart: &responses.ResponseContentPartDoneEventPart{
// 									Type: part["type"].(string),
// 									Text: part["text"].(string),
// 								},
// 							},
// 						}
// 					}
// 				}

// 			case "response.output_item.done":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					outputIndex := int64(data["output_index"].(float64))
// 					if item, ok := data["item"].(map[string]interface{}); ok {
// 						content := []responses.ResponseOutputContentUnion{}
// 						if itemContent, ok := item["content"].([]interface{}); ok {
// 							for _, c := range itemContent {
// 								if contentMap, ok := c.(map[string]interface{}); ok {
// 									content = append(content, responses.ResponseOutputContentUnion{
// 										ResponseOutputContentOutputText: &responses.ResponseOutputContentOutputText{
// 											Type: contentMap["type"].(string),
// 											Text: contentMap["text"].(string),
// 										},
// 									})
// 								}
// 							}
// 						}
// 						return responses.ResponseStreamEventUnion{
// 							Type:           eventType,
// 							SequenceNumber: int64(data["sequence_number"].(float64)),
// 							OutputIndex:    outputIndex,
// 							Item: responses.ResponseOutputItemUnion{
// 								ResponseOutputItemMessage: &responses.ResponseOutputItemMessage{
// 									ID:      item["id"].(string),
// 									Type:    item["type"].(string),
// 									Status:  item["status"].(string),
// 									Role:    item["role"].(string),
// 									Content: content,
// 								},
// 							},
// 						}
// 					}
// 				}

// 			case "response.completed":
// 				var data map[string]interface{}
// 				if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
// 					if response, ok := data["response"].(map[string]interface{}); ok {
// 						output := []responses.ResponseOutputItemUnion{}
// 						if responseOutput, ok := response["output"].([]interface{}); ok {
// 							for _, o := range responseOutput {
// 								if outputMap, ok := o.(map[string]interface{}); ok {
// 									content := []responses.ResponseOutputContentUnion{}
// 									if outputContent, ok := outputMap["content"].([]interface{}); ok {
// 										for _, c := range outputContent {
// 											if contentMap, ok := c.(map[string]interface{}); ok {
// 												content = append(content, responses.ResponseOutputContentUnion{
// 													ResponseOutputContentOutputText: &responses.ResponseOutputContentOutputText{
// 														Type: contentMap["type"].(string),
// 														Text: contentMap["text"].(string),
// 													},
// 												})
// 											}
// 										}
// 									}
// 									output = append(output, responses.ResponseOutputItemUnion{
// 										ResponseOutputItemMessage: &responses.ResponseOutputItemMessage{
// 											ID:      outputMap["id"].(string),
// 											Type:    outputMap["type"].(string),
// 											Status:  outputMap["status"].(string),
// 											Role:    outputMap["role"].(string),
// 											Content: content,
// 										},
// 									})
// 								}
// 							}
// 						}

// 						var usage *responses.Usage
// 						if responseUsage, ok := response["usage"].(map[string]interface{}); ok {
// 							usage = &responses.Usage{
// 								InputTokens:  int64(responseUsage["input_tokens"].(float64)),
// 								OutputTokens: int64(responseUsage["output_tokens"].(float64)),
// 								TotalTokens:  int64(responseUsage["total_tokens"].(float64)),
// 							}
// 						}

// 						return responses.ResponseStreamEventUnion{
// 							Type:           eventType,
// 							SequenceNumber: int64(data["sequence_number"].(float64)),
// 							Response: responses.Response{
// 								ID:     response["id"].(string),
// 								Model:  response["model"].(string),
// 								Status: response["status"].(string),
// 								Output: output,
// 								Usage:  usage,
// 							},
// 						}
// 					}
// 				}
// 			}
// 		}
// 	}

// 	// Return a default event if parsing fails
// 	return responses.ResponseStreamEventUnion{}
// }

// func (m *mockSSEStream) Err() error {
// 	return nil
// }

// func (m *mockSSEStream) Close() error {
// 	m.eventSource.close()
// 	return nil
// }

// func TestStreamIterator_BasicTextResponse(t *testing.T) {
// 	// Simplified events for a basic "Hello!" text response
// 	events := []string{
// 		`event: response.created
// data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_123","model":"gpt-4o","status":"in_progress","output":[],"usage":null}}`,

// 		`event: response.output_item.added
// data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_123","type":"message","status":"in_progress","content":[],"role":"assistant"}}`,

// 		`event: response.content_part.added
// data: {"type":"response.content_part.added","sequence_number":2,"item_id":"msg_123","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,

// 		`event: response.output_text.delta
// data: {"type":"response.output_text.delta","sequence_number":3,"item_id":"msg_123","output_index":0,"content_index":0,"delta":"Hello"}`,

// 		`event: response.output_text.delta
// data: {"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_123","output_index":0,"content_index":0,"delta":"!"}`,

// 		`event: response.output_text.done
// data: {"type":"response.output_text.done","sequence_number":5,"item_id":"msg_123","output_index":0,"content_index":0,"text":"Hello!"}`,

// 		`event: response.content_part.done
// data: {"type":"response.content_part.done","sequence_number":6,"item_id":"msg_123","output_index":0,"content_index":0,"part":{"type":"output_text","text":"Hello!"}}`,

// 		`event: response.output_item.done
// data: {"type":"response.output_item.done","sequence_number":7,"output_index":0,"item":{"id":"msg_123","type":"message","status":"completed","content":[{"type":"output_text","text":"Hello!"}],"role":"assistant"}}`,

// 		`event: response.completed
// data: {"type":"response.completed","sequence_number":8,"response":{"id":"resp_123","model":"gpt-4o","status":"completed","output":[{"id":"msg_123","type":"message","status":"completed","content":[{"type":"output_text","text":"Hello!"}],"role":"assistant"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`,
// 	}

// 	// Create a mock stream
// 	mockStream := newMockSSEStream(events)

// 	// Create the stream iterator
// 	config := &llm.Config{}
// 	iterator := newOpenAIStreamIterator(mockStream, config)

// 	// Collect all events
// 	var receivedEvents []*llm.Event
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		receivedEvents = append(receivedEvents, event)
// 	}

// 	require.NoError(t, iterator.Err())

// 	// Verify the sequence of events
// 	require.GreaterOrEqual(t, len(receivedEvents), 5) // At least: start, content_block_start, deltas, content_block_stop, message_stop

// 	// Check that we get a message_start event first
// 	require.Equal(t, llm.EventTypeMessageStart, receivedEvents[0].Type)
// 	require.NotNil(t, receivedEvents[0].Message)
// 	require.Equal(t, "resp_123", receivedEvents[0].Message.ID)

// 	// Find content block events
// 	var contentBlockStartFound, contentBlockDeltaFound, contentBlockStopFound, messageStopFound bool
// 	var accumulatedText strings.Builder

// 	for _, event := range receivedEvents {
// 		switch event.Type {
// 		case llm.EventTypeContentBlockStart:
// 			contentBlockStartFound = true
// 			require.NotNil(t, event.ContentBlock)
// 			require.Equal(t, "text", event.ContentBlock.Type)

// 		case llm.EventTypeContentBlockDelta:
// 			contentBlockDeltaFound = true
// 			require.NotNil(t, event.Delta)
// 			require.Equal(t, llm.EventDeltaTypeText, event.Delta.Type)
// 			require.NotEmpty(t, event.Delta.Text)
// 			accumulatedText.WriteString(event.Delta.Text)

// 		case llm.EventTypeContentBlockStop:
// 			contentBlockStopFound = true

// 		case llm.EventTypeMessageStop:
// 			messageStopFound = true
// 		}
// 	}

// 	require.True(t, contentBlockStartFound, "Expected content_block_start event")
// 	require.True(t, contentBlockDeltaFound, "Expected content_block_delta event")
// 	require.True(t, contentBlockStopFound, "Expected content_block_stop event")
// 	require.True(t, messageStopFound, "Expected message_stop event")
// 	require.Equal(t, "Hello!", accumulatedText.String())
// }

// func TestStreamIterator_FunctionCallResponse(t *testing.T) {
// 	// Events for a function call response
// 	events := []string{
// 		`event: response.created
// data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_456","model":"gpt-4o","status":"in_progress","output":[],"usage":null}}`,

// 		`event: response.output_item.added
// data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"fc_456","type":"function_call","name":"get_weather","arguments":"","call_id":"call_456"}}`,

// 		`event: response.function_call_arguments.delta
// data: {"type":"response.function_call_arguments.delta","sequence_number":2,"item_id":"fc_456","output_index":0,"delta":"{\"location\""}`,

// 		`event: response.function_call_arguments.delta
// data: {"type":"response.function_call_arguments.delta","sequence_number":3,"item_id":"fc_456","output_index":0,"delta":":\"Paris\"}"}`,

// 		`event: response.function_call_arguments.done
// data: {"type":"response.function_call_arguments.done","sequence_number":4,"item_id":"fc_456","output_index":0,"arguments":"{\"location\":\"Paris\"}"}`,

// 		`event: response.output_item.done
// data: {"type":"response.output_item.done","sequence_number":5,"output_index":0,"item":{"id":"fc_456","type":"function_call","name":"get_weather","arguments":"{\"location\":\"Paris\"}","call_id":"call_456"}}`,

// 		`event: response.completed
// data: {"type":"response.completed","sequence_number":6,"response":{"id":"resp_456","model":"gpt-4o","status":"completed","output":[{"id":"fc_456","type":"function_call","name":"get_weather","arguments":"{\"location\":\"Paris\"}","call_id":"call_456"}],"usage":{"input_tokens":15,"output_tokens":8,"total_tokens":23}}}`,
// 	}

// 	// Create a mock stream
// 	mockStream := newMockSSEStream(events)

// 	// Create the stream iterator
// 	config := &llm.Config{}
// 	iterator := newOpenAIStreamIterator(mockStream, config)

// 	// Collect all events
// 	var receivedEvents []*llm.Event
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		receivedEvents = append(receivedEvents, event)
// 	}

// 	require.NoError(t, iterator.Err())

// 	// Verify we get the expected events for tool calls
// 	var messageStartFound, contentBlockStartFound, contentBlockDeltaFound, contentBlockStopFound, messageStopFound bool
// 	var accumulatedJson strings.Builder

// 	for _, event := range receivedEvents {
// 		switch event.Type {
// 		case llm.EventTypeMessageStart:
// 			messageStartFound = true
// 			require.NotNil(t, event.Message)
// 			require.Equal(t, "resp_456", event.Message.ID)

// 		case llm.EventTypeContentBlockStart:
// 			contentBlockStartFound = true
// 			require.NotNil(t, event.ContentBlock)
// 			require.Equal(t, llm.ContentTypeToolUse, event.ContentBlock.Type)
// 			require.Equal(t, "get_weather", event.ContentBlock.Name)

// 		case llm.EventTypeContentBlockDelta:
// 			contentBlockDeltaFound = true
// 			require.NotNil(t, event.Delta)
// 			require.Equal(t, llm.EventDeltaTypeInputJSON, event.Delta.Type)
// 			accumulatedJson.WriteString(event.Delta.PartialJSON)

// 		case llm.EventTypeContentBlockStop:
// 			contentBlockStopFound = true

// 		case llm.EventTypeMessageStop:
// 			messageStopFound = true
// 		}
// 	}

// 	require.True(t, messageStartFound, "Expected message_start event")
// 	require.True(t, contentBlockStartFound, "Expected content_block_start event for tool use")
// 	require.True(t, contentBlockDeltaFound, "Expected content_block_delta event for tool arguments")
// 	require.True(t, contentBlockStopFound, "Expected content_block_stop event")
// 	require.True(t, messageStopFound, "Expected message_stop event")
// 	require.Equal(t, `{"location":"Paris"}`, accumulatedJson.String())
// }

// func TestStreamIterator_ErrorHandling(t *testing.T) {
// 	// Test error event handling
// 	events := []string{
// 		`event: response.created
// data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_error","model":"gpt-4o","status":"in_progress","output":[],"usage":null}}`,

// 		`event: error
// data: {"type":"error","sequence_number":1,"code":"invalid_request","message":"Invalid request","param":"messages"}`,
// 	}

// 	// Create a mock stream
// 	mockStream := newMockSSEStream(events)

// 	// Create the stream iterator
// 	config := &llm.Config{}
// 	iterator := newOpenAIStreamIterator(mockStream, config)

// 	// Collect all events - should get message_start then error
// 	var receivedEvents []*llm.Event
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		receivedEvents = append(receivedEvents, event)
// 	}

// 	// Should have an error after processing the error event
// 	require.Error(t, iterator.Err())
// 	require.Contains(t, iterator.Err().Error(), "invalid_request")
// 	require.Contains(t, iterator.Err().Error(), "Invalid request")
// }

// func TestStreamIterator_UsageInformation(t *testing.T) {
// 	// Test that usage information is properly captured
// 	events := []string{
// 		`event: response.created
// data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_usage","model":"gpt-4o","status":"in_progress","output":[],"usage":null}}`,

// 		`event: response.completed
// data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_usage","model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":25,"output_tokens":10,"total_tokens":35}}}`,
// 	}

// 	// Create a mock stream
// 	mockStream := newMockSSEStream(events)

// 	// Create the stream iterator
// 	config := &llm.Config{}
// 	iterator := newOpenAIStreamIterator(mockStream, config)

// 	// Collect all events
// 	var receivedEvents []*llm.Event
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		receivedEvents = append(receivedEvents, event)
// 	}

// 	require.NoError(t, iterator.Err())
// 	require.GreaterOrEqual(t, len(receivedEvents), 2) // At least message_start and message_stop

// 	// Check that the last events have usage information
// 	var messageDeltaEvent *llm.Event
// 	for _, event := range receivedEvents {
// 		if event.Type == llm.EventTypeMessageDelta {
// 			messageDeltaEvent = event
// 		}
// 	}

// 	require.NotNil(t, messageDeltaEvent, "Expected message_delta event with stop reason")
// 	require.NotNil(t, messageDeltaEvent.Usage)
// 	require.Equal(t, 25, messageDeltaEvent.Usage.InputTokens)
// 	require.Equal(t, 10, messageDeltaEvent.Usage.OutputTokens)
// }
