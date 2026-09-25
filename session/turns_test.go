package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// completedTurn returns a completed turn record with one exchange.
func completedTurn(id, input, output string) *dive.Turn {
	return &dive.Turn{
		Schema: dive.TurnSchema,
		ID:     id,
		Origin: &dive.TurnOrigin{Kind: dive.TurnOriginInput},
		Status: dive.ResponseStatusCompleted,
		Messages: []*llm.Message{
			llm.NewUserTextMessage(input),
			llm.NewAssistantTextMessage(output),
		},
		Usage: &llm.Usage{InputTokens: 5},
	}
}

// incompleteTurn returns an incomplete turn record, closed with its outcome.
func incompleteTurn(id, input, partial string) *dive.Turn {
	outcome := &dive.TurnOutcome{Reason: dive.TurnReasonOutputLimit, Next: dive.TurnNextContinue}
	return &dive.Turn{
		Schema: dive.TurnSchema,
		ID:     id,
		Status: dive.ResponseStatusIncomplete,
		Messages: dive.CloseTurn([]*llm.Message{
			llm.NewUserTextMessage(input),
			llm.NewAssistantTextMessage(partial),
		}, outcome),
		Usage:   &llm.Usage{InputTokens: 3},
		Outcome: outcome,
	}
}

// suspendedTurn returns a suspended turn waiting for toolu_1, whose sibling
// toolu_2 completed.
func suspendedTurn(id string) *dive.Turn {
	messages := []*llm.Message{
		llm.NewUserTextMessage("go"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "toolu_1", Name: "approve", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "toolu_2", Name: "look", Input: []byte(`{}`)},
		}},
		llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "toolu_2", Content: "seen"}),
	}
	return &dive.Turn{
		Schema:   dive.TurnSchema,
		ID:       id,
		Status:   dive.ResponseStatusSuspended,
		Messages: messages,
		Usage:    &llm.Usage{InputTokens: 4},
		Suspension: &dive.SuspensionState{
			PendingToolCalls: []*dive.PendingToolCall{{ID: "toolu_1", Name: "approve", Input: []byte(`{}`)}},
			TurnMessages:     messages,
			TurnID:           id,
		},
	}
}

// A checkpoint adds a new turn and replaces the open turn with the same ID;
// the revision advances with each write.
func TestCheckpointTurnAddsAndReplaces(t *testing.T) {
	ctx := context.Background()
	sess := session.New("checkpoint")

	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	assert.Equal(t, rev, uint64(1))

	rev, err = sess.CheckpointTurn(ctx, rev, incompleteTurn("turn_2", "count", "one"))
	assert.NoError(t, err)
	assert.Equal(t, rev, uint64(2))

	snap, err := sess.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.Revision, uint64(2))
	assert.Len(t, snap.History, 2)
	assert.NotNil(t, snap.OpenTurn)
	assert.Equal(t, snap.OpenTurn.ID, "turn_2")
	assert.Equal(t, snap.OpenTurn.Status, dive.ResponseStatusIncomplete)
	assert.Equal(t, snap.OpenTurn.Outcome.Reason, dive.TurnReasonOutputLimit)
	assert.Equal(t, snap.OpenTurn.Revision, uint64(2))
	assert.Len(t, snap.OpenTurn.Messages, 3)

	done := completedTurn("turn_2", "count", "one two")
	rev, err = sess.CheckpointTurn(ctx, rev, done)
	assert.NoError(t, err)
	assert.Equal(t, rev, uint64(3))
	assert.Equal(t, sess.EventCount(), 2)

	snap, err = sess.Load(ctx)
	assert.NoError(t, err)
	assert.Nil(t, snap.OpenTurn)
	assert.Len(t, snap.History, 4)
	assert.Equal(t, snap.History[3].Text(), "one two")
}

