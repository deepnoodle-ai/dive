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

`**/` also matches zero directories, so `**/*.go` includes Go files at the
search root. Results are the newest matching regular files, with path order breaking
ties. `MaxResults` limits the returned files; a separate model-visible content
block states when more matches or inaccessible paths exist. The complete tree
is walked to select the newest files, so a broad search may take time.
Directory symlinks are not followed.

### Grep

Search file contents using regular expressions (ripgrep-style):

```go
toolkit.NewGrepTool()
```

Grep searches regular files up to 64 MiB each. It includes hidden and ignored
files except for `DefaultExcludes`; set that option to an empty slice to remove
the built-in excludes. `UseRipgrep` enables an optional streaming ripgrep
backend when `rg` is installed. The Go backend is used otherwise. Both support
`glob`, `type`, context lines, and multiline patterns.

`output_mode` selects matching lines (`content`), unique files
(`files_with_matches`, the default), or per-file match counts (`count`; matching
lines in ordinary mode and multiline matches in multiline mode).
`MaxResults` limits entries per call: lines in content mode, files in the other
modes. `head_limit` can request a smaller page and `offset` fetches later pages.
The result content states the total and next offset when another page exists.
Missing roots and incomplete searches return an error or an explicit warning;
"No matches found" means the requested scope was searched without errors.

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

The result is one text block with stdout and stderr interleaved in emission
order. Exit 0 returns that text verbatim (including an empty string). A nonzero
exit returns `<error>Exit code N\n...merged output...</error>`. Large combined
output is truncated with an explicit marker.

Each invocation starts a fresh shell. `cd`, exported environment variables,
shell options, and exit status do not carry into the next call; an explicit
`working_directory` applies to that call only. Filesystem and other external
changes made by a command do persist.

Cancelling a run cancels the shell command and its process tree. A timeout set
by the `timeout` input is returned as a tool error; cancellation of the agent
run is returned as a context error.

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

Built-in file tools check the run context before expensive or mutating work.
Parallel batches return promptly on cancellation and stop sending stream or
progress events after the batch ends. An arbitrary in-process custom tool that
ignores its context can continue running after the call returns; implement
cooperative cancellation in custom tools that have side effects.

## Tool Annotations

Tools include annotations that describe their behavior:

```go
type ToolAnnotations struct {
    Title              string
    ReadOnlyHint       bool   // Tool only reads data
    DestructiveHint    bool   // Tool may delete/overwrite data
    IdempotentHint     bool   // Safe to call multiple times
    OpenWorldHint      bool   // Accesses external resources
    EditHint           bool   // File edit operation
}
```

## Path Validation

File tools use `PathValidator` to enforce workspace boundaries and prevent path traversal. Configure via the `WorkspaceDir` option on tool constructors.

## Next Steps

- [Custom Tools](custom-tools.md) - Build your own tools
- [Agents Guide](agents.md) - Agent configuration and hooks
