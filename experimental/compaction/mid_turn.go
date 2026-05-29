package compaction

import (
	"context"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// MidTurnOption configures MidTurnCompactionHook.
type MidTurnOption func(*midTurnConfig)

type midTurnConfig struct {
	systemPrompt  string
	summaryPrompt string
	notify        func(*CompactionEvent)
}

// WithMidTurnSystemPrompt sets the system prompt included in the summary
// request (often the agent's own system prompt, for context).
func WithMidTurnSystemPrompt(s string) MidTurnOption {
	return func(c *midTurnConfig) { c.systemPrompt = s }
}

// WithMidTurnSummaryPrompt overrides the summary instruction. Like
// CompactMessages, the prompt must still instruct the model to wrap its output
// in <summary></summary> tags.
func WithMidTurnSummaryPrompt(s string) MidTurnOption {
	return func(c *midTurnConfig) { c.summaryPrompt = s }
}

// WithMidTurnNotify registers a callback invoked after a successful mid-turn
// compaction — e.g. to surface it in a UI. It runs on the agent's goroutine
// inside the tool loop, so it must not block.
func WithMidTurnNotify(fn func(*CompactionEvent)) MidTurnOption {
	return func(c *midTurnConfig) { c.notify = fn }
}

// MidTurnCompactionHook returns a PreIterationHook that compacts the in-memory
// working set within a single agent turn, once its estimated size reaches
// tokenThreshold. It exists to stop a long tool-call loop — many calls, or a
// few large file reads / command dumps — from growing past the model's context
// window before the turn can finish (which otherwise surfaces as a hard
// context-length error with no recovery).
//
// It is deliberately model-facing only. The hook rewrites hctx.Messages — what
// the next iteration sends to the model — but never touches the messages the
// agent persists. The agent saves the turn from its own output accumulator, not
// this working set, so the full, uncompacted turn is still written to the
// session and remains recoverable via AllMessages once the session's
// between-turn checkpoint runs. That keeps compaction non-destructive even
// mid-turn: the model sees a summary to stay under budget, the record keeps
// everything.
//
// Requires an agent that honors PreIteration message rewrites (Dive reads
// hctx.Messages back after PreIteration hooks). PreIteration is also a
// pairing-safe point to compact: by the top of an iteration every prior
// tool_use has its matching tool_result, so nothing is left dangling.
//
// Compaction failures are swallowed rather than aborting the turn — the
// oversized request still goes to the model, which may succeed or surface its
// own overflow error. A summary that does not actually shrink the working set
// is rejected, which also prevents repeated re-compaction.
func MidTurnCompactionHook(model llm.LLM, tokenThreshold int, opts ...MidTurnOption) dive.PreIterationHook {
	var cfg midTurnConfig
	for _, o := range opts {
		o(&cfg)
	}
	if tokenThreshold <= 0 {
		tokenThreshold = DefaultContextTokenThreshold
	}

	return func(ctx context.Context, hctx *dive.HookContext) error {
		msgs := hctx.Messages
		if len(msgs) < 2 {
			return nil // nothing meaningful to summarize
		}
		before := 0
		for _, m := range msgs {
			before += estimateTokens(m)
		}
		if before < tokenThreshold {
			return nil
		}

		compacted, event, err := CompactMessages(ctx, model, msgs, cfg.systemPrompt, cfg.summaryPrompt, before)
		if err != nil {
			return nil // best effort: don't fail the turn on a summarizer error
		}

		after := 0
		for _, m := range compacted {
			after += estimateTokens(m)
		}
		if after >= before {
			return nil // summary didn't help — leave the context as-is
		}

		hctx.Messages = compacted
		if hctx.Values != nil {
			hctx.Values[dive.StateKeyCompactionEvent] = event
		}
		if cfg.notify != nil {
			cfg.notify(event)
		}
		return nil
	}
}
