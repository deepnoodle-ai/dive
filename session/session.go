// Package session provides persistent conversation state for Dive agents.
//
// A Session implements the dive.Session interface and tracks conversation
// history as a sequence of events (one per CreateResponse call). Events
// are an internal detail — the public interface deals only in messages.
//
// Sessions are opt-in. An agent without a session is stateless.
//
// In-memory session (no persistence):
//
//	sess := session.New("my-session")
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:   model,
//	    Session: sess,
//	})
//
// Persistent session with a store:
//
//	store := session.NewFileStore("~/.myapp/sessions")
//	sess, _ := store.Open(ctx, "my-session")
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:   model,
//	    Session: sess,
//	})
//
// Per-call session override (one agent, many sessions):
//
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithInput("Hello"),
//	    dive.WithSession(sess),
//	)
package session

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// eventCounter ensures unique event IDs under concurrent generation.
var eventCounter uint64

// ErrNotFound is returned when a session does not exist.
var ErrNotFound = errors.New("session not found")

// eventType identifies the kind of session event.
type eventType string

const (
	eventTypeTurn       eventType = "turn"
	eventTypeCompaction eventType = "compaction"
)

// event is the internal unit of session persistence. Each CreateResponse
// call produces one event containing the messages added during that turn.
type event struct {
	ID        string         `json:"id"`
	Type      eventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Messages  []*llm.Message `json:"messages"`
	Usage     *llm.Usage     `json:"usage,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (e *event) copy() *event {
	cp := &event{
		ID:        e.ID,
		Type:      e.Type,
		Timestamp: e.Timestamp,
	}
	if len(e.Messages) > 0 {
		cp.Messages = make([]*llm.Message, len(e.Messages))
		for i, msg := range e.Messages {
			cp.Messages[i] = msg.Copy()
		}
	}
	if e.Usage != nil {
		cp.Usage = e.Usage.Copy()
	}
	if e.Metadata != nil {
		cp.Metadata = make(map[string]any, len(e.Metadata))
		maps.Copy(cp.Metadata, e.Metadata)
	}
	return cp
}

// eventAppender is the internal interface used by Session to persist events.
type eventAppender interface {
	appendEvent(ctx context.Context, sessionID string, evt *event) error
	putSession(ctx context.Context, sess *sessionData) error
}

// sessionData is the internal storage representation of a session.
type sessionData struct {
	ID         string         `json:"id"`
	Title      string         `json:"title,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Events     []*event       `json:"events"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	ForkedFrom string         `json:"forked_from,omitempty"`
}

// Session implements dive.Session with event-based persistence.
//
// Create with New for in-memory sessions or store.Open for persistent sessions.
type Session struct {
	mu       sync.RWMutex
	data     *sessionData
	appender eventAppender // nil for in-memory sessions
}

// New creates an in-memory session with the given ID.
// Messages are stored in memory only and lost when the process exits.
func New(id string) *Session {
	now := time.Now()
	return &Session{
		data: &sessionData{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// ID returns the session's unique identifier.
func (s *Session) ID() string {
	return s.data.ID
}

// Messages returns the complete conversation history reconstructed from
// all events. Returns a copy of the messages to prevent mutation.
func (s *Session) Messages(ctx context.Context) ([]*llm.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var msgs []*llm.Message
	for _, e := range s.data.Events {
		for _, msg := range e.Messages {
			msgs = append(msgs, msg.Copy())
		}
	}
	return msgs, nil
}

// SaveTurn persists messages from a single conversation turn.
// The messages should include both the user input and the assistant response.
func (s *Session) SaveTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error {
	evt := &event{
		ID:        newEventID(),
		Type:      eventTypeTurn,
		Timestamp: time.Now(),
		Messages:  messages,
		Usage:     usage,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Events = append(s.data.Events, evt)
	s.data.UpdatedAt = evt.Timestamp
	if s.appender != nil {
		return s.appender.appendEvent(ctx, s.data.ID, evt)
	}
	return nil
}

// Title returns the session title.
func (s *Session) Title() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Title
}

// SetTitle sets the session title.
func (s *Session) SetTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Title = title
}

// Metadata returns a copy of the session's metadata.
func (s *Session) Metadata() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Metadata == nil {
		return nil
	}
	cp := make(map[string]any, len(s.data.Metadata))
	maps.Copy(cp, s.data.Metadata)
	return cp
}

// SetMetadata sets a key-value pair in the session's metadata.
func (s *Session) SetMetadata(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Metadata == nil {
		s.data.Metadata = make(map[string]any)
	}
	s.data.Metadata[key] = value
}

// EventCount returns the number of events in the session.
func (s *Session) EventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data.Events)
}

