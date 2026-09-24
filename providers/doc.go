// Package providers contains the LLM provider registry, shared error types,
// and helpers for provider implementations.
//
// Providers self-register via init() functions using [Register]. The registry
// matches model names to provider factories using configurable matchers
// ([PrefixMatcher], [ContainsMatcher], [EnvMatcher]).
//
// The helpers are for code that encodes Dive messages for a provider API:
// reading tool results ([ToolResultBlocks], [EmptyToolResultText],
// [LiftToolResultImages]), recognizing history another provider's servers
// produced ([IsServerToolContent]), and retrying requests and streams
// ([RetryPolicy], [NewRetryingStreamIterator]).
//
// Individual providers are in subpackages:
//
//   - [github.com/deepnoodle-ai/dive/providers/anthropic] - Claude models
//   - [github.com/deepnoodle-ai/dive/providers/google] - Gemini models
//   - [github.com/deepnoodle-ai/dive/providers/openai] - OpenAI Responses API
//   - [github.com/deepnoodle-ai/dive/providers/openaicompletions] - OpenAI Chat Completions API
//   - [github.com/deepnoodle-ai/dive/providers/grok] - X.AI Grok models
//   - [github.com/deepnoodle-ai/dive/providers/mistral] - Mistral models
//   - [github.com/deepnoodle-ai/dive/providers/ollama] - Local model serving
//   - [github.com/deepnoodle-ai/dive/providers/openrouter] - Multi-provider proxy
package providers
