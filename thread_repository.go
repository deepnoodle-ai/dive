package dive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diveagents/dive/llm"
)

var ErrThreadNotFound = fmt.Errorf("thread not found")

var _ ThreadRepository = &MemoryThreadRepository{}

// Thread represents a chat thread
type Thread struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Messages  []*llm.Message `json:"messages"`
}

// ThreadRepository is an interface for storing and retrieving chat threads
type ThreadRepository interface {

	// PutThread creates or updates a thread
	PutThread(ctx context.Context, thread *Thread) error

	// GetThread retrieves a thread by ID
	GetThread(ctx context.Context, id string) (*Thread, error)

	// DeleteThread deletes a thread by ID
	DeleteThread(ctx context.Context, id string) error
}

// MemoryThreadRepository is an in-memory implementation of ThreadRepository
type MemoryThreadRepository struct {
	mu      sync.RWMutex
	threads map[string]*Thread
}

// NewMemoryThreadRepository creates a new MemoryThreadRepository
func NewMemoryThreadRepository() *MemoryThreadRepository {
	return &MemoryThreadRepository{
		threads: make(map[string]*Thread),
	}
}

// WithThreads initializes the repository with a list of threads
func (r *MemoryThreadRepository) WithThreads(threads []*Thread) *MemoryThreadRepository {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, thread := range threads {
		r.threads[thread.ID] = thread
	}
	return r
}

func (r *MemoryThreadRepository) PutThread(ctx context.Context, thread *Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.threads[thread.ID] = thread
	return nil
}

func (r *MemoryThreadRepository) GetThread(ctx context.Context, id string) (*Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, ok := r.threads[id]
	if !ok {
		return nil, ErrThreadNotFound
	}
	return thread, nil
}

func (r *MemoryThreadRepository) DeleteThread(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.threads, id)
	return nil
}
