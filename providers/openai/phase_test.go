package openai

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

// decodeOutputMessage decodes a single-message response carrying the given
// phase fragment, which callers supply as either `"phase":"...",` or empty.
func decodeOutputMessage(t *testing.T, phaseField string) *llm.Response {
	t.Helper()
	var response responses.Response
	payload := `{
		"id": "resp_1",
		"model": "gpt-5.6-sol",
		"status": "completed",
		"output": [{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"status": "completed",
			` + phaseField + `
			"content": [{"type": "output_text", "text": "Checking that now.", "annotations": []}]
		}]
	}`
	assert.NoError(t, json.Unmarshal([]byte(payload), &response))
	decoded, err := decodeAssistantResponse(&response)
	assert.NoError(t, err)
	return decoded
}

func replayJSON(t *testing.T, messages ...*llm.Message) string {
	t.Helper()
	replayed, err := encodeMessages(messages)
	assert.NoError(t, err)
	data, err := json.Marshal(replayed)
	assert.NoError(t, err)
	return string(data)
}

func TestOutputMessagePhaseRoundTrip(t *testing.T) {
	for _, phase := range []string{"commentary", "final_answer"} {
		t.Run(phase, func(t *testing.T) {
			decoded := decodeOutputMessage(t, `"phase":"`+phase+`",`)

			text, ok := decoded.Content[0].(*llm.TextContent)
			assert.True(t, ok)
			assert.Equal(t, phase, text.Metadata[openAIPhaseMetadataKey])

			assert.Contains(t, replayJSON(t, decoded.Message()), `"phase":"`+phase+`"`)
		})
	}
}

// TestOutputMessagePhaseSurvivesPersistence covers the durable-runtime path:
// the decoded message is serialized, reloaded, and only then replayed.
func TestOutputMessagePhaseSurvivesPersistence(t *testing.T) {
	decoded := decodeOutputMessage(t, `"phase":"commentary",`)

	stored, err := json.Marshal(decoded.Message())
	assert.NoError(t, err)

	var reloaded llm.Message
	assert.NoError(t, json.Unmarshal(stored, &reloaded))

	text, ok := reloaded.Content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "commentary", text.Metadata[openAIPhaseMetadataKey])

	assert.Contains(t, replayJSON(t, &reloaded), `"phase":"commentary"`)
}

func TestOutputMessagePhaseSurvivesCopy(t *testing.T) {
	decoded := decodeOutputMessage(t, `"phase":"final_answer",`)

	copied := decoded.Message().Copy()
	text, ok := copied.Content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "final_answer", text.Metadata[openAIPhaseMetadataKey])

	// The copy must own its metadata rather than aliasing the original.
	text.Metadata[openAIPhaseMetadataKey] = "commentary"
	original := decoded.Content[0].(*llm.TextContent)
	assert.Equal(t, "final_answer", original.Metadata[openAIPhaseMetadataKey])
}

// TestOutputMessageWithoutPhaseStaysUnphased locks in that Dive never invents a
// phase for a message OpenAI left unlabeled.
func TestOutputMessageWithoutPhaseStaysUnphased(t *testing.T) {
	decoded := decodeOutputMessage(t, "")

	text, ok := decoded.Content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, 0, len(text.Metadata))

	assert.NotContains(t, replayJSON(t, decoded.Message()), `"phase"`)
}

