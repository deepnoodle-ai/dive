package threads

import (
	"context"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

var _ dive.ThreadRepository = &MemoryRepository{}

// MemoryRepository is an in-memory implementation of dive.ThreadRepository
type MemoryRepository struct {
	mu      sync.RWMutex
	threads map[string]*dive.Thread
}

// NewMemoryRepository creates a new MemoryRepository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		threads: make(map[string]*dive.Thread),
	}
}

// WithThreads initializes the repository with a list of threads
func (r *MemoryRepository) WithThreads(threads []*dive.Thread) *MemoryRepository {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, thread := range threads {
		r.threads[thread.ID] = thread
	}
	return r
}

func (r *MemoryRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.threads[thread.ID] = thread
	return nil
}

func (r *MemoryRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, ok := r.threads[id]
	if !ok {
		return nil, dive.ErrThreadNotFound
	}
	return thread, nil
}

func (r *MemoryRepository) DeleteThread(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.threads, id)
	return nil
}

func (r *MemoryRepository) ListThreads(ctx context.Context, input *dive.ListThreadsInput) (*dive.ListThreadsOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var threads []*dive.Thread
	for _, thread := range r.threads {
		threads = append(threads, thread)
	}

	if input != nil {
		if input.Offset > 0 {
			if input.Offset < len(threads) {
				threads = threads[input.Offset:]
			} else {
				threads = nil
			}
		}
	}
	if input != nil {
		if input.Limit > 0 && input.Limit < len(threads) {
			threads = threads[:input.Limit]
		}
	}

	return &dive.ListThreadsOutput{Items: threads}, nil
}
