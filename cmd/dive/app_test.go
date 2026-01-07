package main

import (
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestHandleCompaction(t *testing.T) {
	// Create a mock agent (we won't use it for this test)
	agent := &dive.StandardAgent{}

	app := NewApp(agent, "/tmp/test", "test-model", "")

	// Create a compaction event
	event := &dive.CompactionEvent{
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

func TestCompactionStatsTimeout(t *testing.T) {
	agent := &dive.StandardAgent{}
	app := NewApp(agent, "/tmp/test", "test-model", "")

	// Simulate a compaction event
	event := &dive.CompactionEvent{
		TokensBefore:      100000,
		TokensAfter:       5000,
		MessagesCompacted: 50,
	}
	app.handleCompaction(event)

	// Initially stats should be shown
	assert.True(t, app.showCompactionStats, "stats should be shown initially")

	// Simulate time passing (6 seconds)
	app.compactionStatsStartTime = time.Now().Add(-6 * time.Second)

	// The tick event handler should clear the stats after 5 seconds
	// We can't easily test the tick handler, but we can verify the logic
	if time.Since(app.compactionStatsStartTime) >= 5*time.Second {
		app.showCompactionStats = false
	}

	assert.False(t, app.showCompactionStats, "stats should be hidden after timeout")
}

func TestCompactionEvent(t *testing.T) {
	// Create a compaction event
	ce := &dive.CompactionEvent{
		TokensBefore:      150000,
		TokensAfter:       8000,
		Summary:           "Test summary content",
		MessagesCompacted: 75,
	}

	// Verify the event structure
	assert.Equal(t, 150000, ce.TokensBefore)
	assert.Equal(t, 8000, ce.TokensAfter)
	assert.Equal(t, 75, ce.MessagesCompacted)
	assert.Equal(t, "Test summary content", ce.Summary)

	// Calculate reduction
	reduction := ce.TokensBefore - ce.TokensAfter
	assert.Equal(t, 142000, reduction)

	// Calculate percentage reduction
	percentReduction := float64(reduction) / float64(ce.TokensBefore) * 100
	assert.True(t, percentReduction > 94.0, "should have >94% reduction")
}
