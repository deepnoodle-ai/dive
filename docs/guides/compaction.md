# Context Compaction Guide

Dive's compaction feature automatically manages conversation context as it grows, helping you stay within context window limits. When token usage exceeds a configured threshold, the conversation is summarized and the history is replaced with a concise summary.

## Overview

Compaction is managed externally to the agent (typically by the CLI or your application code). This design keeps the agent simple and gives you full control over when and how compaction occurs.

The compaction flow:

1. After each agent response, check token usage from the last LLM call
2. If context tokens exceed the threshold, trigger compaction
3. Generate a structured summary of the conversation
4. Replace the session's message history with the summary
5. Persist the compacted state to the session repository

This enables long-running agent sessions that would otherwise exceed context limits.

## Library Usage

For library users, compaction is performed externally after each `CreateResponse` call. Dive provides utility functions to make this straightforward:

```go
package main

import (
    "context"
    "fmt"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/llm"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

func main() {
    ctx := context.Background()
    model := anthropic.New()
    repo := dive.NewMemorySessionRepository()
    sessionID := "my-session"
    threshold := 100000 // Compact at 100k context tokens

    agent, _ := dive.NewAgent(dive.AgentOptions{
        Name:             "Assistant",
        Model:            model,
        SessionRepository: repo,
    })

    // Track the last usage from the callback (for accurate context size)
    var lastUsage *llm.Usage

    resp, _ := agent.CreateResponse(ctx,
        dive.WithSessionID(sessionID),
        dive.WithInput("Process all files"),
        dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
            if item.Type == dive.ResponseItemTypeMessage && item.Usage != nil {
                lastUsage = item.Usage
            }
            return nil
        }),
    )

    // Check if compaction is needed after the response
    if lastUsage != nil {
        session, _ := repo.GetSession(ctx, sessionID)
        if dive.ShouldCompact(lastUsage, len(session.Messages), threshold) {
            compactedMsgs, event, err := dive.CompactMessages(
                ctx,
                model,
                session.Messages,
                "",  // system prompt (optional)
                "",  // use default summary prompt
                dive.CalculateContextTokens(lastUsage),
            )
            if err == nil {
                session.Messages = compactedMsgs
                session.CompactionHistory = append(session.CompactionHistory, dive.CompactionRecord{
                    Timestamp:         time.Now(),
                    TokensBefore:      event.TokensBefore,
                    TokensAfter:       event.TokensAfter,
                    MessagesCompacted: event.MessagesCompacted,
                })
                repo.PutSession(ctx, session)
                fmt.Printf("Compacted: %d -> %d tokens\n", event.TokensBefore, event.TokensAfter)
            }
        }
    }
}
```

## Compaction Functions

### ShouldCompact

Check if compaction should be triggered:

```go
func ShouldCompact(usage *llm.Usage, messageCount int, threshold int) bool
```

- `usage`: Token usage from the last LLM call
- `messageCount`: Number of messages in the session
- `threshold`: Token threshold (0 uses default of 100,000)

Returns `true` if context tokens >= threshold and messageCount >= 2.

### CalculateContextTokens

Calculate the context window size from usage:

```go
func CalculateContextTokens(usage *llm.Usage) int
```

Returns `InputTokens + CacheReadInputTokens`. This represents the actual context size sent to the LLM.

Note: `CacheCreationInputTokens` is a subset of `InputTokens` (not additive), and `OutputTokens` are not part of the input context.

### CompactMessages

Generate a summary and return compacted messages:

```go
func CompactMessages(
    ctx context.Context,
    model llm.LLM,
    messages []*llm.Message,
    systemPrompt string,
    summaryPrompt string,
    tokensBefore int,
) ([]*llm.Message, *CompactionEvent, error)
```

- `model`: LLM to use for generating the summary
- `messages`: The conversation messages to compact
- `systemPrompt`: System prompt to include (can be empty)
- `summaryPrompt`: Custom prompt for summary generation (empty uses default)
- `tokensBefore`: Pre-compaction token count for accurate event reporting

Returns the compacted messages (a single user message containing the summary), a `CompactionEvent` with stats, and any error.

## Configuration

### CompactionConfig

The `CompactionConfig` struct configures compaction behavior:

```go
type CompactionConfig struct {
    ContextTokenThreshold int     // Token count that triggers compaction (default: 100000)
    Model                 llm.LLM // Optional model for summary generation
    SummaryPrompt         string  // Custom prompt for summary generation
}
```

| Field                   | Type      | Default         | Description                           |
| ----------------------- | --------- | --------------- | ------------------------------------- |
| `ContextTokenThreshold` | `int`     | `100000`        | Context token count that triggers compaction |
| `Model`                 | `llm.LLM` | (required)      | Model for summary generation          |
| `SummaryPrompt`         | `string`  | Built-in prompt | Custom prompt for summary generation  |

## Choosing a Threshold

