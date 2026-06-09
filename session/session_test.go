package session_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	// Compaction appends a checkpoint rather than collapsing the log, so the
	// turn events remain (2 turns + 1 checkpoint = 3 events).
	assert.Equal(t, 3, sess.EventCount())

	// The active window is just the summary.
	msgs, _ := sess.Messages(ctx)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, "Summary of 4 messages", msgs[0].Text())

	// The originals remain recoverable: 4 turn messages + the summary.
	all, _ := sess.AllMessages(ctx)
	assert.Equal(t, 5, len(all))
	assert.Equal(t, "msg1", all[0].Text())
	assert.Equal(t, "reply2", all[3].Text())
	assert.Equal(t, "Summary of 4 messages", all[4].Text())
}

func TestCompactSuspendedSession(t *testing.T) {
	ctx := context.Background()
	sess := session.New("s1")

	msgs := suspendedTurnMessages()
	err := sess.SaveSuspendedTurn(ctx, msgs, nil, singleSuspensionState())
	assert.NoError(t, err)

	err = sess.Compact(ctx, func(_ context.Context, _ []*llm.Message) ([]*llm.Message, error) {
		return nil, nil
	})
	assert.ErrorIs(t, err, session.ErrSuspendedSession)
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

	t.Run("each compaction is a distinct checkpoint in history", func(t *testing.T) {
		sess := session.New("s3")
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("orig"),
			llm.NewAssistantTextMessage("reply"),
		}, nil)
		// First compaction summarizes the two original messages.
		err := sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("summary-1")}, nil
		})
		assert.NoError(t, err)

		// Add another turn and compact again. The active window is now
		// summary-1 + the new turn, so that is what the second checkpoint
		// summarizes and replaces.
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("new"),
			llm.NewAssistantTextMessage("new-reply"),
		}, nil)
		err = sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("summary-2")}, nil
		})
		assert.NoError(t, err)

		// Genuine history: two checkpoints, two records.
		records, err := sess.CompactionHistory(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(records))

		// First checkpoint replaced the two originals and produced summary-1.
		assert.Equal(t, 2, len(records[0].ReplacedMessages))
		assert.Equal(t, "orig", records[0].ReplacedMessages[0].Text())
		assert.Equal(t, "summary-1", records[0].Summary[0].Text())

		// Second checkpoint replaced summary-1 + the new turn (3 messages).
		assert.Equal(t, 3, len(records[1].ReplacedMessages))
		assert.Equal(t, "summary-1", records[1].ReplacedMessages[0].Text())
		assert.Equal(t, "new", records[1].ReplacedMessages[1].Text())
		assert.Equal(t, "summary-2", records[1].Summary[0].Text())

		// The active window is the latest summary only.
		msgs, _ := sess.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "summary-2", msgs[0].Text())

		// And the full transcript still holds every original.
		all, _ := sess.AllMessages(ctx)
		assert.Equal(t, "orig", all[0].Text())
		assert.Equal(t, "new", all[3].Text())
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

	t.Run("CompactionHistory persists ReplacedMessages to file", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)
		ctx := context.Background()

		sess, _ := store.Open(ctx, "s1")
		sess.SaveTurn(ctx, []*llm.Message{
			llm.NewUserTextMessage("original-user"),
			llm.NewAssistantTextMessage("original-assistant"),
		}, nil)

		err := sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return []*llm.Message{llm.NewAssistantTextMessage("Summary")}, nil
		})
		assert.NoError(t, err)

		// Re-open and verify CompactionHistory round-trips correctly.
		got, _ := store.Open(ctx, "s1")
		records, err := got.CompactionHistory(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(records))

		rec := records[0]
		assert.Equal(t, 1, len(rec.Summary))
		assert.Equal(t, "Summary", rec.Summary[0].Text())
		assert.Equal(t, 2, len(rec.ReplacedMessages))
		assert.Equal(t, "original-user", rec.ReplacedMessages[0].Text())
		assert.Equal(t, "original-assistant", rec.ReplacedMessages[1].Text())
		assert.False(t, rec.CompactedAt.IsZero())
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

// ---------------------------------------------------------------------------
// FileStore durability
// ---------------------------------------------------------------------------

