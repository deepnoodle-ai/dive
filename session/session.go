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
	"encoding/json"
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

// ErrSuspendedSession is returned when an operation is not permitted on a
// session that is currently suspended. For example, Compact refuses to run
// on a suspended session because compaction would destroy the in-progress
// tool_use/tool_result messages the resume path depends on.
var ErrSuspendedSession = errors.New("session is suspended")

// ErrNotSuspended is returned from SaveResumedTurn when the session is not
// actually in a suspended state. Protects against accidental overwrites of
// the last event.
var ErrNotSuspended = errors.New("session is not suspended")

// PendingCall is the internal, serializable shape of a suspended tool call.
// Kept in the session package so sessionData/sessionHeader stay self-contained
// (session does not import dive). Callers in the dive package convert to and
// from the public dive.PendingToolCall type at the boundary.
type PendingCall struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input,omitempty"`
	Prompt   string          `json:"prompt,omitempty"`
	Metadata map[string]any  `json:"metadata,omitempty"`
}

// clonePendingCalls returns a deep copy of pending calls so callers cannot
// mutate the session's internal state through the returned slice.
func clonePendingCalls(src []PendingCall) []PendingCall {
	if src == nil {
		return nil
	}
	out := make([]PendingCall, len(src))
	for i, p := range src {
		cp := PendingCall{
			ID:     p.ID,
			Name:   p.Name,
			Prompt: p.Prompt,
		}
		if p.Input != nil {
			cp.Input = append(json.RawMessage(nil), p.Input...)
		}
		if p.Metadata != nil {
			cp.Metadata = make(map[string]any, len(p.Metadata))
			for k, v := range p.Metadata {
				cp.Metadata[k] = deepCopyJSONValue(v)
			}
		}
		out[i] = cp
	}
	return out
}

// deepCopyJSONValue returns a deep copy of a JSON-like value so callers
// cannot mutate nested maps or slices held inside session internals.
// Scalars (string, bool, numbers, nil) are returned as-is; maps and slices
// are recursively copied. Unknown types fall back to a json round-trip so
// we stay correct even when tools attach exotic payloads.
func deepCopyJSONValue(v any) any {
	switch val := v.(type) {
	case nil, bool, string, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, float32, float64,
		json.Number:
		return val
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, item := range val {
			cp[k] = deepCopyJSONValue(item)
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopyJSONValue(item)
		}
		return cp
	case json.RawMessage:
		return append(json.RawMessage(nil), val...)
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return val
		}
		var out any
		if err := json.Unmarshal(data, &out); err != nil {
			return val
		}
		return out
	}
}

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

	// Suspended tracks the authoritative suspend flag. Cleared on a normal
	// SaveTurn or on AbandonSuspension.
	Suspended bool `json:"suspended,omitempty"`

	// PendingCalls is the set of tool_call IDs awaiting external results,
	// along with the prompt and metadata the tool attached to its
	// SuspendResult. Valid only when Suspended is true. Persisting the full
	// payload lets cross-process resume and partial-resume-again flows
	// surface the original Prompt/Metadata to callers without having to
	// reconstruct it from the assistant tool_use message.
	PendingCalls []PendingCall `json:"pending_calls,omitempty"`
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
//
// Returns ErrSuspendedSession if the session is currently suspended: resume
// completions must go through SaveResumedTurn, which is the only path that
// clears the suspend flag and rewrites the store header. Allowing SaveTurn
// to silently clear suspension would drop the partial tool_use/tool_result
// messages the resume path depends on.
func (s *Session) SaveTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Suspended {
		return ErrSuspendedSession
	}
	evt := &event{
		ID:        newEventID(),
		Type:      eventTypeTurn,
		Timestamp: time.Now(),
		Messages:  messages,
		Usage:     usage,
	}
	s.data.Events = append(s.data.Events, evt)
	s.data.UpdatedAt = evt.Timestamp
	if s.appender != nil {
		return s.appender.appendEvent(ctx, s.data.ID, evt)
	}
	return nil
}

// LastEventMessageCount returns the number of messages in the most recent
// event, or 0 if the session has no events. The agent uses this during
// resume to compute the boundary between the suspended turn and the rest
// of the conversation history.
func (s *Session) LastEventMessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.data.Events) == 0 {
		return 0
	}
	return len(s.data.Events[len(s.data.Events)-1].Messages)
}

// Suspended reports whether the session is awaiting external tool results.
func (s *Session) Suspended() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Suspended
}

// PendingCalls returns a deep copy of the tool calls awaiting external
// results. Valid only when Suspended() is true.
func (s *Session) PendingCalls() []PendingCall {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePendingCalls(s.data.PendingCalls)
}

