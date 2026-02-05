# Built-in Tools Guide

Dive provides built-in tools in the `toolkit` package. All tool constructors return `*dive.TypedToolAdapter[T]`, which satisfies `dive.Tool` and can be passed directly to `AgentOptions.Tools`.

For creating custom tools, see the [Custom Tools Guide](custom-tools.md).

## File Operations

### ReadFile

Read file contents with optional line range:

```go
toolkit.NewReadFileTool()
```

### WriteFile

Create or overwrite files:

```go
toolkit.NewWriteFileTool()
```

### Edit

Exact string replacement in files:

```go
toolkit.NewEditTool()
```

### Glob

Find files using glob patterns:

```go
toolkit.NewGlobTool()
```

### Grep

Search file contents using regular expressions (ripgrep-style):

```go
toolkit.NewGrepTool()
```

### ListDirectory

List directory contents with metadata:

```go
toolkit.NewListDirectoryTool()
```

### TextEditor

Advanced file editor (Anthropic-compatible):

```go
toolkit.NewTextEditorTool()
```

## Shell

### Bash

Execute shell commands with timeout and output capture:

```go
toolkit.NewBashTool(toolkit.BashToolOptions{
    WorkspaceDir:    "/path/to/workspace",
    MaxOutputLength: 50000,
})
```

## Web

### WebSearch

Search the web. Requires a `web.Searcher` implementation (e.g. from `wonton/web`):

```go
toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
    Searcher: searcher, // e.g. google.NewSearcher() or kagi.NewSearcher()
})
```

### Fetch

Fetch and extract content from web pages. Requires a `fetch.Fetcher` implementation:

```go
toolkit.NewFetchTool(toolkit.FetchToolOptions{
    Fetcher: fetcher, // e.g. fetch.NewHTTPFetcher()
})
```

## User Interaction

### AskUser

Ask users questions with various input types:

```go
toolkit.NewAskUserTool()
```

## Using Tools with an Agent

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Research Agent",
    SystemPrompt: "You are a research assistant with file and web access.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewReadFileTool(),
        toolkit.NewWriteFileTool(),
        toolkit.NewGlobTool(),
        toolkit.NewGrepTool(),
        toolkit.NewBashTool(),
    },
})
```

## Tool Annotations

Tools include annotations that describe their behavior:

```go
type ToolAnnotations struct {
    Title           string
    ReadOnlyHint    bool   // Tool only reads data
    DestructiveHint bool   // Tool may delete/overwrite data
    IdempotentHint  bool   // Safe to call multiple times
    OpenWorldHint   bool   // Accesses external resources
    EditHint        bool   // File edit operation
}
```

## Path Validation

File tools use `PathValidator` to enforce workspace boundaries and prevent path traversal. Configure via the `WorkspaceDir` option on tool constructors.

## Next Steps

- [Custom Tools](custom-tools.md) - Build your own tools
- [Agents Guide](agents.md) - Agent configuration and hooks
