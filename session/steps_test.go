package session_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// runningTurn returns a running turn record with the given messages and
// call states, as a step checkpoint stores it.
func runningTurn(id string, messages []*llm.Message, calls ...dive.ToolCallRecord) *dive.Turn {
	return &dive.Turn{
		Schema:    dive.TurnSchema,
		ID:        id,
		Origin:    &dive.TurnOrigin{Kind: dive.TurnOriginInput},
		Status:    dive.ResponseStatusRunning,
		Messages:  messages,
		Usage:     &llm.Usage{InputTokens: 7},
		ToolCalls: calls,
	}
}

// stepMessages returns the messages of a turn that called two tools, as its
// steps grow: the input, the model's calls, one result, then both.
func stepMessages() [][]*llm.Message {
	input := llm.NewUserTextMessage("look twice")
	calls := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
		&llm.ToolUseContent{ID: "toolu_1", Name: "look", Input: []byte(`{}`)},
		&llm.ToolUseContent{ID: "toolu_2", Name: "write", Input: []byte(`{}`)},
	}}
	one := llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "toolu_1", Content: "seen"})
	both := llm.NewToolResultMessage(
		&llm.ToolResultContent{ToolUseID: "toolu_1", Content: "seen"},
		&llm.ToolResultContent{ToolUseID: "toolu_2", Content: "written"},
	)
	return [][]*llm.Message{
		{input},
		{input, calls},
		{input, calls, one},
		{input, calls, both},
	}
}

