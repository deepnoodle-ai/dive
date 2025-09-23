<div align="center">

<h1>Dive</h1>

<a href="https://www.anthropic.com"><img alt="Claude" src="https://img.shields.io/badge/Claude-6B48FF.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://www.openai.com"><img alt="GPT-4" src="https://img.shields.io/badge/GPT--4o%20|%20o1%20|%20o3-10A37F.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://cloud.google.com/vertex-ai"><img alt="Gemini" src="https://img.shields.io/badge/Gemini-4285F4.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://x.ai"><img alt="Grok" src="https://img.shields.io/badge/Grok-1DA1F2.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://ollama.ai"><img alt="Ollama" src="https://img.shields.io/badge/Ollama-1E2952.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://openrouter.ai"><img alt="OpenRouter" src="https://img.shields.io/badge/OpenRouter-7C3AED.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://deepnoodle.ai"><img alt="Made by Deep Noodle" src="https://img.shields.io/badge/MADE%20BY%20Deep%20Noodle-000000.svg?style=for-the-badge&labelColor=000000"></a>

</div>

Dive is an AI toolkit for Go that can be used to create specialized teams of AI
agents and quickly integrate with the leading LLMs.

- 🚀 Embed it in your Go apps
- 🤖 Create specialized agents
- 🛠️ Arm agents with tools
- ⚡ Stream responses in real-time

Dive includes both a CLI and a polished set of APIs for easy integration into
existing Go applications. It comes batteries-included, but also has the
modularity you need for extensive customization.

## Project Status

Dive is shaping up nicely, but is still a young project.

- **Feedback is highly valued** on concepts, APIs, and usability
- **Some breaking changes will happen** as the API matures
- **Not yet recommended for production use**

