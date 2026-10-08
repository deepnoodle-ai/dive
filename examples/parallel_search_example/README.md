# Parallel Search MCP example

Search the web or fetch a page through Dive's experimental MCP client and tool
adapter. The [Parallel Search MCP](https://docs.parallel.ai/integrations/mcp/search-mcp)
endpoint supports anonymous search and fetch without a Parallel API key. Free
access has rate limits and is intended for exploration and light use.

From the repository's `examples` directory, with Go 1.26.8 installed:

```bash
go mod download
go run ./parallel_search_example -query "Dive Go MCP integration"
go run ./parallel_search_example -url "https://github.com/deepnoodle-ai/dive"
```

To keep related calls in one session, pass the same `-session-id` to each:

```bash
go run ./parallel_search_example -session-id my-research-1 -query "Dive Go MCP integration"
go run ./parallel_search_example -session-id my-research-1 -url "https://github.com/deepnoodle-ai/dive"
```

The example embeds `mcp.json`, loads it with `mcp.ParseServersJSON`, discovers the
server's tools, and calls `web_search` or `web_fetch` through
`mcp.NewQualifiedToolAdapter`. It prints the returned text, including source URLs
and excerpts. Requests carry `User-Agent: dive-parallel-search-example/1.0`.
Each invocation makes one tool call with a 60-second timeout and Ctrl-C
cancellation. It uses the `-session-id` value when given and a new UUID otherwise.

This example calls tools directly and makes no LLM request, so it needs no model
API key either. To build an agent, pass the discovered adapters in
`dive.AgentOptions.Tools` as described in the
[MCP integration guide](../../docs/guides/experimental/mcp-integration.md#programmatic-usage).
Model inference has separate credentials and costs. Keep the same `session_id`
across related search and fetch calls within a conversation.
