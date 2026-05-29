# Context Compaction Guide

> **Experimental**: This package is in `experimental/compaction/`. The API may change.

Compaction manages conversation context as it grows, summarizing older messages to stay within context window limits.

## Core: CompactionHook

The simplest approach uses `dive.CompactionHook` as a `PreGenerationHook`:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        model,
    Hooks: dive.Hooks{
        PreGeneration: []dive.PreGenerationHook{
            dive.CompactionHook(50, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
                // Custom summarization logic
                return summarize(ctx, msgs)
            }),
        },
    },
})
```

This triggers compaction when the message count exceeds the threshold.

## Experimental: CompactMessages

The `experimental/compaction` package provides `CompactMessages` for full compaction with token estimation:

```go
import "github.com/deepnoodle-ai/dive/experimental/compaction"

messages, _ := sess.Messages(ctx)

compactedMsgs, event, err := compaction.CompactMessages(
    ctx,
    model,
    messages,
    "",  // system prompt (optional)
    "",  // summary prompt (empty = default)
    compaction.CalculateContextTokens(lastUsage),
)
```

### ShouldCompact

Check if compaction should be triggered:

```go
if compaction.ShouldCompact(lastUsage, len(messages), 100000) {
    // Trigger compaction
}
```

Returns `true` if context tokens >= threshold and messageCount >= 2.

### Using a Different Model for Summaries

Use a faster model for generating summaries:

```go
haiku := anthropic.New(anthropic.WithModel("claude-haiku-4-5"))

compactedMsgs, event, err := compaction.CompactMessages(
    ctx,
    haiku,
    messages,
    "", "", tokensBefore,
)
```

### Custom Summary Prompts

Provide a domain-specific prompt. Your prompt must instruct the model to wrap the summary in `<summary></summary>` tags:

```go
customPrompt := `Summarize the research conducted so far, including:
- Sources consulted and key findings
- Questions answered and remaining unknowns
Wrap your summary in <summary></summary> tags.`

compactedMsgs, event, err := compaction.CompactMessages(
    ctx, model, msgs, "", customPrompt, tokensBefore,
)
```

## When compaction runs

There are three points in the agent lifecycle where compaction can happen:

1. **Turn start** — a `PreGeneration` hook (`CompactionHook` / `HookWithModel`)
   fires once before the tool loop and may rewrite the history. Good for
   compacting prior history before a turn begins.
2. **Between turns** — your app calls `session.Compact` (or `CompactMessages`)
   after `CreateResponse` returns, using the final turn's usage. This is what
   the Dive CLI does, and it produces the durable, non-destructive checkpoint
   (`AllMessages` / `CompactionHistory` retain the originals).
3. **Mid-turn** — a `PreIteration` hook (`MidTurnCompactionHook`) fires before
   each LLM call *inside* the tool loop. Needed because a single long turn
   (many tool calls, or a few large file reads / command dumps) can grow past
   the model's context window before the turn finishes — which otherwise
   surfaces as a hard context-length error with no recovery.

### Mid-turn compaction

`MidTurnCompactionHook` summarizes the working set when its estimated size
crosses the threshold, keeping a long tool loop under budget:

```go
import "github.com/deepnoodle-ai/dive/experimental/compaction"

agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: systemPrompt,
    Model:        mainModel,
    Hooks: dive.Hooks{
        PreIteration: []dive.PreIterationHook{
            compaction.MidTurnCompactionHook(summaryModel, 100000,
                compaction.WithMidTurnSystemPrompt(systemPrompt),
                compaction.WithMidTurnNotify(func(e *compaction.CompactionEvent) {
                    // surface the event in a UI, log it, etc.
                }),
            ),
        },
    },
})
```

It is **model-facing only**: the hook rewrites what the next iteration sends to
the model but never the messages the agent persists, so the full turn is still
saved and stays recoverable via `AllMessages` once the between-turn checkpoint
runs. Compaction failures are swallowed (the turn proceeds), and a summary that
doesn't actually shrink the set is rejected (which also prevents repeated
re-compaction). Requires an agent that honors `PreIteration` message rewrites —
Dive reads `hctx.Messages` back after `PreIteration` hooks.

Pair it with between-turn compaction: mid-turn keeps the *current* turn under
budget (ephemeral, in-memory), while the between-turn checkpoint shrinks the
*next* turn's active window durably.

## Configuration

| Setting                 | Default  | Description                          |
| ----------------------- | -------- | ------------------------------------ |
| `ContextTokenThreshold` | `100000` | Token count that triggers compaction |
| `SummaryPrompt`         | Built-in | Custom prompt for summary generation |

In the Dive CLI, compaction is **on by default** (`--compaction`, disable with
`--compaction=false`) at a 100k-token threshold (`--compaction-threshold`); it
runs both between turns and mid-turn, and `/compact` forces it on demand.

## Best Practices

1. Start with the default 100k threshold
2. Track usage from `ResponseItemTypeMessage` events for accurate context measurement
3. Use faster models (Haiku, Flash) for summary generation
4. Test that important context survives compaction
5. Handle compaction errors gracefully (don't crash on failure)