You can also use [GitHub Discussions](https://github.com/deepnoodle-ai/dive/discussions) for questions, suggestions, or feedback.

We welcome your input! 🙌

Please leave a GitHub star if you're interested in the project!

## Features

* **Agents**: Chat or assign work to specialized agents with configurable reasoning
* **Supervisor Patterns**: Create hierarchical agent systems with work delegation
* **Declarative Configuration**: Define agents using YAML
* **Multiple LLMs**: Switch between Anthropic, OpenAI, Google, OpenRouter, Grok, Ollama, and others
* **Extended Reasoning**: Configure reasoning effort and budget for deep thinking
* **Model Context Protocol (MCP)**: Connect to MCP servers for external tool access
* **Advanced Model Settings**: Fine-tune temperature, penalties, caching, and tool behavior
* **Tools**: Give agents rich capabilities to interact with the world
* **Tool Annotations**: Semantic hints about tool behavior (read-only, destructive, etc.)
* **Streaming**: Stream agent events for realtime UI updates
* **CLI**: Run agents, chat with agents, and more
* **Thread Management**: Persistent conversation threads with memory
* **Confirmation System**: Built-in confirmation system for destructive operations
* **Deep Research**: Use multiple agents to perform deep research
* **Semantic Diff**: AI-powered analysis of text differences for output drift detection

## Quick Start

### Environment Setup

You will need some environment variables set to use the Dive CLI, both for
the LLM provider and for any tools that you'd like your agents to use.

```bash
# LLM Provider API Keys
export ANTHROPIC_API_KEY="your-key-here"
export OPENAI_API_KEY="your-key-here"
export GEMINI_API_KEY="your-key-here"
export GROK_API_KEY="your-key-here"
export OPENROUTER_API_KEY="your-key-here"

# Tool API Keys
export GOOGLE_SEARCH_API_KEY="your-key-here"
export GOOGLE_SEARCH_CX="your-key-here"
export FIRECRAWL_API_KEY="your-key-here"
```

Firecrawl is used to retrieve webpage content. Create an account with
[Firecrawl](https://firecrawl.com) to get a free key to experiment with.

Generating a Google Custom Search key is also quite easy, assuming you have a
Google Cloud account. See the [Google Custom Search documentation](https://developers.google.com/custom-search/v1/overview).

### Using the Library

To get started with Dive as a library, use go get:

```
go get github.com/deepnoodle-ai/dive
```

Here's a quick example of creating a chat agent:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Research Assistant",
    Instructions: "You are an enthusiastic and deeply curious researcher.",
    Model:        anthropic.New(),
})

// Start chatting with the agent
response, err := agent.CreateResponse(ctx, dive.WithInput("Hello there!"))
// Or stream the response
stream, err := agent.StreamResponse(ctx, dive.WithInput("Hello there!"))
```

Or use the Dive LLM interface directly:

```go
model := anthropic.New()
response, err := model.Generate(
  context.Background(),
  llm.WithMessages(llm.NewUserTextMessage("Hello there!")),
  llm.WithMaxTokens(2048),
  llm.WithTemperature(0.7),
)
if err != nil {
  log.Fatal(err)
}
fmt.Println(response.Message.Text())
```

### Dive Configurations

Dive configurations offer a declarative approach to defining agents and MCP servers:

```yaml title="config.yaml"
Name: Research
Description: Research a topic

Config:
  LogLevel: debug
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  ConfirmationMode: if-destructive

Agents:
  - Name: Research Assistant
    Backstory: You are an enthusiastic and deeply curious researcher.
    Tools:
      - web_search
      - fetch
```

### Use the Dive CLI

For the moment, you'll need to build the CLI yourself:

```bash
git clone git@github.com:deepnoodle-ai/dive.git
cd dive/cmd/dive
go install .
```

Available CLI commands include:

* `dive chat --provider anthropic --model claude-sonnet-4-20250514`: Chat with an agent
* `dive classify --text "input text" --labels "label1,label2,label3"`: Classify text with confidence scores
* `dive config check /path/to/config.yaml`: Validate a Dive configuration
* `dive diff old.txt new.txt --explain-changes`: Semantic diff between texts using LLMs to explain changes

### Semantic Diff

The `dive diff` command provides AI-powered semantic analysis of differences between text files, which is especially useful for detecting output drift and understanding meaningful changes:

```bash
# Basic diff - shows file size changes and suggests AI analysis
dive diff old_output.txt new_output.txt

# AI-powered semantic analysis with detailed explanations
dive diff old_output.txt new_output.txt --explain-changes

# Different output formats
dive diff old.txt new.txt --explain-changes --format markdown
dive diff old.txt new.txt --explain-changes --format json

# Use with different LLM providers
dive diff old.txt new.txt --explain-changes --provider openai --model gpt-4o
dive diff old.txt new.txt --explain-changes --provider anthropic --model claude-sonnet-4-20250514
```

This is particularly useful for:
- **Output Drift Detection**: Comparing AI-generated outputs over time
- **Code Review**: Understanding semantic changes in generated code
- **Content Analysis**: Analyzing changes in documentation or text content
- **Quality Assurance**: Detecting meaningful changes in test outputs

## LLM Providers

Dive provides a unified interface for working with different LLM providers:

* **Anthropic** (Claude Sonnet, Haiku, Opus)
* **OpenAI** (GPT-5, GPT-4, o1, o3)
* **OpenRouter** (Access to 200+ models from multiple providers with unified API)
* **Google** (Gemini models)
* **Grok** (Grok 4, Grok Code Fast)
* **Ollama** (Local model serving)

Each provider implementation handles API communication, token counting,
tool calling, and other details.

```go
provider := anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))

provider := openai.New(openai.WithModel("gpt-5-2025-08-07"))

provider := openrouter.New(openrouter.WithModel("openai/gpt-4o"))

provider := google.New(google.WithModel("gemini-2.5-flash"))

