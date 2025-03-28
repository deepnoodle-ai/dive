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

* **Agents**: Chat or assign work to specialized agents
* **Workflows**: Define multi-step workflows for automation
* **Declarative Configuration**: Define agents and workflows using YAML
* **Multiple LLMs**: Switch between Anthropic, OpenAI, Groq, and others
* **Extended Reasoning**: Configure the effort level for Agent reasoning
* **Tools**: Give agents the ability to interact with the world
* **Streaming**: Stream agent and workflow events for realtime UI updates
* **CLI**: Run workflows, chat with agents, and more
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

### Using the Library

To get started with Dive as a library, use go get:

```
go get github.com/diveagents/dive
```

Here's a quick example of creating a chat agent:

```go
agent, err := agent.New(agent.Options{
    Name:      "Research Assistant",
    Backstory: "You are an enthusiastic and deeply curious researcher.",
    Model:     anthropic.New(),
    AutoStart: true,
})

// Start chatting with the agent
iterator, err := agent.Chat(ctx, llm.NewSingleUserMessage("Hello there!"))
// Iterate over the events...
```

Or use the Dive LLM interface directly:

```go
model := anthropic.New()
response, err := model.Generate(
  context.Background(),
  llm.NewSingleUserMessage("Hello there!"),
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
  LLM:
    DefaultProvider: anthropic
    DefaultModel: claude-3-7-sonnet-20250219

Agents:
  - Name: Research Assistant
    Backstory: You are an enthusiastic and deeply curious researcher.
    Tools:
      - Google.Search
      - Firecrawl.Scrape

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
* `dive chat --provider anthropic --model claude-3-7-sonnet-20250219`: Chat with an agent
* `dive config check /path/to/workflow.yaml`: Validate a Dive configuration

## LLM Providers

Dive provides a unified interface for working with different LLM providers:

* **Anthropic** (Claude Sonnet, Haiku)
* **OpenAI** (GPT-4, o1, o3)
* **Groq** (Llama, DeepSeek, Qwen)

Each provider implementation handles API communication, token counting,
tool calling, and other details.

```go
provider := anthropic.New(anthropic.WithModel("claude-3-7-sonnet-20250219"))

provider := openai.New(openai.WithModel("gpt-4o"))

provider := groq.New(groq.WithModel("deepseek-r1-distill-llama-70b"))
```

### Verified Models

These are the models that have been verified to work in Dive:

| Provider  | Model                           | Tools Supported |
| --------- | ------------------------------- | --------------- |
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

## Tool Use

Tools extend agent capabilities. Dive includes these built-in tools:

* **Google.Search**: Web search using Google Custom Search
* **Firecrawl.Scrape**: Web scraping with content extraction
* **Document.Write**: Write content to files
* **Document.Read**: Read content from files

Creating custom tools is straightforward:

```go
type WeatherTool struct {
    apiKey string
}

func (t *WeatherTool) Definition() *llm.ToolDefinition {
    return &llm.ToolDefinition{
        Name: "GetWeather",
        Description: "Get the current weather for a location",
        Parameters: llm.Schema{
            Type: "object",
            Required: []string{"location"},
            Properties: map[string]*llm.SchemaProperty{
                "location": {
                    Type: "string",
                    Description: "The city and state/country",
                },
            },
        },
    }
}
```

## Contributors

We're looking for contributors! Whether you're fixing bugs, adding features,
improving documentation, or spreading the word, your help is appreciated.

<!-- <a href="https://github.com/diveagents/dive/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=diveagents/dive" width="100%"/>
</a> -->

## Roadmap

- Docs site
- MCP support
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
- Ollama support
- Hugging Face support

## FAQ

### Can I use Dive with Ollama?

Soon!

### Is there a hosted or managed version available?

Not at this time. Dive is provided as an open-source framework that you can
self-host and integrate into your own applications.

### Who is Behind Dive?

Dive is developed by [Stingrai](https://www.getstingrai.com).
