package session

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// ErrNoTurn is returned by RemoveLastTurn when the session has no turn to
// remove: it is empty, or its last event is a compaction.
// errors.Is(err, dive.ErrSaveRejected) holds.
var ErrNoTurn error = &rejectedError{"session has no turn to remove"}

// Session implements dive.TurnStore.
var _ dive.TurnStore = (*Session)(nil)

// eventTurnID returns the ID of the turn an event holds: its recorded turn
// ID, or the event ID for an event saved without a turn record.
func eventTurnID(e *event) string {
	if e.Turn != nil && e.Turn.ID != "" {
		return e.Turn.ID
	}
	return e.ID
}

// eventStatusLocked returns the status of the turn the i-th event holds.
// The header's suspended flag is the authority for the last event. Caller
// must hold s.mu.
func (s *Session) eventStatusLocked(i int) dive.ResponseStatus {
	e := s.data.Events[i]
	last := i == len(s.data.Events)-1
	if last && s.data.Suspended {
		return dive.ResponseStatusSuspended
	}
	if e.Turn != nil && e.Turn.Status != "" && e.Turn.Status != dive.ResponseStatusSuspended {
		return e.Turn.Status
	}
	if _, ok := e.Metadata["outcome"]; ok {
		return dive.ResponseStatusIncomplete
	}
	return dive.ResponseStatusCompleted
}

// openTurnIndexLocked returns the index of the open turn, the last event
// when it holds a suspended or incomplete turn, or -1. Caller must hold s.mu.
func (s *Session) openTurnIndexLocked() int {
	n := len(s.data.Events)
	if n == 0 || s.data.Events[n-1].Type != eventTypeTurn {
		return -1
	}
	if s.eventStatusLocked(n-1) == dive.ResponseStatusCompleted {
		return -1
	}
	return n - 1
}

// turnLocked builds the record of the turn the i-th event holds. Caller
// must hold s.mu.
func (s *Session) turnLocked(i int) *dive.Turn {
	e := s.data.Events[i]
	status := s.eventStatusLocked(i)
	turn := &dive.Turn{
		ID:          eventTurnID(e),
		Revision:    e.Revision,
		Status:      status,
		Messages:    copyMessages(e.Messages),
		Usage:       copyUsage(e.Usage),
		Persistence: dive.PersistenceSaved,
	}
	if turn.Messages == nil {
		turn.Messages = []*llm.Message{}
	}
	if turn.Usage == nil {
		turn.Usage = &llm.Usage{}
	}
	if st := e.Turn; st != nil {
		turn.Schema = st.Schema
		if st.Origin != nil {
			origin := *st.Origin
			turn.Origin = &origin
		}
		turn.Outcome = copyOutcome(st.Outcome)
		turn.ToolCalls = slices.Clone(st.ToolCalls)
	}
	if status == dive.ResponseStatusIncomplete && turn.Outcome == nil {
		turn.Outcome, _ = dive.FindLatestTurnOutcome(e.Messages)
	}
	if status == dive.ResponseStatusSuspended {
		turn.Suspension = cloneSuspensionState(&dive.SuspensionState{
			PendingToolCalls:   s.data.PendingToolCalls,
			CompletedToolCalls: s.data.CompletedToolCalls,
			TurnMessages:       e.Messages,
			BatchHalted:        s.data.BatchHalted,
			TurnID:             turn.ID,
			Usage:              e.Usage,
		})
	}
	return turn
}

// Load returns the active conversation before the open turn, the open turn
// (the last turn, when it is suspended or incomplete), and the session
// revision. It satisfies dive.TurnStore.
func (s *Session) Load(ctx context.Context) (*dive.SessionSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	open := s.openTurnIndexLocked()
	snap := &dive.SessionSnapshot{Revision: s.data.Revision}
	active := s.activeEventsLocked()
	offset := len(s.data.Events) - len(active)
	for i, e := range active {
		if offset+i == open {
			continue
		}
		for _, msg := range e.Messages {
			snap.History = append(snap.History, msg.Copy())
		}
	}
	if open >= 0 {
		snap.OpenTurn = s.turnLocked(open)
	}
	return snap, nil
}