The threshold determines when compaction occurs based on context tokens (InputTokens + CacheReadInputTokens):

- **Lower thresholds** (e.g., 50,000): More frequent compaction, smaller context windows
- **Higher thresholds** (e.g., 150,000): Less frequent compaction, preserves more context

The default of 100,000 works well for most use cases with Claude models.

## Using a Different Model for Summaries

You can use a faster or cheaper model for generating summaries:

```go
import "github.com/deepnoodle-ai/dive/providers/anthropic"

haiku := anthropic.New(anthropic.WithModel(anthropic.Claude35Haiku))

// Use haiku for summary generation
compactedMsgs, event, err := dive.CompactMessages(
    ctx,
    haiku,  // Faster model for summaries
    session.Messages,
    "",
    "",
    tokensBefore,
)
```

## Custom Summary Prompts

Provide a custom prompt for domain-specific needs:

```go
customPrompt := `Summarize the research conducted so far, including:
- Sources consulted and key findings
- Questions answered and remaining unknowns
- Recommended next steps

Wrap your summary in <summary></summary> tags.`

compactedMsgs, event, err := dive.CompactMessages(
    ctx,
    model,
    session.Messages,
    "",
    customPrompt,
    tokensBefore,
)
```

Your prompt must instruct the model to wrap the summary in `<summary></summary>` tags.

### Default Summary Prompt

The built-in prompt instructs the model to create a structured continuation summary including:

1. **Task Overview**: The user's core request, success criteria, and constraints
2. **Current State**: What has been completed, files modified, and artifacts produced
3. **Important Discoveries**: Technical constraints, decisions made, errors resolved
4. **Next Steps**: Specific actions needed, blockers, and priority order
5. **Context to Preserve**: User preferences, domain-specific details, and commitments

## Session Persistence

When using a `SessionRepository`, compaction affects the session:

1. `Session.Messages` is replaced with the compacted summary (a single user message)
2. `Session.CompactionHistory` records when compaction occurred

```go
// After compaction, retrieve the session
session, _ := repo.GetSession(ctx, sessionID)
fmt.Printf("Messages: %d\n", len(session.Messages))  // Will be 1 (the summary)
fmt.Printf("Compaction events: %d\n", len(session.CompactionHistory))
```

### Compacted Summary Format

The summary is stored as a user message to ensure compatibility with all LLM providers (which typically require the first message to be from the user role):

```go
session, _ := repo.GetSession(ctx, sessionID)
if len(session.Messages) > 0 && session.Messages[0].Role == llm.User {
    for _, content := range session.Messages[0].Content {
        if text, ok := content.(*llm.TextContent); ok {
            fmt.Printf("Compacted summary:\n%s\n", text.Text)
        }
    }
}
```

## Edge Cases

### Pending Tool Calls

When compacting, any pending `tool_use` blocks in the last assistant message (without corresponding `tool_result`) are automatically filtered out. This prevents malformed message sequences.

### Minimum Messages

Compaction is skipped if there are fewer than 2 messages in the conversation.

### Compaction Failure

If summary generation fails (e.g., model error, missing `<summary>` tags), `CompactMessages` returns an error. Your application can decide how to handle this (skip compaction, retry, etc.).

## CLI Usage

The Dive CLI manages compaction automatically and provides flags to configure it:

```bash
# Default behavior (compaction enabled at 100k tokens)
dive

# Disable compaction
dive --compaction=false

# Adjust compaction threshold
dive --compaction-threshold=50000  # Compact at 50k tokens
dive --compaction-threshold=150000 # Compact at 150k tokens

# Environment variables
export DIVE_COMPACTION=true
export DIVE_COMPACTION_THRESHOLD=75000
dive
```

### Visual Feedback

When compaction occurs in the CLI, you'll see:

1. **Live notification** (3 seconds): A yellow indicator appears in the live view with token reduction stats
2. **Footer stats** (5 seconds): Detailed compaction statistics appear in the footer:
   ```
   Context compacted: 102,450 -> 1,250 tokens (47 messages summarized)
   ```

## When to Use Compaction

**Good use cases:**

- Long-running agent tasks that process many files
- Research workflows that accumulate large amounts of information
- Multi-step tasks with clear, measurable progress
- Tasks that produce artifacts (files, reports) that persist outside the conversation

**Less ideal use cases:**

- Tasks requiring precise recall of early conversation details
- Workflows that need to maintain exact state across many variables
- Short conversations that won't exceed context limits

## Best Practices

1. **Start with defaults** - The default 100k threshold works well for most use cases
2. **Track last usage** - Use the callback to track usage from `ResponseItemTypeMessage` events for accurate context measurement
3. **Use faster models for summaries** - Haiku can generate quality summaries quickly
4. **Test your workflows** - Verify important context survives compaction
5. **Combine with session persistence** - Use SessionRepository to maintain compaction history
6. **Handle errors gracefully** - Compaction failures shouldn't crash your application
