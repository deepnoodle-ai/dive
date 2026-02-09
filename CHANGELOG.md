# Changelog

## v1.0.0

Initial stable release of Dive, a Go library for building AI agents and
LLM-powered applications.

### Core

- **Agent** - Tool-calling agent loop with `NewAgent` and `CreateResponse`.
  Manages the generate-call-repeat cycle with configurable hooks at each stage.
- **LLM abstraction** - Unified `LLM` and `StreamingLLM` interfaces across all
  providers. Supports text, images, documents, tool calls, thinking/reasoning,
  and structured output.
- **Tool system** - `Tool` and `TypedTool[T]` interfaces with auto-generated
  JSON schemas via struct tags. `FuncTool` creates tools from plain functions.
  `Toolset` enables dynamic tool resolution per request. Panics in tools are
  automatically recovered.
- **Hook system** - `PreGeneration`, `PostGeneration`, `PreToolUse`,
  `PostToolUse`, `PostToolUseFailure`, `PreIteration`, and `Stop` hooks.
  All hooks receive `*HookContext` for state sharing.
- **Session management** - `Session` interface for persistent conversation
  state. In-memory and file-backed stores. Fork and compact operations.
- **Permission system** - Rule-based tool permission management with modes
  (`Default`, `Plan`, `AcceptEdits`, `BypassPermissions`, `DontAsk`),
  specifier patterns (`MatchGlob`, `MatchPath`, `MatchDomain`), and
  session allowlists.

### Providers

Eight LLM providers with self-registering init():

- Anthropic (default: claude-opus-4-5)
- OpenAI (default: gpt-5.2)
- Google (default: gemini-2.5-pro)
- Grok
- Groq
- Mistral
- Ollama
- OpenRouter

### Built-in Toolkit

Tools that align with Claude Code's patterns:

- `BashTool` - Shell command execution
- `ReadFileTool` - File reading with line range support
- `WriteFileTool` - File creation
- `EditTool` - Exact string replacement editing
- `TextEditorTool` - Combined view/create/edit operations
- `GlobTool` - File pattern matching
- `GrepTool` - Content search with regex support
- `ListDirectoryTool` - Directory listing
- `FetchTool` - HTTP fetching
- `WebSearchTool` - Web search via Brave API
- `AskUserTool` - Interactive user input

### Experimental

Functional but unstable APIs in `experimental/`:

- MCP client integration
- Sandboxing (macOS Seatbelt, Docker/Podman)
- Settings management
- Skill system
- Slash commands
- Subagent orchestration
- Context compaction
- Todo tracking
- Additional toolkit extensions
- CLI (`experimental/cmd/dive/`)
