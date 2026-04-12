package compaction

// Context compaction utilities for managing long conversations.
//
// Compaction is managed externally to the agent (typically by the CLI or your
// application code). This design keeps the agent simple and gives you full
// control over when and how compaction occurs.
//
// Basic usage:
//
//	// After each CreateResponse call, check if compaction is needed
//	if dive.ShouldCompact(lastUsage, len(session.Messages), threshold) {
//	    compactedMsgs, event, err := dive.CompactMessages(ctx, model, session.Messages, "", "", tokensBefore)
//	    if err == nil {
//	        session.Messages = compactedMsgs
//	        sessionRepo.PutSession(ctx, session)
//	    }
//	}
//
// See the compaction guide in docs/guides/compaction.md for detailed usage.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// DefaultContextTokenThreshold is the default token count that triggers compaction.
const DefaultContextTokenThreshold = 100000

// DefaultCompactionSummaryPrompt is the default prompt used to generate summaries.
// Based on Anthropic's SDK compaction spec.
const DefaultCompactionSummaryPrompt = `You have been working on the task described above but have not yet completed it. Write a continuation summary that will allow you (or another instance of yourself) to resume work efficiently in a future context window where the conversation history will be replaced with this summary. Your summary should be structured, concise, and actionable. Include:

1. Task Overview
The user's core request and success criteria
Any clarifications or constraints they specified

2. Current State
What has been completed so far
Files created, modified, or analyzed (with paths if relevant)
Key outputs or artifacts produced

3. Important Discoveries
Technical constraints or requirements uncovered
Decisions made and their rationale
Errors encountered and how they were resolved
What approaches were tried that didn't work (and why)

4. Next Steps
Specific actions needed to complete the task
Any blockers or open questions to resolve
Priority order if multiple steps remain

5. Context to Preserve
User preferences or style requirements
Domain-specific details that aren't obvious
Any promises made to the user

Be concise but complete—err on the side of including information that would prevent duplicate work or repeated mistakes. Write in a way that enables immediate resumption of the task.

Wrap your summary in <summary></summary> tags.`

// CompactionConfig configures context compaction behavior.
type CompactionConfig struct {
	// ContextTokenThreshold is the context window token count that triggers compaction.
	// Default: 100000 (100k tokens).
	// Context tokens are calculated as: InputTokens + CacheReadInputTokens.
	ContextTokenThreshold int `json:"context_token_threshold,omitempty"`

	// Model is an optional LLM to use for summary generation.
	// If nil, must be provided to CompactMessages.
	Model llm.LLM `json:"-"`

	// SummaryPrompt is the prompt used to generate summaries.
	// If empty, uses DefaultCompactionSummaryPrompt.
	SummaryPrompt string `json:"summary_prompt,omitempty"`
}

// CompactionEvent is emitted when context compaction occurs.
type CompactionEvent struct {
	// TokensBefore is the total token count before compaction.
	TokensBefore int `json:"tokens_before"`

	// TokensAfter is the token count after compaction.
	TokensAfter int `json:"tokens_after"`

	// Summary is the generated summary text.
	Summary string `json:"summary"`

	// MessagesCompacted is the number of messages that were replaced.
	MessagesCompacted int `json:"messages_compacted"`
}

// CompactionRecord tracks a compaction event in thread history.
type CompactionRecord struct {
	// Timestamp is when the compaction occurred.
	Timestamp time.Time `json:"timestamp"`

	// TokensBefore is the total token count before compaction.
	TokensBefore int `json:"tokens_before"`

	// TokensAfter is the token count after compaction.
	TokensAfter int `json:"tokens_after"`

	// MessagesCompacted is the number of messages that were replaced.
	MessagesCompacted int `json:"messages_compacted"`
}

// CalculateContextTokens returns the context window token count from usage.
// Per Anthropic API: input_tokens are non-cached tokens, cache_read_input_tokens are
// tokens read from cache. Together they represent the actual context size.
// Note: cache_creation_input_tokens is a subset of input_tokens, not additive.
func CalculateContextTokens(usage *llm.Usage) int {
	if usage == nil {
		return 0
	}
	return usage.InputTokens + usage.CacheReadInputTokens
}

// ShouldCompact returns true if compaction should be triggered based on token usage.
func ShouldCompact(usage *llm.Usage, messageCount int, threshold int) bool {
	if threshold <= 0 {
		threshold = DefaultContextTokenThreshold
	}
	// Never compact if there are fewer than 2 messages
	if messageCount < 2 {
		return false
	}
	return CalculateContextTokens(usage) >= threshold
}