// A checkpoint at a stale revision writes nothing and is a rejection.
func TestCheckpointTurnRevisionConflict(t *testing.T) {
	ctx := context.Background()
	sess := session.New("conflict")
	_, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)

	_, err = sess.CheckpointTurn(ctx, 0, completedTurn("turn_2", "again", "hello"))
	assert.True(t, errors.Is(err, dive.ErrRevisionConflict))
	assert.True(t, errors.Is(err, dive.ErrSaveRejected))
	assert.Equal(t, sess.EventCount(), 1)
	assert.Equal(t, sess.Revision(), uint64(1))

	// A plain save advances the revision too.
	assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("x")}, nil))
	_, err = sess.CheckpointTurn(ctx, 1, completedTurn("turn_3", "y", "z"))
	assert.True(t, errors.Is(err, dive.ErrRevisionConflict))
}

// A turn ID already recorded and not open cannot be written again, and no
// new turn is added over a suspended one.
func TestCheckpointTurnRefusals(t *testing.T) {
	ctx := context.Background()
	sess := session.New("refusals")
	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	_, err = sess.CheckpointTurn(ctx, rev, completedTurn("turn_1", "hi", "hello again"))
	assert.True(t, errors.Is(err, dive.ErrRevisionConflict))

	rev, err = sess.CheckpointTurn(ctx, rev, suspendedTurn("turn_2"))
	assert.NoError(t, err)
	assert.True(t, sess.IsSuspended())
	_, err = sess.CheckpointTurn(ctx, rev, completedTurn("turn_3", "new", "input"))
	assert.True(t, errors.Is(err, session.ErrSuspendedSession))

	_, err = sess.CheckpointTurn(ctx, rev, &dive.Turn{ID: "turn_4", Status: dive.ResponseStatusSuspended})
	assert.True(t, errors.Is(err, dive.ErrSaveRejected))
	_, err = sess.CheckpointTurn(ctx, rev, &dive.Turn{Status: dive.ResponseStatusCompleted})
	assert.True(t, errors.Is(err, dive.ErrSaveRejected))
}

// A suspended turn's checkpoint sets the suspension state, and a checkpoint
// of the same turn completed clears it.
func TestCheckpointSuspendedTurn(t *testing.T) {
	ctx := context.Background()
	sess := session.New("suspended")
	rev, err := sess.CheckpointTurn(ctx, 0, suspendedTurn("turn_1"))
	assert.NoError(t, err)

	state := sess.LoadSuspension()
	assert.NotNil(t, state)
	assert.Equal(t, state.TurnID, "turn_1")
	assert.Len(t, state.PendingToolCalls, 1)
	snap, err := sess.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.Status, dive.ResponseStatusSuspended)
	assert.Equal(t, snap.OpenTurn.Suspension.PendingToolCalls[0].ID, "toolu_1")
	assert.Len(t, snap.History, 0)

	done := completedTurn("turn_1", "go", "approved")
	_, err = sess.CheckpointTurn(ctx, rev, done)
	assert.NoError(t, err)
	assert.False(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 1)
}

// FileStore keeps the revision and the turn record across a reopen, for an
// appended checkpoint and for a rewrite.
func TestFileStoreKeepsTurnRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := store.Open(ctx, "records")
	assert.NoError(t, err)
	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	rev, err = sess.CheckpointTurn(ctx, rev, incompleteTurn("turn_2", "count", "one"))
	assert.NoError(t, err)

	reopened, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess2, err := reopened.Open(ctx, "records")
	assert.NoError(t, err)
	assert.Equal(t, sess2.Revision(), rev)
	snap, err := sess2.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.ID, "turn_2")
	assert.Equal(t, snap.OpenTurn.Outcome.Reason, dive.TurnReasonOutputLimit)

	// Replacing the open turn rewrites the file.
	rev, err = sess2.CheckpointTurn(ctx, rev, completedTurn("turn_2", "count", "one two"))
	assert.NoError(t, err)
	third, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess3, err := third.Open(ctx, "records")
	assert.NoError(t, err)
	assert.Equal(t, sess3.Revision(), rev)
	turns, err := sess3.Turns(ctx)
	assert.NoError(t, err)
	assert.Len(t, turns, 2)
	assert.Equal(t, turns[0].ID, "turn_1")
	assert.Equal(t, turns[0].Origin.Kind, dive.TurnOriginInput)
	assert.Equal(t, turns[1].Status, dive.ResponseStatusCompleted)
}

