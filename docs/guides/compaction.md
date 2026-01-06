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

### Compacted Summary Format

When compaction occurs, the summary is stored as a regular `TextContent` block in an assistant message. This ensures compatibility with all LLM providers:

```go
// After compaction, the thread will have an assistant message with the summary
thread, _ := repo.GetThread(ctx, threadID)
if len(thread.Messages) > 0 && thread.Messages[0].Role == llm.Assistant {
    // The first message after compaction contains the summary
    for _, content := range thread.Messages[0].Content {
        if text, ok := content.(*llm.TextContent); ok {
            fmt.Printf("Compacted summary:\n%s\n", text.Text)
        }
    }
}
```

## Edge Cases

### Pending Tool Calls

**Compaction is automatically deferred when there are pending tool calls.** If the current response contains `tool_use` blocks that need execution, compaction will not occur until after:
1. The tool calls are executed
2. Tool results are added to the conversation
3. The next LLM response is generated

This ensures that `tool_use` and `tool_result` blocks remain properly paired, preventing API errors about missing tool_use references.

### Partial Tool Use (Before Deferral)

When compacting historical messages (before the current turn), if the message history contains unpaired `tool_use` blocks (without corresponding `tool_result`), these are automatically filtered out before summarization. This cleanup prevents malformed message sequences.

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

## CLI Usage

The Dive CLI enables compaction by default and provides flags to configure it:

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

1. **Live notification** (3 seconds): A yellow ⚡ indicator appears in the live view with token reduction stats
2. **Footer stats** (5 seconds): Detailed compaction statistics appear in the footer:
   ```
   ⚡ Context compacted: 102,450 → 1,250 tokens (47 messages summarized)
   ```

This visual feedback helps you understand when compaction is occurring and how much context is being preserved.

### CLI Print Mode

Print mode (non-interactive) also supports compaction:

```bash
# Enable compaction for long-running tasks
dive --print --compaction-threshold=50000 "Process all files in the repository"

# Output compaction stats in JSON format
dive --print --output-format=json "Long task..." | jq .
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
