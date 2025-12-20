package anthropic

const (
	// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking?q=extended+output#extended-output-capabilities-beta
	FeatureOutput128k    = "output-128k-2025-02-19"
	FeatureExtendedCache = "extended-cache-ttl-2025-04-11"
	FeaturePromptCaching = "prompt-caching-2024-07-31"
	FeatureMCPClient     = "mcp-client-2025-04-04"

	// Code execution tool beta headers
	// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool
	FeatureCodeExecution       = "code-execution-2025-08-25" // Current version with bash and text_editor
	FeatureCodeExecutionLegacy = "code-execution-2025-05-22" // Legacy version (Python only)

	// Computer use tool beta headers
	// https://docs.anthropic.com/en/docs/agents-and-tools/computer-use
	FeatureComputerUse       = "computer-use-2025-01-24" // Sonnet 4, Sonnet 4.5, Haiku 4.5, Opus 4, Opus 4.1, Sonnet 3.7
	FeatureComputerUseOpus45 = "computer-use-2025-11-24" // Opus 4.5 (adds zoom action)
)