provider := ollama.New(ollama.WithModel("llama3.2:3b"))
```

## Model Context Protocol (MCP)

Dive supports the Model Context Protocol (MCP) for connecting to external tools and services:

```go
response, err := anthropic.New().Generate(
    context.Background(),
    llm.WithMessages(llm.NewUserTextMessage("What are the open tickets?")),
    llm.WithMCPServers(
        llm.MCPServerConfig{
            Type:               "url",
            Name:               "linear",
            URL:                "https://mcp.linear.app/sse",
            AuthorizationToken: "your-token-here",
        },
    ),
)
```

MCP servers can also be configured in YAML configurations for declarative setup.

### Verified Models

These are the models that have been verified to work in Dive:

| Provider  | Model                           | Tools Supported |
| --------- | ------------------------------- | --------------- |
| Anthropic | `claude-sonnet-4-20250514`      | Yes             |
| Anthropic | `claude-opus-4-20250514`        | Yes             |
| Anthropic | `claude-3-7-sonnet-20250219`    | Yes             |
| Anthropic | `claude-3-5-sonnet-20241022`    | Yes             |
| Anthropic | `claude-3-5-haiku-20241022`     | Yes             |
| Google    | `gemini-2.5-flash`              | Yes             |
| Google    | `gemini-2.5-flash-lite`         | Yes             |
| Google    | `gemini-2.5-pro`                | Yes             |
| Grok      | `grok-4-0709`                   | Yes             |
| Grok      | `grok-code-fast-1`              | Yes             |
| OpenAI    | `gpt-5-2025-08-07`              | Yes             |
| OpenAI    | `gpt-4o`                        | Yes             |
| OpenAI    | `gpt-4.5-preview`               | Yes             |
| OpenAI    | `o1`                            | Yes             |
| OpenAI    | `o1-mini`                       | No              |
| OpenAI    | `o3-mini`                       | Yes             |
| Ollama    | `llama3.2:*`                    | Yes             |
| Ollama    | `mistral:*`                     | No              |

## Tool Use

Tools extend agent capabilities. Dive includes these built-in tools:

* **list_directory**: List directory contents
* **read_file**: Read content from files
* **write_file**: Write content to files
* **text_editor**: Advanced file editing with view, create, replace, and insert operations
* **web_search**: Search the web using Google Custom Search or Kagi Search
* **fetch**: Fetch and extract content from webpages using Firecrawl
* **command**: Execute external commands
* **generate_image**: Generate images using OpenAI's gpt-image-1

### Tool Annotations

Dive's tool system includes rich annotations that provide hints about tool behavior:

```go
type ToolAnnotations struct {
    Title           string      // Human-readable title
    ReadOnlyHint    bool        // Tool only reads, doesn't modify
    DestructiveHint bool        // Tool may make destructive changes
    IdempotentHint  bool        // Tool is safe to call multiple times
    OpenWorldHint   bool        // Tool accesses external resources
}
```

### Custom Tools

Creating custom tools is straightforward using the `TypedTool` interface:

```go
type SearchTool struct{}

func (t *SearchTool) Name() string { return "search" }
func (t *SearchTool) Description() string { return "Search for information" }
func (t *SearchTool) Schema() schema.Schema { /* define parameters */ }
func (t *SearchTool) Annotations() dive.ToolAnnotations { /* tool hints */ }
func (t *SearchTool) Call(ctx context.Context, input *SearchInput) (*dive.ToolResult, error) {
    // Tool implementation
}

// Use with ToolAdapter for type safety
tool := dive.ToolAdapter(searchTool)
```

Go interfaces are in-place to support swapping in different tool implementations.

## Contributors

We're looking for contributors! Whether you're fixing bugs, adding features,
improving documentation, or spreading the word, your help is appreciated.

## Roadmap

- ✅ Ollama support
- ✅ MCP support
- ✅ Google Cloud Vertex AI support
- Docs site
- Server mode
- Documented approach for RAG
- AWS Bedrock support
- Voice interactions
- Agent memory interface
- Integrations (Slack, Google Drive, etc.)
- Expanded CLI
- Hugging Face support

## FAQ

### Is there a hosted or managed version available?

Not at this time. Dive is provided as an open-source framework that you can
self-host and integrate into your own applications.

## Advanced Agent Features

### Supervisor Patterns

Agents can be configured as supervisors to delegate work to other agents:

```go
supervisor, err := agent.New(agent.Options{
    Name:         "Research Manager",
    Instructions: "You coordinate research tasks across multiple specialists.",
    IsSupervisor: true,
    Subordinates: []string{"Data Analyst", "Web Researcher"},
    Model:        anthropic.New(),
})
```

Supervisor agents automatically get an `assign_work` tool for delegating tasks.

### Model Settings

Fine-tune LLM behavior with advanced model settings:

```go
agent, err := agent.New(agent.Options{
    Name: "Assistant",
    ModelSettings: &agent.ModelSettings{
        Temperature:       ptr(0.7),
        ReasoningBudget:   ptr(50000),
        ReasoningEffort:   "high",
        MaxTokens:         4096,
        ParallelToolCalls: ptr(true),
        Caching:           ptr(true),
    },
    Model: anthropic.New(),
})
```

### Thread Management

Agents support persistent conversation threads:

```go
response, err := agent.CreateResponse(ctx,
    dive.WithThreadID("conversation-123"),
    dive.WithInput("Continue our discussion"),
)
```
