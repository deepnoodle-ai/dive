package main

import (
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestHandleCompaction(t *testing.T) {
	// Create a mock agent (we won't use it for this test)
	agent := &dive.Agent{}

	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", nil)

	// Create a compaction event
	event := &compaction.CompactionEvent{
		TokensBefore:      100000,
		TokensAfter:       5000,
		Summary:           "Test summary",
		MessagesCompacted: 50,
	}

	// Handle the compaction
	app.handleCompaction(event)

	// Verify state was updated correctly
	assert.NotNil(t, app.lastCompactionEvent, "lastCompactionEvent should be set")
	assert.Equal(t, event, app.lastCompactionEvent, "lastCompactionEvent should match the event")
	assert.True(t, app.showCompactionStats, "showCompactionStats should be true")

	// Verify the event values
	assert.Equal(t, 100000, app.lastCompactionEvent.TokensBefore)
	assert.Equal(t, 5000, app.lastCompactionEvent.TokensAfter)
	assert.Equal(t, 50, app.lastCompactionEvent.MessagesCompacted)

	// Verify timestamps are recent
	assert.True(t, time.Since(app.compactionEventTime) < time.Second,
		"compactionEventTime should be recent")
	assert.True(t, time.Since(app.compactionStatsStartTime) < time.Second,
		"compactionStatsStartTime should be recent")
}
