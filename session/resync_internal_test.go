package session

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

var errInjected = errors.New("injected store failure")

// faultyStore wraps a FileStore and fails its writes, either before
// anything reaches the file or after the write has landed.
type faultyStore struct {
	*FileStore
	afterWrite bool
	// tornBytes, when set, is written to the session file as a partial
	// line before failing, the way an interrupted append leaves it.
	tornBytes string
}

func (f *faultyStore) appendEvent(ctx context.Context, id string, evt *event) error {
	if f.tornBytes != "" {
		p, err := f.path(id)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, _ = file.WriteString(f.tornBytes)
		_ = file.Close()
		return errInjected
	}
	if f.afterWrite {
		if err := f.FileStore.appendEvent(ctx, id, evt); err != nil {
			return err
		}
	}
	return errInjected
}

func (f *faultyStore) putSession(ctx context.Context, data *sessionData) error {
	if f.afterWrite {
		if err := f.FileStore.putSession(ctx, data); err != nil {
			return err
		}
	}
	return errInjected
}

func openFaulty(t *testing.T, dir string, fs *FileStore, faulty *faultyStore) *Session {
	t.Helper()
	sess, err := fs.Open(context.Background(), "resync")
	assert.NoError(t, err)
	assert.NoError(t, sess.SaveTurn(context.Background(), []*llm.Message{
		llm.NewUserTextMessage("first"),
		llm.NewAssistantTextMessage("one"),
	}, nil))
	faulty.FileStore = fs
	sess.appender = faulty
	return sess
}

func reopen(t *testing.T, dir string) *Session {
	t.Helper()
	fresh, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := fresh.Open(context.Background(), "resync")
	assert.NoError(t, err)
	return sess
}

func suspendedTurn() ([]*llm.Message, *dive.SuspensionState) {
	return []*llm.Message{
			llm.NewUserTextMessage("second"),
			{Role: llm.Assistant, Content: []llm.Content{
				&llm.ToolUseContent{ID: "toolu_a", Name: "approve", Input: []byte(`{}`)},
			}},
		}, &dive.SuspensionState{
			PendingToolCalls: []*dive.PendingToolCall{{ID: "toolu_a", Name: "approve", Input: []byte(`{}`)}},
		}
}

func TestSaveTurnErrorAfterAppendResyncs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess := openFaulty(t, dir, fs, &faultyStore{afterWrite: true})

	err = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("second")}, nil)
	assert.True(t, errors.Is(err, errInjected))

	// The line reached the file, so the cached session keeps the turn
	// instead of rolling it back.
	assert.Equal(t, reopen(t, dir).EventCount(), 2)
	assert.Equal(t, sess.EventCount(), 2)
}

func TestSaveTurnErrorBeforeAppendRollsBack(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess := openFaulty(t, dir, fs, &faultyStore{})

	err = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("second")}, nil)
	assert.True(t, errors.Is(err, errInjected))
	assert.Equal(t, reopen(t, dir).EventCount(), 1)
	assert.Equal(t, sess.EventCount(), 1)
}

func TestSaveTurnTornAppendIsHealed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess := openFaulty(t, dir, fs, &faultyStore{tornBytes: `{"type":"event","data":{"id":`})

	err = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("lost")}, nil)
	assert.True(t, errors.Is(err, errInjected))
	assert.Equal(t, sess.EventCount(), 1)

	// The resync healed the torn line, so the next append does not
	// concatenate onto it and corrupt the file.
	sess.appender = fs
	assert.NoError(t, sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("third")}, nil))
	fresh := reopen(t, dir)
	assert.Equal(t, fresh.EventCount(), 2)
	msgs, err := fresh.Messages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, msgs[len(msgs)-1].Text(), "third")
}

func TestSaveSuspendedTurnErrorAfterWriteResyncs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess := openFaulty(t, dir, fs, &faultyStore{afterWrite: true})

	msgs, state := suspendedTurn()
	err = sess.SaveSuspendedTurn(ctx, msgs, nil, state)
	assert.True(t, errors.Is(err, errInjected))

	// The file was replaced before the error, so the session is suspended
	// on disk; the cached session agrees rather than restoring its snapshot.
	assert.True(t, reopen(t, dir).IsSuspended())
	assert.True(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 2)
	assert.Equal(t, len(sess.LoadSuspension().PendingToolCalls), 1)

	// The same holds for the write that completes the resumed turn.
	err = sess.SaveResumedTurn(ctx, append(msgs, llm.NewAssistantTextMessage("done")), nil)
	assert.True(t, errors.Is(err, errInjected))
	assert.False(t, reopen(t, dir).IsSuspended())
	assert.False(t, sess.IsSuspended())
}

func TestSaveSuspendedTurnErrorBeforeWriteRollsBack(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess := openFaulty(t, dir, fs, &faultyStore{})

	msgs, state := suspendedTurn()
	err = sess.SaveSuspendedTurn(ctx, msgs, nil, state)
	assert.True(t, errors.Is(err, errInjected))
	assert.False(t, reopen(t, dir).IsSuspended())
	assert.False(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 1)
}

func TestResyncKeepsUnsavedTitleAndMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	assert.NoError(t, err)
	sess := openFaulty(t, dir, fs, &faultyStore{})
	sess.SetTitle("renamed")
	sess.SetMetadata("k", "v")

	err = sess.SaveTurn(ctx, []*llm.Message{llm.NewUserTextMessage("second")}, nil)
	assert.True(t, errors.Is(err, errInjected))
	assert.Equal(t, sess.Title(), "renamed")
	assert.Equal(t, sess.Metadata()["k"], "v")
}
