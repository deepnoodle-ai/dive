package agent

import (
	"context"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

var _ dive.ThreadRepository = &MemoryThreadRepository{}

// MemoryThreadRepository is an in-memory implementation of ThreadRepository
type MemoryThreadRepository struct {
	mu      sync.RWMutex
	threads map[string]*dive.Thread
}

// NewMemoryThreadRepository creates a new MemoryThreadRepository
func NewMemoryThreadRepository() *MemoryThreadRepository {
	return &MemoryThreadRepository{
		threads: make(map[string]*dive.Thread),
	}
}

// WithThreads initializes the repository with a list of threads
func (r *MemoryThreadRepository) WithThreads(threads []*dive.Thread) *MemoryThreadRepository {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, thread := range threads {
		r.threads[thread.ID] = thread
	}
	return r
}

func (r *MemoryThreadRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.threads[thread.ID] = thread
	return nil
}

func (r *MemoryThreadRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, ok := r.threads[id]
	if !ok {
		return nil, dive.ErrThreadNotFound
	}
	return thread, nil
}

func (r *MemoryThreadRepository) DeleteThread(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.threads, id)
	return nil
}

func (r *MemoryThreadRepository) ListThreads(ctx context.Context, input *dive.ListThreadsInput) (*dive.ListThreadsOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var threads []*dive.Thread
	for _, thread := range r.threads {
		threads = append(threads, thread)
	}
	return &dive.ListThreadsOutput{Items: threads}, nil
}
