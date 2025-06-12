package workflow

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileExecutionEventStore implements ExecutionEventStore using file system storage
type FileExecutionEventStore struct {
	basePath string
	mutex    sync.RWMutex
}

// NewFileExecutionEventStore creates a new file-based event store
func NewFileExecutionEventStore(basePath string) *FileExecutionEventStore {
	return &FileExecutionEventStore{
		basePath: basePath,
	}
}

// AppendEvents appends events to the execution's event log file
func (f *FileExecutionEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	if len(events) == 0 {
		return nil
	}

	executionID := events[0].ExecutionID
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Create execution directory if it doesn't exist
	execDir := filepath.Join(f.basePath, executionID)
	if err := os.MkdirAll(execDir, 0755); err != nil {
		return fmt.Errorf("failed to create execution directory: %w", err)
	}

	// Open events file for appending
	eventsFile := filepath.Join(execDir, "events.jsonl")
	file, err := os.OpenFile(eventsFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open events file: %w", err)
	}
	defer file.Close()

	// Write events as JSON Lines
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return fmt.Errorf("failed to encode event: %w", err)
		}
	}

	return nil
}

// GetEvents retrieves events for an execution starting from a specific sequence number
func (f *FileExecutionEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	eventsFile := filepath.Join(f.basePath, executionID, "events.jsonl")
	file, err := os.Open(eventsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ExecutionEvent{}, nil
		}
		return nil, fmt.Errorf("failed to open events file: %w", err)
	}
	defer file.Close()

	var events []*ExecutionEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event ExecutionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("failed to decode event: %w", err)
		}

		if event.Sequence >= fromSeq {
			events = append(events, &event)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read events file: %w", err)
	}

	return events, nil
}

// GetEventHistory retrieves the complete event history for an execution
func (f *FileExecutionEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return f.GetEvents(ctx, executionID, 1)
}

// SaveSnapshot saves an execution snapshot to a JSON file
func (f *FileExecutionEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Create execution directory if it doesn't exist
	execDir := filepath.Join(f.basePath, snapshot.ID)
	if err := os.MkdirAll(execDir, 0755); err != nil {
		return fmt.Errorf("failed to create execution directory: %w", err)
	}

	// Write snapshot to temporary file first (atomic write)
	snapshotFile := filepath.Join(execDir, "snapshot.json")
	tempFile := snapshotFile + ".tmp"

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp snapshot file: %w", err)
	}
	defer file.Close()

	// Update snapshot metadata
	snapshot.UpdatedAt = time.Now()
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = snapshot.UpdatedAt
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to encode snapshot: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, snapshotFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename snapshot file: %w", err)
	}

	return nil
}

// GetSnapshot retrieves an execution snapshot from a JSON file
func (f *FileExecutionEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	snapshotFile := filepath.Join(f.basePath, executionID, "snapshot.json")
	file, err := os.Open(snapshotFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot not found for execution %s", executionID)
		}
		return nil, fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	var snapshot ExecutionSnapshot
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("failed to decode snapshot: %w", err)
	}

	return &snapshot, nil
}

// ListExecutions returns a list of executions matching the filter criteria
func (f *FileExecutionEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	if err := filter.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// Read all execution directories
	entries, err := os.ReadDir(f.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ExecutionSnapshot{}, nil
		}
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	var snapshots []*ExecutionSnapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		executionID := entry.Name()
		snapshot, err := f.GetSnapshot(ctx, executionID)
		if err != nil {
			// Skip executions without snapshots
			continue
		}

		// Apply filters
		if filter.Status != nil && snapshot.Status != *filter.Status {
			continue
		}
		if filter.WorkflowName != nil && snapshot.WorkflowName != *filter.WorkflowName {
			continue
		}

		snapshots = append(snapshots, snapshot)
	}

	// Sort by creation time (newest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	// Apply pagination
	start := filter.Offset
	if start >= len(snapshots) {
		return []*ExecutionSnapshot{}, nil
	}

	end := len(snapshots)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}

	return snapshots[start:end], nil
}

// DeleteExecution removes all files associated with an execution
func (f *FileExecutionEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	execDir := filepath.Join(f.basePath, executionID)
	if err := os.RemoveAll(execDir); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete execution directory: %w", err)
	}

	return nil
}

// CleanupCompletedExecutions removes executions that completed before the specified time
func (f *FileExecutionEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	filter := ExecutionFilter{}
	snapshots, err := f.ListExecutions(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list executions: %w", err)
	}

	var deletedCount int
	for _, snapshot := range snapshots {
		// Only delete completed executions older than the specified time
		if (snapshot.Status == "completed" || snapshot.Status == "failed") &&
			snapshot.EndTime.Before(olderThan) {

			if err := f.DeleteExecution(ctx, snapshot.ID); err != nil {
				return fmt.Errorf("failed to delete execution %s: %w", snapshot.ID, err)
			}
			deletedCount++
		}
	}

	return nil
}