// TestMixedPhasesPreservedPerOutputMessage covers a response whose output items
// carry different phases: commentary, a tool call, then the final answer. Each
// replayed message must keep the phase of the item it came from.
func TestMixedPhasesPreservedPerOutputMessage(t *testing.T) {
	var response responses.Response
	assert.NoError(t, json.Unmarshal([]byte(`{
		"id": "resp_1",
		"model": "gpt-5.6-sol",
		"status": "completed",
		"output": [
			{"id":"rs_1","type":"reasoning","status":"completed","summary":[],"encrypted_content":"enc"},
			{"id":"msg_1","type":"message","role":"assistant","status":"completed","phase":"commentary",
			 "content":[{"type":"output_text","text":"Checking that now.","annotations":[]}]},
			{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}","status":"completed"},
			{"id":"msg_2","type":"message","role":"assistant","status":"completed","phase":"final_answer",
			 "content":[{"type":"output_text","text":"The answer is 42.","annotations":[]}]}
		]
	}`), &response))

	decoded, err := decodeAssistantResponse(&response)
	assert.NoError(t, err)

	phases := map[string]string{}
	for _, content := range decoded.Content {
		if text, ok := content.(*llm.TextContent); ok {
			phases[text.Text] = text.Metadata[openAIPhaseMetadataKey]
		}
	}
	assert.Equal(t, "commentary", phases["Checking that now."])
	assert.Equal(t, "final_answer", phases["The answer is 42."])

	// Both phases must reach the wire, each attached to its own output message.
	var replayed []map[string]any
	assert.NoError(t, json.Unmarshal([]byte(replayJSON(t, decoded.Message())), &replayed))

	byPhase := map[string]string{}
	for _, item := range replayed {
		if item["type"] != "message" {
			continue
		}
		content := item["content"].([]any)[0].(map[string]any)
		byPhase[item["phase"].(string)] = content["text"].(string)
	}
	assert.Equal(t, 2, len(byPhase))
	assert.Equal(t, "Checking that now.", byPhase["commentary"])
	assert.Equal(t, "The answer is 42.", byPhase["final_answer"])
}

// TestUserMessagesNeverCarryPhase guards the rule that phase belongs only to
// assistant output messages, even if a user block somehow carries the metadata.
func TestUserMessagesNeverCarryPhase(t *testing.T) {
	userMessage := &llm.Message{
		Role: llm.User,
		Content: []llm.Content{
			&llm.TextContent{
				Text:     "hello",
				Metadata: llm.ProviderMetadata{openAIPhaseMetadataKey: "commentary"},
			},
		},
	}
	assert.NotContains(t, replayJSON(t, userMessage), `"phase"`)
}

