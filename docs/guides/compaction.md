# Context Compaction Guide

Dive's compaction feature automatically manages conversation context as it grows, helping you stay within context window limits. When token usage exceeds a configured threshold, the conversation is summarized and the history is replaced with a concise summary.

## Overview

Compaction is a client-side feature that:

1. Monitors token usage after each LLM response
2. Triggers when total tokens exceed a threshold
3. Generates a structured summary of the conversation
4. Replaces the message history with the summary
5. Persists the compacted state to the thread repository

This enables long-running agent sessions that would otherwise exceed context limits.

## Basic Usage

Enable compaction at the agent level:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:  "Assistant",
    Model: anthropic.New(),
    Compaction: &dive.CompactionConfig{
        Enabled: true,
    },
})
```

Or enable per-request:

```go
resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Process all files in the directory"),
    dive.WithCompaction(&dive.CompactionConfig{
        Enabled:               true,
        ContextTokenThreshold: 50000,
    }),
)
```

## Configuration Options

| Option                  | Type      | Default         | Description                           |
| ----------------------- | --------- | --------------- | ------------------------------------- |
| `Enabled`               | `bool`    | `false`         | Must be `true` to activate compaction |
| `ContextTokenThreshold` | `int`     | `100000`        | Token count that triggers compaction  |
| `Model`                 | `llm.LLM` | Agent's model   | Optional model for summary generation |
| `SummaryPrompt`         | `string`  | Built-in prompt | Custom prompt for summary generation  |

### Token Calculation

Total tokens are calculated as:

```
InputTokens + OutputTokens + CacheCreationInputTokens + CacheReadInputTokens
```

This matches Anthropic's SDK calculation for consistency.

## Choosing a Threshold

The threshold determines when compaction occurs:

```go
// More frequent compaction for memory-constrained scenarios
Compaction: &dive.CompactionConfig{
    Enabled:               true,
    ContextTokenThreshold: 50000,
}

// Less frequent compaction when you need more context
Compaction: &dive.CompactionConfig{
    Enabled:               true,
    ContextTokenThreshold: 150000,
}
```

Lower thresholds mean more frequent compactions with smaller context windows. Higher thresholds preserve more context but risk hitting limits.

## Using a Different Model for Summaries

You can use a faster or cheaper model for generating summaries:

```go
import "github.com/deepnoodle-ai/dive/providers/anthropic"

haiku := anthropic.New(anthropic.WithModel(anthropic.Claude35Haiku))

Compaction: &dive.CompactionConfig{
    Enabled:               true,
    ContextTokenThreshold: 100000,
    Model:                 haiku,
}
```

## Custom Summary Prompts

Provide a custom prompt for domain-specific needs:

```go
Compaction: &dive.CompactionConfig{
    Enabled: true,
    SummaryPrompt: `Summarize the research conducted so far, including:
- Sources consulted and key findings
- Questions answered and remaining unknowns
- Recommended next steps

Wrap your summary in <summary></summary> tags.`,
}
```

Your prompt must instruct the model to wrap the summary in `<summary></summary>` tags.

### Default Summary Prompt

The built-in prompt instructs the model to create a structured continuation summary including:

1. **Task Overview**: The user's core request, success criteria, and constraints
2. **Current State**: What has been completed, files modified, and artifacts produced
3. **Important Discoveries**: Technical constraints, decisions made, errors resolved
4. **Next Steps**: Specific actions needed, blockers, and priority order
5. **Context to Preserve**: User preferences, domain-specific details, and commitments

## Monitoring Compaction Events

Use an event callback to monitor when compaction occurs:

```go
resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("..."),
    dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        if item.Type == dive.ResponseItemTypeCompaction {
            event := item.Compaction
            fmt.Printf("Compaction occurred:\n")
            fmt.Printf("  Tokens before: %d\n", event.TokensBefore)
            fmt.Printf("  Tokens after: %d\n", event.TokensAfter)
            fmt.Printf("  Messages compacted: %d\n", event.MessagesCompacted)
        }
        return nil
    }),
)
```

## Thread Persistence

When using a `ThreadRepository`, compaction affects the thread:

1. `Thread.Messages` is replaced with the summary (not appended)
2. `Thread.CompactionHistory` records when compaction occurred

```go
repo := dive.NewMemoryThreadRepository()
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:             "Assistant",
    Model:            anthropic.New(),
    ThreadRepository: repo,
    Compaction: &dive.CompactionConfig{
        Enabled: true,
    },
})

