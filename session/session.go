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
	"slices"
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
// tool_use/tool_result messages the resume path depends on. A write refused
// with it wrote nothing: errors.Is(err, dive.ErrSaveRejected) holds.
var ErrSuspendedSession error = &rejectedError{"session is suspended"}

// ErrNotSuspended is returned from SaveResumedTurn when the session is not
// actually in a suspended state. Protects against accidental overwrites of
// the last event. errors.Is(err, dive.ErrSaveRejected) holds.
var ErrNotSuspended error = &rejectedError{"session is not suspended"}

// rejectedError is a write the session refused before writing anything. It
// matches dive.ErrSaveRejected, so the agent reports the write as failed
// rather than unknown.
type rejectedError struct{ msg string }

func (e *rejectedError) Error() string { return e.msg }

func (e *rejectedError) Is(target error) bool { return target == dive.ErrSaveRejected }

// cloneSuspensionState returns a deep copy of a SuspensionState so callers
// cannot mutate the session's internal state through the returned pointer,
// and the session cannot be changed through the state it was handed.
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
		out.TurnMessages = copyMessages(src.TurnMessages)
		if out.TurnMessages == nil {
			out.TurnMessages = []*llm.Message{}
		}
	}
	out.BatchHalted = src.BatchHalted
	out.TurnID = src.TurnID
	out.Usage = copyUsage(src.Usage)
	return out
}

func clonePendingToolCall(p *dive.PendingToolCall) *dive.PendingToolCall {
	if p == nil {
		return nil
	}
	c := *p
	cp := &c
	cp.Input = nil
	cp.Metadata = nil
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
	cp.Result = cloneToolResult(c.Result)
	return cp
}

// cloneToolResult returns a deep copy of a completed call's result. The
// unexported background payload is not persisted and is carried by pointer.
func cloneToolResult(r *dive.ToolResult) *dive.ToolResult {
	if r == nil {
		return nil
	}
	cp := *r
	if r.Content != nil {
		cp.Content = make([]*dive.ToolResultContent, len(r.Content))
		for i, c := range r.Content {
			if c == nil {
				continue
			}
			cc := *c
			if c.Annotations != nil {
				cc.Annotations = deepCopyJSONValue(c.Annotations).(map[string]any)
			}
			cp.Content[i] = &cc
		}
	}
	if r.Suspend != nil {
		sr := *r.Suspend
		if r.Suspend.Metadata != nil {
			sr.Metadata = deepCopyJSONValue(r.Suspend.Metadata).(map[string]any)
		}
		cp.Suspend = &sr
	}
	return &cp
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
	ID        string         `json:"id"`
	Type      eventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Messages  []*llm.Message `json:"messages"`
	Usage     *llm.Usage     `json:"usage,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`

	// Revision is the session revision the write that recorded this event
	// advanced the session to.
	Revision uint64 `json:"revision,omitempty"`

	// Turn is the record of the turn the event holds, written by
	// CheckpointTurn. Events saved otherwise have none: their turn ID is the
	// event ID and their state is read from the header and the metadata.
	Turn *turnState `json:"turn,omitempty"`
}

// turnState is the part of a dive.Turn record an event stores besides its
// messages and usage.
type turnState struct {
	Schema    int                   `json:"schema,omitempty"`
	ID        string                `json:"id"`
	Origin    *dive.TurnOrigin      `json:"origin,omitempty"`
	Status    dive.ResponseStatus   `json:"status"`
	Outcome   *dive.TurnOutcome     `json:"outcome,omitempty"`
	ToolCalls []dive.ToolCallRecord `json:"tool_calls,omitempty"`
}

// copy returns a deep copy of the state.
func (t *turnState) copy() *turnState {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Origin != nil {
		origin := *t.Origin
		cp.Origin = &origin
	}
	cp.Outcome = copyOutcome(t.Outcome)
	cp.ToolCalls = slices.Clone(t.ToolCalls)
	return &cp
}

