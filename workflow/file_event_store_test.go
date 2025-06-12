package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileExecutionEventStore(t *testing.T) {
	// Create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "dive-event-store-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	store := NewFileExecutionEventStore(tempDir)
	ctx := context.Background()

	t.Run("AppendEvents and GetEvents", func(t *testing.T) {
		executionID := "test-exec-1"
		events := []*ExecutionEvent{
			{
				ID:          "event-1",
				ExecutionID: executionID,
				PathID:      "path-1",
				Sequence:    1,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
				Data:        map[string]interface{}{"key": "value1"},
			},
			{
				ID:          "event-2",
				ExecutionID: executionID,
				PathID:      "path-1",
				Sequence:    2,
				Timestamp:   time.Now(),
				EventType:   EventStepStarted,
				StepName:    "step1",
				Data:        map[string]interface{}{"key": "value2"},
			},
		}

		// Append events
		err := store.AppendEvents(ctx, events)
		require.NoError(t, err)

		// Get all events
		retrievedEvents, err := store.GetEvents(ctx, executionID, 1)
		require.NoError(t, err)
		require.Len(t, retrievedEvents, 2)

		// Verify event content
		require.Equal(t, events[0].ID, retrievedEvents[0].ID)
		require.Equal(t, events[0].EventType, retrievedEvents[0].EventType)
		require.Equal(t, events[1].StepName, retrievedEvents[1].StepName)

		// Get events from sequence 2
		retrievedEvents, err = store.GetEvents(ctx, executionID, 2)
		require.NoError(t, err)
		require.Len(t, retrievedEvents, 1)
		require.Equal(t, events[1].ID, retrievedEvents[0].ID)
	})

	t.Run("GetEventHistory", func(t *testing.T) {
		executionID := "test-exec-2"
		events := []*ExecutionEvent{
			{
				ID:          "event-3",
				ExecutionID: executionID,
				Sequence:    1,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
			},
		}

		err := store.AppendEvents(ctx, events)
		require.NoError(t, err)

		history, err := store.GetEventHistory(ctx, executionID)
		require.NoError(t, err)
		require.Len(t, history, 1)
		require.Equal(t, events[0].ID, history[0].ID)
	})

	t.Run("SaveSnapshot and GetSnapshot", func(t *testing.T) {
		snapshot := &ExecutionSnapshot{
			ID:           "snap-1",
			WorkflowName: "test-workflow",
			Status:       "completed",
			StartTime:    time.Now(),
			EndTime:      time.Now(),
			LastEventSeq: 10,
			Inputs:       map[string]interface{}{"param": "value"},
			Outputs:      map[string]interface{}{"result": "success"},
		}

		// Save snapshot
		err := store.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)

		// Get snapshot
		retrieved, err := store.GetSnapshot(ctx, snapshot.ID)
		require.NoError(t, err)
		require.Equal(t, snapshot.ID, retrieved.ID)
		require.Equal(t, snapshot.WorkflowName, retrieved.WorkflowName)
		require.Equal(t, snapshot.Status, retrieved.Status)
		require.Equal(t, snapshot.LastEventSeq, retrieved.LastEventSeq)
		require.NotZero(t, retrieved.CreatedAt)
		require.NotZero(t, retrieved.UpdatedAt)
	})

	t.Run("ListExecutions", func(t *testing.T) {
		// Create multiple snapshots
		snapshots := []*ExecutionSnapshot{
			{
				ID:           "list-exec-1",
				WorkflowName: "workflow-a",
				Status:       "completed",
				StartTime:    time.Now(),
				LastEventSeq: 5,
			},
			{
				ID:           "list-exec-2",
				WorkflowName: "workflow-b",
				Status:       "failed",
				StartTime:    time.Now(),
				LastEventSeq: 3,
			},
		}

		for _, snapshot := range snapshots {
			err := store.SaveSnapshot(ctx, snapshot)
			require.NoError(t, err)
		}

		// List all executions
		filter := ExecutionFilter{}
		executions, err := store.ListExecutions(ctx, filter)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(executions), 2)

		// Filter by status
		completedStatus := "completed"
		filter = ExecutionFilter{Status: &completedStatus}
		executions, err = store.ListExecutions(ctx, filter)
		require.NoError(t, err)

		found := false
		for _, exec := range executions {
			if exec.ID == "list-exec-1" {
				found = true
				break
			}
		}
		require.True(t, found, "Should find the completed execution")

		// Test pagination
		filter = ExecutionFilter{Limit: 1, Offset: 0}
		executions, err = store.ListExecutions(ctx, filter)
		require.NoError(t, err)
		require.Len(t, executions, 1)
	})

	t.Run("DeleteExecution", func(t *testing.T) {
		executionID := "delete-test"

		// Create an execution with events and snapshot
		events := []*ExecutionEvent{
			{
				ID:          "delete-event-1",
				ExecutionID: executionID,
				Sequence:    1,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
			},
		}
		err := store.AppendEvents(ctx, events)
		require.NoError(t, err)

		snapshot := &ExecutionSnapshot{
			ID:           executionID,
			WorkflowName: "delete-workflow",
			Status:       "completed",
			LastEventSeq: 1,
		}
		err = store.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)

		// Verify files exist
		execDir := filepath.Join(tempDir, executionID)
		_, err = os.Stat(execDir)
		require.NoError(t, err)

		// Delete execution
		err = store.DeleteExecution(ctx, executionID)
		require.NoError(t, err)

		// Verify files are deleted
		_, err = os.Stat(execDir)
		require.True(t, os.IsNotExist(err))

		// Verify snapshot is not found
		_, err = store.GetSnapshot(ctx, executionID)
		require.Error(t, err)
	})

	t.Run("GetSnapshot_NotFound", func(t *testing.T) {
		_, err := store.GetSnapshot(ctx, "nonexistent")
		require.Error(t, err)
		require.Contains(t, err.Error(), "snapshot not found")
	})

	t.Run("GetEvents_NonexistentExecution", func(t *testing.T) {
		events, err := store.GetEvents(ctx, "nonexistent", 1)
		require.NoError(t, err)
		require.Empty(t, events)
	})
}

func TestFileExecutionEventStore_ConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dive-concurrent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	store := NewFileExecutionEventStore(tempDir)
	ctx := context.Background()
	executionID := "concurrent-test"

	// Test concurrent writes to the same execution
	done := make(chan bool, 2)

	for i := 0; i < 2; i++ {
		go func(workerID int) {
			defer func() { done <- true }()

			for j := 0; j < 10; j++ {
				event := &ExecutionEvent{
					ID:          fmt.Sprintf("worker-%d-event-%d", workerID, j),
					ExecutionID: executionID,
					Sequence:    int64(workerID*10 + j + 1),
					Timestamp:   time.Now(),
					EventType:   EventStepStarted,
				}

				err := store.AppendEvents(ctx, []*ExecutionEvent{event})
				require.NoError(t, err)
			}
		}(i)
	}

	// Wait for both workers to complete
	<-done
	<-done

	// Verify all events were written
	events, err := store.GetEventHistory(ctx, executionID)
	require.NoError(t, err)
	require.Len(t, events, 20)
}
