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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// DefaultContextTokenThreshold is the default token count that triggers compaction.
const DefaultContextTokenThreshold = 100000

// DefaultMaxSummaryInputTokens bounds how large the transcript handed to the
// summarizer LLM may be, so summarization itself does not overflow the model's
// context window. Conservative for a ~200k-context model, leaving room for the
// prompt and the generated summary. Override per call with WithMaxInputTokens
// when the summarizer model is smaller or larger.
const DefaultMaxSummaryInputTokens = 150000

// summaryOutputReserveTokens is held back from the input budget for the
// generated summary.
const summaryOutputReserveTokens = 8192

// minSummaryItemTokens is the floor below which the reducer stops shrinking an
// individual message — truncating further yields little and risks dropping the
// gist of a turn.
const minSummaryItemTokens = 256

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

// CompactMessagesOption configures a CompactMessages call.
type CompactMessagesOption func(*compactMessagesConfig)

type compactMessagesConfig struct {
	maxInputTokens int
}

// WithMaxInputTokens overrides the token budget for the transcript handed to
// the summarizer (default DefaultMaxSummaryInputTokens). Set it to your
// summarizer model's context window minus headroom for the summary output.
func WithMaxInputTokens(n int) CompactMessagesOption {
	return func(c *compactMessagesConfig) {
		if n > 0 {
			c.maxInputTokens = n
		}
	}
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
//   - opts: Optional settings such as WithMaxInputTokens
//
// Returns the compacted messages, a compaction event with stats, and any error.
func CompactMessages(
	ctx context.Context,
	model llm.LLM,
	messages []*llm.Message,
	systemPrompt string,
	summaryPrompt string,
	tokensBefore int,
	opts ...CompactMessagesOption,
) ([]*llm.Message, *CompactionEvent, error) {
	if model == nil {
		return nil, nil, fmt.Errorf("model is required for compaction")
	}
	var cfg compactMessagesConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Step 1: Filter out pending tool use blocks
	cleanedMessages := filterPendingToolUse(messages)
	if len(cleanedMessages) == 0 {
		return nil, nil, fmt.Errorf("no messages to compact after filtering")
	}

	// Track original message count for accurate reporting. The reducer below
	// truncates oversized messages but never drops them, so the count is stable.
	originalMessageCount := len(cleanedMessages)

	// Step 2: Resolve the prompt and reduce the transcript to fit the
	// summarizer's own context window. Reducing by SIZE (not message count)
	// keeps a single oversized item — a large file read or command output —
	// from overflowing the summary request. Messages are truncated rather than
	// dropped, so tool_use/tool_result pairing is preserved.
	if summaryPrompt == "" {
		summaryPrompt = DefaultCompactionSummaryPrompt
	}
	maxInput := cfg.maxInputTokens
	if maxInput <= 0 {
		maxInput = DefaultMaxSummaryInputTokens
	}
	inputBudget := maxInput - len(summaryPrompt)/4 - summaryOutputReserveTokens
	// A small or misconfigured maxInput can drive the budget to zero or below.
	// reduceToSummaryBudget treats a non-positive budget as "no reduction" — the
	// opposite of intent — so clamp to a positive floor. An oversized transcript
	// then still gets bounded (each message is floored at minSummaryItemTokens).
	if inputBudget < 1 {
		inputBudget = 1
	}
	cleanedMessages = reduceToSummaryBudget(cleanedMessages, inputBudget)

	// Step 3: Build summary request.
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

	// Step 6: Create new message list with the summary as a user message.
	// Using the User role keeps the first message from the User, which most
	// LLM APIs require. The prefix frames the summary as a predecessor's
	// handoff rather than the model's own recollection, so the model treats it
	// as authoritative notes to continue from (the framing Codex uses).
	summaryPrefix := "Your conversation history was compacted to free up context. " +
		"A previous instance of you was working on this task and left the handoff " +
		"notes below. Treat them as an accurate record of what happened and continue " +
		"the work seamlessly.\n\n"
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

// estimateTokens approximates a message's token footprint from its serialized
// JSON size (~4 bytes per token). Marshaling the whole message counts tool
// inputs and results — often the largest payloads — which Message.Text (text
// content only) would miss. Best effort: a marshal error yields 0.
func estimateTokens(m *llm.Message) int {
	return messageBytes(m) / 4
}

func messageBytes(m *llm.Message) int {
	data, err := json.Marshal(m)
	if err != nil {
		return 0
	}
	return len(data)
}

// reduceToSummaryBudget shrinks the largest messages until the estimated total
// fits within budget, returning a new slice. The inputs are never mutated —
// they may be the session's stored originals — so oversized messages are
// replaced with truncated copies while the rest are reused as-is.
//
// It works iteratively, the way you would by hand: each pass finds the single
// largest message and shrinks it just enough to clear the overage (down to a
// floor), so the biggest items are clipped first. Messages are never dropped,
// which keeps tool_use/tool_result pairing intact. Oversized text, tool
// results, tool inputs, images, and documents are all reduced (see
// shrinkMessage). Only pairing-sensitive or unrecognized block types are left
// untouched, so in rare cases the total may stay above budget — a best-effort
// guard, not a hard guarantee.
func reduceToSummaryBudget(messages []*llm.Message, budget int) []*llm.Message {
	if budget <= 0 || len(messages) == 0 {
		return messages
	}
	sizes := make([]int, len(messages))
	total := 0
	for i, m := range messages {
		sizes[i] = estimateTokens(m)
		total += sizes[i]
	}
	if total <= budget {
		return messages
	}

	out := make([]*llm.Message, len(messages))
	copy(out, messages)

	// Bounded: each successful pass strictly reduces total.
	for iter := 0; total > budget && iter < len(messages)*2; iter++ {
		largest := 0
		for i, s := range sizes {
			if s > sizes[largest] {
				largest = i
			}
		}
		target := sizes[largest] - (total - budget)
		if target < minSummaryItemTokens {
			target = minSummaryItemTokens
		}
		if target >= sizes[largest] {
			break // largest is already at the floor; nothing more to give
		}
		shrunk := shrinkMessage(messages[largest], target*4)
		newSize := estimateTokens(shrunk)
		if newSize >= sizes[largest] {
			break // no progress (e.g. dominated by non-truncatable content)
		}
		out[largest] = shrunk
		total += newSize - sizes[largest]
		sizes[largest] = newSize
	}
	return out
}

// truncationMinKeepBytes is the floor below which an individual text block is
// not truncated further.
const truncationMinKeepBytes = 256

// culledToolInput replaces an oversized tool_use input. Must be valid JSON:
// ToolUseContent rejects invalid input on marshal.
var culledToolInput = json.RawMessage(`{"_omitted":"tool input omitted for summarization"}`)

// shrinkMessage returns a copy of m whose largest reducible content blocks are
// shrunk so the message's serialized size approaches targetBytes. The original
// is left untouched.
//
// Text-bearing blocks (text, tool results) are truncated head+tail; oversized
// tool_use inputs are culled to a placeholder while keeping the block so its
// matching tool_result stays paired; and standalone bulky blocks that can't be
// truncated as text — images, documents — are culled to a short placeholder.
// Culling an un-shrinkable payload is better than letting it overflow the
// summarizer and break compaction. Pairing-sensitive or unrecognized block
// types are left intact rather than risk corrupting the request.
func shrinkMessage(m *llm.Message, targetBytes int) *llm.Message {
	out := *m
	out.Content = make([]llm.Content, len(m.Content))
	copy(out.Content, m.Content)

	prev := -1
	// Bounded: each pass shrinks one block; the no-progress guard stops us once
	// the largest reducible block is already at its floor.
	for pass := 0; pass < len(out.Content)*2+2; pass++ {
		size := messageBytes(&out)
		if size <= targetBytes || size == prev {
			break
		}
		prev = size
		idx := largestReducibleBlock(out.Content)
		if idx < 0 {
			break // nothing left we can safely reduce
		}
		out.Content[idx] = reduceBlock(out.Content[idx], size-targetBytes)
	}
	return &out
}

// largestReducibleBlock returns the index of the largest content block that
// reduceBlock can meaningfully shrink, or -1 if none.
func largestReducibleBlock(content []llm.Content) int {
	best, bestSize := -1, 0
	for i, c := range content {
		if !reducible(c) {
			continue
		}
		if size := contentBytes(c); size > bestSize {
			best, bestSize = i, size
		}
	}
	return best
}

// reducible reports whether reduceBlock can shrink c below its current size.
func reducible(c llm.Content) bool {
	switch cc := c.(type) {
	case *llm.TextContent:
		return len(cc.Text) > truncationMinKeepBytes
	case *llm.ToolResultContent:
		return len(toolResultText(cc)) > truncationMinKeepBytes
	case *llm.ToolUseContent:
		return len(cc.Input) > len(culledToolInput)
	case *llm.ImageContent, *llm.DocumentContent:
		return true
	default:
		return false
	}
}

// reduceBlock returns a smaller version of c — text-bearing blocks truncated by
// `excess` bytes, tool_use inputs and image/document payloads culled to a
// placeholder. Always a fresh struct, so the original block is never mutated.
func reduceBlock(c llm.Content, excess int) llm.Content {
	switch cc := c.(type) {
	case *llm.TextContent:
		return &llm.TextContent{Text: truncateText(cc.Text, len(cc.Text)-excess), CacheControl: cc.CacheControl, Citations: cc.Citations}
	case *llm.ToolResultContent:
		txt := toolResultText(cc)
		return &llm.ToolResultContent{ToolUseID: cc.ToolUseID, Content: truncateText(txt, len(txt)-excess), IsError: cc.IsError, CacheControl: cc.CacheControl}
	case *llm.ToolUseContent:
		return &llm.ToolUseContent{ID: cc.ID, Name: cc.Name, Input: culledToolInput}
	case *llm.ImageContent:
		return &llm.TextContent{Text: "[image content omitted for summarization]"}
	case *llm.DocumentContent:
		return &llm.TextContent{Text: "[document content omitted for summarization]"}
	default:
		return c
	}
}

// contentBytes is the serialized size of a single content block.
func contentBytes(c llm.Content) int {
	data, err := json.Marshal(c)
	if err != nil {
		return 0
	}
	return len(data)
}

// toolResultText projects a tool result's content to a string for truncation.
func toolResultText(c *llm.ToolResultContent) string {
	switch v := c.Content.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
		return ""
	}
}

// truncateText keeps the head and tail of s within roughly maxBytes, eliding
// the middle with a marker. Slices are repaired to valid UTF-8.
func truncateText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	marker := fmt.Sprintf("\n\n[... %d bytes truncated for summarization ...]\n\n", len(s)-maxBytes)
	budget := maxBytes - len(marker)
	if budget < 0 {
		budget = 0
	}
	head := budget * 2 / 3
	tail := budget - head
	return strings.ToValidUTF8(s[:head], "") + marker + strings.ToValidUTF8(s[len(s)-tail:], "")
}
