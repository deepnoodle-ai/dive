// Package llm defines the unified abstraction layer over different LLM providers.
//
// It provides core interfaces, message types, and content blocks used across
// all providers:
//
//   - [LLM] and [StreamingLLM] are the provider interfaces.
//   - [Message] carries content to and from an LLM.
//   - [Content] blocks represent text, images, documents, tool calls, and other
//     message components.
//   - [Option] functions configure LLM requests (model, temperature, tools, etc.).
//   - [Tool] describes a callable tool at the LLM level.
//
// Most users interact with this package indirectly through [github.com/deepnoodle-ai/dive.Agent].
// Direct usage is needed when building custom providers or working with the LLM
// layer directly.
package llm
