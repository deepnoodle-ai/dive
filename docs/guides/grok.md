# Grok (xAI) Guide

The Grok provider (`providers/grok`) connects Dive to xAI's models through the
xAI Responses API. It is a separate Go module
(`github.com/deepnoodle-ai/dive/providers/grok`) built on top of the OpenAI
Responses provider, so it inherits streaming, tool calling, prompt caching, and
the server-side tool plumbing described below.

## Setup

```go
import "github.com/deepnoodle-ai/dive/providers/grok"

model := grok.New() // defaults to grok-4.3
```

**Env:** `XAI_API_KEY` (preferred) or `GROK_API_KEY`.
**Models:** See `providers/grok/models.go`. `grok-4.3` is the current flagship
and default; `grok-build-0.1` is the coding model.

## Server-side tools

Grok runs several tools on xAI's servers: you attach a tool, and the API
executes it and folds the results into the response. Pass them with
`llm.WithTools(...)` (or via `AgentOptions.Tools` on an agent).

All of these tools return citations for the sources they used; citations are
attached to the assistant's text content blocks (see [Citations](#citations)).

### Web search

```go
webSearch, err := grok.NewWebSearchTool(grok.WebSearchToolOptions{
    AllowedDomains:           []string{"wikipedia.org"}, // max 5; or ExcludedDomains
    EnableImageUnderstanding: true,                      // analyze images while browsing
    EnableImageSearch:        true,                      // find & embed relevant images
})
```

`AllowedDomains` and `ExcludedDomains` are mutually exclusive (max 5 each).
`EnableImageUnderstanding` lets Grok inspect images it finds while browsing;
`EnableImageSearch` lets Grok search for images and embed them in the response
as Markdown.

### X (Twitter) search

```go
xSearch, err := grok.NewXSearchTool(grok.XSearchToolOptions{
    AllowedXHandles:          []string{"xai", "elonmusk"}, // max 20; or ExcludedXHandles
    FromDate:                 "2025-10-01",                // ISO-8601 YYYY-MM-DD
    ToDate:                   "2025-10-10",
    EnableImageUnderstanding: true,
    EnableVideoUnderstanding: true, // X search only
})
```

### Code execution

Grok writes and runs Python in a sandbox for precise calculation and data
analysis (the xAI API's `code_interpreter` tool).

```go
codeExec := grok.NewCodeExecutionTool(grok.CodeExecutionToolOptions{
    IncludeOutputs: true, // return the executed code's outputs in the response
})
```

When `IncludeOutputs` is set, the request opts into the
`code_interpreter_call.outputs` include. The executed code and its results come
back as a `*openai.CodeInterpreterCallContent` content block.

See `examples/grok_code_execution_example`.

### Collections search

Grok can search your uploaded knowledge bases (collections), mapping to the
xAI API's `file_search` tool where collection IDs are the vector store IDs.
Create collections and upload documents with the xAI console or SDK first, then:

```go
collections, err := grok.NewCollectionsSearchTool(grok.CollectionsSearchToolOptions{
    CollectionIDs:  []string{"collection_3be0..."}, // at least one required
    MaxNumResults:  10,                             // optional, 1–50
    IncludeResults: true,                           // return matched chunks
})
```

With `IncludeResults`, matched document chunks come back as a
`*openai.FileSearchCallContent` content block (opting into the
`file_search_call.results` include).

### Remote MCP servers

Grok can connect to remote MCP servers through Dive's core MCP configuration —
no Grok-specific code required:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model: grok.New(),
    ModelSettings: &dive.ModelSettings{
        MCPServers: []llm.MCPServerConfig{{
            Type:               "url",
            URL:                "https://mcp.deepwiki.com/mcp",
            Name:               "deepwiki",
            AuthorizationToken: "optional-token",
            ToolConfiguration:  &llm.MCPToolConfiguration{AllowedTools: []string{"ask_question"}},
        }},
    },
})
```

`AllowedTools` restricts which of the server's tools Grok may call. (At the
`llm` layer, the same is available via `llm.WithMCPServers(...)`.)

## Citations

The sources Grok used are attached to the assistant's text content as
`*llm.WebSearchResultLocation` citations:

```go
for _, content := range response.Content {
    if text, ok := content.(*llm.TextContent); ok {
        for _, citation := range text.Citations {
            if loc, ok := citation.(*llm.WebSearchResultLocation); ok {
                fmt.Printf("- %s (%s)\n", loc.Title, loc.URL)
            }
        }
    }
}
```

See `examples/grok_search_example`.

## Reasoning token usage

Grok's reasoning models report how many output tokens were spent thinking.
Dive surfaces this on `llm.Usage.ReasoningTokens` (a subset of `OutputTokens`):

```go
fmt.Printf("reasoning tokens: %d\n", response.Usage.ReasoningTokens)
```

## Prompt caching

Route requests in a conversation to the same cache with `WithPromptCacheKey`:

```go
provider := grok.New(grok.WithPromptCacheKey("conversation-uuid"))
```

Cache hits are reported on `response.Usage.CacheReadInputTokens`.
