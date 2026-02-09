package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// ---------------------------------------------------------------------------
// Session (in-memory via New)
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	sess := New("s1")
	assert.Equal(t, "s1", sess.ID())
	assert.Equal(t, 0, sess.EventCount())
}

func TestSessionMessages(t *testing.T) {
	ctx := context.Background()
	sess := New("s1")

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
	sess := New("s1")
	msgs, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(msgs))
}

func TestSessionMessagesReturnsCopy(t *testing.T) {
	ctx := context.Background()
	sess := New("s1")

	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

	msgs1, _ := sess.Messages(ctx)
	msgs2, _ := sess.Messages(ctx)

	// Mutating one copy should not affect the other.
	msgs1[0] = llm.NewUserTextMessage("Modified")
	assert.Equal(t, "Hello", msgs2[0].Text())
}

func TestSessionTotalUsage(t *testing.T) {
	ctx := context.Background()
	sess := New("s1")

	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("a")}, &llm.Usage{InputTokens: 10, OutputTokens: 5})
	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("b")}, &llm.Usage{InputTokens: 20, OutputTokens: 15})
	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("c")}, nil) // nil usage

	total := sess.TotalUsage()
	assert.Equal(t, 30, total.InputTokens)
	assert.Equal(t, 20, total.OutputTokens)
}

func TestSessionTitle(t *testing.T) {
	sess := New("s1")
	assert.Equal(t, "", sess.Title())
	sess.SetTitle("My Chat")
	assert.Equal(t, "My Chat", sess.Title())
}

func TestSessionMetadata(t *testing.T) {
	sess := New("s1")

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
	sess := New("original")
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
	sess := New("s1")

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

// ---------------------------------------------------------------------------
// dive.Session interface
// ---------------------------------------------------------------------------

func TestSessionImplementsDiveSession(t *testing.T) {
	var _ dive.Session = New("test")
}

// ---------------------------------------------------------------------------
// MemoryStore
// ---------------------------------------------------------------------------

func TestMemoryStore(t *testing.T) {
	t.Run("open creates new session", func(t *testing.T) {
		store := NewMemoryStore()
		ctx := context.Background()

		sess, err := store.Open(ctx, "s1")
		assert.NoError(t, err)
		assert.Equal(t, "s1", sess.ID())
		assert.Equal(t, 0, sess.EventCount())
	})

	t.Run("open returns existing session", func(t *testing.T) {
		store := NewMemoryStore()
		ctx := context.Background()

		sess1, _ := store.Open(ctx, "s1")
		sess1.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

		sess2, _ := store.Open(ctx, "s1")
		msgs, _ := sess2.Messages(ctx)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "Hello", msgs[0].Text())
	})

	t.Run("put saves forked session", func(t *testing.T) {
		store := NewMemoryStore()
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
		store := NewMemoryStore()
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
		store := NewMemoryStore()
		ctx := context.Background()
		store.Open(ctx, "s1")

		err := store.Delete(ctx, "s1")
		assert.NoError(t, err)

		// After delete, Open creates a new empty session.
		sess, _ := store.Open(ctx, "s1")
		assert.Equal(t, 0, sess.EventCount())
	})

	t.Run("delete idempotent", func(t *testing.T) {
		store := NewMemoryStore()
		err := store.Delete(context.Background(), "nope")
		assert.NoError(t, err)
	})

	t.Run("list sessions", func(t *testing.T) {
		store := NewMemoryStore()
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
		store := NewMemoryStore()
		ctx := context.Background()

		for _, id := range []string{"sA", "sB", "sC", "sD", "sE"} {
			store.Open(ctx, id)
		}

		result, _ := store.List(ctx, &ListOptions{Limit: 2})
		assert.Equal(t, 2, len(result.Sessions))

		result, _ = store.List(ctx, &ListOptions{Offset: 10})
		assert.Equal(t, 0, len(result.Sessions))
	})
}

// ---------------------------------------------------------------------------
// FileStore
// ---------------------------------------------------------------------------

func TestFileStore(t *testing.T) {
	t.Run("open creates new session", func(t *testing.T) {
		dir := t.TempDir()
		store, err := NewFileStore(dir)
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
		store, _ := NewFileStore(dir)
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
		store, _ := NewFileStore(dir)
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
		store, _ := NewFileStore(dir)
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
		store, _ := NewFileStore(dir)
		ctx := context.Background()

		store.Open(ctx, "s1")
		err := store.Delete(ctx, "s1")
		assert.NoError(t, err)

		_, err = os.Stat(filepath.Join(dir, "s1.jsonl"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete idempotent", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := NewFileStore(dir)
		err := store.Delete(context.Background(), "nope")
		assert.NoError(t, err)
	})

	t.Run("list sessions", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := NewFileStore(dir)
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
		store, _ := NewFileStore(dir)
		ctx := context.Background()

		for _, id := range []string{"sA", "sB", "sC", "sD", "sE"} {
			store.Open(ctx, id)
		}

		result, _ := store.List(ctx, &ListOptions{Limit: 2})
		assert.Equal(t, 2, len(result.Sessions))

		result, _ = store.List(ctx, &ListOptions{Offset: 10})
		assert.Equal(t, 0, len(result.Sessions))
	})

	t.Run("updated at derived from last event", func(t *testing.T) {
		dir := t.TempDir()
		store, _ := NewFileStore(dir)
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
		store, _ := NewFileStore(dir)
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
	store := NewMemoryStore()
	ctx := context.Background()

	sess, _ := store.Open(ctx, "original")
	sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("Hello")}, nil)

	forked, err := ForkSession(ctx, store, "original", "forked")
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
	sess := New("my-session")
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