// Turns saved before turn records read as turns whose ID is their event's,
// and a legacy incomplete turn is the open turn.
func TestLegacyTurnsLoad(t *testing.T) {
	ctx := context.Background()
	sess := session.New("legacy")
	assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("hi"), llm.NewAssistantTextMessage("hello"),
	}, nil))
	closed := incompleteTurn("", "count", "one").Messages
	assert.NoError(t, sess.SaveTurn(ctx, closed, &llm.Usage{InputTokens: 3}))

	snap, err := sess.Load(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, snap.OpenTurn)
	assert.NotEqual(t, snap.OpenTurn.ID, "")
	assert.Equal(t, snap.OpenTurn.Outcome.Reason, dive.TurnReasonOutputLimit)
	assert.Equal(t, snap.Revision, uint64(2))

	// The legacy open turn can be replaced by its ID.
	_, err = sess.CheckpointTurn(ctx, snap.Revision, completedTurn(snap.OpenTurn.ID, "count", "one two"))
	assert.NoError(t, err)
	assert.Equal(t, sess.EventCount(), 2)
}

// An incomplete turn later turns followed is listed as superseded.
func TestTurnsMarksSuperseded(t *testing.T) {
	ctx := context.Background()
	sess := session.New("superseded")
	rev, err := sess.CheckpointTurn(ctx, 0, incompleteTurn("turn_1", "count", "one"))
	assert.NoError(t, err)
	turns, err := sess.Turns(ctx)
	assert.NoError(t, err)
	assert.False(t, turns[0].Superseded)

	_, err = sess.CheckpointTurn(ctx, rev, completedTurn("turn_2", "other", "thing"))
	assert.NoError(t, err)
	turns, err = sess.Turns(ctx)
	assert.NoError(t, err)
	assert.Len(t, turns, 2)
	assert.True(t, turns[0].Superseded)
	assert.False(t, turns[1].Superseded)

	snap, err := sess.Load(ctx)
	assert.NoError(t, err)
	assert.Nil(t, snap.OpenTurn)
	assert.Len(t, snap.History, 5)
}

// RemoveLastTurn deletes the last turn whatever its state, and refuses when
// there is none.
func TestRemoveLastTurn(t *testing.T) {
	ctx := context.Background()
	sess := session.New("remove")
	assert.True(t, errors.Is(sess.RemoveLastTurn(ctx), session.ErrNoTurn))

	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	_, err = sess.CheckpointTurn(ctx, rev, suspendedTurn("turn_2"))
	assert.NoError(t, err)
	assert.NoError(t, sess.RemoveLastTurn(ctx))
	assert.False(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 1)
	assert.Equal(t, sess.Revision(), uint64(3))

	assert.NoError(t, sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		return []*llm.Message{llm.NewUserTextMessage("summary")}, nil
	}))
	assert.True(t, errors.Is(sess.RemoveLastTurn(ctx), session.ErrNoTurn))
}

