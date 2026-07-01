package anthropic

const (
	// Claude 3.5 models
	ModelClaude35Haiku20241022  = "claude-3-5-haiku-20241022"
	ModelClaude35Sonnet20241022 = "claude-3-5-sonnet-20241022"
	ModelClaude37Sonnet20250219 = "claude-3-7-sonnet-20250219"

	// Claude 4 models
	ModelClaudeSonnet420250514 = "claude-sonnet-4-20250514"
	ModelClaudeOpus420250514   = "claude-opus-4-20250514"
	ModelClaudeOpus4120250805  = "claude-opus-4-1-20250805"

	// Claude 4.5 models (dated)
	ModelClaudeHaiku4520251001  = "claude-haiku-4-5-20251001"
	ModelClaudeSonnet4520250929 = "claude-sonnet-4-5-20250929"
	ModelClaudeOpus4520251101   = "claude-opus-4-5-20251101"

	// Claude 4.5 models (latest, without date suffix)
	ModelClaudeHaiku45  = "claude-haiku-4-5"
	ModelClaudeSonnet45 = "claude-sonnet-4-5"
	ModelClaudeOpus45   = "claude-opus-4-5"

	// Claude 4.6 models
	ModelClaudeSonnet46 = "claude-sonnet-4-6"
	ModelClaudeOpus46   = "claude-opus-4-6"

	// Claude 4.7 models
	ModelClaudeOpus47 = "claude-opus-4-7"

	// Claude 4.8 models
	ModelClaudeOpus48 = "claude-opus-4-8"

	// Claude 5 models (latest). Fable 5 is generally available; Mythos 5 is
	// limited availability via Project Glasswing. Sonnet 5 is a drop-in upgrade
	// for Sonnet 4.6 with adaptive thinking on by default and a new tokenizer.
	ModelClaudeFable5  = "claude-fable-5"
	ModelClaudeMythos5 = "claude-mythos-5"
	ModelClaudeSonnet5 = "claude-sonnet-5"
)
