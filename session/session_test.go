package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// ---------------------------------------------------------------------------
// Session (in-memory via New)
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	sess := session.New("s1")
	assert.Equal(t, "s1", sess.ID())
	assert.Equal(t, 0, sess.EventCount())
}

func TestSessionMessages(t *testing.T) {
	ctx := context.Background()
	sess := session.New("s1")

	err := sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("Hello"),
		llm.NewAssistantTextMessage("Hi"),
	}, &llm.Usage{InputTokens: 10})
	assert.NoError(t, err)

	err = sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("How are you?"),
		llm.NewAssistantTextMessage("Good!"),
	}, &llm.Usage{InputTokens: 20})
	assert.NoError(t, err)

	msgs, err := sess.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(msgs))
	assert.Equal(t, "Hello", msgs[0].Text())
	assert.Equal(t, "Good!", msgs[3].Text())
	assert.Equal(t, 2, sess.EventCount())
}

func TestSessionMessagesEmpty(t *testing.T) {
	sess := session.New("s1")
	msgs, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(msgs))
}

func TestSessionMessagesReturnsCopy(t *testing.T) {
	ctx := context.Background()
	sess := session.New("s1")

	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

	msgs1, _ := sess.Messages(ctx)
	msgs2, _ := sess.Messages(ctx)

	// Mutating one copy should not affect the other.
	msgs1[0] = llm.NewUserTextMessage("Modified")
	assert.Equal(t, "Hello", msgs2[0].Text())
}

func TestSessionTotalUsage(t *testing.T) {
	ctx := context.Background()
	sess := session.New("s1")

	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("a")}, &llm.Usage{InputTokens: 10, OutputTokens: 5})
	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("b")}, &llm.Usage{InputTokens: 20, OutputTokens: 15})
	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("c")}, nil) // nil usage

	total := sess.TotalUsage()
	assert.Equal(t, 30, total.InputTokens)
	assert.Equal(t, 20, total.OutputTokens)
}

func TestSessionTitle(t *testing.T) {
	sess := session.New("s1")
	assert.Equal(t, "", sess.Title())
	sess.SetTitle("My Chat")
	assert.Equal(t, "My Chat", sess.Title())
}

func TestSessionMetadata(t *testing.T) {
	sess := session.New("s1")

	// Initially nil
	assert.Nil(t, sess.Metadata())

	// Set a value
	sess.SetMetadata("workspace", "/tmp/test")
	meta := sess.Metadata()
	assert.Equal(t, "/tmp/test", meta["workspace"])

	// Set another value
	sess.SetMetadata("user", "alice")
	meta = sess.Metadata()
	assert.Equal(t, "/tmp/test", meta["workspace"])
	assert.Equal(t, "alice", meta["user"])

	// Returns a copy (mutations don't affect session)
	meta["workspace"] = "mutated"
	assert.Equal(t, "/tmp/test", sess.Metadata()["workspace"])
}

func TestSessionFork(t *testing.T) {
	ctx := context.Background()
	sess := session.New("original")
	sess.SetTitle("Test Session")

	sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("Hello"),
		llm.NewAssistantTextMessage("Hi"),
	}, nil)

	forked := sess.Fork("forked-id")

	assert.Equal(t, "forked-id", forked.ID())
	assert.Equal(t, "Test Session", forked.Title())
	assert.Equal(t, 1, forked.EventCount())

	forkedMsgs, _ := forked.Messages(ctx)
	assert.Equal(t, 2, len(forkedMsgs))
	assert.Equal(t, "Hello", forkedMsgs[0].Text())

	// Mutating forked should not affect original.
	forked.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Extra")}, nil)
	origMsgs, _ := sess.Messages(ctx)
	assert.Equal(t, 2, len(origMsgs))
}

func TestSessionCompact(t *testing.T) {
	ctx := context.Background()
	sess := session.New("s1")

	sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("msg1"),
		llm.NewAssistantTextMessage("reply1"),
	}, nil)
	sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("msg2"),
		llm.NewAssistantTextMessage("reply2"),
	}, nil)
	assert.Equal(t, 2, sess.EventCount())

	err := sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		return []*llm.Message{llm.NewAssistantTextMessage("Summary of 4 messages")}, nil
	})
	assert.NoError(t, err)

	assert.Equal(t, 1, sess.EventCount())
	msgs, _ := sess.Messages(ctx)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, "Summary of 4 messages", msgs[0].Text())
}

func TestCompactionHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty when never compacted", func(t *testing.T) {
		sess := session.New("s1")
		records, err := sess.CompactionHistory(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(records))
	})

	t.Run("ReplacedMessages contains original messages", func(t *testing.T) {
		sess := session.New("s2")
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("turn1-user"),
			llm.NewAssistantTextMessage("turn1-assistant"),
		}, nil)
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("turn2-user"),
			llm.NewAssistantTextMessage("turn2-assistant"),
		}, nil)

		err := sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("summary")}, nil
		})
		assert.NoError(t, err)

		records, err := sess.CompactionHistory(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(records))

		rec := records[0]
		assert.Equal(t, 1, len(rec.Summary))
		assert.Equal(t, "summary", rec.Summary[0].Text())
		assert.Equal(t, 4, len(rec.ReplacedMessages))
		assert.Equal(t, "turn1-user", rec.ReplacedMessages[0].Text())
		assert.False(t, rec.CompactedAt.IsZero())
	})

	t.Run("second compaction replaces everything including first summary", func(t *testing.T) {
		sess := session.New("s3")
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("orig"),
			llm.NewAssistantTextMessage("reply"),
		}, nil)
		// First compaction
		err := sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("summary-1")}, nil
		})
		assert.NoError(t, err)

		// Add another turn and compact again
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("new"),
			llm.NewAssistantTextMessage("new-reply"),
		}, nil)
		err = sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("summary-2")}, nil
		})
		assert.NoError(t, err)

		// Session now has one compaction event (the second compact replaced everything)
		records, err := sess.CompactionHistory(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(records))
		// The second compaction's replaced messages include summary-1 + new turn
		assert.Equal(t, 3, len(records[0].ReplacedMessages))
	})
}

// ---------------------------------------------------------------------------
// dive.Session interface
// ---------------------------------------------------------------------------

func TestSessionImplementsDiveSession(t *testing.T) {
	var _ dive.Session = session.New("test")
}

// ---------------------------------------------------------------------------
// MemoryStore
// ---------------------------------------------------------------------------

func TestMemoryStore(t *testing.T) {
	t.Run("open creates new session", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()

		sess, err := store.Open(ctx, "s1")
		assert.NoError(t, err)
		assert.Equal(t, "s1", sess.ID())
		assert.Equal(t, 0, sess.EventCount())
	})

	t.Run("open returns existing session", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()

		sess1, _ := store.Open(ctx, "s1")
		sess1.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

		sess2, _ := store.Open(ctx, "s1")
		msgs, _ := sess2.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "Hello", msgs[0].Text())
	})

	t.Run("put saves forked session", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

		forked := sess.Fork("s2")
		err := store.Put(ctx, forked)
		assert.NoError(t, err)

		got, _ := store.Open(ctx, "s2")
		msgs, _ := got.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
	})

	t.Run("save turn persists automatically", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

		// Re-open and verify.
		sess2, _ := store.Open(ctx, "s1")
		msgs, _ := sess2.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "Hello", msgs[0].Text())
	})

	t.Run("delete", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()
		store.Open(ctx, "s1")

		err := store.Delete(ctx, "s1")
		assert.NoError(t, err)

		// After delete, Open creates a new empty session.
		sess, _ := store.Open(ctx, "s1")
		assert.Equal(t, 0, sess.EventCount())
	})

	t.Run("delete idempotent", func(t *testing.T) {
		store := session.NewMemoryStore()
		err := store.Delete(context.Background(), "nope")
		assert.NoError(t, err)
	})

	t.Run("list sessions", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()

		for _, id := range []string{"sA", "sB", "sC"} {
			sess, _ := store.Open(ctx, id)
			sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage(id)}, nil)
		}

		result, err := store.List(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result.Sessions))
	})

	t.Run("list sessions pagination", func(t *testing.T) {
		store := session.NewMemoryStore()
		ctx := context.Background()

		for _, id := range []string{"sA", "sB", "sC", "sD", "sE"} {
			store.Open(ctx, id)
		}

		result, _ := store.List(ctx, &session.ListOptions{Limit: 2})
		assert.Equal(t, 2, len(result.Sessions))

		result, _ = store.List(ctx, &session.ListOptions{Offset: 10})
		assert.Equal(t, 0, len(result.Sessions))
	})
}

// ---------------------------------------------------------------------------
// FileStore
// ---------------------------------------------------------------------------

