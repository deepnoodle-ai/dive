package a2a

import (
	"context"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

// TaskStore persists A2A task state and the optional Dive SuspensionState
// that accompanies a suspended task. The server adapter reads and writes
// this store while handling JSON-RPC methods. Implementations must be
// safe for concurrent use.
//
// The prototype ships with an in-memory store. Real deployments should
// plug in a database-backed implementation so task state survives process
// restarts.
type TaskStore interface {
	// Put writes or updates a TaskRecord.
	Put(ctx context.Context, rec *TaskRecord) error

	// Get returns the record for the given task ID, or (nil, false) if
	// it is not present.
	Get(ctx context.Context, id string) (*TaskRecord, bool, error)

	// Delete removes a task record.
	Delete(ctx context.Context, id string) error
}

// TaskRecord is everything the adapter tracks about one A2A task. The
// A2A Task is the wire representation; Suspension and SessionID carry the
// Dive-side state needed to resume the task when a client sends follow-up
// input.
type TaskRecord struct {
	Task       *Task
	Suspension *dive.SuspensionState
	SessionID  string
}

// MemoryTaskStore is a simple in-memory TaskStore suitable for prototypes
// and tests.
type MemoryTaskStore struct {
	mu      sync.Mutex
	records map[string]*TaskRecord
}

// NewMemoryTaskStore returns a new empty in-memory TaskStore.
func NewMemoryTaskStore() *MemoryTaskStore {
	return &MemoryTaskStore{records: make(map[string]*TaskRecord)}
}

// Put implements TaskStore.
func (s *MemoryTaskStore) Put(ctx context.Context, rec *TaskRecord) error {
	if rec == nil || rec.Task == nil || rec.Task.ID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.Task.ID] = rec
	return nil
}

// Get implements TaskStore.
func (s *MemoryTaskStore) Get(ctx context.Context, id string) (*TaskRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	return rec, ok, nil
}

// Delete implements TaskStore.
func (s *MemoryTaskStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}