// Revision returns the session revision, which every write to the
// conversation or the suspension state advances (see dive.TurnStore).
func (s *Session) Revision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Revision
}

// Turns returns the record of every turn in the session, oldest first,
// including those a compaction summarized. An incomplete turn that later
// turns followed is marked Superseded. Messages are closed, as the model saw
// them.
func (s *Session) Turns(ctx context.Context) ([]*dive.Turn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var turns []*dive.Turn
	for i, e := range s.data.Events {
		if e.Type != eventTypeTurn {
			continue
		}
		turns = append(turns, s.turnLocked(i))
	}
	for i, turn := range turns {
		if turn.Status == dive.ResponseStatusIncomplete && i < len(turns)-1 {
			turn.Superseded = true
		}
	}
	return turns, nil
}

// CheckpointTurn records a turn and returns the new session revision. It
// satisfies dive.TurnStore: a turn whose ID is the open turn's replaces it,
// any other turn is added at the end, and an expectedRevision that is not
// the session's fails with dive.ErrRevisionConflict. A new turn is refused
// with ErrSuspendedSession while the session is suspended, and a turn whose
// ID is already recorded, but not open, is refused as a conflict. The
// session stores a copy of turn and does not modify it.
func (s *Session) CheckpointTurn(ctx context.Context, expectedRevision uint64, turn *dive.Turn) (uint64, error) {
	if turn == nil || turn.ID == "" {
		return 0, &rejectedError{"session: a checkpointed turn needs an ID"}
	}
	switch turn.Status {
	case dive.ResponseStatusCompleted, dive.ResponseStatusIncomplete:
	case dive.ResponseStatusSuspended:
		if turn.Suspension == nil {
			return 0, &rejectedError{"session: a suspended turn needs its suspension state"}
		}
	default:
		return 0, &rejectedError{fmt.Sprintf("session: cannot checkpoint a turn with status %q", turn.Status)}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Revision != expectedRevision {
		return 0, fmt.Errorf("%w: session %s is at revision %d, not %d", dive.ErrRevisionConflict, s.data.ID, s.data.Revision, expectedRevision)
	}
	open := s.openTurnIndexLocked()
	replace := open >= 0 && eventTurnID(s.data.Events[open]) == turn.ID
	if !replace {
		if s.data.Suspended {
			return 0, ErrSuspendedSession
		}
		for _, e := range s.data.Events {
			if e.Type == eventTypeTurn && eventTurnID(e) == turn.ID {
				return 0, fmt.Errorf("%w: turn %s is recorded and not open", dive.ErrRevisionConflict, turn.ID)
			}
		}
	}

	now := time.Now()
	evt := &event{
		ID:        newEventID(),
		Type:      eventTypeTurn,
		Timestamp: now,
		Messages:  copyMessages(turn.Messages),
		Usage:     copyUsage(turn.Usage),
		Metadata:  checkpointMetadata(turn),
		Revision:  s.data.Revision + 1,
		Turn: (&turnState{
			Schema:    turn.Schema,
			ID:        turn.ID,
			Origin:    turn.Origin,
			Status:    turn.Status,
			Outcome:   turn.Outcome,
			ToolCalls: turn.ToolCalls,
		}).copy(),
	}
	if replace {
		evt.ID = s.data.Events[open].ID
	}
	suspended := turn.Status == dive.ResponseStatusSuspended

	// A completed or incomplete turn added to a session that is not
	// suspended changes nothing but the log: append it. Any other write
	// replaces an event or the suspension state, so the store rewrites the
	// session.
	if !replace && !suspended {
		return evt.Revision, s.appendLocked(ctx, evt)
	}
	err := s.withRollback(ctx, func() {
		if replace {
			s.data.Events[open] = evt
		} else {
			s.data.Events = append(s.data.Events, evt)
		}
		s.data.Suspended = suspended
		s.data.PendingToolCalls = nil
		s.data.CompletedToolCalls = nil
		s.data.BatchHalted = false
		if suspended {
			state := cloneSuspensionState(turn.Suspension)
			s.data.PendingToolCalls = state.PendingToolCalls
			s.data.CompletedToolCalls = state.CompletedToolCalls
			s.data.BatchHalted = state.BatchHalted
		}
		s.data.UpdatedAt = now
	})
	if err != nil {
		return 0, err
	}
	return evt.Revision, nil
}

// checkpointMetadata is the metadata of a checkpointed turn's event, the
// same keys the other writes set: "suspended" for a suspended turn and
// "outcome" for an incomplete one.
func checkpointMetadata(turn *dive.Turn) map[string]any {
	switch turn.Status {
	case dive.ResponseStatusSuspended:
		return map[string]any{"suspended": true}
	case dive.ResponseStatusIncomplete:
		if turn.Outcome != nil {
			return map[string]any{"outcome": string(turn.Outcome.Reason)}
		}
		return outcomeMetadata(turn.Messages)
	}
	return nil
}

// RemoveLastTurn deletes the last turn from the session, whatever its state,
// and clears the suspension state. The turn is gone from the log, unlike a
// turn closed with Agent.CancelSuspendedTurn. It returns ErrNoTurn when the
// session is empty or its last event is a compaction.
func (s *Session) RemoveLastTurn(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.data.Events)
	if n == 0 || s.data.Events[n-1].Type != eventTypeTurn {
		return ErrNoTurn
	}
	return s.withRollback(ctx, func() {
		s.data.Events = s.data.Events[:n-1]
		s.data.Suspended = false
		s.data.PendingToolCalls = nil
		s.data.CompletedToolCalls = nil
		s.data.BatchHalted = false
		s.data.UpdatedAt = time.Now()
	})
}