// copyOutcome returns a deep copy of an outcome, or nil.
func copyOutcome(o *dive.TurnOutcome) *dive.TurnOutcome {
	if o == nil {
		return nil
	}
	cp := *o
	cp.ToolCalls = slices.Clone(o.ToolCalls)
	return &cp
}

func (e *event) copy() *event {
	cp := &event{
		ID:        e.ID,
		Type:      e.Type,
		Timestamp: e.Timestamp,
		Revision:  e.Revision,
		Turn:      e.Turn.copy(),
	}
	if len(e.Messages) > 0 {
		cp.Messages = copyMessages(e.Messages)
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

// copyMessages returns a deep copy of a message slice, or nil when empty, so
// callers cannot mutate the session's internal messages through the result.
// It is also used on ingestion (SaveTurn and friends) so caller mutations
// after a save cannot rewrite stored history. llm.Message.Copy is a JSON
// round-trip; a structural copy is a possible future optimization.
func copyMessages(msgs []*llm.Message) []*llm.Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]*llm.Message, len(msgs))
	for i, msg := range msgs {
		out[i] = msg.Copy()
	}
	return out
}

// outcomeMetadata returns the metadata of an event whose messages record an
// incomplete turn's outcome (dive.FindTurnOutcome): "outcome" is its reason.
// It returns nil for any other turn.
func outcomeMetadata(messages []*llm.Message) map[string]any {
	outcome, ok := dive.FindLatestTurnOutcome(messages)
	if !ok {
		return nil
	}
	return map[string]any{"outcome": string(outcome.Reason)}
}

// copyUsage returns a deep copy of usage, or nil when usage is nil.
func copyUsage(usage *llm.Usage) *llm.Usage {
	if usage == nil {
		return nil
	}
	return usage.Copy()
}

// sumUsage returns a new Usage holding the sum of a and b without mutating
// either, or nil when both are nil. Used when a resumed (or re-suspended)
// turn replaces a suspended event: the replaced event's usage covers tokens
// already paid before suspension and must be carried forward so TotalUsage
// does not undercount.
func sumUsage(a, b *llm.Usage) *llm.Usage {
	if a == nil && b == nil {
		return nil
	}
	total := &llm.Usage{}
	if a != nil {
		total.Add(a)
	}
	if b != nil {
		total.Add(b)
	}
	return total
}

// eventAppender is the internal interface used by Session to persist events.
type eventAppender interface {
	appendEvent(ctx context.Context, sessionID string, evt *event) error
	appendStep(ctx context.Context, sessionID string, step *stepRecord) error
	putSession(ctx context.Context, sess *sessionData) error
}