// Fork leaves an open turn out unless asked; with ForkWithOpenTurn a
// suspended turn is copied closed and the fork is not suspended.
func TestForkOpenTurn(t *testing.T) {
	ctx := context.Background()
	sess := session.New("fork-src")
	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	_, err = sess.CheckpointTurn(ctx, rev, suspendedTurn("turn_2"))
	assert.NoError(t, err)

	fork := sess.Fork("fork-default")
	assert.Equal(t, fork.EventCount(), 1)
	assert.False(t, fork.IsSuspended())

	fork = sess.Fork("fork-open", session.ForkWithOpenTurn())
	assert.Equal(t, fork.EventCount(), 2)
	assert.False(t, fork.IsSuspended())
	snap, err := fork.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.ID, "turn_2")
	assert.Equal(t, snap.OpenTurn.Status, dive.ResponseStatusIncomplete)
	outcome := snap.OpenTurn.Outcome
	assert.Equal(t, outcome.Reason, dive.TurnReasonCanceled)
	assert.Equal(t, outcome.Next, dive.TurnNextReconcile)
	assert.Len(t, outcome.ToolCalls, 2)
	assert.Equal(t, outcome.ToolCalls[0].State, dive.ToolCallStateUnknown)
	assert.Equal(t, outcome.ToolCalls[1].State, dive.ToolCallStateCompleted)
	// Every call is answered, so the fork's history can be sent.
	assert.Len(t, llm.AnswerUnansweredToolCalls(snap.OpenTurn.Messages), len(snap.OpenTurn.Messages))

	// The original keeps its suspended turn.
	assert.True(t, sess.IsSuspended())

	// An incomplete open turn is copied as it is.
	sess2 := session.New("fork-src-2")
	_, err = sess2.CheckpointTurn(ctx, 0, incompleteTurn("turn_1", "count", "one"))
	assert.NoError(t, err)
	assert.Equal(t, sess2.Fork("f").EventCount(), 0)
	fork = sess2.Fork("f", session.ForkWithOpenTurn())
	snap, err = fork.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.Outcome.Reason, dive.TurnReasonOutputLimit)
}

// The latest turn's outcome is reported after a compaction closed it, and
// cleared once a later turn completes.
func TestLoadLatestOutcome(t *testing.T) {
	ctx := context.Background()
	sess := session.New("latest-outcome")
	rev, err := sess.CheckpointTurn(ctx, 0, incompleteTurn("turn_1", "count", "one"))
	assert.NoError(t, err)
	snap, err := sess.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.LatestOutcome.Reason, dive.TurnReasonOutputLimit)

	assert.NoError(t, sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		return []*llm.Message{llm.NewUserTextMessage("summary")}, nil
	}))
	snap, err = sess.Load(ctx)
	assert.NoError(t, err)
	assert.Nil(t, snap.OpenTurn)
	assert.Equal(t, snap.LatestOutcome.Reason, dive.TurnReasonOutputLimit)

	_, err = sess.CheckpointTurn(ctx, rev+1, completedTurn("turn_2", "next", "done"))
	assert.NoError(t, err)
	snap, err = sess.Load(ctx)
	assert.NoError(t, err)
	assert.Nil(t, snap.LatestOutcome)
}

// A suspension write that replaces a checkpointed turn keeps its identity.
func TestSuspensionWritesKeepTurnIdentity(t *testing.T) {
	ctx := context.Background()
	sess := session.New("identity")
	turn := suspendedTurn("turn_1")
	turn.Origin = &dive.TurnOrigin{Kind: dive.TurnOriginInput}
	_, err := sess.CheckpointTurn(ctx, 0, turn)
	assert.NoError(t, err)

	assert.NoError(t, sess.SaveSuspendedTurn(ctx, turn.Messages, nil, turn.Suspension))
	assert.Equal(t, sess.LoadSuspension().TurnID, "turn_1")

	assert.NoError(t, sess.SaveResumedTurn(ctx, completedTurn("turn_1", "go", "done").Messages, nil))
	turns, err := sess.Turns(ctx)
	assert.NoError(t, err)
	assert.Len(t, turns, 1)
	assert.Equal(t, turns[0].ID, "turn_1")
	assert.Equal(t, turns[0].Origin.Kind, dive.TurnOriginInput)
	assert.Equal(t, turns[0].Status, dive.ResponseStatusCompleted)
}
