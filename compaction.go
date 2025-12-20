package dive

import (
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

// CompactionConfig configures client-side context compaction.
// When enabled, the agent will monitor token usage and automatically
// summarize the conversation when thresholds are exceeded.
type CompactionConfig struct {
	// Enabled must be true to activate compaction.
	Enabled bool `json:"enabled"`

	// ContextTokenThreshold is the token count that triggers compaction.
	// Default: 100000 (100k tokens).
	// Total tokens are calculated as: InputTokens + OutputTokens +
	// CacheCreationInputTokens + CacheReadInputTokens.
	ContextTokenThreshold int `json:"context_token_threshold,omitempty"`

	// Model is an optional LLM to use for summary generation.
	// If nil, uses the agent's primary model.
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