// After compaction, retrieve the thread
thread, _ := repo.GetThread(ctx, threadID)
fmt.Printf("Messages: %d\n", len(thread.Messages))  // Will be 1 (the summary)
fmt.Printf("Compaction events: %d\n", len(thread.CompactionHistory))
```

## Content Types

### SummaryContent

When compaction occurs, the summary is stored as a `SummaryContent` block:

```go
// Check if a message contains a summary
for _, content := range message.Content {
    if summary, ok := content.(*llm.SummaryContent); ok {
        fmt.Printf("This message is a compacted summary:\n%s\n", summary.Summary)
    }
}
```

## Edge Cases

### Pending Tool Use

If compaction triggers while a tool call is pending (tool_use without tool_result), the pending tool_use blocks are removed before summarization. Claude may re-issue the tool call after resuming from the summary if still needed.

### Minimum Messages

Compaction is skipped if there are fewer than 2 messages in the conversation.

### Compaction Failure

If summary generation fails (e.g., model error, missing `<summary>` tags), compaction is skipped with a warning logged. The conversation continues with uncompacted context.

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

func main() {
    // Create a repository for thread persistence
    repo := dive.NewMemoryThreadRepository()

    // Create agent with compaction enabled
    agent, err := dive.NewAgent(dive.AgentOptions{
        Name:         "Research Assistant",
        Instructions: "You are a thorough researcher.",
        Model:        anthropic.New(),
        ThreadRepository: repo,
        Compaction: &dive.CompactionConfig{
            Enabled:               true,
            ContextTokenThreshold: 50000,  // Compact at 50k tokens
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Track compaction events
    callback := func(ctx context.Context, item *dive.ResponseItem) error {
        switch item.Type {
        case dive.ResponseItemTypeCompaction:
            fmt.Printf("Context compacted: %d -> %d tokens\n",
                item.Compaction.TokensBefore,
                item.Compaction.TokensAfter)
        case dive.ResponseItemTypeMessage:
            fmt.Printf("Assistant: %s\n", item.Message.Text())
        }
        return nil
    }

    // Long conversation that may trigger compaction
    threadID := "research-session"
    queries := []string{
        "Research the history of programming languages",
        "Tell me about functional programming paradigms",
        "Compare Go and Rust for systems programming",
        // ... more queries that build up context
    }

    for _, query := range queries {
        _, err := agent.CreateResponse(context.Background(),
            dive.WithThreadID(threadID),
            dive.WithInput(query),
            dive.WithEventCallback(callback),
        )
        if err != nil {
            log.Printf("Error: %v", err)
            continue
        }
    }

    // Check compaction history
    thread, _ := repo.GetThread(context.Background(), threadID)
    if len(thread.CompactionHistory) > 0 {
        fmt.Printf("\nCompaction occurred %d time(s)\n", len(thread.CompactionHistory))
        for i, record := range thread.CompactionHistory {
            fmt.Printf("  %d: %d messages compacted at %s\n",
                i+1, record.MessagesCompacted, record.Timestamp)
        }
    }
}
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
2. **Monitor compaction events** - Use callbacks to track when compaction occurs
3. **Use faster models for summaries** - Haiku can generate quality summaries quickly
4. **Test your workflows** - Verify important context survives compaction
5. **Combine with thread persistence** - Use ThreadRepository to maintain compaction history
6. **Set appropriate thresholds** - Lower for memory-constrained scenarios, higher when you need more context