// SaveSuspendedTurn persists a partial turn: messages ending in an assistant
// tool_use message and an (optionally partial) tool_result message. Sets the
// session's Suspended flag and records pending call info.
//
// If the session is already suspended and the last event is the corresponding
// turn, this replaces it rather than appending, so repeated partial resumes
// do not grow the event log without bound.
func (s *Session) SaveSuspendedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, pending []PendingCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	replaceLast := s.data.Suspended && len(s.data.Events) > 0 && s.data.Events[len(s.data.Events)-1].Type == eventTypeTurn

	// Snapshot fields we are about to mutate so we can roll back if the
	// store write fails. Otherwise the in-memory Session would diverge from
	// what's on disk.
	prevSuspended := s.data.Suspended
	prevPending := clonePendingCalls(s.data.PendingCalls)
	prevUpdatedAt := s.data.UpdatedAt
	var prevLastEvent *event
	prevEventsLen := len(s.data.Events)
	if replaceLast {
		prevLastEvent = s.data.Events[len(s.data.Events)-1]
	}

	var evtID string
	if replaceLast {
		evtID = s.data.Events[len(s.data.Events)-1].ID
	} else {
		evtID = newEventID()
	}
	evt := &event{
		ID:        evtID,
		Type:      eventTypeTurn,
		Timestamp: now,
		Messages:  messages,
		Usage:     usage,
		Metadata: map[string]any{
			"suspended": true,
		},
	}
	if replaceLast {
		s.data.Events[len(s.data.Events)-1] = evt
	} else {
		s.data.Events = append(s.data.Events, evt)
	}
	s.data.Suspended = true
	s.data.PendingCalls = clonePendingCalls(pending)
	s.data.UpdatedAt = now

	if s.appender != nil {
		// Suspend transitions always take the full-rewrite path to keep the
		// header's suspend flag in sync and to support the replace-last
		// optimization.
		if err := s.appender.putSession(ctx, s.data); err != nil {
			if replaceLast {
				s.data.Events[len(s.data.Events)-1] = prevLastEvent
			} else {
				s.data.Events = s.data.Events[:prevEventsLen]
			}
			s.data.Suspended = prevSuspended
			s.data.PendingCalls = prevPending
			s.data.UpdatedAt = prevUpdatedAt
			return err
		}
	}
	return nil
}

// SaveResumedTurn replaces the last (suspended) event with a completed turn
// and clears the suspended flag. Called by the agent when a resumed generate
// run completes normally.
//
// Returns ErrNotSuspended if the session is not currently suspended. This
// prevents accidental overwrite of the last event when a custom Session
// implementation or caller plumbing routes a normal turn into this method.
func (s *Session) SaveResumedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.data.Suspended {
		return ErrNotSuspended
	}

	now := time.Now()

	// Snapshot mutated fields for rollback on store failure.
	prevSuspended := s.data.Suspended
	prevPending := clonePendingCalls(s.data.PendingCalls)
	prevUpdatedAt := s.data.UpdatedAt
	hasLastEvent := len(s.data.Events) > 0
	var prevLastEvent *event
	prevEventsLen := len(s.data.Events)
	if hasLastEvent {
		prevLastEvent = s.data.Events[len(s.data.Events)-1]
	}

	var evtID string
	if hasLastEvent {
		evtID = s.data.Events[len(s.data.Events)-1].ID
	} else {
		evtID = newEventID()
	}
	evt := &event{
		ID:        evtID,
		Type:      eventTypeTurn,
		Timestamp: now,
		Messages:  messages,
		Usage:     usage,
	}
	if hasLastEvent {
		s.data.Events[len(s.data.Events)-1] = evt
	} else {
		s.data.Events = append(s.data.Events, evt)
	}
	s.data.Suspended = false
	s.data.PendingCalls = nil
	s.data.UpdatedAt = now

	if s.appender != nil {
		if err := s.appender.putSession(ctx, s.data); err != nil {
			if hasLastEvent {
				s.data.Events[len(s.data.Events)-1] = prevLastEvent
			} else {
				s.data.Events = s.data.Events[:prevEventsLen]
			}
			s.data.Suspended = prevSuspended
			s.data.PendingCalls = prevPending
			s.data.UpdatedAt = prevUpdatedAt
			return err
		}
	}
	return nil
}

// AbandonSuspension marks the session as no longer suspended without writing
// new messages. Used when a caller gives up on a suspended turn without
// supplying results, or as a rollback path when post-suspend hooks abort.
//
// Note: the session's last event still contains an unanswered tool_use, so
// calling CreateResponse on an abandoned session will typically fail at the
// LLM layer. Callers who want to recover should Compact or roll back the
// last event first.
func (s *Session) AbandonSuspension(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.data.Suspended {
		return nil
	}
	prevPending := clonePendingCalls(s.data.PendingCalls)
	prevUpdatedAt := s.data.UpdatedAt
	s.data.Suspended = false
	s.data.PendingCalls = nil
	s.data.UpdatedAt = time.Now()
	if s.appender != nil {
		if err := s.appender.putSession(ctx, s.data); err != nil {
			s.data.Suspended = true
			s.data.PendingCalls = prevPending
			s.data.UpdatedAt = prevUpdatedAt
			return err
		}
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
//
// A forked session is never suspended, even if the original was: pending
// out-of-band tool calls are owned by whoever launched the original suspend
// and cannot be resumed against a divergent branch. The fork's last event
// still carries any unanswered assistant tool_use blocks, so callers of a
// forked-from-suspended session should Compact or roll back the last event
// before attempting a new turn.
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
			// Suspended and PendingCalls intentionally not copied.
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
//
// Returns ErrSuspendedSession if the session is currently suspended.
// Compaction destroys the in-progress tool_use/tool_result messages that
// the resume path depends on, so the caller must first resume to completion
// or call AbandonSuspension.
func (s *Session) Compact(ctx context.Context, summarize CompactFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Suspended {
		return ErrSuspendedSession
	}
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

	// Suspended indicates the session is awaiting external tool results.
	Suspended bool `json:"suspended,omitempty"`
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
