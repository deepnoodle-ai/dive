package compaction

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMidTurnCompactionHook(t *testing.T) {
	ctx := context.Background()
	// Each message is ~big enough that a few cross the threshold.
	big := func(s string) *llm.Message { return llm.NewUserTextMessage(s + strings.Repeat("x", 2000)) }

	t.Run("compacts when the working set crosses the threshold", func(t *testing.T) {
		stub := &stubLLM{}
		var notified *CompactionEvent
		hook := MidTurnCompactionHook(stub, 500, WithMidTurnNotify(func(e *CompactionEvent) { notified = e }))

		hctx := dive.NewHookContext()
		hctx.Messages = []*llm.Message{big("a"), big("b"), big("c"), big("d")}

		assert.NoError(t, hook(ctx, hctx))

		// Working set collapsed to the single handoff-framed summary.
		assert.Len(t, hctx.Messages, 1)
		assert.Equal(t, llm.User, hctx.Messages[0].Role)
		assert.Contains(t, hctx.Messages[0].Text(), "STUB SUMMARY")
		assert.True(t, estimateTokens(hctx.Messages[0]) < 500)
		// The whole working set reached the summarizer.
		assert.Equal(t, 4, stub.sawMessages)
		// Event surfaced both via the callback and the hook-context values.
		assert.NotNil(t, notified)
		evt, ok := hctx.Values[dive.StateKeyCompactionEvent].(*CompactionEvent)
		assert.True(t, ok)
		assert.Equal(t, notified, evt)
	})

	t.Run("no-op below the threshold", func(t *testing.T) {
		stub := &stubLLM{}
		called := false
		hook := MidTurnCompactionHook(stub, 1_000_000, WithMidTurnNotify(func(*CompactionEvent) { called = true }))

		hctx := dive.NewHookContext()
		original := []*llm.Message{llm.NewUserTextMessage("hi"), llm.NewAssistantTextMessage("hello")}
		hctx.Messages = original

		assert.NoError(t, hook(ctx, hctx))
		assert.Equal(t, 2, len(hctx.Messages)) // untouched
		assert.Equal(t, 0, stub.sawMessages)   // summarizer never called
		assert.False(t, called)
	})

	t.Run("no-op with fewer than two messages", func(t *testing.T) {
		stub := &stubLLM{}
		hook := MidTurnCompactionHook(stub, 1) // threshold trivially exceeded

		hctx := dive.NewHookContext()
		hctx.Messages = []*llm.Message{big("only")}

		assert.NoError(t, hook(ctx, hctx))
		assert.Len(t, hctx.Messages, 1)
		assert.Equal(t, 0, stub.sawMessages)
	})
}
