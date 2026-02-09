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

## Configuration

| Setting                 | Default  | Description                          |
| ----------------------- | -------- | ------------------------------------ |
| `ContextTokenThreshold` | `100000` | Token count that triggers compaction |
| `SummaryPrompt`         | Built-in | Custom prompt for summary generation |

## Best Practices

1. Start with the default 100k threshold
2. Track usage from `ResponseItemTypeMessage` events for accurate context measurement
3. Use faster models (Haiku, Flash) for summary generation
4. Test that important context survives compaction
5. Handle compaction errors gracefully (don't crash on failure)
