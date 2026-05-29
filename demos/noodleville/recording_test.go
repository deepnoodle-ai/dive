package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestRunRecorderWritesHeaderAndTicks(t *testing.T) {
	ctx := context.Background()
	town, err := NewTown(ctx, TownOptions{
		Model:         NewScriptedPlanner(1 * time.Millisecond),
		SessionDir:    t.TempDir(),
		Parallelism:   2,
		VillagerCount: 5,
		SeedParty:     true,
	})
	assert.NoError(t, err)

	path := filepath.Join(t.TempDir(), "run.jsonl")
	recorder, err := NewRunRecorder(path, RunMetadata{
		Provider:    "scripted",
		Model:       "scripted-noodleville",
		Villagers:   5,
		Parallelism: 2,
		SeedParty:   true,
	}, town.Snapshot())
	assert.NoError(t, err)

	report, err := town.RunTick(ctx)
	assert.NoError(t, err)
	assert.NoError(t, recorder.RecordTick(report))
	assert.NoError(t, recorder.Close())

	file, err := os.Open(path)
	assert.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var records []map[string]any
	for scanner.Scan() {
		var record map[string]any
		assert.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	assert.NoError(t, scanner.Err())
	assert.Equal(t, len(records), 2)
	assert.Equal(t, records[0]["type"], "run")
	assert.Equal(t, records[1]["type"], "tick")
	assert.True(t, records[0]["initial_snapshot"] != nil, "expected initial snapshot in run header")
	assert.True(t, records[1]["report"] != nil, "expected tick report in recording")
}
