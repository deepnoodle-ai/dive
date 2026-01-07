package dive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// ErrSessionNotFound is returned when attempting to access a session that does not exist.
var ErrSessionNotFound = fmt.Errorf("session not found")

// Session represents a conversation session containing a sequence of messages.
//
// A Session maintains the complete conversation history between a user and an agent,
// enabling multi-turn conversations with context preservation. Sessions can be
// persisted using a SessionRepository and resumed across multiple CreateResponse calls.
//
// Session IDs can be user-provided or auto-generated. Auto-generated IDs follow the
// format "session-<random>" where random is a large random integer.
type Session struct {
	// ID uniquely identifies this session. Used with WithSessionID or WithResume
	// to continue a conversation.
	ID string `json:"id"`

	// UserID identifies the user participating in this conversation.
	// Optional; set via WithUserID during CreateResponse.
	UserID string `json:"user_id,omitempty"`

	// AgentID is the identifier of the agent that owns this session.
	AgentID string `json:"agent_id,omitempty"`

	// AgentName is the name of the agent that owns this session.
	AgentName string `json:"agent_name,omitempty"`

	// Title is an optional human-readable title for the conversation.
	Title string `json:"title,omitempty"`

	// CreatedAt is when this session was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this session was last modified.
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

// ListSessionsInput specifies pagination parameters for listing sessions.
type ListSessionsInput struct {
	// Offset is the number of sessions to skip before returning results.
	Offset int

	// Limit is the maximum number of sessions to return. Zero means no limit.
	Limit int
}

// ListSessionsOutput contains the results of a ListSessions query.
type ListSessionsOutput struct {
	// Items contains the sessions matching the query criteria.
	Items []*Session
}

// SessionRepository is an interface for storing and retrieving conversation sessions.
//
// Implementations of this interface enable persistent multi-turn conversations by
// storing and loading session state. The repository is responsible for maintaining
// message history and session metadata across sessions.
//
// A SessionRepository is optional when creating an agent. Without one, sessions still
// work within a single CreateResponse call but are not persisted between calls.
//
// Example implementation is MemorySessionRepository for in-memory storage.
// For production use, implement this interface with a database backend.
type SessionRepository interface {
	// PutSession creates a new session or updates an existing one.
	// The session ID is used as the key; if a session with that ID exists,
	// it will be replaced.
	PutSession(ctx context.Context, session *Session) error

	// GetSession retrieves a session by its ID.
	// Returns ErrSessionNotFound if the session does not exist.
	GetSession(ctx context.Context, id string) (*Session, error)

	// DeleteSession removes a session by its ID.
	// Returns nil if the session does not exist (idempotent).
	DeleteSession(ctx context.Context, id string) error

	// ListSessions returns sessions matching the pagination criteria.
	// Pass nil for input to retrieve all sessions.
	ListSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error)

	// ForkSession creates a new session containing a deep copy of all messages
	// from the specified session. The new session receives an auto-generated ID.
	//
	// This enables branching conversations to explore alternative paths while
	// preserving the original session unchanged.
	//
	// Returns ErrSessionNotFound if the source session does not exist.
	ForkSession(ctx context.Context, sessionID string) (*Session, error)
}

// MemorySessionRepository is an in-memory implementation of SessionRepository.
//
// This implementation is suitable for development, testing, and single-instance
// deployments. Data is not persisted across process restarts.
//
// All operations are thread-safe using a read-write mutex.
//
// Example:
//
//	repo := dive.NewMemorySessionRepository()
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Name:              "Assistant",
//	    Model:             model,
//	    SessionRepository: repo,
//	})
type MemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewMemorySessionRepository creates a new empty MemorySessionRepository.
//
// The repository is ready to use immediately. Use WithSessions to pre-populate
// it with existing sessions if needed.
func NewMemorySessionRepository() *MemorySessionRepository {
	return &MemorySessionRepository{
		sessions: make(map[string]*Session),
	}
}

// WithSessions initializes the repository with a list of sessions.
//
// This is a builder-pattern method that returns the repository for chaining.
// Useful for setting up test fixtures or restoring state from external storage.
//
// Example:
//
//	repo := dive.NewMemorySessionRepository().WithSessions(savedSessions)
func (r *MemorySessionRepository) WithSessions(sessions []*Session) *MemorySessionRepository {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, session := range sessions {
		r.sessions[session.ID] = session
	}
	return r
}

// PutSession stores a session, creating it if new or replacing if it exists.
func (r *MemorySessionRepository) PutSession(ctx context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[session.ID] = session
	return nil
}

// GetSession retrieves a session by ID.
// Returns ErrSessionNotFound if the session does not exist.
func (r *MemorySessionRepository) GetSession(ctx context.Context, id string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// DeleteSession removes a session by ID.
// This operation is idempotent; deleting a non-existent session returns nil.
func (r *MemorySessionRepository) DeleteSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, id)
	return nil
}

// ListSessions returns sessions with optional pagination.
//
// Note: The order of returned sessions is not guaranteed due to Go map iteration
// semantics. For consistent ordering, sort the results after retrieval.
func (r *MemorySessionRepository) ListSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*Session
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}

	if input != nil {
		if input.Offset > 0 {
			if input.Offset < len(sessions) {
				sessions = sessions[input.Offset:]
			} else {
				sessions = nil
			}
		}
	}
	if input != nil {
		if input.Limit > 0 && input.Limit < len(sessions) {
			sessions = sessions[:input.Limit]
		}
	}

	return &ListSessionsOutput{Items: sessions}, nil
}

// ForkSession creates a new session as a copy of an existing session.
//
// The forked session receives a new auto-generated ID and contains deep copies
// of all messages from the original session. The original session is not modified.
//
// All session metadata (UserID, AgentID, AgentName, Title, Metadata) is copied
// to the forked session. Timestamps are set to the current time.
//
// The forked session is automatically stored in the repository before being returned.
//
// Returns ErrSessionNotFound if the source session does not exist.
func (r *MemorySessionRepository) ForkSession(ctx context.Context, sessionID string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	original, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	// Deep copy the messages using Message.Copy() to ensure independence
	messagesCopy := make([]*llm.Message, len(original.Messages))
	for i, msg := range original.Messages {
		messagesCopy[i] = msg.Copy()
	}

	// Create forked session with new auto-generated ID
	forked := &Session{
		ID:        newSessionID(),
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

	// Store the forked session immediately
	r.sessions[forked.ID] = forked

	return forked, nil
}
