package anthropic

const (
	// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking?q=extended+output#extended-output-capabilities-beta
	FeatureOutput128k    = "output-128k-2025-02-19"
	FeatureExtendedCache = "extended-cache-ttl-2025-04-11"
	FeaturePromptCaching = "prompt-caching-2024-07-31"
	// Deprecated: Use FeatureMCPClientV2 instead.
	FeatureMCPClient         = "mcp-client-2025-04-04"
	FeatureContextManagement = "context-management-2025-06-27"

	// Code execution tool beta headers
	// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool
	FeatureCodeExecution       = "code-execution-2025-08-25" // Current version with bash and text_editor
	FeatureCodeExecutionLegacy = "code-execution-2025-05-22" // Legacy version (Python only)

	// Computer use tool beta headers
	// https://docs.anthropic.com/en/docs/agents-and-tools/computer-use
	FeatureComputerUse      = "computer-use-2025-01-24" // Sonnet 4, Sonnet 4.5, Haiku 4.5, Opus 4, Opus 4.1, Sonnet 3.7
	FeatureComputerUse45_46 = "computer-use-2025-11-24" // Opus 4.5, Sonnet 4.6, Opus 4.6 (adds zoom action)

	// Deprecated: Use FeatureComputerUse45_46 instead.
	FeatureComputerUseOpus45 = FeatureComputerUse45_46

	// 1M context window beta (Opus 4.6, Sonnet 4.6, Sonnet 4.5, Sonnet 4).
	// Opus 4.7 and 4.8 include the 1M context window by default with no beta
	// header required.
	FeatureContext1M = "context-1m-2025-08-07"

	// Fast mode: higher output tokens per second at premium pricing (research
	// preview). Supported on Opus 4.6, 4.7, and 4.8. Sent automatically when
	// llm.WithSpeed(llm.SpeedFast) is used.
	FeatureFastMode = "fast-mode-2026-02-01"

	// Extended output up to 300k tokens on the Message Batches API (Opus 4.6,
	// 4.7, 4.8, Sonnet 4.6).
	FeatureOutput300k = "output-300k-2026-03-24"

	// Server-side compaction (Opus 4.6, Sonnet 4.6)
	FeatureCompact = "compact-2026-01-12"

	// MCP client connector (updated from mcp-client-2025-04-04)
	FeatureMCPClientV2 = "mcp-client-2025-11-20"

	// Files API for upload/download/reuse
	FeatureFilesAPI = "files-api-2025-04-14"

	// Interleaved thinking beta for Opus 4.5 and earlier Claude 4 models.
	// Adaptive-thinking models handle interleaved thinking automatically.
	FeatureInterleavedThinking = "interleaved-thinking-2025-05-14"
)