func TestFileStoreTornAppendRecovery(t *testing.T) {
	ctx := context.Background()

	t.Run("corrupt final line is dropped and healed", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)

		sess, _ := store.Open(ctx, "s1")
		assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("one")}, nil))
		assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("two")}, nil))

		// Simulate a crash mid-append: a partial JSONL line with no
		// trailing newline.
		p := filepath.Join(dir, "s1.jsonl")
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.Write([]byte(`{"line_type":"event","data":{"trunc`))
		assert.NoError(t, err)
		assert.NoError(t, f.Close())

		// Open succeeds with the prior events intact.
		store2, _ := session.NewFileStore(dir)
		got, err := store2.Open(ctx, "s1")
		assert.NoError(t, err)
		msgs, err := got.Messages(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(msgs))
		assert.Equal(t, "one", msgs[0].Text())
		assert.Equal(t, "two", msgs[1].Text())

		// Open healed the file, so subsequent appends must not concatenate
		// onto the torn garbage and corrupt the session.
		assert.NoError(t, got.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("three")}, nil))
		store3, _ := session.NewFileStore(dir)
		again, err := store3.Open(ctx, "s1")
		assert.NoError(t, err)
		msgs, err = again.Messages(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(msgs))
		assert.Equal(t, "three", msgs[2].Text())
	})

	t.Run("mid-file corruption still errors", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := session.NewFileStore(dir)

		sess, _ := store.Open(ctx, "s1")
		assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("one")}, nil))
		assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("two")}, nil))

		// Corrupt a line in the middle of the file (a bad line followed by
		// valid lines is real corruption, not a torn append).
		p := filepath.Join(dir, "s1.jsonl")
		b, err := os.ReadFile(p)
		assert.NoError(t, err)
		lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		assert.Equal(t, 3, len(lines)) // header + 2 events
		lines[1] = `{"line_type":"event","data":{"trunc`
		assert.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0644))

		store2, _ := session.NewFileStore(dir)
		_, err = store2.Open(ctx, "s1")
		assert.Error(t, err)
	})
}

func TestFileStoreLargeEvent(t *testing.T) {
	dir := t.TempDir()
	store, _ := session.NewFileStore(dir)
	ctx := context.Background()

	// 2MB message — beyond the 1MB line cap the old scanner-based reader
	// imposed, which made oversized sessions unreadable after a save.
	large := strings.Repeat("x", 2*1024*1024)
	sess, _ := store.Open(ctx, "big")
	assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage(large)}, nil))

	store2, _ := session.NewFileStore(dir)
	got, err := store2.Open(ctx, "big")
	assert.NoError(t, err)
	msgs, err := got.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, large, msgs[0].Text())

	// List must also tolerate the oversized line.
	result, err := store2.List(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Sessions))
}

func TestFileStoreUnrecognizedHeaderNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	store, _ := session.NewFileStore(dir)
	ctx := context.Background()

	// A file with an unrecognized first line type (e.g. written by a future
	// version) must cause Open to fail, NOT to treat the session as missing
	// and destructively overwrite the file with a fresh empty session.
	content := `{"line_type":"header_v2","data":{"id":"s1"}}` + "\n"
	p := filepath.Join(dir, "s1.jsonl")
	assert.NoError(t, os.WriteFile(p, []byte(content), 0644))

	_, err := store.Open(ctx, "s1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unrecognized first line type")

	// The file must be untouched.
	b, err := os.ReadFile(p)
	assert.NoError(t, err)
	assert.Equal(t, content, string(b))
}

func TestFileStorePutConcurrentWithSessionMutation(t *testing.T) {
	dir := t.TempDir()
	store, _ := session.NewFileStore(dir)
	ctx := context.Background()

	sess, _ := store.Open(ctx, "s1")

	// Put reads sess.data (events + metadata) while other goroutines mutate
	// the session. Put must take the session lock; run under -race to catch
	// regressions.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			sess.SetMetadata("k", i)
			_ = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("hi")}, nil)
		}
	}()
	for i := 0; i < 50; i++ {
		assert.NoError(t, store.Put(ctx, sess))
	}
	<-done
}

// ---------------------------------------------------------------------------
// MemoryStore metadata isolation
// ---------------------------------------------------------------------------

func TestMemoryStoreListMetadataIsCopy(t *testing.T) {
	store := session.NewMemoryStore()
	ctx := context.Background()

	sess, _ := store.Open(ctx, "s1")
	sess.SetMetadata("key", "original")

	result, err := store.List(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Sessions))

	// Mutating the returned metadata must not affect the live session.
	result.Sessions[0].Metadata["key"] = "mutated"
	assert.Equal(t, "original", sess.Metadata()["key"])
}