func TestFileStore(t *testing.T) {
	t.Run("open creates new session", func(t *testing.T) {
		dir := t.TempDir()
		store, err := session.NewFileStore(dir)
		assert.NoError(t, err)

		sess, err := store.Open(context.Background(), "s1")
		assert.NoError(t, err)
		assert.Equal(t, "s1", sess.ID())

		// Verify JSONL file exists.
		_, err = os.Stat(filepath.Join(dir, "s1.jsonl"))
		assert.NoError(t, err)
	})

	t.Run("open returns existing session", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, &llm.Usage{InputTokens: 10})

		// Re-open reads from file.
		sess2, err := store.Open(ctx, "s1")
		assert.NoError(t, err)

		msgs, _ := sess2.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "Hello", msgs[0].Text())
		assert.Equal(t, 1, sess2.EventCount())
	})

	t.Run("multiple save turns", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("More"),
			llm.NewAssistantTextMessage("Reply"),
		}, nil)

		// Re-open and verify.
		got, _ := store.Open(ctx, "s1")
		msgs, _ := got.Messages(ctx)
		assert.Equal(t, 3, len(msgs))
		assert.Equal(t, "Hello", msgs[0].Text())
		assert.Equal(t, "Reply", msgs[2].Text())
	})

	t.Run("put saves forked session", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

		forked := sess.Fork("s2")
		err := store.Put(ctx, forked)
		assert.NoError(t, err)

		_, err = os.Stat(filepath.Join(dir, "s2.jsonl"))
		assert.NoError(t, err)

		got, _ := store.Open(ctx, "s2")
		msgs, _ := got.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
	})

	t.Run("delete", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		store.Open(ctx, "s1")
		err := store.Delete(ctx, "s1")
		assert.NoError(t, err)

		_, err = os.Stat(filepath.Join(dir, "s1.jsonl"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete idempotent", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		err := store.Delete(context.Background(), "nope")
		assert.NoError(t, err)
	})

	t.Run("list sessions", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		for _, id := range []string{"sA", "sB", "sC"} {
			store.Open(ctx, id)
		}

		result, err := store.List(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result.Sessions))
	})

	t.Run("list sessions pagination", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		for _, id := range []string{"sA", "sB", "sC", "sD", "sE"} {
			store.Open(ctx, id)
		}

		result, _ := store.List(ctx, &session.ListOptions{Limit: 2})
		assert.Equal(t, 2, len(result.Sessions))

		result, _ = store.List(ctx, &session.ListOptions{Offset: 10})
		assert.Equal(t, 0, len(result.Sessions))
	})

	t.Run("updated at derived from last event", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

		// Re-open and verify UpdatedAt was updated.
		got, _ := store.Open(ctx, "s1")
		msgs, _ := got.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
	})

	t.Run("compact persists to file", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("msg1"),
			llm.NewAssistantTextMessage("reply1"),
		}, nil)
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("msg2"),
			llm.NewAssistantTextMessage("reply2"),
		}, nil)

		err := sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("Summary")}, nil
		})
		assert.NoError(t, err)

		// Re-open and verify compaction was persisted.
		got, _ := store.Open(ctx, "s1")
		msgs, _ := got.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "Summary", msgs[0].Text())
	})
}

// ---------------------------------------------------------------------------
// Utility functions
// ---------------------------------------------------------------------------

func TestForkSession(t *testing.T) {
	store := session.NewMemoryStore()
	ctx := context.Background()

	sess, _ := store.Open(ctx, "original")
	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

	forked, err := session.ForkSession(ctx, store, "original", "forked")
	assert.NoError(t, err)
	assert.Equal(t, "forked", forked.ID())

	msgs, _ := forked.Messages(ctx)
	assert.Equal(t, 1, len(msgs))

	// Verify it was persisted.
	got, err := store.Open(ctx, "forked")
	assert.NoError(t, err)
	gotMsgs, _ := got.Messages(ctx)
	assert.Equal(t, 1, len(gotMsgs))
}

// ---------------------------------------------------------------------------
// WithSession (core dive package)
// ---------------------------------------------------------------------------

func TestWithSession(t *testing.T) {
	sess := session.New("my-session")
	var opts dive.CreateResponseOptions
	dive.WithSession(sess)(&opts)
	assert.Equal(t, sess, opts.Session)
}

// ---------------------------------------------------------------------------
// WithValue (core dive package)
// ---------------------------------------------------------------------------

func TestWithValue(t *testing.T) {
	var opts dive.CreateResponseOptions
	dive.WithValue("key1", "val1")(&opts)
	dive.WithValue("key2", "val2")(&opts)
	assert.Equal(t, "val1", opts.Values["key1"])
	assert.Equal(t, "val2", opts.Values["key2"])
}

// ---------------------------------------------------------------------------
// Suspend / Resume persistence
// ---------------------------------------------------------------------------