// CompactMessages generates a summary of the conversation and returns compacted messages.
// This is the main entry point for external compaction.
//
// Parameters:
//   - ctx: Context for the LLM call
//   - model: LLM to use for generating the summary
//   - messages: The conversation messages to compact
//   - systemPrompt: The system prompt to include in the summary request
//   - summaryPrompt: The prompt instructing how to generate the summary (use DefaultCompactionSummaryPrompt if empty)
//   - tokensBefore: The pre-compaction context token count (for accurate event reporting)
//
// Returns the compacted messages, a compaction event with stats, and any error.
func CompactMessages(
	ctx context.Context,
	model llm.LLM,
	messages []*llm.Message,
	systemPrompt string,
	summaryPrompt string,
	tokensBefore int,
) ([]*llm.Message, *CompactionEvent, error) {
	if model == nil {
		return nil, nil, fmt.Errorf("model is required for compaction")
	}

	// Step 1: Filter out pending tool use blocks
	cleanedMessages := filterPendingToolUse(messages)
	if len(cleanedMessages) == 0 {
		return nil, nil, fmt.Errorf("no messages to compact after filtering")
	}

	// Track original message count before any trimming for accurate reporting
	originalMessageCount := len(cleanedMessages)

	// Step 2: Trim messages if too many to avoid exceeding context during summarization
	// Keep first message (often contains important context) + recent messages
	const maxMessagesForSummary = 50
	if len(cleanedMessages) > maxMessagesForSummary {
		cleanedMessages = append(
			cleanedMessages[:1], // Keep first message
			cleanedMessages[len(cleanedMessages)-maxMessagesForSummary+1:]..., // Keep recent messages
		)
	}

	// Step 3: Build summary request
	if summaryPrompt == "" {
		summaryPrompt = DefaultCompactionSummaryPrompt
	}

	// Add summary instruction as a user message
	summaryMessages := make([]*llm.Message, len(cleanedMessages)+1)
	copy(summaryMessages, cleanedMessages)
	summaryMessages[len(cleanedMessages)] = llm.NewUserTextMessage(summaryPrompt)

	// Step 4: Generate summary (non-streaming for simplicity)
	summaryOpts := []llm.Option{
		llm.WithMessages(summaryMessages...),
	}
	if systemPrompt != "" {
		summaryOpts = append(summaryOpts, llm.WithSystemPrompt(systemPrompt))
	}

	summaryResp, err := model.Generate(ctx, summaryOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("compaction summary generation failed: %w", err)
	}

	// Step 5: Extract summary from response
	summaryText := extractSummary(summaryResp.Message().Text())
	if summaryText == "" {
		return nil, nil, fmt.Errorf("no summary found in compaction response (missing <summary> tags)")
	}

	// Step 6: Create new message list with the summary as a user message
	// This ensures the first message is from the User role, which is required by most LLM APIs
	summaryPrefix := "Here is a summary of our conversation so far:\n\n"
	compactedMessages := []*llm.Message{
		llm.NewUserTextMessage(summaryPrefix + summaryText),
	}

	// Step 7: Build compaction event
	// TokensAfter is estimated from full summary message length (rough heuristic: ~4 chars per token)
	fullSummaryLen := len(summaryPrefix) + len(summaryText)
	tokensAfter := fullSummaryLen / 4
	if tokensAfter < 100 {
		tokensAfter = 100 // Minimum reasonable estimate
	}

	event := &CompactionEvent{
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
		Summary:           summaryText,
		MessagesCompacted: originalMessageCount,
	}

	return compactedMessages, event, nil
}

// extractSummary extracts content from <summary></summary> tags.
// Matching is case-insensitive to handle variations like <Summary> or <SUMMARY>.
func extractSummary(text string) string {
	lower := strings.ToLower(text)
	startTag := "<summary>"
	endTag := "</summary>"

	startIdx := strings.Index(lower, startTag)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startTag)

	endIdx := strings.Index(lower[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	// Extract from original text (not lowercase) to preserve case of summary content
	return strings.TrimSpace(text[startIdx : startIdx+endIdx])
}

// filterPendingToolUse removes tool_use blocks that don't have corresponding tool_result.
// If the last assistant message contains only tool_use blocks, remove the entire message.
func filterPendingToolUse(messages []*llm.Message) []*llm.Message {
	if len(messages) == 0 {
		return messages
	}

	// Check if the last message is an assistant message with tool_use
	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != llm.Assistant {
		return messages
	}

	// Count tool use blocks in the last message
	toolUseCount := 0
	nonToolUseCount := 0
	for _, content := range lastMsg.Content {
		if _, ok := content.(*llm.ToolUseContent); ok {
			toolUseCount++
		} else {
			nonToolUseCount++
		}
	}

	// If no tool use, return as-is
	if toolUseCount == 0 {
		return messages
	}

	// If all content was tool use, remove the entire message
	if nonToolUseCount == 0 {
		return messages[:len(messages)-1]
	}

	// Otherwise, filter out tool use blocks from the last message
	filteredContent := make([]llm.Content, 0, nonToolUseCount)
	for _, content := range lastMsg.Content {
		if _, isToolUse := content.(*llm.ToolUseContent); !isToolUse {
			filteredContent = append(filteredContent, content)
		}
	}

	// Create a copy with filtered content
	result := make([]*llm.Message, len(messages))
	copy(result, messages)
	result[len(result)-1] = &llm.Message{
		ID:      lastMsg.ID,
		Role:    lastMsg.Role,
		Content: filteredContent,
	}
	return result
}