// fileLines returns the line types of a session file, in order, and the
// data of each line.
func fileLines(t *testing.T, path string) ([]string, []json.RawMessage) {
	t.Helper()
	f, err := os.Open(path)
	assert.NoError(t, err)
	defer f.Close()
	var types []string
	var data []json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var line struct {
			LineType string          `json:"line_type"`
			Data     json.RawMessage `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(scanner.Bytes(), &line))
		types = append(types, line.LineType)
		data = append(data, line.Data)
	}
	return types, data
}

// legacyEvents returns the IDs of the events a reader that skips step lines,
// as versions before step checkpoints do, sees in a session file, and the
// number of messages of each.
func legacyEvents(t *testing.T, path string) ([]string, []int) {
	t.Helper()
	types, data := fileLines(t, path)
	var ids []string
	var counts []int
	for i, lineType := range types {
		if lineType != "event" {
			continue
		}
		var evt struct {
			ID       string            `json:"id"`
			Messages []json.RawMessage `json:"messages"`
		}
		assert.NoError(t, json.Unmarshal(data[i], &evt))
		ids = append(ids, evt.ID)
		counts = append(counts, len(evt.Messages))
	}
	return ids, counts
}

// Step checkpoints of a running turn each append one line, the change since
// the last; the finished record appends the event, which replaces them. A
// reader that skips step lines sees each finished turn once.
func TestStepCheckpointsAppend(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := store.Open(ctx, "steps")
	assert.NoError(t, err)
	path := filepath.Join(dir, "steps.jsonl")

	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)

	steps := stepMessages()
	states := [][]dive.ToolCallRecord{
		nil,
		{{ID: "toolu_1", Name: "look", State: dive.ToolCallStateRunning}, {ID: "toolu_2", Name: "write", State: dive.ToolCallStateRunning}},
		{{ID: "toolu_1", Name: "look", State: dive.ToolCallStateCompleted}, {ID: "toolu_2", Name: "write", State: dive.ToolCallStateRunning}},
		{{ID: "toolu_1", Name: "look", State: dive.ToolCallStateCompleted}, {ID: "toolu_2", Name: "write", State: dive.ToolCallStateCompleted}},
	}
	for i, messages := range steps {
		rev, err = sess.CheckpointTurn(ctx, rev, runningTurn("turn_2", messages, states[i]...))
		assert.NoError(t, err)
		types, _ := fileLines(t, path)
		assert.Equal(t, len(types), 3+i, "one line per step")
		assert.Equal(t, types[len(types)-1], "step")
	}

	// The third step replaced the one-result message with the two-result
	// one: the line holds that message alone.
	_, data := fileLines(t, path)
	var change struct {
		From     int               `json:"from"`
		Messages []json.RawMessage `json:"messages"`
	}
	assert.NoError(t, json.Unmarshal(data[len(data)-1], &change))
	assert.Equal(t, change.From, 2)
	assert.Len(t, change.Messages, 1)

	// Another process sees the running turn as the last step left it.
	other, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	running, err := other.Open(ctx, "steps")
	assert.NoError(t, err)
	assert.Equal(t, running.Revision(), rev)
	snap, err := running.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.ID, "turn_2")
	assert.Equal(t, snap.OpenTurn.Status, dive.ResponseStatusRunning)
	assert.Len(t, snap.OpenTurn.Messages, 3)
	assert.Equal(t, snap.OpenTurn.Messages[2].Content[1].(*llm.ToolResultContent).Content, "written")
	assert.Equal(t, snap.OpenTurn.ToolCalls[1].State, dive.ToolCallStateCompleted)
	assert.Len(t, snap.History, 2)
	assert.Nil(t, snap.LatestOutcome)

	// A reader that skips step lines sees only the finished turn.
	ids, _ := legacyEvents(t, path)
	assert.Len(t, ids, 1)

	final := completedTurn("turn_2", "look twice", "done")
	final.Messages = append(steps[3], llm.NewAssistantTextMessage("done"))
	rev, err = sess.CheckpointTurn(ctx, rev, final)
	assert.NoError(t, err)
	types, _ := fileLines(t, path)
	assert.Equal(t, types[len(types)-1], "event")
	assert.Len(t, types, 7)

	ids, counts := legacyEvents(t, path)
	assert.Len(t, ids, 2)
	assert.Equal(t, counts[1], 4)

	third, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	reopened, err := third.Open(ctx, "steps")
	assert.NoError(t, err)
	assert.Equal(t, reopened.Revision(), rev)
	assert.Equal(t, reopened.EventCount(), 2)
	turns, err := reopened.Turns(ctx)
	assert.NoError(t, err)
	assert.Equal(t, turns[1].Status, dive.ResponseStatusCompleted)
	assert.Len(t, turns[1].Messages, 4)

	// A rewrite leaves one line per turn.
	assert.NoError(t, reopened.RemoveLastTurn(ctx))
	types, _ = fileLines(t, path)
	assert.Equal(t, types, []string{"header", "event"})
}

// A running turn is written as a step line when the session is rewritten,
// so a reader that skips step lines never sees it.
func TestRunningTurnRewrite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := store.Open(ctx, "rewrite")
	assert.NoError(t, err)
	path := filepath.Join(dir, "rewrite.jsonl")

	// A step of a suspended turn clears the suspension, which rewrites the
	// file.
	rev, err := sess.CheckpointTurn(ctx, 0, suspendedTurn("turn_1"))
	assert.NoError(t, err)
	messages := suspendedTurn("turn_1").Messages
	rev, err = sess.CheckpointTurn(ctx, rev, runningTurn("turn_1", messages))
	assert.NoError(t, err)
	assert.False(t, sess.IsSuspended())
	types, _ := fileLines(t, path)
	assert.Equal(t, types, []string{"header", "step"})
	ids, _ := legacyEvents(t, path)
	assert.Len(t, ids, 0)

	reopened, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess2, err := reopened.Open(ctx, "rewrite")
	assert.NoError(t, err)
	assert.False(t, sess2.IsSuspended())
	snap, err := sess2.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.Status, dive.ResponseStatusRunning)
	assert.Equal(t, snap.Revision, rev)

	// Continuing an incomplete turn is written the same way.
	_, err = sess2.CheckpointTurn(ctx, rev, incompleteTurn("turn_1", "go", "partial"))
	assert.NoError(t, err)
	rev = sess2.Revision()
	_, err = sess2.CheckpointTurn(ctx, rev, runningTurn("turn_1", incompleteTurn("turn_1", "go", "partial").Messages[:2]))
	assert.NoError(t, err)
	ids, _ = legacyEvents(t, path)
	assert.Len(t, ids, 0)
}

// A new running turn is refused on a suspended session, like any new turn.
func TestRunningTurnRefusedWhileSuspended(t *testing.T) {
	ctx := context.Background()
	sess := session.New("refused")
	rev, err := sess.CheckpointTurn(ctx, 0, suspendedTurn("turn_1"))
	assert.NoError(t, err)
	_, err = sess.CheckpointTurn(ctx, rev, runningTurn("turn_2", []*llm.Message{llm.NewUserTextMessage("new")}))
	assert.True(t, errors.Is(err, session.ErrSuspendedSession))
}

// A fork leaves a running turn out, or copies it closed with its running
// calls unknown.
func TestForkRunningTurn(t *testing.T) {
	ctx := context.Background()
	sess := session.New("fork-running")
	rev, err := sess.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	_, err = sess.CheckpointTurn(ctx, rev, runningTurn("turn_2", stepMessages()[2],
		dive.ToolCallRecord{ID: "toolu_1", Name: "look", State: dive.ToolCallStateCompleted},
		dive.ToolCallRecord{ID: "toolu_2", Name: "write", State: dive.ToolCallStateRunning}))
	assert.NoError(t, err)

	assert.Equal(t, sess.Fork("plain").EventCount(), 1)

	forked := sess.Fork("open", session.ForkWithOpenTurn())
	snap, err := forked.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.Status, dive.ResponseStatusIncomplete)
	outcome := snap.OpenTurn.Outcome
	assert.Equal(t, outcome.Reason, dive.TurnReasonCanceled)
	assert.Equal(t, outcome.Next, dive.TurnNextReconcile)
	assert.Equal(t, outcome.ToolCalls[0].State, dive.ToolCallStateCompleted)
	assert.Equal(t, outcome.ToolCalls[1].State, dive.ToolCallStateUnknown)
}

// A claim excludes other owners until it is released or expires, and a
// session claimed by another process reads back what that process wrote.
func TestFileStoreClaims(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storeA, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	storeB, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	a, err := storeA.Open(ctx, "claimed")
	assert.NoError(t, err)
	b, err := storeB.Open(ctx, "claimed")
	assert.NoError(t, err)

	assert.NoError(t, a.ClaimSession(ctx, "A", time.Minute))
	err = b.ClaimSession(ctx, "B", time.Minute)
	assert.True(t, errors.Is(err, dive.ErrSessionClaimed))
	assert.NoError(t, a.ClaimSession(ctx, "A", time.Minute), "renewal")

	_, err = a.CheckpointTurn(ctx, 0, completedTurn("turn_1", "hi", "hello"))
	assert.NoError(t, err)
	assert.NoError(t, a.ReleaseSession(ctx, "A"))

	// B opened the session before A wrote: its claim reads the write back.
	assert.Equal(t, b.EventCount(), 0)
	assert.NoError(t, b.ClaimSession(ctx, "B", 20*time.Millisecond))
	assert.Equal(t, b.EventCount(), 1)
	assert.Equal(t, b.Revision(), uint64(1))
	assert.NoError(t, b.ReleaseSession(ctx, "A"), "not A's to release")
	err = a.ClaimSession(ctx, "A", time.Minute)
	assert.True(t, errors.Is(err, dive.ErrSessionClaimed))

	// B's claim expires and A takes the session: B's writes are refused.
	time.Sleep(30 * time.Millisecond)
	assert.NoError(t, a.ClaimSession(ctx, "A", time.Minute))
	_, err = b.CheckpointTurn(ctx, 1, completedTurn("turn_2", "late", "write"))
	assert.True(t, errors.Is(err, dive.ErrSessionClaimed))
	assert.True(t, errors.Is(err, dive.ErrSaveRejected))
	assert.Equal(t, a.EventCount(), 1)

	// Deleting the session removes its claim.
	assert.NoError(t, storeA.Delete(ctx, "claimed"))
	_, err = os.Stat(filepath.Join(dir, "claimed.claim"))
	assert.True(t, os.IsNotExist(err))
}

// A session in memory keeps its claim itself.
func TestMemoryClaims(t *testing.T) {
	ctx := context.Background()
	sess := session.New("memory-claims")
	assert.NoError(t, sess.ClaimSession(ctx, "A", time.Minute))
	assert.True(t, errors.Is(sess.ClaimSession(ctx, "B", time.Minute), dive.ErrSessionClaimed))
	assert.NoError(t, sess.ReleaseSession(ctx, "A"))
	assert.NoError(t, sess.ClaimSession(ctx, "B", time.Millisecond))
	time.Sleep(5 * time.Millisecond)
	assert.NoError(t, sess.ClaimSession(ctx, "A", time.Minute), "expired")
	assert.Error(t, sess.ClaimSession(ctx, "", time.Minute))
	assert.Error(t, sess.ClaimSession(ctx, "A", 0))
}
