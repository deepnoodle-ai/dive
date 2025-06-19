package environment

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSQLiteExecutionEventStore(t *testing.T) {
	// Create temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create store with default options
	store, err := NewSQLiteExecutionEventStore(dbPath, DefaultSQLiteStoreOptions())
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Test event operations
	t.Run("AppendAndGetEvents", func(t *testing.T) {
		executionID := "test-execution-1"

		events := []*ExecutionEvent{
			{
				ID:          "event-1",
				ExecutionID: executionID,
				Path:        "path-1",
				Sequence:    1,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
				Data: map[string]interface{}{
					"workflow_name": "test-workflow",
					"inputs":        map[string]interface{}{"key": "value"},
				},
			},
			{
				ID:          "event-2",
				ExecutionID: executionID,
				Path:        "path-1",
				Sequence:    2,
				Timestamp:   time.Now(),
				EventType:   EventStepStarted,
				Step:        "step1",
				Data: map[string]interface{}{
					"step_type": "prompt",
				},
			},
			{
				ID:          "event-3",
				ExecutionID: executionID,
				Path:        "path-1",
				Sequence:    3,
				Timestamp:   time.Now(),
				EventType:   EventStepCompleted,
				Step:        "step1",
				Data: map[string]interface{}{
					"output":          "step output",
					"stored_variable": "var1",
				},
			},
		}

		// Append events
		err := store.AppendEvents(ctx, events)
		require.NoError(t, err)

		// Get all events
		retrievedEvents, err := store.GetEventHistory(ctx, executionID)
		require.NoError(t, err)
		require.Len(t, retrievedEvents, 3)

		// Verify event data
		require.Equal(t, events[0].ID, retrievedEvents[0].ID)
		require.Equal(t, events[0].ExecutionID, retrievedEvents[0].ExecutionID)
		require.Equal(t, events[0].EventType, retrievedEvents[0].EventType)
		require.Equal(t, events[0].Data["workflow_name"], retrievedEvents[0].Data["workflow_name"])

		// Get events from sequence 2
		partialEvents, err := store.GetEvents(ctx, executionID, 2)
		require.NoError(t, err)
		require.Len(t, partialEvents, 2)
		require.Equal(t, int64(2), partialEvents[0].Sequence)
		require.Equal(t, int64(3), partialEvents[1].Sequence)
	})

	// Test snapshot operations
	t.Run("SaveAndGetSnapshot", func(t *testing.T) {
		executionID := "test-execution-2"

		snapshot := &ExecutionSnapshot{
			ID:           executionID,
			WorkflowName: "test-workflow",
			WorkflowHash: "workflow-hash-123",
			InputsHash:   "inputs-hash-456",
			Status:       "running",
			StartTime:    time.Now().Add(-time.Hour),
			CreatedAt:    time.Now().Add(-time.Hour),
			UpdatedAt:    time.Now(),
			LastEventSeq: 5,
			WorkflowData: []byte("workflow data"),
			Inputs: map[string]interface{}{
				"param1": "value1",
				"param2": 42,
			},
			Outputs: map[string]interface{}{
				"result": "success",
			},
		}

		// Save snapshot
		err := store.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)

		// Retrieve snapshot
		retrievedSnapshot, err := store.GetSnapshot(ctx, executionID)
		require.NoError(t, err)

		// Verify snapshot data
		require.Equal(t, snapshot.ID, retrievedSnapshot.ID)
		require.Equal(t, snapshot.WorkflowName, retrievedSnapshot.WorkflowName)
		require.Equal(t, snapshot.WorkflowHash, retrievedSnapshot.WorkflowHash)
		require.Equal(t, snapshot.Status, retrievedSnapshot.Status)
		require.Equal(t, snapshot.LastEventSeq, retrievedSnapshot.LastEventSeq)
		require.Equal(t, snapshot.Inputs["param1"], retrievedSnapshot.Inputs["param1"])
		require.Equal(t, snapshot.Outputs["result"], retrievedSnapshot.Outputs["result"])

		// Test upsert behavior
		snapshot.Status = "completed"
		snapshot.LastEventSeq = 10
		err = store.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)

		updatedSnapshot, err := store.GetSnapshot(ctx, executionID)
		require.NoError(t, err)
		require.Equal(t, "completed", updatedSnapshot.Status)
		require.Equal(t, int64(10), updatedSnapshot.LastEventSeq)
	})

	// Test listing executions
	t.Run("ListExecutions", func(t *testing.T) {
		// Create test snapshots
		snapshots := []*ExecutionSnapshot{
			{
				ID:           "exec-1",
				WorkflowName: "workflow-a",
				WorkflowHash: "hash-1",
				InputsHash:   "input-hash-1",
				Status:       "completed",
				CreatedAt:    time.Now().Add(-2 * time.Hour),
				UpdatedAt:    time.Now().Add(-time.Hour),
				LastEventSeq: 10,
				Inputs:       map[string]interface{}{"key": "value1"},
				Outputs:      map[string]interface{}{"result": "success1"},
			},
			{
				ID:           "exec-2",
				WorkflowName: "workflow-b",
				WorkflowHash: "hash-2",
				InputsHash:   "input-hash-2",
				Status:       "failed",
				CreatedAt:    time.Now().Add(-time.Hour),
				UpdatedAt:    time.Now().Add(-30 * time.Minute),
				LastEventSeq: 5,
				Inputs:       map[string]interface{}{"key": "value2"},
				Outputs:      map[string]interface{}{"result": "error"},
			},
		}

		for _, snapshot := range snapshots {
			err := store.SaveSnapshot(ctx, snapshot)
			require.NoError(t, err)
		}

		// List all executions
		allExecutions, err := store.ListExecutions(ctx, ExecutionFilter{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(allExecutions), 2)

		// Filter by status
		completedExecutions, err := store.ListExecutions(ctx, ExecutionFilter{
			Status: "completed",
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(completedExecutions), 1)

		// Verify status filter worked
		for _, exec := range completedExecutions {
			require.Equal(t, "completed", exec.Status)
		}

		// Filter by workflow name
		workflowExecutions, err := store.ListExecutions(ctx, ExecutionFilter{
			WorkflowName: "workflow-a",
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(workflowExecutions), 1)

		// Verify workflow filter worked
		for _, exec := range workflowExecutions {
			require.Equal(t, "workflow-a", exec.WorkflowName)
		}

		// Test limit and offset
		limitedExecutions, err := store.ListExecutions(ctx, ExecutionFilter{
			Limit: 1,
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(limitedExecutions))
	})

	// Test deletion
	t.Run("DeleteExecution", func(t *testing.T) {
		executionID := "test-execution-delete"

		// Create events and snapshot
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
			WorkflowHash: "delete-hash",
			InputsHash:   "delete-input-hash",
			Status:       "running",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			LastEventSeq: 1,
			Inputs:       map[string]interface{}{},
			Outputs:      map[string]interface{}{},
		}

		err = store.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)

		// Verify they exist
		retrievedEvents, err := store.GetEventHistory(ctx, executionID)
		require.NoError(t, err)
		require.Len(t, retrievedEvents, 1)

		_, err = store.GetSnapshot(ctx, executionID)
		require.NoError(t, err)

		// Delete execution
		err = store.DeleteExecution(ctx, executionID)
		require.NoError(t, err)

		// Verify deletion
		retrievedEventsAfter, err := store.GetEventHistory(ctx, executionID)
		require.NoError(t, err)
		require.Len(t, retrievedEventsAfter, 0)

		_, err = store.GetSnapshot(ctx, executionID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	// Test cleanup
	t.Run("CleanupCompletedExecutions", func(t *testing.T) {
		// Create old completed execution
		oldExecID := "old-completed-exec"
		oldTime := time.Now().Add(-48 * time.Hour)

		oldSnapshot := &ExecutionSnapshot{
			ID:           oldExecID,
			WorkflowName: "old-workflow",
			WorkflowHash: "old-hash",
			InputsHash:   "old-input-hash",
			Status:       "completed",
			CreatedAt:    oldTime,
			UpdatedAt:    oldTime,
			LastEventSeq: 1,
			Inputs:       map[string]interface{}{},
			Outputs:      map[string]interface{}{},
		}

		err := store.SaveSnapshot(ctx, oldSnapshot)
		require.NoError(t, err)

		// Create recent running execution
		recentExecID := "recent-running-exec"

		recentSnapshot := &ExecutionSnapshot{
			ID:           recentExecID,
			WorkflowName: "recent-workflow",
			WorkflowHash: "recent-hash",
			InputsHash:   "recent-input-hash",
			Status:       "running",
			CreatedAt:    time.Now().Add(-time.Hour),
			UpdatedAt:    time.Now(),
			LastEventSeq: 1,
			Inputs:       map[string]interface{}{},
			Outputs:      map[string]interface{}{},
		}

		err = store.SaveSnapshot(ctx, recentSnapshot)
		require.NoError(t, err)

		// Cleanup executions older than 24 hours
		cutoffTime := time.Now().Add(-24 * time.Hour)
		err = store.CleanupCompletedExecutions(ctx, cutoffTime)
		require.NoError(t, err)

		// Verify old completed execution was deleted
		_, err = store.GetSnapshot(ctx, oldExecID)
		require.Error(t, err)

		// Verify recent running execution still exists
		_, err = store.GetSnapshot(ctx, recentExecID)
		require.NoError(t, err)
	})
}

func TestSQLiteExecutionEventStore_ConcurrentAccess(t *testing.T) {
	// Create temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "concurrent_test.db")

	store, err := NewSQLiteExecutionEventStore(dbPath, DefaultSQLiteStoreOptions())
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	executionID := "concurrent-test-execution"

	// Test concurrent event appending
	t.Run("ConcurrentEventAppending", func(t *testing.T) {
		const numGoroutines = 10
		const eventsPerGoroutine = 5

		// Channel to coordinate goroutines
		done := make(chan error, numGoroutines)

		// Start multiple goroutines appending events
		for i := 0; i < numGoroutines; i++ {
			go func(goroutineID int) {
				var events []*ExecutionEvent
				for j := 0; j < eventsPerGoroutine; j++ {
					eventID := fmt.Sprintf("event-%d-%d", goroutineID, j)
					event := &ExecutionEvent{
						ID:          eventID,
						ExecutionID: executionID,
						Path:        fmt.Sprintf("path-%d", goroutineID),
						Sequence:    int64(goroutineID*eventsPerGoroutine + j + 1),
						Timestamp:   time.Now(),
						EventType:   EventStepCompleted,
						Step:        fmt.Sprintf("step-%d-%d", goroutineID, j),
						Data: map[string]interface{}{
							"goroutine_id": goroutineID,
							"event_index":  j,
						},
					}
					events = append(events, event)
				}
				done <- store.AppendEvents(ctx, events)
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			err := <-done
			require.NoError(t, err)
		}

		// Verify all events were stored
		allEvents, err := store.GetEventHistory(ctx, executionID)
		require.NoError(t, err)
		require.Len(t, allEvents, numGoroutines*eventsPerGoroutine)
	})

	// Test concurrent snapshot updates
	t.Run("ConcurrentSnapshotUpdates", func(t *testing.T) {
		const numGoroutines = 5
		snapshotID := "concurrent-snapshot-test"

		// Channel to coordinate goroutines
		done := make(chan error, numGoroutines)

		// Start multiple goroutines updating the same snapshot
		for i := 0; i < numGoroutines; i++ {
			go func(goroutineID int) {
				snapshot := &ExecutionSnapshot{
					ID:           snapshotID,
					WorkflowName: "concurrent-workflow",
					WorkflowHash: "concurrent-hash",
					InputsHash:   "concurrent-input-hash",
					Status:       fmt.Sprintf("status-%d", goroutineID),
					CreatedAt:    time.Now(),
					UpdatedAt:    time.Now(),
					LastEventSeq: int64(goroutineID),
					Inputs:       map[string]interface{}{"goroutine": goroutineID},
					Outputs:      map[string]interface{}{"result": goroutineID},
				}
				done <- store.SaveSnapshot(ctx, snapshot)
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			err := <-done
			require.NoError(t, err)
		}

		// Verify snapshot exists (last write wins due to upsert)
		finalSnapshot, err := store.GetSnapshot(ctx, snapshotID)
		require.NoError(t, err)
		require.Equal(t, snapshotID, finalSnapshot.ID)
		require.Equal(t, "concurrent-workflow", finalSnapshot.WorkflowName)
	})
}

func TestSQLiteExecutionEventStore_ErrorHandling(t *testing.T) {
	// Test with invalid database path
	t.Run("InvalidDatabasePath", func(t *testing.T) {
		invalidPath := "/invalid/path/database.db"
		_, err := NewSQLiteExecutionEventStore(invalidPath, DefaultSQLiteStoreOptions())
		require.Error(t, err)
	})

	// Test with valid store
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "error_test.db")

	store, err := NewSQLiteExecutionEventStore(dbPath, DefaultSQLiteStoreOptions())
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Test invalid event validation
	t.Run("InvalidEventValidation", func(t *testing.T) {
		invalidEvents := []*ExecutionEvent{
			{
				// Missing required fields
				ID: "invalid-event",
			},
		}

		err := store.AppendEvents(ctx, invalidEvents)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid event")
	})

	// Test getting non-existent snapshot
	t.Run("NonExistentSnapshot", func(t *testing.T) {
		_, err := store.GetSnapshot(ctx, "non-existent-execution")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	// Test invalid filter
	t.Run("InvalidFilter", func(t *testing.T) {
		invalidFilter := ExecutionFilter{
			Limit:  -1, // Invalid negative limit
			Offset: -1, // Invalid negative offset
		}

		_, err := store.ListExecutions(ctx, invalidFilter)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid filter")
	})
}

func TestSQLiteExecutionEventStore_Performance(t *testing.T) {
	// Skip performance tests in short mode
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "performance_test.db")

	// Use larger batch size for performance test
	options := DefaultSQLiteStoreOptions()
	options.BatchSize = 1000

	store, err := NewSQLiteExecutionEventStore(dbPath, options)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	executionID := "performance-test-execution"

	// Test large batch insert performance
	t.Run("LargeBatchInsert", func(t *testing.T) {
		const numEvents = 10000

		var events []*ExecutionEvent
		for i := 0; i < numEvents; i++ {
			event := &ExecutionEvent{
				ID:          fmt.Sprintf("perf-event-%d", i),
				ExecutionID: executionID,
				Path:        fmt.Sprintf("path-%d", i%10),
				Sequence:    int64(i + 1),
				Timestamp:   time.Now(),
				EventType:   EventStepCompleted,
				Step:        fmt.Sprintf("step-%d", i),
				Data: map[string]interface{}{
					"index":  i,
					"output": fmt.Sprintf("output for step %d", i),
				},
			}
			events = append(events, event)
		}

		start := time.Now()
		err := store.AppendEvents(ctx, events)
		duration := time.Since(start)

		require.NoError(t, err)
		t.Logf("Inserted %d events in %v (%.2f events/sec)",
			numEvents, duration, float64(numEvents)/duration.Seconds())

		// Verify all events were inserted
		retrievedEvents, err := store.GetEventHistory(ctx, executionID)
		require.NoError(t, err)
		require.Len(t, retrievedEvents, numEvents)
	})

	// Test query performance
	t.Run("QueryPerformance", func(t *testing.T) {
		const numQueries = 100

		start := time.Now()
		for i := 0; i < numQueries; i++ {
			_, err := store.GetEvents(ctx, executionID, int64(i*10))
			require.NoError(t, err)
		}
		duration := time.Since(start)

		t.Logf("Executed %d queries in %v (%.2f queries/sec)",
			numQueries, duration, float64(numQueries)/duration.Seconds())
	})
}
