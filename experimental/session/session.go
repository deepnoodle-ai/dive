// Package session provides session management for Dive agents.
//
// This package contains the Session type and SessionRepository interface
// for persisting conversation history. It also provides hook helpers for
// integrating session management with the new generation hook system.
//
// # Migration from AgentOptions.SessionRepository
//
// Previously, sessions were managed by setting SessionRepository on AgentOptions:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:             model,
//	    SessionRepository: dive.NewMemorySessionRepository(),
//	})
//
// With the new hook-based approach, use the Hooks helper function:
//
//	repo := session.NewMemoryRepository()
//	preHook, postHook := session.Hooks(repo)
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:          model,
//	    PreGeneration:  []dive.PreGenerationHook{preHook},
//	    PostGeneration: []dive.PostGenerationHook{postHook},
//	})
//
// This approach gives you more control over when and how sessions are loaded
// and saved, and allows you to add custom logic around session management.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/llm"
)

// Session represents a conversation session containing a sequence of messages.
//
// A Session maintains the complete conversation history between a user and an agent,
// enabling multi-turn conversations with context preservation. Sessions can be
// persisted using a Repository and resumed across multiple CreateResponse calls.
type Session struct {
	// ID uniquely identifies this session.
	ID string `json:"id"`

	// UserID identifies the user participating in this conversation.
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
	Messages []*llm.Message `json:"messages"`

	// Metadata stores arbitrary key-value pairs for application-specific data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CompactionHistory tracks when context compaction has occurred.
	CompactionHistory []compaction.CompactionRecord `json:"compaction_history,omitempty"`
}

// Repository is an interface for storing and retrieving conversation sessions.
//
// Implementations of this interface enable persistent multi-turn conversations by
// storing and loading session state. The repository is responsible for maintaining
// message history and session metadata across sessions.
type Repository interface {
	// PutSession creates a new session or updates an existing one.
	PutSession(ctx context.Context, session *Session) error

	// GetSession retrieves a session by its ID.
	// Returns ErrSessionNotFound if the session does not exist.
	GetSession(ctx context.Context, id string) (*Session, error)

	// DeleteSession removes a session by its ID.
	// Returns nil if the session does not exist (idempotent).
	DeleteSession(ctx context.Context, id string) error

	// ListSessions returns sessions matching the pagination criteria.
	ListSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error)

	// ForkSession creates a new session containing a deep copy of all messages
	// from the specified session.
	ForkSession(ctx context.Context, sessionID string) (*Session, error)
}

// ListSessionsInput specifies pagination parameters for listing sessions.
type ListSessionsInput struct {
	Offset int
	Limit  int
}

// ListSessionsOutput contains the results of a ListSessions query.
type ListSessionsOutput struct {
	Items []*Session
}

// ErrSessionNotFound is returned when attempting to access a session that does not exist.
var ErrSessionNotFound = fmt.Errorf("session not found")

// MemoryRepository is an in-memory implementation of Repository.
//
// This implementation is suitable for development, testing, and single-instance
// deployments. Data is not persisted across process restarts.
type MemoryRepository struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewMemoryRepository creates a new empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		sessions: make(map[string]*Session),
	}
}

// WithSessions initializes the repository with a list of sessions.
func (r *MemoryRepository) WithSessions(sessions []*Session) *MemoryRepository {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, session := range sessions {
		r.sessions[session.ID] = session
	}
	return r
}

// PutSession stores a session.
func (r *MemoryRepository) PutSession(ctx context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[session.ID] = session
	return nil
}

// GetSession retrieves a session by ID.
func (r *MemoryRepository) GetSession(ctx context.Context, id string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// DeleteSession removes a session by ID.
func (r *MemoryRepository) DeleteSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

// ListSessions returns sessions with optional pagination.
func (r *MemoryRepository) ListSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error) {
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
		if input.Limit > 0 && input.Limit < len(sessions) {
			sessions = sessions[:input.Limit]
		}
	}

	return &ListSessionsOutput{Items: sessions}, nil
}

// ForkSession creates a new session as a copy of an existing session.
func (r *MemoryRepository) ForkSession(ctx context.Context, sessionID string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	original, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	// Deep copy the messages
	messagesCopy := make([]*llm.Message, len(original.Messages))
	for i, msg := range original.Messages {
		messagesCopy[i] = msg.Copy()
	}

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

	if original.Metadata != nil {
		forked.Metadata = make(map[string]interface{}, len(original.Metadata))
		for k, v := range original.Metadata {
			forked.Metadata[k] = v
		}
	}

	r.sessions[forked.ID] = forked
	return forked, nil
}

// newSessionID returns a new unique session identifier.
func newSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}
