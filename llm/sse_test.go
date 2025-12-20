package llm

import (
	"os"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestServerSentEventsReader(t *testing.T) {
	data, err := os.Open("../providers/openai/fixtures/events-hello.txt")
	assert.NoError(t, err)

	reader := NewServerSentEventsReader[map[string]any](data)

	var events []map[string]any

	for {
		event, ok := reader.Next()
		if !ok {
			break
		}
		events = append(events, event)
	}

	assert.Equal(t, 17, len(events))
	ev0 := events[0]
	assert.Equal(t, "response.created", ev0["type"])
	assert.Equal(t, float64(0), ev0["sequence_number"])

	ev1 := events[1]
	assert.Equal(t, "response.in_progress", ev1["type"])
	assert.Equal(t, float64(1), ev1["sequence_number"])

	ev2 := events[2]
	assert.Equal(t, "response.output_item.added", ev2["type"])
	assert.Equal(t, float64(2), ev2["sequence_number"])
}