// TestSuspendResumeUsageAccumulates pins that the usage paid before a
// suspension is carried into the completed turn when SaveResumedTurn
// replaces the suspended event, so TotalUsage reflects both phases
// (review finding §3.5).
func TestSuspendResumeUsageAccumulates(t *testing.T) {
	ctx := context.Background()
	sess := session.New("usage-sum")

	// Suspend with usage A.
	err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(),
		&llm.Usage{InputTokens: 10, OutputTokens: 5}, singleSuspensionState())
	assert.NoError(t, err)

	// Resume with usage B.
	complete := append(suspendedTurnMessages(), llm.NewAssistantTextMessage("done"))
	err = sess.SaveResumedTurn(ctx, complete, &llm.Usage{InputTokens: 20, OutputTokens: 7})
	assert.NoError(t, err)

	total := sess.TotalUsage()
	assert.Equal(t, total.InputTokens, 30)
	assert.Equal(t, total.OutputTokens, 12)
}

// TestPartialResumeUsageAccumulates pins the replace-last branch of
// SaveSuspendedTurn: a partial resume that re-suspends must also carry the
// already-paid usage forward, and a final SaveResumedTurn adds its own on
// top (review finding §3.5).
func TestPartialResumeUsageAccumulates(t *testing.T) {
	ctx := context.Background()
	sess := session.New("usage-partial")

	// Initial suspend with usage A.
	err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(),
		&llm.Usage{InputTokens: 10, OutputTokens: 1}, singleSuspensionState())
	assert.NoError(t, err)

	// Partial resume re-suspends, replacing the suspended event with usage B.
	err = sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(),
		&llm.Usage{InputTokens: 20, OutputTokens: 2}, singleSuspensionState())
	assert.NoError(t, err)
	assert.Equal(t, sess.EventCount(), 1, "replace-last must not append a second event")

	// Final resume completes the turn with usage C.
	complete := append(suspendedTurnMessages(), llm.NewAssistantTextMessage("done"))
	err = sess.SaveResumedTurn(ctx, complete, &llm.Usage{InputTokens: 40, OutputTokens: 4})
	assert.NoError(t, err)

	total := sess.TotalUsage()
	assert.Equal(t, total.InputTokens, 70)
	assert.Equal(t, total.OutputTokens, 7)
}

// TestSuspendResumeNilUsage pins that summation tolerates nil usage on
// either side of a suspend/resume pair.
func TestSuspendResumeNilUsage(t *testing.T) {
	ctx := context.Background()
	sess := session.New("usage-nil")

	err := sess.SaveSuspendedTurn(ctx, suspendedTurnMessages(),
		&llm.Usage{InputTokens: 10}, singleSuspensionState())
	assert.NoError(t, err)
	err = sess.SaveResumedTurn(ctx,
		append(suspendedTurnMessages(), llm.NewAssistantTextMessage("done")), nil)
	assert.NoError(t, err)
	assert.Equal(t, sess.TotalUsage().InputTokens, 10)
}

// TestSaveTurnCopiesCallerMessages pins that SaveTurn deep-copies messages
// and usage on ingestion: mutating the caller's message (or usage) after
// the call must not rewrite stored history (review finding §3.4).
func TestSaveTurnCopiesCallerMessages(t *testing.T) {
	ctx := context.Background()
	sess := session.New("no-alias")

	userMsg := llm.NewUserTextMessage("original input")
	assistantMsg := llm.NewAssistantTextMessage("original output")
	usage := &llm.Usage{InputTokens: 10}
	err := sess.SaveTurn(ctx, []*llm.Message{userMsg, assistantMsg}, usage)
	assert.NoError(t, err)

	// Mutate the caller-owned messages and usage after the save.
	userMsg.Content[0].(*llm.TextContent).Text = "mutated input"
	assistantMsg.Content[0].(*llm.TextContent).Text = "mutated output"
	usage.InputTokens = 999

	msgs, err := sess.Messages(ctx)
	assert.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, msgs[0].Text(), "original input")
	assert.Equal(t, msgs[1].Text(), "original output")
	assert.Equal(t, sess.TotalUsage().InputTokens, 10)
}

// TestSaveSuspendedTurnCopiesCallerMessages extends the §3.4 aliasing fix to
// the suspend/resume ingestion paths.
func TestSaveSuspendedTurnCopiesCallerMessages(t *testing.T) {
	ctx := context.Background()
	sess := session.New("no-alias-suspend")

	turn := suspendedTurnMessages()
	err := sess.SaveSuspendedTurn(ctx, turn, nil, singleSuspensionState())
	assert.NoError(t, err)

	// Mutate the caller's copy of the user message after the save.
	turn[0].Content[0].(*llm.TextContent).Text = "mutated"

	msgs, err := sess.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, msgs[0].Text(), "start")

	// Same for the resume completion path.
	complete := append(suspendedTurnMessages(), llm.NewAssistantTextMessage("done"))
	err = sess.SaveResumedTurn(ctx, complete, nil)
	assert.NoError(t, err)
	complete[len(complete)-1].Content[0].(*llm.TextContent).Text = "mutated done"

	msgs, err = sess.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, msgs[len(msgs)-1].Text(), "done")
}