func suspendedTurnMessages() []*llm.Message {
	assistant := &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ToolUseContent{ID: "toolu_a", Name: "approve", Input: []byte(`{}`)},
		},
	}
	toolResult := llm.NewToolResultMessage()
	return []*llm.Message{llm.NewUserTextMessage("start"), assistant, toolResult}
}

func singleSuspensionState() *dive.SuspensionState {
	return &dive.SuspensionState{
		PendingToolCalls: []*dive.PendingToolCall{
			{ID: "toolu_a", Name: "approve", Input: []byte(`{}`)},
		},
		// TurnMessages is populated by the session from the last event
		// on LoadSuspension — we don't need to seed it here because the
		// round-trip tests read the state through that path.
	}
}

func TestMemoryStoreSuspendRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := session.NewMemoryStore()

	sess, err := store.Open(ctx, "s1")
	assert.NoError(t, err)

	msgs := suspendedTurnMessages()
	err = sess.SaveSuspendedTurn(ctx, msgs, &llm.Usage{InputTokens: 5}, singleSuspensionState())
	assert.NoError(t, err)

	state := sess.LoadSuspension()
	assert.NotNil(t, state)
	assert.Equal(t, len(state.PendingToolCalls), 1)
	assert.Equal(t, state.PendingToolCalls[0].ID, "toolu_a")
	// LoadSuspension populates TurnMessages from the last event so stateless
	// and session-backed callers see the same shape.
	assert.Equal(t, len(state.TurnMessages), len(msgs))

	got, _ := sess.Messages(ctx)
	assert.Equal(t, len(got), len(msgs))
}

func TestFileStoreSuspendRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)

	sess, err := store.Open(ctx, "s1")
	assert.NoError(t, err)

	msgs := suspendedTurnMessages()
	err = sess.SaveSuspendedTurn(ctx, msgs, &llm.Usage{InputTokens: 5, OutputTokens: 2}, singleSuspensionState())
	assert.NoError(t, err)

	// Re-open from disk and verify.
	store2, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess2, err := store2.Open(ctx, "s1")
	assert.NoError(t, err)

	state := sess2.LoadSuspension()
	assert.NotNil(t, state)
	assert.Equal(t, len(state.PendingToolCalls), 1)
	assert.Equal(t, state.PendingToolCalls[0].ID, "toolu_a")
	assert.Equal(t, len(state.TurnMessages), len(msgs),
		"LoadSuspension must carry the in-progress turn across process restarts")
	got, _ := sess2.Messages(ctx)
	assert.Equal(t, len(got), len(msgs))
}

func TestListFilterSuspended(t *testing.T) {
	ctx := context.Background()

	stores := []struct {
		name string
		open func(t *testing.T) session.Store
	}{
		{
			name: "memory",
			open: func(t *testing.T) session.Store { return session.NewMemoryStore() },
		},
		{
			name: "file",
			open: func(t *testing.T) session.Store {
				s, err := session.NewFileStore(t.TempDir())
				assert.NoError(t, err)
				return s
			},
		},
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.open(t)

			normal, _ := store.Open(ctx, "normal")
			_ = normal.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("hi")}, nil)

			suspended, _ := store.Open(ctx, "suspended")
			_ = suspended.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, singleSuspensionState())

			trueVal, falseVal := true, false

			// Suspended-only
			res, err := store.List(ctx, &session.ListOptions{Suspended: &trueVal})
			assert.NoError(t, err)
			assert.Equal(t, len(res.Sessions), 1)
			assert.Equal(t, res.Sessions[0].ID, "suspended")

			// Non-suspended only
			res, err = store.List(ctx, &session.ListOptions{Suspended: &falseVal})
			assert.NoError(t, err)
			assert.Equal(t, len(res.Sessions), 1)
			assert.Equal(t, res.Sessions[0].ID, "normal")

			// No filter returns both
			res, err = store.List(ctx, nil)
			assert.NoError(t, err)
			assert.Equal(t, len(res.Sessions), 2)
		})
	}
}

func TestFileStoreListReportsSuspended(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, _ := session.NewFileStore(dir)

	// Normal session
	s1, _ := store.Open(ctx, "normal")
	_ = s1.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("hi")}, nil)

	// Suspended session
	s2, _ := store.Open(ctx, "suspended")
	_ = s2.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, singleSuspensionState())

	res, err := store.List(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(res.Sessions), 2)
	for _, info := range res.Sessions {
		if info.ID == "suspended" {
			assert.True(t, info.Suspended)
		}
		if info.ID == "normal" {
			assert.False(t, info.Suspended)
		}
	}
}