// TotalUsage sums token usage across all events.
func (s *Session) TotalUsage() *llm.Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := &llm.Usage{}
	for _, e := range s.data.Events {
		if e.Usage != nil {
			total.Add(e.Usage)
		}
	}
	return total
}

// Fork creates a new in-memory session with a deep copy of all events.
// The forked session records the original as its parent.
// To persist the fork, save it to a store with store.Put.
func (s *Session) Fork(newID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]*event, len(s.data.Events))
	for i, e := range s.data.Events {
		events[i] = e.copy()
	}
	now := time.Now()
	forked := &Session{
		data: &sessionData{
			ID:         newID,
			Title:      s.data.Title,
			CreatedAt:  now,
			UpdatedAt:  now,
			Events:     events,
			ForkedFrom: s.data.ID,
		},
	}
	if s.data.Metadata != nil {
		forked.data.Metadata = make(map[string]any, len(s.data.Metadata))
		maps.Copy(forked.data.Metadata, s.data.Metadata)
	}
	return forked
}

// CompactFunc summarizes a conversation into a shorter form.
type CompactFunc func(ctx context.Context, messages []*llm.Message) ([]*llm.Message, error)

// Compact replaces all events with a single compaction event containing
// the summarized messages. If the session is backed by a store, the
// compacted session is persisted.
func (s *Session) Compact(ctx context.Context, summarize CompactFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var msgs []*llm.Message
	for _, e := range s.data.Events {
		msgs = append(msgs, e.Messages...)
	}
	compacted, err := summarize(ctx, msgs)
	if err != nil {
		return err
	}
	s.data.Events = []*event{{
		ID:        newEventID(),
		Type:      eventTypeCompaction,
		Timestamp: time.Now(),
		Messages:  compacted,
		Metadata: map[string]any{
			"original_event_count":   len(s.data.Events),
			"original_message_count": len(msgs),
		},
	}}
	s.data.UpdatedAt = time.Now()
	if s.appender != nil {
		return s.appender.putSession(ctx, s.data)
	}
	return nil
}

// Store is the storage abstraction for persistent sessions.
type Store interface {
	// Open loads an existing session or creates a new one with the given ID.
	// The returned session is connected to the store: SaveTurn calls persist
	// automatically.
	Open(ctx context.Context, id string) (*Session, error)

	// Put saves a session to the store, connecting it for future SaveTurn calls.
	// Use this after Fork to persist the forked session.
	Put(ctx context.Context, sess *Session) error

	// List returns lightweight session summaries.
	List(ctx context.Context, opts *ListOptions) (*ListResult, error)

	// Delete removes a session. Idempotent: returns nil if absent.
	Delete(ctx context.Context, id string) error
}

// ListOptions specifies pagination for List.
type ListOptions struct {
	Limit  int
	Offset int
}

// ListResult contains the result of a List call.
type ListResult struct {
	Sessions []*SessionInfo
}

// SessionInfo is a lightweight session summary returned by List.
type SessionInfo struct {
	ID         string         `json:"id"`
	Title      string         `json:"title,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	EventCount int            `json:"event_count"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ForkSession loads a session, forks it, and saves the fork to the store.
func ForkSession(ctx context.Context, store Store, fromID, newID string) (*Session, error) {
	original, err := store.Open(ctx, fromID)
	if err != nil {
		return nil, err
	}
	forked := original.Fork(newID)
	if err := store.Put(ctx, forked); err != nil {
		return nil, err
	}
	return forked, nil
}

// newEventID generates a unique event identifier.
func newEventID() string {
	n := atomic.AddUint64(&eventCounter, 1)
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), n)
}