// ---------------------------------------------------------------------------
// FileStore Open caching (review §3.3: double-Open divergence)
// ---------------------------------------------------------------------------

func TestFileStoreOpenReturnsSharedInstance(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	h1, err := store.Open(ctx, "s1")
	assert.NoError(t, err)
	h2, err := store.Open(ctx, "s1")
	assert.NoError(t, err)

	// Both handles must be the same shared instance, mirroring MemoryStore.
	assert.True(t, h1 == h2)

	// A turn saved through one handle is visible through the other.
	assert.NoError(t, h1.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("one")}, nil))
	msgs, err := h2.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, "one", msgs[0].Text())

	// Distinct IDs still get distinct sessions.
	other, err := store.Open(ctx, "s2")
	assert.NoError(t, err)
	assert.True(t, h1 != other)
}

func TestFileStoreDoubleOpenFullRewriteNoDataLoss(t *testing.T) {
	// The original §3.3 data-loss scenario: open the same ID twice, append
	// a turn through each handle, then trigger a full file rewrite through
	// one of them. Before Open caching, each Open returned a divergent
	// in-memory copy and the rewrite silently deleted the other handle's
	// turn from disk.
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	h1, err := store.Open(ctx, "s1")
	assert.NoError(t, err)
	h2, err := store.Open(ctx, "s1")
	assert.NoError(t, err)

	assert.NoError(t, h1.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("from-h1")}, nil))
	assert.NoError(t, h2.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("from-h2")}, nil))

	// SaveSuspendedTurn performs a full rewrite of the JSONL file from h1's
	// in-memory state.
	assert.NoError(t, h1.SaveSuspendedTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("suspend-turn"),
	}, nil, singleSuspensionState()))

	// Re-read from disk with a fresh store: every turn must have survived.
	store2, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	got, err := store2.Open(ctx, "s1")
	assert.NoError(t, err)
	msgs, err := got.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(msgs))
	assert.Equal(t, "from-h1", msgs[0].Text())
	assert.Equal(t, "from-h2", msgs[1].Text())
	assert.Equal(t, "suspend-turn", msgs[2].Text())
	assert.True(t, got.IsSuspended())

	// store.Put is the other full-rewrite path; it must also rewrite from
	// the single shared state.
	assert.NoError(t, h2.CancelSuspension(ctx))
	assert.NoError(t, store.Put(ctx, h1))
	store3, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	got, err = store3.Open(ctx, "s1")
	assert.NoError(t, err)
	msgs, err = got.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(msgs))
	assert.Equal(t, "from-h1", msgs[0].Text())
	assert.Equal(t, "from-h2", msgs[1].Text())
}

func TestFileStoreDeleteEvictsCachedSession(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	h1, err := store.Open(ctx, "s1")
	assert.NoError(t, err)
	assert.NoError(t, h1.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("old")}, nil))

	assert.NoError(t, store.Delete(ctx, "s1"))

	// A subsequent Open must create fresh state, not resurrect the deleted
	// session from the cache.
	h2, err := store.Open(ctx, "s1")
	assert.NoError(t, err)
	assert.True(t, h1 != h2)
	msgs, err := h2.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(msgs))
}

func TestFileStorePutAdoptsSessionIntoCache(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	orig, err := store.Open(ctx, "orig")
	assert.NoError(t, err)
	assert.NoError(t, orig.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("hello")}, nil))

	// Fork creates a detached session; Put must both persist it and adopt
	// it into the cache so Open aliases the same instance (as MemoryStore
	// does).
	forked := orig.Fork("forked")
	assert.NoError(t, store.Put(ctx, forked))

	got, err := store.Open(ctx, "forked")
	assert.NoError(t, err)
	assert.True(t, got == forked)
}

func TestFileStoreConcurrentOpenSingleInstance(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	// Concurrent Opens of the same ID must all resolve to one instance and
	// concurrent SaveTurns through them must not race (run under -race).
	const n = 8
	handles := make([]*session.Session, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h, err := store.Open(ctx, "s1")
			assert.NoError(t, err)
			assert.NoError(t, h.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("hi")}, nil))
			handles[i] = h
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		assert.True(t, handles[0] == handles[i])
	}
	msgs, err := handles[0].Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, n, len(msgs))
}
