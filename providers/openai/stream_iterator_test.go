package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

// mockStreamSource replays pre-parsed SDK events through the StreamSource
// interface used by the iterator.
type mockStreamSource struct {
	events []responses.ResponseStreamEventUnion
	pos    int
}

func (m *mockStreamSource) Next() bool {
	if m.pos < len(m.events) {
		m.pos++
		return true
	}
	return false
}

func (m *mockStreamSource) Current() responses.ResponseStreamEventUnion {
	return m.events[m.pos-1]
}

func (m *mockStreamSource) Err() error { return nil }

func (m *mockStreamSource) Close() error { return nil }

// loadFixtureEvents parses an SSE fixture file into SDK stream events.
func loadFixtureEvents(t *testing.T, path string) []responses.ResponseStreamEventUnion {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	var events []responses.ResponseStreamEventUnion
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event))
		events = append(events, event)
	}
	assert.NoError(t, scanner.Err())
	assert.NotEmpty(t, events)
	return events
}

// TestStreamIteratorZeroBasedIndices verifies that content block events carry
// the zero-based output_index from the OpenAI Responses API as-is. The fixture
// stream has a single message item at output_index 0, so every indexed event
// must carry index 0 (never -1).
func TestStreamIteratorZeroBasedIndices(t *testing.T) {
	source := &mockStreamSource{events: loadFixtureEvents(t, "fixtures/events-hello.txt")}
	iterator := newOpenAIStreamIterator(source, &llm.Config{})
	defer iterator.Close()

	accumulator := llm.NewResponseAccumulator()
	var indexedEventCount int
	var firstBlockStartIndex *int
	for iterator.Next() {
		event := iterator.Event()
		if event.Index != nil {
			indexedEventCount++
			assert.Equal(t, 0, *event.Index)
		}
		if event.Type == llm.EventTypeContentBlockStart && firstBlockStartIndex == nil {
			firstBlockStartIndex = event.Index
		}
		assert.NoError(t, accumulator.AddEvent(event))
	}
	assert.NoError(t, iterator.Err())

	// The first content block start must carry index 0.
	assert.NotNil(t, firstBlockStartIndex)
	assert.Equal(t, 0, *firstBlockStartIndex)
	assert.NotEmpty(t, indexedEventCount)

	// The accumulated response should match the fixture.
	assert.True(t, accumulator.IsComplete())
	response := accumulator.Response()
	assert.Equal(t, "Hello! How can I assist you today?", response.Message().Text())
	assert.Equal(t, 140, response.Usage.InputTokens)
	assert.Equal(t, 11, response.Usage.OutputTokens)
}