// closedSuspendedEventLocked returns a copy of the suspended turn's event
// closed incomplete: each pending call answered as unknown, the other calls
// of the batch as completed or not started. Caller must hold s.mu.
func (s *Session) closedSuspendedEventLocked(e *event) *event {
	cp := e.copy()
	pending := map[string]bool{}
	for _, call := range s.data.PendingToolCalls {
		pending[call.ID] = true
	}
	answered := map[string]bool{}
	var assistant *llm.Message
	for _, msg := range e.Messages {
		for _, c := range msg.Content {
			switch c := c.(type) {
			case *llm.ToolResultContent:
				answered[c.ToolUseID] = true
			case *llm.ToolUseContent:
				assistant = msg
			}
		}
	}
	var records []dive.ToolCallRecord
	if assistant != nil {
		for _, c := range assistant.Content {
			call, ok := c.(*llm.ToolUseContent)
			if !ok {
				continue
			}
			state := dive.ToolCallStateNotStarted
			switch {
			case pending[call.ID]:
				state = dive.ToolCallStateUnknown
			case answered[call.ID]:
				state = dive.ToolCallStateCompleted
			}
			records = append(records, dive.ToolCallRecord{ID: call.ID, Name: call.Name, State: state})
		}
	}
	outcome := &dive.TurnOutcome{
		Reason:    dive.TurnReasonCanceled,
		Error:     "the suspended turn was not carried into a fork",
		ToolCalls: records,
		Next:      dive.TurnNextReconcile,
	}
	cp.Messages = dive.CloseTurn(cp.Messages, outcome)
	recorded, _ := dive.FindLatestTurnOutcome(cp.Messages)
	cp.Metadata = map[string]any{"outcome": string(dive.TurnReasonCanceled)}
	cp.Turn = &turnState{
		Schema:  dive.TurnSchema,
		ID:      eventTurnID(e),
		Status:  dive.ResponseStatusIncomplete,
		Outcome: recorded,
	}
	if e.Turn != nil {
		cp.Turn.Origin = e.Turn.Origin
	}
	cp.Turn.ToolCalls = slices.Clone(recorded.ToolCalls)
	return cp
}
