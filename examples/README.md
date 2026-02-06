# Dive Examples

Standalone example programs demonstrating Dive capabilities. Run from the
`examples/` directory:

```bash
cd examples
```

## Programs

| Example | Provider | Description |
| --- | --- | --- |
| `code_execution_example` | Anthropic | Claude runs Python to compute 53^4 |
| `server_tools_example` | Anthropic | Agent with web search |
| `image_example` | Anthropic | Vision: describe an image from a URL |
| `citations_example` | Anthropic | Document analysis with source citations |
| `pdf_example` | Anthropic | PDF document analysis |
| `llm_example` | Anthropic | Direct LLM usage without the agent loop |
| `mcp_servers_example` | Anthropic | Model Context Protocol tool integration |
| `openai_responses_example` | OpenAI | Web search, reasoning, structured output, and MCP |
| `openai_responses_pdf_example` | OpenAI | PDF analysis via the Responses API |
| `google_example` | Google | Gemini model usage |
| `google_tool_example` | Google | Gemini with tool calling |
| `ollama_example` | Ollama | Local model usage |
| `openrouter_example` | OpenRouter | Multi-provider routing |
| `oauth_client` | — | OAuth client credential flow |

Run any example with `go run`:

```bash
go run ./code_execution_example
go run ./server_tools_example
```
