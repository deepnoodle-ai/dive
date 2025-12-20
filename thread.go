package dive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// ErrThreadNotFound is returned when attempting to access a thread that does not exist.
var ErrThreadNotFound = fmt.Errorf("thread not found")

// Thread represents a conversation thread containing a sequence of messages.
//
// A Thread maintains the complete conversation history between a user and an agent,
// enabling multi-turn conversations with context preservation. Threads can be
// persisted using a ThreadRepository and resumed across multiple CreateResponse calls.
//
// Thread IDs can be user-provided or auto-generated. Auto-generated IDs follow the
// format "thread-<random>" where random is a large random integer.
type Thread struct {
	// ID uniquely identifies this thread. Used with WithThreadID or WithResume
	// to continue a conversation.
	ID string `json:"id"`

	// UserID identifies the user participating in this conversation.
	// Optional; set via WithUserID during CreateResponse.
	UserID string `json:"user_id,omitempty"`

	// AgentID is the identifier of the agent that owns this thread.
	AgentID string `json:"agent_id,omitempty"`

	// AgentName is the name of the agent that owns this thread.
	AgentName string `json:"agent_name,omitempty"`

	// Title is an optional human-readable title for the conversation.
	Title string `json:"title,omitempty"`

	// CreatedAt is when this thread was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this thread was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// Messages contains the complete conversation history in chronological order.
	// This includes both user messages and assistant responses.
	Messages []*llm.Message `json:"messages"`

	// Metadata stores arbitrary key-value pairs for application-specific data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CompactionHistory tracks when context compaction has occurred.
	// Each record contains information about a compaction event.
	CompactionHistory []CompactionRecord `json:"compaction_history,omitempty"`
}

// ListThreadsInput specifies pagination parameters for listing threads.
type ListThreadsInput struct {
	// Offset is the number of threads to skip before returning results.
	Offset int

	// Limit is the maximum number of threads to return. Zero means no limit.
	Limit int
}

// ListThreadsOutput contains the results of a ListThreads query.
type ListThreadsOutput struct {
	// Items contains the threads matching the query criteria.
	Items []*Thread
}

// ThreadRepository is an interface for storing and retrieving conversation threads.
//
// Implementations of this interface enable persistent multi-turn conversations by
// storing and loading thread state. The repository is responsible for maintaining
// message history and thread metadata across sessions.
//
// A ThreadRepository is optional when creating an agent. Without one, threads still
// work within a single CreateResponse call but are not persisted between calls.
//
// Example implementation is MemoryThreadRepository for in-memory storage.
// For production use, implement this interface with a database backend.
type ThreadRepository interface {
	// PutThread creates a new thread or updates an existing one.
	// The thread ID is used as the key; if a thread with that ID exists,
	// it will be replaced.
	PutThread(ctx context.Context, thread *Thread) error

	// GetThread retrieves a thread by its ID.
	// Returns ErrThreadNotFound if the thread does not exist.
	GetThread(ctx context.Context, id string) (*Thread, error)

	// DeleteThread removes a thread by its ID.
	// Returns nil if the thread does not exist (idempotent).
	DeleteThread(ctx context.Context, id string) error

	// ListThreads returns threads matching the pagination criteria.
	// Pass nil for input to retrieve all threads.
	ListThreads(ctx context.Context, input *ListThreadsInput) (*ListThreadsOutput, error)

	// ForkThread creates a new thread containing a deep copy of all messages
	// from the specified thread. The new thread receives an auto-generated ID.
	//
	// This enables branching conversations to explore alternative paths while
	// preserving the original thread unchanged.
	//
	// Returns ErrThreadNotFound if the source thread does not exist.
	ForkThread(ctx context.Context, threadID string) (*Thread, error)
}

// MemoryThreadRepository is an in-memory implementation of ThreadRepository.
//
// This implementation is suitable for development, testing, and single-instance
// deployments. Data is not persisted across process restarts.
//
// All operations are thread-safe using a read-write mutex.
//
// Example:
//
//	repo := dive.NewMemoryThreadRepository()
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Name:             "Assistant",
//	    Model:            model,
//	    ThreadRepository: repo,
//	})
type MemoryThreadRepository struct {
	mu      sync.RWMutex
	threads map[string]*Thread
}

// NewMemoryThreadRepository creates a new empty MemoryThreadRepository.
//
// The repository is ready to use immediately. Use WithThreads to pre-populate
// it with existing threads if needed.
func NewMemoryThreadRepository() *MemoryThreadRepository {
	return &MemoryThreadRepository{
		threads: make(map[string]*Thread),
	}
}

// WithThreads initializes the repository with a list of threads.
//
// This is a builder-pattern method that returns the repository for chaining.
// Useful for setting up test fixtures or restoring state from external storage.
//
// Example:
//
//	repo := dive.NewMemoryThreadRepository().WithThreads(savedThreads)
func (r *MemoryThreadRepository) WithThreads(threads []*Thread) *MemoryThreadRepository {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, thread := range threads {
		r.threads[thread.ID] = thread
	}
	return r
}

// PutThread stores a thread, creating it if new or replacing if it exists.
func (r *MemoryThreadRepository) PutThread(ctx context.Context, thread *Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.threads[thread.ID] = thread
	return nil
}

// GetThread retrieves a thread by ID.
// Returns ErrThreadNotFound if the thread does not exist.
func (r *MemoryThreadRepository) GetThread(ctx context.Context, id string) (*Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, ok := r.threads[id]
	if !ok {
		return nil, ErrThreadNotFound
	}
	return thread, nil
}

// DeleteThread removes a thread by ID.
// This operation is idempotent; deleting a non-existent thread returns nil.
func (r *MemoryThreadRepository) DeleteThread(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.threads, id)
	return nil
}

// ListThreads returns threads with optional pagination.
//
// Note: The order of returned threads is not guaranteed due to Go map iteration
// semantics. For consistent ordering, sort the results after retrieval.
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

// ForkThread creates a new thread as a copy of an existing thread.
//
// The forked thread receives a new auto-generated ID and contains deep copies
// of all messages from the original thread. The original thread is not modified.
//
// All thread metadata (UserID, AgentID, AgentName, Title, Metadata) is copied
// to the forked thread. Timestamps are set to the current time.
//
// The forked thread is automatically stored in the repository before being returned.
//
// Returns ErrThreadNotFound if the source thread does not exist.
func (r *MemoryThreadRepository) ForkThread(ctx context.Context, threadID string) (*Thread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	original, ok := r.threads[threadID]
	if !ok {
		return nil, ErrThreadNotFound
	}

	// Deep copy the messages using Message.Copy() to ensure independence
	messagesCopy := make([]*llm.Message, len(original.Messages))
	for i, msg := range original.Messages {
		messagesCopy[i] = msg.Copy()
	}

	// Create forked thread with new auto-generated ID
	forked := &Thread{
		ID:        newThreadID(),
		UserID:    original.UserID,
		AgentID:   original.AgentID,
		AgentName: original.AgentName,
		Title:     original.Title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  messagesCopy,
	}

	// Shallow copy metadata (sufficient for typical use cases)
	if original.Metadata != nil {
		forked.Metadata = make(map[string]interface{}, len(original.Metadata))
		for k, v := range original.Metadata {
			forked.Metadata[k] = v
		}
	}

	// Store the forked thread immediately
	r.threads[forked.ID] = forked

	return forked, nil
}
