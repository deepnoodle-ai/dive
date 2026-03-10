package openai

const (
	// GPT-5 models (latest)
	ModelGPT54          = "gpt-5.4"
	ModelGPT53ChatLast  = "gpt-5.3-chat-latest"
	ModelGPT52          = "gpt-5.2"
	ModelGPT52Pro       = "gpt-5.2-pro"
	ModelGPT51          = "gpt-5.1"
	ModelGPT51Mini      = "gpt-5.1-mini"
	ModelGPT5           = "gpt-5"
	ModelGPT5Pro        = "gpt-5-pro"
	ModelGPT5Mini       = "gpt-5-mini"
	ModelGPT5Nano       = "gpt-5-nano"

	// GPT-4 models
	ModelGPT41 = "gpt-4.1"
	ModelGPT4o = "gpt-4o"

	// o-series reasoning models
	ModelO3                 = "o3"
	ModelO3Pro              = "o3-pro"
	ModelO3Mini             = "o3-mini"
	ModelO4Mini             = "o4-mini"
	ModelO3DeepResearch     = "o3-deep-research"
	ModelO4MiniDeepResearch = "o4-mini-deep-research"

	// Codex models (coding-optimized)
	ModelCodexMiniLatest = "codex-mini-latest"
	ModelGPT51Codex      = "gpt-5.1-codex"

	// Deprecated: Use ModelCodexMiniLatest instead.
	ModelCodex = ModelCodexMiniLatest
	// Deprecated: Use ModelCodexMiniLatest instead.
	ModelCodexAsk = "codex-ask"
)
