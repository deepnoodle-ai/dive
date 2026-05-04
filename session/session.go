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

	"github.com/deepnoodle-ai/dive"
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

// cloneSuspensionState returns a deep copy of a SuspensionState so callers
// cannot mutate the session's internal state through the returned pointer.
// The TurnMessages slice is copied shallowly by pointer — individual messages
// are treated as immutable once produced.
func cloneSuspensionState(src *dive.SuspensionState) *dive.SuspensionState {
	if src == nil {
		return nil
	}
	out := &dive.SuspensionState{}
	if src.PendingToolCalls != nil {
		out.PendingToolCalls = make([]*dive.PendingToolCall, len(src.PendingToolCalls))
		for i, p := range src.PendingToolCalls {
			out.PendingToolCalls[i] = clonePendingToolCall(p)
		}
	}
	if src.CompletedToolCalls != nil {
		out.CompletedToolCalls = make([]*dive.CompletedToolCall, len(src.CompletedToolCalls))
		for i, c := range src.CompletedToolCalls {
			out.CompletedToolCalls[i] = cloneCompletedToolCall(c)
		}
	}
	if src.TurnMessages != nil {
		out.TurnMessages = make([]*llm.Message, len(src.TurnMessages))
		copy(out.TurnMessages, src.TurnMessages)
	}
	return out
}