func streamPhase(t *testing.T, addedPhase, donePhase string) (*llm.Message, []*llm.Event) {
	t.Helper()
	payloads := []string{
		`{"type":"response.created","sequence_number":1,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"in_progress"}}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","status":"in_progress"` + addedPhase + `,"content":[]}}`,
		`{"type":"response.content_part.added","sequence_number":3,"item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Checking that now."}`,
		`{"type":"response.output_text.done","sequence_number":5,"item_id":"msg_1","output_index":0,"content_index":0,"text":"Checking that now."}`,
		`{"type":"response.output_item.done","sequence_number":6,"output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","status":"completed"` + donePhase + `,"content":[{"type":"output_text","text":"Checking that now.","annotations":[]}]}}`,
		`{"type":"response.completed","sequence_number":7,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
	}
	events := make([]responses.ResponseStreamEventUnion, 0, len(payloads))
	for _, payload := range payloads {
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal([]byte(payload), &event))
		events = append(events, event)
	}

	iterator := newOpenAIStreamIterator(&mockStreamSource{events: events}, &llm.Config{}, nil)
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })

	accumulator := llm.NewResponseAccumulator()
	var streamed []*llm.Event
	for iterator.Next() {
		event := iterator.Event()
		streamed = append(streamed, event)
		assert.NoError(t, accumulator.AddEvent(event))
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())
	return accumulator.Response().Message(), streamed
}

// streamPhaseEvents returns only the accumulated message for tests that do not
// inspect the event stream itself.
func streamPhaseEvents(t *testing.T, addedPhase, donePhase string) *llm.Message {
	t.Helper()
	message, _ := streamPhase(t, addedPhase, donePhase)
	return message
}

// TestStreamingPhaseFromOutputItemDone proves that reading phase only from the
// added event is insufficient: OpenAI may label the message just on done.
func TestStreamingPhaseFromOutputItemDone(t *testing.T) {
	message := streamPhaseEvents(t, "", `,"phase":"commentary"`)

	text, ok := message.Content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "Checking that now.", text.Text)
	assert.Equal(t, "commentary", text.Metadata[openAIPhaseMetadataKey])

	assert.Contains(t, replayJSON(t, message), `"phase":"commentary"`)
}

func TestStreamingPhaseFromOutputItemAdded(t *testing.T) {
	message := streamPhaseEvents(t, `,"phase":"final_answer"`, "")

	text, ok := message.Content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "final_answer", text.Metadata[openAIPhaseMetadataKey])

	assert.Contains(t, replayJSON(t, message), `"phase":"final_answer"`)
}

// TestStreamingDonePhaseWins covers a message relabeled between added and done.
func TestStreamingDonePhaseWins(t *testing.T) {
	message := streamPhaseEvents(t, `,"phase":"commentary"`, `,"phase":"final_answer"`)

	text := message.Content[0].(*llm.TextContent)
	assert.Equal(t, "final_answer", text.Metadata[openAIPhaseMetadataKey])
}

func TestStreamingWithoutPhaseStaysUnphased(t *testing.T) {
	message := streamPhaseEvents(t, "", "")

	text := message.Content[0].(*llm.TextContent)
	assert.Equal(t, 0, len(text.Metadata))
	assert.NotContains(t, replayJSON(t, message), `"phase"`)
}

// phaseOfDelta returns the phase carried by a metadata delta, or "" for any
// other event, so ordering assertions can read the stream positionally.
func phaseOfDelta(event *llm.Event) string {
	if event.Type != llm.EventTypeContentBlockDelta || event.Delta == nil {
		return ""
	}
	if event.Delta.Type != llm.EventDeltaTypeMetadata {
		return ""
	}
	return event.Delta.Metadata[openAIPhaseMetadataKey]
}

// TestStreamingLatePhaseArrivesBeforeBlockStop guards the event contract for
// consumers that rebuild messages from the stream: a phase OpenAI only reveals
// on the done event must reach the text block while it is still open, or those
// consumers finalize the block and drop it.
func TestStreamingLatePhaseArrivesBeforeBlockStop(t *testing.T) {
	_, events := streamPhase(t, "", `,"phase":"commentary"`)

	phaseIndex, stopIndex := -1, -1
	for i, event := range events {
		if phaseOfDelta(event) == "commentary" {
			assert.Equal(t, -1, phaseIndex, "phase delta must be emitted once")
			phaseIndex = i
		}
		if event.Type == llm.EventTypeContentBlockStop {
			assert.Equal(t, -1, stopIndex, "text block must stop once")
			stopIndex = i
		}
	}
	assert.True(t, phaseIndex >= 0, "expected a phase metadata delta")
	assert.True(t, stopIndex >= 0, "expected a content block stop")
	assert.True(t, phaseIndex < stopIndex, "phase delta must precede content_block_stop")
}

// TestStreamingEarlyPhaseSkipsRedundantDelta locks in that a message labeled on
// the added event announces its phase once, on the block's start event.
func TestStreamingEarlyPhaseSkipsRedundantDelta(t *testing.T) {
	_, events := streamPhase(t, `,"phase":"final_answer"`, `,"phase":"final_answer"`)

	var starts, deltas int
	for _, event := range events {
		if event.Type == llm.EventTypeContentBlockStart && event.ContentBlock != nil {
			if event.ContentBlock.Metadata[openAIPhaseMetadataKey] == "final_answer" {
				starts++
			}
		}
		if phaseOfDelta(event) != "" {
			deltas++
		}
	}
	assert.Equal(t, 1, starts)
	assert.Equal(t, 0, deltas)
}

// TestStreamingRelabeledPhaseEmitsSingleDelta covers a message relabeled between
// added and done: the correction reaches consumers as one in-block delta.
func TestStreamingRelabeledPhaseEmitsSingleDelta(t *testing.T) {
	message, events := streamPhase(t, `,"phase":"commentary"`, `,"phase":"final_answer"`)

	var phases []string
	for _, event := range events {
		if phase := phaseOfDelta(event); phase != "" {
			phases = append(phases, phase)
		}
	}
	assert.Equal(t, []string{"final_answer"}, phases)
	assert.Equal(t, "final_answer", message.Content[0].(*llm.TextContent).Metadata[openAIPhaseMetadataKey])
}
