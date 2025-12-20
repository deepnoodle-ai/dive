package dive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

var ErrThreadNotFound = fmt.Errorf("thread not found")

// Thread represents a conversation thread
type Thread struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	AgentName string                 `json:"agent_name,omitempty"`
	Title     string                 `json:"title,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Messages  []*llm.Message         `json:"messages"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ListThreadsInput specifies search criteria for threads in a thread repository
type ListThreadsInput struct {
	Offset int
	Limit  int
}

// ListThreadsOutput is the output for listing threads
type ListThreadsOutput struct {
	Items []*Thread
}

// ThreadRepository is an interface for storing and retrieving conversation threads
type ThreadRepository interface {

	// PutThread creates or updates a thread
	PutThread(ctx context.Context, thread *Thread) error

	// GetThread retrieves a thread by ID
	GetThread(ctx context.Context, id string) (*Thread, error)

	// DeleteThread deletes a thread by ID
	DeleteThread(ctx context.Context, id string) error

	// ListThreads returns all threads (used for session management)
	ListThreads(ctx context.Context, input *ListThreadsInput) (*ListThreadsOutput, error)
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

func (r *MemoryThreadRepository) ListThreads(ctx context.Context, input *ListThreadsInput) (*ListThreadsOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var threads []*Thread
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

	return &ListThreadsOutput{Items: threads}, nil
}
