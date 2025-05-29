<div align="center">

<h1>Dive - The AI Toolkit for Go</h1>

<a href="https://www.anthropic.com"><img alt="Claude" src="https://img.shields.io/badge/Claude-6B48FF.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://www.openai.com"><img alt="GPT-4" src="https://img.shields.io/badge/GPT--4o%20|%20o1%20|%20o3-10A37F.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://www.groq.com"><img alt="Groq Models" src="https://img.shields.io/badge/DeepSeek%20|%20Llama%20|%20Qwen-FF6B4A.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://www.getstingrai.com"><img alt="Made by Stingrai" src="https://img.shields.io/badge/MADE%20BY%20Stingrai-000000.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://discord.gg/yrcuURWk"><img alt="Join our Discord community" src="https://img.shields.io/badge/Join%20our%20community-5865F2.svg?style=for-the-badge&logo=discord&labelColor=000000&logoWidth=20"></a>

</div>

Dive is an AI toolkit for Go that can be used to create specialized AI agents, automate
workflows, and quickly integrate with the leading LLMs.

- 🚀 Embed it in your Go apps
- 🤖 Create specialized agents
- 🪄 Define multi-step workflows
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

Join our [Discord community](https://discord.gg/yrcuURWk) to chat with the team and other users.

You can also use [GitHub Discussions](https://github.com/diveagents/dive/discussions) for questions, suggestions, or feedback.

We welcome your input! 🙌

Please leave a GitHub star if you're interested in the project!

## Features

* **Agents**: Chat or assign work to specialized agents with configurable reasoning
* **Supervisor Patterns**: Create hierarchical agent systems with work delegation
* **Workflows**: Define multi-step workflows for automation
* **Declarative Configuration**: Define agents and workflows using YAML
* **Multiple LLMs**: Switch between Anthropic, OpenAI, Groq, Ollama, and others
* **Extended Reasoning**: Configure reasoning effort and budget for deep thinking
* **Model Context Protocol (MCP)**: Connect to MCP servers for external tool access
* **Advanced Model Settings**: Fine-tune temperature, penalties, caching, and tool behavior
* **Tools**: Give agents rich capabilities to interact with the world
* **Tool Annotations**: Semantic hints about tool behavior (read-only, destructive, etc.)
* **Streaming**: Stream agent and workflow events for realtime UI updates
* **CLI**: Run workflows, chat with agents, and more
* **Thread Management**: Persistent conversation threads with memory
* **Confirmation System**: Built-in confirmation system for destructive operations
* **Scripting**: Embed scripts in workflows for extensibility
* **Deep Research**: Use multiple agents to perform deep research

## Quick Start

### Environment Setup

You will need some environment variables set to use the Dive CLI, both for
the LLM provider and for any tools that you'd like your agents to use.

```bash
# LLM Provider API Keys
export ANTHROPIC_API_KEY="your-key-here"
export OPENAI_API_KEY="your-key-here"
export GROQ_API_KEY="your-key-here"

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
go get github.com/diveagents/dive
```

Here's a quick example of creating a chat agent:

```go
agent, err := agent.New(agent.Options{
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

### Using Workflows

Workflows offer a declarative approach to automating multi-step processes:

```yaml title="workflow.yaml"
Name: Research
Description: Research a Topic

Config:
  LogLevel: debug
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  ConfirmationMode: if-destructive

Tools:
  - Name: Web.Search
    Enabled: true
  - Name: Web.Fetch
    Enabled: true

Agents:
  - Name: Research Assistant
    Backstory: You are an enthusiastic and deeply curious researcher.
    Tools:
      - Web.Search
      - Web.Fetch

Workflows:
  - Name: Research
    Inputs:
      - Name: topic
        Type: string
    Steps:
      - Name: Research the Topic
        Agent: Research Assistant
        Prompt:
          Text: "Research the following topic: ${inputs.topic}"
          Output: A three paragraph overview of the topic
          OutputFormat: markdown
        Store: overview
      - Name: Save the Research
        Action: Document.Write
        Parameters:
          Path: research/${inputs.topic}.md
          Content: ${overview}
```

Run a workflow using the Dive CLI:

```bash
dive run workflow.yaml --vars "topic=history of the internet"
```

### Use the Dive CLI

For the moment, you'll need to build the CLI yourself:

```bash
git clone git@github.com:diveagents/dive.git
cd dive/cmd/dive
go install .
```

Available CLI commands include:

* `dive run /path/to/workflow.yaml`: Run a workflow
* `dive chat --provider anthropic --model claude-sonnet-4-20250514`: Chat with an agent
* `dive config check /path/to/workflow.yaml`: Validate a Dive configuration

## LLM Providers

Dive provides a unified interface for working with different LLM providers:

* **Anthropic** (Claude Sonnet, Haiku)
* **OpenAI** (GPT-4, o1, o3)
* **Groq** (Llama, DeepSeek, Qwen)

Each provider implementation handles API communication, token counting,
tool calling, and other details.

```go
provider := anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))