// stepRecord is a step checkpoint of a running turn as a store appends it:
// the whole event for the turn's first step, or the change since the last.
// A store that rewrites the session writes a running turn's event as a
// step record with Event set, and a finished turn's as an event, so that a
// reader that skips step records, as versions before step checkpoints do,
// sees each finished turn once and no running one.
type stepRecord struct {
	// Event is the running turn's whole event, for its first step.
	Event *event `json:"event,omitempty"`

	// EventID names the event a change applies to. The event keeps its
	// first From messages, followed by Messages, and takes the usage, turn
	// state, revision and timestamp given here.
	EventID   string         `json:"event_id,omitempty"`
	From      int            `json:"from,omitempty"`
	Messages  []*llm.Message `json:"messages,omitempty"`
	Usage     *llm.Usage     `json:"usage,omitempty"`
	Turn      *turnState     `json:"turn,omitempty"`
	Revision  uint64         `json:"revision,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// apply returns the event the change makes of prev.
func (r *stepRecord) apply(prev *event) (*event, error) {
	if r.From < 0 || r.From > len(prev.Messages) {
		return nil, fmt.Errorf("step for event %s keeps %d of its %d messages", prev.ID, r.From, len(prev.Messages))
	}
	evt := prev.copy()
	evt.Messages = append(evt.Messages[:r.From:r.From], r.Messages...)
	evt.Usage = r.Usage
	evt.Turn = r.Turn
	evt.Revision = r.Revision
	evt.Timestamp = r.Timestamp
	return evt, nil
}

// stepChange returns the step record that makes next of prev, two versions
// of one running turn's event: the messages they share, compared by their
// encoding, are kept.
func stepChange(prev, next *event) *stepRecord {
	from := 0
	for from < len(prev.Messages) && from < len(next.Messages) && sameMessage(prev.Messages[from], next.Messages[from]) {
		from++
	}
	return &stepRecord{
		EventID:   next.ID,
		From:      from,
		Messages:  next.Messages[from:],
		Usage:     next.Usage,
		Turn:      next.Turn,
		Revision:  next.Revision,
		Timestamp: next.Timestamp,
	}
}

// sameMessage reports whether two messages encode the same.
func sameMessage(a, b *llm.Message) bool {
	if a == b {
		return true
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
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

	// BatchHalted mirrors dive.SuspensionState.BatchHalted: a halting call
	// in the suspended batch failed.
	BatchHalted bool `json:"batch_halted,omitempty"`

	// Revision is advanced by every write to the conversation or the
	// suspension state (see dive.TurnStore).
	Revision uint64 `json:"revision,omitempty"`
}

// Session implements dive.Session with event-based persistence.
//
// Create with New for in-memory sessions or store.Open for persistent sessions.
type Session struct {
	mu       sync.RWMutex
	data     *sessionData
	appender eventAppender // nil for in-memory sessions

	// claim is the session's claim when its store does not keep claims
	// itself (see ClaimSession), and claimOwner the owner this instance
	// last claimed the session as, or empty.
	claim      *sessionClaim
	claimOwner string
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

// activeEventsLocked returns the events that make up the live conversation:
// everything from the most recent compaction checkpoint onward. A checkpoint's
// summary stands in for all the events before it, so those earlier events stay
// in the log (for AllMessages and CompactionHistory) but are not part of the
// active context window. Caller must hold s.mu.
func (s *Session) activeEventsLocked() []*event {
	last := -1
	for i, e := range s.data.Events {
		if e.Type == eventTypeCompaction {
			last = i
		}
	}
	if last < 0 {
		return s.data.Events
	}
	return s.data.Events[last:]
}

// Messages returns the active conversation window: the most recent compaction
// summary (if any) plus every message recorded after it. This is what the
// agent sends to the model, so compaction shrinks it. Returns copies to
// prevent mutation. Use AllMessages to recover the pre-compaction originals.
func (s *Session) Messages(ctx context.Context) ([]*llm.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var msgs []*llm.Message
	for _, e := range s.activeEventsLocked() {
		for _, msg := range e.Messages {
			msgs = append(msgs, msg.Copy())
		}
	}
	return msgs, nil
}

// AllMessages returns the complete, uncompacted transcript: every message from
// every event in order, with each compaction summary appearing inline at the
// point it was created. Where Messages returns only the active window,
// AllMessages returns everything ever recorded.
//
// This is the substrate for retrieval. A summary is lossy compression — the
// specific tool results, file contents, and decision rationale it flattened
// are gone from the active window but remain here, searchable and recoverable
// after compaction.
func (s *Session) AllMessages(ctx context.Context) ([]*llm.Message, error) {
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
		// Deep-copy on ingestion: reads already deep-copy, so without this
		// a caller mutating e.g. response.OutputMessages after SaveTurn
		// would silently rewrite stored history.
		Messages: copyMessages(messages),
		Usage:    copyUsage(usage),
		Metadata: outcomeMetadata(messages),
		Revision: s.data.Revision + 1,
	}
	return s.appendLocked(ctx, evt)
}

// appendLocked adds evt to the end of the log and advances the revision to
// evt.Revision, persisting it with the store's append. On a store error the
// session resyncs from the store. Caller must hold s.mu.
func (s *Session) appendLocked(ctx context.Context, evt *event) error {
	return s.recordLocked(ctx, len(s.data.Events), evt, func(a eventAppender) error {
		return a.appendEvent(ctx, s.data.ID, evt)
	})
}

// recordLocked puts evt at index i of the log, or at its end when i is its
// length, advances the revision to evt.Revision, and persists the change
// with persist, which appends to the store. On a store error the session
// resyncs from the store. Caller must hold s.mu.
func (s *Session) recordLocked(ctx context.Context, i int, evt *event, persist func(eventAppender) error) error {
	if err := s.checkClaimLocked(ctx); err != nil {
		return err
	}
	prevLen := len(s.data.Events)
	prevUpdatedAt := s.data.UpdatedAt
	prevRevision := s.data.Revision
	var prev *event
	if i < prevLen {
		prev = s.data.Events[i]
		s.data.Events[i] = evt
	} else {
		s.data.Events = append(s.data.Events, evt)
	}
	s.data.UpdatedAt = evt.Timestamp
	s.data.Revision = evt.Revision
	if s.appender != nil {
		if err := persist(s.appender); err != nil {
			s.resyncAfterWriteError(ctx, func() {
				if prev != nil {
					s.data.Events[i] = prev
				} else {
					s.data.Events = s.data.Events[:prevLen]
				}
				s.data.UpdatedAt = prevUpdatedAt
				s.data.Revision = prevRevision
			})
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
		BatchHalted:        s.data.BatchHalted,
	}
	// TurnMessages carries the last event's messages (the in-progress
	// suspended turn). The agent uses len(TurnMessages) to locate the turn
	// boundary on resume and stateless callers — or anyone rendering the
	// in-progress turn to a UI — read this field directly.
	if len(s.data.Events) > 0 {
		last := s.data.Events[len(s.data.Events)-1]
		state.TurnMessages = last.Messages
		state.Usage = last.Usage
		state.TurnID = eventTurnID(last)
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
	halted    bool
	updatedAt time.Time
	revision  uint64
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
		halted:    s.data.BatchHalted,
		updatedAt: s.data.UpdatedAt,
		revision:  s.data.Revision,
	}
}

// restoreSnapshot reverts mutations after a failed store write.
func (s *Session) restoreSnapshot(snap sessionSnapshot) {
	s.data.Events = snap.events
	s.data.Suspended = snap.suspended
	s.data.PendingToolCalls = snap.pending
	s.data.CompletedToolCalls = snap.completed
	s.data.BatchHalted = snap.halted
	s.data.UpdatedAt = snap.updatedAt
	s.data.Revision = snap.revision
}

// withRollback advances the revision, runs mutate to apply state changes,
// then asks the store to persist them. mutate reads the new revision from
// s.data.Revision. On store failure the session resyncs from the store (see
// resyncAfterWriteError) so the in-memory session stays consistent with what
// is actually durable.
//
// When the session has no appender (in-memory mode), mutate still runs but
// no rollback is performed — there is nothing to recover from.
func (s *Session) withRollback(ctx context.Context, mutate func()) error {
	if err := s.checkClaimLocked(ctx); err != nil {
		return err
	}
	snap := s.snapshotMutated()
	s.data.Revision++
	mutate()
	if s.appender == nil {
		return nil
	}
	if err := s.appender.putSession(ctx, s.data); err != nil {
		s.resyncAfterWriteError(ctx, func() { s.restoreSnapshot(snap) })
		return err
	}
	return nil
}

// storeReloader is implemented by stores whose writes can fail after
// reaching storage, so a session can read back what the store holds.
type storeReloader interface {
	reloadSession(ctx context.Context, id string) (*sessionData, error)
}

// resyncAfterWriteError runs with s.mu held after a store write returned an
// error. A write error does not prove that nothing was written: FileStore can
// fail after its rename has replaced the file, or after an appended line
// reached the file. So the session reads its conversation and suspension
// state back from the store rather than trusting its pre-write snapshot.
// restore is the fallback when the store cannot be read back. Title and
// metadata keep their in-memory values, which the store may not have yet.
func (s *Session) resyncAfterWriteError(ctx context.Context, restore func()) {
	if r, ok := s.appender.(storeReloader); ok {
		if stored, err := r.reloadSession(ctx, s.data.ID); err == nil {
			s.data.Events = stored.Events
			s.data.Suspended = stored.Suspended
			s.data.PendingToolCalls = stored.PendingToolCalls
			s.data.CompletedToolCalls = stored.CompletedToolCalls
			s.data.BatchHalted = stored.BatchHalted
			s.data.UpdatedAt = stored.UpdatedAt
			s.data.Revision = stored.Revision
			return
		}
	}
	restore()
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
		var priorUsage *llm.Usage
		var identity *turnState
		if replaceLast {
			prev := s.data.Events[len(s.data.Events)-1]
			evtID = prev.ID
			identity = turnIdentity(prev)
			// The replaced suspended event's usage covers tokens already
			// paid before this partial resume; carry it forward so
			// TotalUsage does not undercount.
			priorUsage = prev.Usage
		} else {
			evtID = newEventID()
		}
		evt := &event{
			ID:        evtID,
			Type:      eventTypeTurn,
			Timestamp: now,
			Messages:  copyMessages(messages),
			Usage:     sumUsage(priorUsage, usage),
			Metadata: map[string]any{
				"suspended": true,
			},
			Revision: s.data.Revision,
			Turn:     identity,
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
			s.data.BatchHalted = cloned.BatchHalted
		} else {
			s.data.PendingToolCalls = nil
			s.data.CompletedToolCalls = nil
			s.data.BatchHalted = false
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
		prev := s.data.Events[len(s.data.Events)-1]
		evt := &event{
			ID:        prev.ID,
			Type:      eventTypeTurn,
			Timestamp: now,
			Messages:  copyMessages(messages),
			// The replaced suspended event's usage covers tokens already
			// paid before suspension; the agent passes only the resume
			// call's own usage. Sum so TotalUsage reflects both phases.
			Usage:    sumUsage(prev.Usage, usage),
			Metadata: outcomeMetadata(messages),
			Revision: s.data.Revision,
			Turn:     turnIdentity(prev),
		}
		s.data.Events[len(s.data.Events)-1] = evt
		s.data.Suspended = false
		s.data.PendingToolCalls = nil
		s.data.CompletedToolCalls = nil
		s.data.BatchHalted = false
		s.data.UpdatedAt = now
	})
}

// CancelSuspension abandons a suspended turn, clearing the suspension
// state and removing the partial turn's event from the session history.
// After cancellation the session is ready for a fresh turn as if the
// suspended turn never happened. Returns ErrNotSuspended if the session
// is not currently suspended.
//
// Deprecated: Use RemoveLastTurn, which this is for a suspended session, or
// Agent.CancelSuspendedTurn, which keeps the turn closed with its completed
// calls' results.
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
		s.data.BatchHalted = false
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

// ForkOption configures Fork.
type ForkOption func(*forkOptions)

type forkOptions struct {
	openTurn bool
}

// ForkWithOpenTurn copies an open turn into the fork as well. An incomplete
// turn is copied as it is. A suspended or running turn is copied closed, as
// Agent.CancelSuspendedTurn closes one: its pending or running calls are
// answered as unknown, since the original's owner may still run them, and
// the turn is incomplete with TurnReasonCanceled and TurnNextReconcile.
func ForkWithOpenTurn() ForkOption {
	return func(o *forkOptions) { o.openTurn = true }
}

// Fork creates a new in-memory session with a deep copy of the events up to
// the last completed turn. The forked session records the original as its
// parent. To persist the fork, save it to a store with store.Put.
//
// An open turn, a last turn that is suspended, incomplete or running, is
// left out unless ForkWithOpenTurn is passed. A forked session is never
// suspended: pending out-of-band tool calls are owned by whoever launched
// the original suspend and cannot be resumed against a divergent branch.
func (s *Session) Fork(newID string, opts ...ForkOption) *Session {
	var o forkOptions
	for _, opt := range opts {
		opt(&o)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	open := s.openTurnIndexLocked()
	var events []*event
	for i, e := range s.data.Events {
		if i != open {
			events = append(events, e.copy())
			continue
		}
		if !o.openTurn {
			continue
		}
		if s.data.Suspended || s.eventStatusLocked(i) == dive.ResponseStatusRunning {
			events = append(events, s.closedOpenEventLocked(e))
		} else {
			events = append(events, e.copy())
		}
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
			Revision:   s.data.Revision,
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

// Compact summarizes the active conversation window and appends a compaction
// checkpoint. The checkpoint's summary becomes the new active window (see
// Messages), shrinking the tokens sent to the model, while every original
// message stays in the log and remains recoverable (see AllMessages and
// CompactionHistory). If the session is backed by a store, it is persisted.
//
// Compaction inserts a barrier rather than deleting history. The trade-off is
// storage: a compacted session is larger on disk than its summary alone. That
// is deliberate — retaining the originals is what makes post-compaction
// retrieval possible, recovering the tool results and rationale a lossy
// summary drops.
//
// Returns ErrSuspendedSession if the session is currently suspended.
// Compaction would strand the in-progress tool_use/tool_result messages that
// the resume path depends on, so the caller must first resume the turn to
// completion (via WithResume/WithToolResults) before compacting.
func (s *Session) Compact(ctx context.Context, summarize CompactFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Suspended {
		return ErrSuspendedSession
	}
	active := s.activeEventsLocked()
	var msgs []*llm.Message
	for _, e := range active {
		msgs = append(msgs, e.Messages...)
	}
	compacted, err := summarize(ctx, msgs)
	if err != nil {
		return err
	}
	now := time.Now()
	// withRollback restores Events/UpdatedAt if putSession fails, so a
	// persistence error never leaves the in-memory checkpoint diverged from
	// the store (matching SaveTurn and the suspend paths).
	return s.withRollback(ctx, func() {
		s.data.Events = append(s.data.Events, &event{
			ID:        newEventID(),
			Type:      eventTypeCompaction,
			Timestamp: now,
			Messages:  compacted,
			Metadata: map[string]any{
				"original_event_count":   len(active),
				"original_message_count": len(msgs),
			},
			Revision: s.data.Revision,
		})
		s.data.UpdatedAt = now
	})
}

// CompactionRecord describes a single compaction checkpoint.
type CompactionRecord struct {
	// Summary is the messages the checkpoint produced — the compacted form
	// that became the active context window.
	Summary []*llm.Message
	// ReplacedMessages are the messages this checkpoint superseded: the turns
	// since the previous checkpoint, plus the previous checkpoint's summary if
	// there was one. They remain in the session log and stay recoverable.
	ReplacedMessages []*llm.Message
	// CompactedAt is when the compaction occurred.
	CompactedAt time.Time
}

// CompactionHistory returns one record per compaction checkpoint, in order.
// Because Compact appends a checkpoint rather than collapsing the log, this is
// a genuine history: a session compacted three times returns three records,
// each reporting what it summarized and what it replaced. Returns an empty
// slice when the session has never been compacted.
func (s *Session) CompactionHistory(_ context.Context) ([]CompactionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var records []CompactionRecord
	var replaced []*llm.Message // accumulated since the previous checkpoint
	for _, e := range s.data.Events {
		if e.Type != eventTypeCompaction {
			replaced = append(replaced, e.Messages...)
			continue
		}
		records = append(records, CompactionRecord{
			Summary:          copyMessages(e.Messages),
			ReplacedMessages: copyMessages(replaced),
			CompactedAt:      e.Timestamp,
		})
		// This checkpoint's summary is in turn replaced by the next one.
		replaced = append([]*llm.Message(nil), e.Messages...)
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
func ForkSession(ctx context.Context, store Store, fromID, newID string, opts ...ForkOption) (*Session, error) {
	original, err := store.Open(ctx, fromID)
	if err != nil {
		return nil, err
	}
	forked := original.Fork(newID, opts...)
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
