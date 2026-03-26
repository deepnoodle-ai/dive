package openai

const (
	// GPT-5 models (latest)
	ModelGPT54           = "gpt-5.4"
	ModelGPT54Mini       = "gpt-5.4-mini"
	ModelGPT53ChatLatest = "gpt-5.3-chat-latest"
	ModelGPT53ChatLast   = ModelGPT53ChatLatest // Deprecated: Use ModelGPT53ChatLatest.
	ModelGPT52           = "gpt-5.2"
	ModelGPT52Pro        = "gpt-5.2-pro"
	ModelGPT51           = "gpt-5.1"
	ModelGPT51Mini       = "gpt-5.1-mini"
	ModelGPT5            = "gpt-5"
	ModelGPT5Pro         = "gpt-5-pro"
	ModelGPT5Mini        = "gpt-5-mini"
	ModelGPT5Nano        = "gpt-5-nano"

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
	ModelGPT53Codex      = "gpt-5.3-codex"
	ModelGPT53CodexSpark = "gpt-5.3-codex-spark"
	ModelGPT52Codex      = "gpt-5.2-codex"
	ModelGPT51CodexMax   = "gpt-5.1-codex-max"
	ModelGPT51Codex      = "gpt-5.1-codex"
	ModelGPT5Codex       = "gpt-5-codex"
	ModelGPT5CodexMini   = "gpt-5-codex-mini"
	ModelCodexMiniLatest = "codex-mini-latest"

	// Deprecated: Use ModelGPT53Codex instead.
	ModelCodex = ModelCodexMiniLatest
	// Deprecated: Use ModelCodexMiniLatest instead.
	ModelCodexAsk = "codex-ask"
)