provider := openai.New(openai.WithModel("gpt-4o"))

provider := groq.New(groq.WithModel("deepseek-r1-distill-llama-70b"))
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

MCP servers can also be configured in YAML workflows and agent definitions for declarative setup.

### Verified Models

These are the models that have been verified to work in Dive:

| Provider  | Model                           | Tools Supported |
| --------- | ------------------------------- | --------------- |
| Anthropic | `claude-sonnet-4-20250514`      | Yes             |
| Anthropic | `claude-3-7-sonnet-20250219`    | Yes             |
| Anthropic | `claude-3-5-sonnet-20241022`    | Yes             |
| Anthropic | `claude-3-5-haiku-20241022`     | Yes             |
| Groq      | `deepseek-r1-distill-llama-70b` | Yes             |
| Groq      | `llama-3.3-70b-versatile`       | Yes             |
| Groq      | `qwen-2.5-32b`                  | Yes             |
| OpenAI    | `gpt-4o`                        | Yes             |
| OpenAI    | `gpt-4.5-preview`               | Yes             |
| OpenAI    | `o1`                            | Yes             |
| OpenAI    | `o1-mini`                       | No              |
| OpenAI    | `o3-mini`                       | Yes             |
| Ollama    | `llama3.2:*`                    | Yes             |
| Ollama    | `mistral:*`                     | No              |

## Tool Use

Tools extend agent capabilities. Dive includes these built-in tools:

* **Web.Search**: Search the web using Google Custom Search or Kagi Search
* **Web.Fetch**: Fetch and extract content from webpages using Firecrawl
* **Document.Write**: Write content to files with path validation and confirmations
* **Document.Read**: Read content from files with binary detection and size limits
* **Directory.List**: List directory contents with permission controls
* **Text.Editor**: Advanced file editing with view, create, replace, and insert operations
* **Command**: Execute external commands with allow/deny list controls

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

Go interfaces are in-place to support swapping in different tool implementations
while keeping the same workflows and usage. For example, Brave Search could be
added as an alternative Web.Search tool backend.

## Contributors

We're looking for contributors! Whether you're fixing bugs, adding features,
improving documentation, or spreading the word, your help is appreciated.

## Roadmap

- ✅ Ollama support
- ✅ MCP support
- Docs site
- Server mode
- Documented approach for RAG
- AWS Bedrock support
- Google Cloud Vertex AI support
- Workflow actions with Risor scripts
- Voice interactions
- Agent memory interface
- Workflow persistence
- Integrations (Slack, Google Drive, etc.)
- Expanded CLI
- Hugging Face support

## FAQ

### Is there a hosted or managed version available?

Not at this time. Dive is provided as an open-source framework that you can
self-host and integrate into your own applications.

### Who is Behind Dive?

Dive is developed by [Stingrai](https://www.getstingrai.com).

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

## Environment System

Dive uses an Environment to orchestrate agents and manage shared resources:

```go
import "github.com/diveagents/dive/environment"

env := environment.New(environment.Options{
    Name: "Research Lab",
})

// Add multiple agents to the environment
researcher, _ := agent.New(agent.Options{
    Name:        "Researcher",
    Environment: env,
})

analyst, _ := agent.New(agent.Options{
    Name:        "Data Analyst",
    Environment: env,
})

// Agents can now reference each other and share resources
```

The Environment provides:
- **Agent Discovery**: Agents can find and delegate to each other
- **Shared Document Repository**: Common file system access
- **Thread Management**: Persistent conversation storage
- **Confirmation System**: Centralized user confirmation handling

### Use the Dive CLI

For the moment, you'll need to build the CLI yourself:
