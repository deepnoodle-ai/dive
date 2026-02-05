// Package providers contains the LLM provider registry and shared error types.
//
// Providers self-register via init() functions using [Register]. The registry
// matches model names to provider factories using configurable matchers
// ([PrefixMatcher], [ContainsMatcher], [EnvMatcher]).
//
// Individual providers are in subpackages:
//
//   - [github.com/deepnoodle-ai/dive/providers/anthropic] - Claude models
//   - [github.com/deepnoodle-ai/dive/providers/google] - Gemini models
//   - [github.com/deepnoodle-ai/dive/providers/openai] - OpenAI Responses API
//   - [github.com/deepnoodle-ai/dive/providers/openaicompletions] - OpenAI Chat Completions API
//   - [github.com/deepnoodle-ai/dive/providers/grok] - X.AI Grok models
//   - [github.com/deepnoodle-ai/dive/providers/groq] - Groq inference engine
//   - [github.com/deepnoodle-ai/dive/providers/mistral] - Mistral models
//   - [github.com/deepnoodle-ai/dive/providers/ollama] - Local model serving
//   - [github.com/deepnoodle-ai/dive/providers/openrouter] - Multi-provider proxy
package providers