func TestSaveTurnClearsSuspension(t *testing.T) {
	ctx := context.Background()
	sess := session.New("s1")
	err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, singleSuspensionState())
	assert.NoError(t, err)
	assert.NotNil(t, sess.LoadSuspension())

	err = sess.SaveResumedTurn(ctx, append(suspendedTurnMessages(), llm.NewAssistantTextMessage("done")), nil)
	assert.NoError(t, err)
	assert.Nil(t, sess.LoadSuspension())
}

func TestCrossProcessResume(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// "Process A": create session, suspend.
	{
		store, _ := session.NewFileStore(dir)
		sess, _ := store.Open(ctx, "cross")
		err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, singleSuspensionState())
		assert.NoError(t, err)
	}

	// "Process B": fresh store and session, resume the suspended turn.
	{
		store, _ := session.NewFileStore(dir)
		sess, _ := store.Open(ctx, "cross")
		state := sess.LoadSuspension()
		assert.NotNil(t, state)
		assert.Equal(t, len(state.PendingToolCalls), 1)
		assert.Equal(t, state.PendingToolCalls[0].ID, "toolu_a")

		// Simulate completion.
		complete := suspendedTurnMessages()
		// Add a final assistant text message to simulate what generate would append.
		complete = append(complete, llm.NewAssistantTextMessage("done"))
		err := sess.SaveResumedTurn(ctx, complete, nil)
		assert.NoError(t, err)
	}

	// Re-open once more and verify the final state.
	store, _ := session.NewFileStore(dir)
	sess, _ := store.Open(ctx, "cross")
	assert.Nil(t, sess.LoadSuspension())
	msgs, _ := sess.Messages(ctx)
	assert.Equal(t, msgs[len(msgs)-1].Text(), "done")
}

func TestCancelSuspension(t *testing.T) {
	ctx := context.Background()
	sess := session.New("cancel-test")

	// Suspend the session.
	err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, singleSuspensionState())
	assert.NoError(t, err)
	assert.True(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 1)

	// Cancel the suspension.
	err = sess.CancelSuspension(ctx)
	assert.NoError(t, err)
	assert.False(t, sess.IsSuspended())
	assert.Nil(t, sess.LoadSuspension())
	assert.Equal(t, sess.EventCount(), 0)

	// Session is now ready for a fresh turn.
	err = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("fresh start")}, nil)
	assert.NoError(t, err)
	msgs, _ := sess.Messages(ctx)
	assert.Equal(t, len(msgs), 1)
	assert.Equal(t, msgs[0].Text(), "fresh start")
}

func TestCancelSuspensionNotSuspended(t *testing.T) {
	ctx := context.Background()
	sess := session.New("not-suspended")
	err := sess.CancelSuspension(ctx)
	assert.Error(t, err)
	assert.Equal(t, err, session.ErrNotSuspended)
}

func TestCancelSuspensionWithFileStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)

	sess, err := store.Open(ctx, "cancel-file")
	assert.NoError(t, err)

	// Add a normal turn first, then suspend.
	err = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("turn one")}, nil)
	assert.NoError(t, err)
	err = sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, singleSuspensionState())
	assert.NoError(t, err)
	assert.True(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 2)

	err = sess.CancelSuspension(ctx)
	assert.NoError(t, err)

	// Re-open from disk and verify.
	store2, _ := session.NewFileStore(dir)
	sess2, _ := store2.Open(ctx, "cancel-file")
	assert.False(t, sess2.IsSuspended())
	assert.Equal(t, sess2.EventCount(), 1)
	msgs, _ := sess2.Messages(ctx)
	assert.Equal(t, len(msgs), 1)
	assert.Equal(t, msgs[0].Text(), "turn one")
}

func TestSuspendReasonRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, _ := session.NewFileStore(dir)
	sess, _ := store.Open(ctx, "reason")

	state := &dive.SuspensionState{
		PendingToolCalls: []*dive.PendingToolCall{
			{
				ID:     "toolu_a",
				Name:   "auth_gate",
				Input:  []byte(`{}`),
				Reason: dive.SuspendReasonAuth,
				Prompt: "Sign in to continue",
			},
		},
	}
	err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(), nil, state)
	assert.NoError(t, err)

	// Re-open and verify reason survives persistence.
	store2, _ := session.NewFileStore(dir)
	sess2, _ := store2.Open(ctx, "reason")
	loaded := sess2.LoadSuspension()
	assert.NotNil(t, loaded)
	assert.Equal(t, loaded.PendingToolCalls[0].Reason, dive.SuspendReasonAuth)
}