func clonePendingToolCall(p *dive.PendingToolCall) *dive.PendingToolCall {
	if p == nil {
		return nil
	}
	cp := &dive.PendingToolCall{
		ID:     p.ID,
		Name:   p.Name,
		Prompt: p.Prompt,
		Reason: p.Reason,
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
	return cp
}

func cloneCompletedToolCall(c *dive.CompletedToolCall) *dive.CompletedToolCall {
	if c == nil {
		return nil
	}
	cp := &dive.CompletedToolCall{
		ID:    c.ID,
		Name:  c.Name,
		Error: c.Error,
	}
	if c.Input != nil {
		cp.Input = append(json.RawMessage(nil), c.Input...)
	}
	// Result is not deep-cloned; tool results are treated as immutable
	// once produced.
	cp.Result = c.Result
	return cp
}

// deepCopyJSONValue returns a deep copy of a JSON-like value so callers
// cannot mutate nested maps or slices held inside session internals.
// Scalars (string, bool, numbers, nil) are returned as-is; maps and slices
// are recursively copied. Unknown types fall back to a json round-trip so
// we stay correct even when tools attach exotic payloads.
//
// Values must be JSON-friendly. After a round trip through this helper or
// through the on-disk store, numeric values come back as float64 and custom
// struct types become generic map[string]any — tool authors attaching
// structured metadata should expect that loss of type fidelity.
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
	ID               string         `json:"id"`
	Type             eventType      `json:"type"`
	Timestamp        time.Time      `json:"timestamp"`
	Messages         []*llm.Message `json:"messages"`
	ReplacedMessages []*llm.Message `json:"replaced_messages,omitempty"`
	Usage            *llm.Usage     `json:"usage,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
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
	if len(e.ReplacedMessages) > 0 {
		cp.ReplacedMessages = make([]*llm.Message, len(e.ReplacedMessages))
		for i, msg := range e.ReplacedMessages {
			cp.ReplacedMessages[i] = msg.Copy()
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
	// SaveTurn or SaveResumedTurn.
	Suspended bool `json:"suspended,omitempty"`

	// PendingToolCalls is the set of tool calls awaiting external results,
	// along with the Prompt and Metadata the tool attached to its
	// SuspendResult. Valid only when Suspended is true. Persisting the full
	// payload lets cross-process resume and partial-resume-again flows
	// surface the original Prompt/Metadata to callers without having to
	// reconstruct it from the assistant tool_use message.
	PendingToolCalls []*dive.PendingToolCall `json:"pending_tool_calls,omitempty"`

	// CompletedToolCalls are sibling tool calls that ran alongside the
	// suspending tools in the same iteration. Informational — the results
	// are also present in the tool_result message on the last event.
	CompletedToolCalls []*dive.CompletedToolCall `json:"completed_tool_calls,omitempty"`
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
	prevLen := len(s.data.Events)
	prevUpdatedAt := s.data.UpdatedAt
	s.data.Events = append(s.data.Events, evt)
	s.data.UpdatedAt = evt.Timestamp
	if s.appender != nil {
		if err := s.appender.appendEvent(ctx, s.data.ID, evt); err != nil {
			s.data.Events = s.data.Events[:prevLen]
			s.data.UpdatedAt = prevUpdatedAt
			return err
		}
	}
	return nil
}

// LoadSuspension returns a deep copy of the stored suspension state, or
// nil if the session is not currently suspended. Satisfies the
// dive.SuspendableSession interface.
func (s *Session) LoadSuspension() *dive.SuspensionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.data.Suspended {
		return nil
	}
	state := &dive.SuspensionState{
		PendingToolCalls:   s.data.PendingToolCalls,
		CompletedToolCalls: s.data.CompletedToolCalls,
	}
	// TurnMessages carries the last event's messages (the in-progress
	// suspended turn). The agent uses len(TurnMessages) to locate the turn
	// boundary on resume and stateless callers — or anyone rendering the
	// in-progress turn to a UI — read this field directly.
	if len(s.data.Events) > 0 {
		state.TurnMessages = s.data.Events[len(s.data.Events)-1].Messages
	}
	return cloneSuspensionState(state)
}

// sessionSnapshot captures the fields of sessionData we mutate during
// suspend/resume writes so the in-memory state can be rolled back if the
// store write fails. Without rollback the Session would diverge from its
// backing file/DB.
type sessionSnapshot struct {
	events    []*event // full events slice (pointer-level copy)
	suspended bool
	pending   []*dive.PendingToolCall
	completed []*dive.CompletedToolCall
	updatedAt time.Time
}

// snapshotMutated returns a shallow snapshot of the fields that
// withRollback restores on store-write failure. The events slice is
// duplicated so an in-place replacement of the last event can be undone
// cleanly.
func (s *Session) snapshotMutated() sessionSnapshot {
	eventsCopy := make([]*event, len(s.data.Events))
	copy(eventsCopy, s.data.Events)
	return sessionSnapshot{
		events:    eventsCopy,
		suspended: s.data.Suspended,
		pending:   s.data.PendingToolCalls,
		completed: s.data.CompletedToolCalls,
		updatedAt: s.data.UpdatedAt,
	}
}

// restoreSnapshot reverts mutations after a failed store write.
func (s *Session) restoreSnapshot(snap sessionSnapshot) {
	s.data.Events = snap.events
	s.data.Suspended = snap.suspended
	s.data.PendingToolCalls = snap.pending
	s.data.CompletedToolCalls = snap.completed
	s.data.UpdatedAt = snap.updatedAt
}

// withRollback runs mutate to apply state changes, then asks the store to
// persist them. On store failure the pre-mutation state is restored so the
// in-memory session stays consistent with what is actually durable.
//
// When the session has no appender (in-memory mode), mutate still runs but
// no rollback is performed — there is nothing to recover from.
func (s *Session) withRollback(ctx context.Context, mutate func()) error {
	snap := s.snapshotMutated()
	mutate()
	if s.appender == nil {
		return nil
	}
	if err := s.appender.putSession(ctx, s.data); err != nil {
		s.restoreSnapshot(snap)
		return err
	}
	return nil
}

// SaveSuspendedTurn persists a partial turn: messages ending in an assistant
// tool_use message and an (optionally partial) tool_result message. Sets the
// session's Suspended flag and records the supplied SuspensionState.
//
// If the session is already suspended and the last event is the corresponding
// turn, this replaces it rather than appending, so repeated partial resumes
// do not grow the event log without bound.
func (s *Session) SaveSuspendedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, state *dive.SuspensionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withRollback(ctx, func() {
		now := time.Now()
		replaceLast := s.data.Suspended &&
			len(s.data.Events) > 0 &&
			s.data.Events[len(s.data.Events)-1].Type == eventTypeTurn

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
		cloned := cloneSuspensionState(state)
		s.data.Suspended = true
		if cloned != nil {
			s.data.PendingToolCalls = cloned.PendingToolCalls
			s.data.CompletedToolCalls = cloned.CompletedToolCalls
		} else {
			s.data.PendingToolCalls = nil
			s.data.CompletedToolCalls = nil
		}
		s.data.UpdatedAt = now
	})
}

// SaveResumedTurn replaces the last (suspended) event with a completed turn
// and clears the suspension state. Called by the agent when a resumed
// generate run completes normally.
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
	if len(s.data.Events) == 0 {
		return ErrNotSuspended
	}

	return s.withRollback(ctx, func() {
		now := time.Now()
		evtID := s.data.Events[len(s.data.Events)-1].ID
		evt := &event{
			ID:        evtID,
			Type:      eventTypeTurn,
			Timestamp: now,
			Messages:  messages,
			Usage:     usage,
		}
		s.data.Events[len(s.data.Events)-1] = evt
		s.data.Suspended = false
		s.data.PendingToolCalls = nil
		s.data.CompletedToolCalls = nil
		s.data.UpdatedAt = now
	})
}

// CancelSuspension abandons a suspended turn, clearing the suspension
// state and removing the partial turn's event from the session history.
// After cancellation the session is ready for a fresh turn as if the
// suspended turn never happened. Returns ErrNotSuspended if the session
// is not currently suspended.
func (s *Session) CancelSuspension(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.data.Suspended {
		return ErrNotSuspended
	}
	return s.withRollback(ctx, func() {
		if len(s.data.Events) > 0 {
			s.data.Events = s.data.Events[:len(s.data.Events)-1]
		}
		s.data.Suspended = false
		s.data.PendingToolCalls = nil
		s.data.CompletedToolCalls = nil
		s.data.UpdatedAt = time.Now()
	})
}

// IsSuspended reports whether the session is awaiting external tool
// results. Thin helper around LoadSuspension() != nil, retained for stores
// and UI code that need a boolean without the full state.
func (s *Session) IsSuspended() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Suspended
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
			// Suspended / PendingToolCalls / CompletedToolCalls intentionally
			// not copied: pending out-of-band tool calls are owned by whoever
			// launched the original suspend and cannot be resumed against a
			// divergent branch.
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
// the resume path depends on, so the caller must first resume the turn to
// completion (via WithResume/WithToolResults) before compacting.
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
		ID:               newEventID(),
		Type:             eventTypeCompaction,
		Timestamp:        time.Now(),
		Messages:         compacted,
		ReplacedMessages: msgs,
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

// CompactionRecord describes a single compaction event in the session history.
type CompactionRecord struct {
	// Summary contains the compacted/summarized messages.
	Summary []*llm.Message
	// ReplacedMessages are the original messages that were replaced by compaction.
	// Empty for sessions compacted before this feature was added.
	ReplacedMessages []*llm.Message
	// CompactedAt is when the compaction occurred.
	CompactedAt time.Time
}

// CompactionHistory returns all compaction records in chronological order.
// Returns an empty slice when the session has never been compacted.
func (s *Session) CompactionHistory(_ context.Context) ([]CompactionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var records []CompactionRecord
	for _, e := range s.data.Events {
		if e.Type != eventTypeCompaction {
			continue
		}
		rec := CompactionRecord{
			CompactedAt: e.Timestamp,
		}
		if len(e.Messages) > 0 {
			rec.Summary = make([]*llm.Message, len(e.Messages))
			for i, msg := range e.Messages {
				rec.Summary[i] = msg.Copy()
			}
		}
		if len(e.ReplacedMessages) > 0 {
			rec.ReplacedMessages = make([]*llm.Message, len(e.ReplacedMessages))
			for i, msg := range e.ReplacedMessages {
				rec.ReplacedMessages[i] = msg.Copy()
			}
		}
		records = append(records, rec)
	}
	return records, nil
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

// ListOptions specifies pagination and filters for List.
type ListOptions struct {
	// Limit caps the number of results returned (0 = no limit).
	Limit int
	// Offset skips this many leading results (0 = start from the top).
	Offset int

	// Suspended filters by the session's suspended state when non-nil:
	//   Suspended = &true  → only sessions awaiting external tool results
	//   Suspended = &false → only sessions in a normal (non-suspended) state
	//
	// This is the canonical way to find stale suspended sessions that a
	// caller may want to abandon or reap. Typical pattern: periodically
	// list Suspended=&true sessions and, for those older than some SLA,
	// cancel the workflow by constructing error ToolResults for each
	// pending call and resuming to drain them.
	Suspended *bool
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
