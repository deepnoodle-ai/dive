package a2a_test

import (
	"context"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestCancelWaitsForSessionLock pins that Cancel removes a suspension only
// after the run holding the session lock has written it, instead of racing
// that write and leaving the suspension behind.
func TestCancelWaitsForSessionLock(t *testing.T) {
	ctx := context.Background()
	sess := session.New("cancel-lock")
	agent := buildAgent(t, &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("unused"), nil
	}})
	exec := a2a.NewExecutor(agent, a2a.WithSessionProvider(func(ctx context.Context, contextID string) (dive.Session, error) {
		return sess, nil
	}))

	// Stand in for a run in progress: it holds the lock and has not yet
	// written its suspension.
	lockedCtx, unlock, err := dive.LockSession(ctx, sess.ID())
	assert.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range exec.Cancel(ctx, &a2asrv.ExecutorContext{TaskID: "task-1", ContextID: "ctx-1"}) {
		}
	}()
	select {
	case <-done:
		t.Fatal("Cancel finished while a run held the session lock")
	case <-time.After(30 * time.Millisecond):
	}

	// The run persists its suspension, then releases the lock.
	err = sess.SaveSuspendedTurn(lockedCtx, []*llm.Message{
		llm.NewUserTextMessage("start"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "ask", Input: []byte(`{}`)},
		}},
	}, nil, &dive.SuspensionState{
		PendingToolCalls: []*dive.PendingToolCall{{ID: "call_1", Name: "ask", Input: []byte(`{}`)}},
	})
	assert.NoError(t, err)
	unlock()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Cancel did not finish after the lock was released")
	}
	assert.False(t, sess.IsSuspended())
}
