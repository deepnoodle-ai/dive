package a2a_test

import (
	"context"
	"errors"
	"testing"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

func unusedModelAgent(t *testing.T) *dive.Agent {
	return buildAgent(t, &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("unused"), nil
	}})
}

func suspend(t *testing.T, ctx context.Context, sess dive.SuspendableSession) {
	t.Helper()
	err := sess.SaveSuspendedTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("start"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "ask", Input: []byte(`{}`)},
		}},
	}, nil, &dive.SuspensionState{
		PendingToolCalls: []*dive.PendingToolCall{{ID: "call_1", Name: "ask", Input: []byte(`{}`)}},
	})
	assert.NoError(t, err)
}

type cancelResult struct {
	canceled bool
	err      error
}

// runCancel drives Executor.Cancel and reports whether it yielded the
// canceled status or an error.
func runCancel(ctx context.Context, exec *a2a.Executor) cancelResult {
	var res cancelResult
	for event, err := range exec.Cancel(ctx, &a2asrv.ExecutorContext{TaskID: "task-1", ContextID: "ctx-1"}) {
		if err != nil {
			res.err = err
			continue
		}
		if update, ok := event.(*a2asdk.TaskStatusUpdateEvent); ok && update.Status.State == a2asdk.TaskStateCanceled {
			res.canceled = true
		}
	}
	return res
}

// TestCancelWaitsForSessionLock pins that Cancel removes a suspension only
// after the run holding the session lock has written it, instead of racing
// that write and leaving the suspension behind.
func TestCancelWaitsForSessionLock(t *testing.T) {
	ctx := context.Background()
	sess := session.New("cancel-lock")
	exec := a2a.NewExecutor(unusedModelAgent(t), a2a.WithSessionProvider(func(ctx context.Context, contextID string) (dive.Session, error) {
		return sess, nil
	}))

	// Stand in for a run in progress: it holds the lock and has not yet
	// written its suspension.
	lockedCtx, unlock, err := dive.LockSession(ctx, sess.ID())
	assert.NoError(t, err)

	done := make(chan cancelResult, 1)
	go func() { done <- runCancel(ctx, exec) }()
	select {
	case <-done:
		t.Fatal("Cancel finished while a run held the session lock")
	case <-time.After(30 * time.Millisecond):
	}

	// The run persists its suspension, then releases the lock.
	suspend(t, lockedCtx, sess)
	unlock()

	select {
	case res := <-done:
		assert.NoError(t, res.err)
		assert.True(t, res.canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("Cancel did not finish after the lock was released")
	}
	assert.False(t, sess.IsSuspended())
}

// TestCancelReloadsSessionUnderLock pins that Cancel acts on the session as
// it stands after the run's write, when the provider hands out a separate
// instance per call over shared storage.
func TestCancelReloadsSessionUnderLock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	open := func() *session.Session {
		store, err := session.NewFileStore(dir)
		assert.NoError(t, err)
		sess, err := store.Open(ctx, "cancel-reload")
		assert.NoError(t, err)
		return sess
	}
	exec := a2a.NewExecutor(unusedModelAgent(t), a2a.WithSessionProvider(func(ctx context.Context, contextID string) (dive.Session, error) {
		return open(), nil
	}))

	runSess := open()
	lockedCtx, unlock, err := dive.LockSession(ctx, runSess.ID())
	assert.NoError(t, err)

	done := make(chan cancelResult, 1)
	go func() { done <- runCancel(ctx, exec) }()
	time.Sleep(30 * time.Millisecond)

	suspend(t, lockedCtx, runSess)
	unlock()

	res := <-done
	assert.NoError(t, res.err)
	assert.True(t, res.canceled)
	assert.False(t, open().IsSuspended())
}

// TestCancelReportsLockWaitFailure pins that Cancel does not report the task
// canceled when it gave up waiting for the session and so could not remove
// the suspension.
func TestCancelReportsLockWaitFailure(t *testing.T) {
	ctx := context.Background()
	sess := session.New("cancel-lock-timeout")
	suspend(t, ctx, sess)
	exec := a2a.NewExecutor(unusedModelAgent(t), a2a.WithSessionProvider(func(ctx context.Context, contextID string) (dive.Session, error) {
		return sess, nil
	}))

	_, unlock, err := dive.LockSession(ctx, sess.ID())
	assert.NoError(t, err)
	defer unlock()

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	res := runCancel(waitCtx, exec)
	assert.True(t, errors.Is(res.err, context.DeadlineExceeded))
	assert.False(t, res.canceled)
	assert.True(t, sess.IsSuspended())
}

var errCancelWrite = errors.New("cancel write failed")

type failingCancelSession struct {
	*session.Session
}

func (s failingCancelSession) CancelSuspension(ctx context.Context) error {
	return errCancelWrite
}

// TestCancelReportsCleanupFailure pins that Cancel does not report the task
// canceled when removing the suspension fails.
func TestCancelReportsCleanupFailure(t *testing.T) {
	ctx := context.Background()
	sess := session.New("cancel-write-fails")
	suspend(t, ctx, sess)
	exec := a2a.NewExecutor(unusedModelAgent(t), a2a.WithSessionProvider(func(ctx context.Context, contextID string) (dive.Session, error) {
		return failingCancelSession{sess}, nil
	}))

	res := runCancel(ctx, exec)
	assert.True(t, errors.Is(res.err, errCancelWrite))
	assert.False(t, res.canceled)
}

// TestCancelWithoutSuspension pins that cancelling a task whose session is
// not suspended still reports it canceled.
func TestCancelWithoutSuspension(t *testing.T) {
	sess := session.New("cancel-not-suspended")
	exec := a2a.NewExecutor(unusedModelAgent(t), a2a.WithSessionProvider(func(ctx context.Context, contextID string) (dive.Session, error) {
		return sess, nil
	}))
	res := runCancel(context.Background(), exec)
	assert.NoError(t, res.err)
	assert.True(t, res.canceled)
}
