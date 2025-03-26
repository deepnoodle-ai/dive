<p align="center">
  <img src="https://getstingrai-public.s3.us-east-1.amazonaws.com/static/images/dive/dive-logo-2025-02-25-1024.jpg" width="200" height="200">
  <h1 align="center">
    <a href="https://github.com/diveagents/dive">Dive - AI Agent Framework</a>
  </h1>
</p>

## Introduction

Dive is a flexible Go framework for building AI agent systems. Whether you need
a single specialized agent or a complex workflow of AI tasks, Dive makes it
easy to accomplish tasks with AI.

Dive can be embedded into existing Go applications or run standalone using workflow definitions.

## Project Status

**⚠️ Early Development Stage ⚠️**

Dive is in early development. While much core functionality is in place, the
project is still evolving rapidly.

- **Not recommended for production use** at this time
- **Breaking changes will happen** as the API matures
- **Feedback is highly valued** on concepts, APIs, and usability

We welcome your input! Please reach out in
[GitHub Discussions](https://github.com/diveagents/dive/discussions) with
questions, suggestions, or feedback.

## Features

* **Workflow-Based Architecture**: Define complex AI tasks as workflows with multiple steps
* **Flexible Agent System**: Create specialized agents with different roles and capabilities
* **Declarative Configuration**: Define workflows, agents, and tasks using YAML or programmatically in Go
* **Multi-Provider Support**: Unified Go interface for multiple LLM providers (Anthropic, OpenAI, Groq)
* **Tool System**: Extend agent capabilities with tools like web search, document retrieval, and more
* **Streaming Support**: Stream events for chats and task progress in real-time
* **Variable Support**: Pass variables into workflows for dynamic execution

## Quick Start

### Prerequisites

- Go 1.20 or higher
- API keys for any LLM providers you plan to use (Anthropic, OpenAI, Groq)
- API keys for any external tools you plan to use (Google Search, Firecrawl, etc.)

### Environment Setup

Set up your shell environment:

```bash
# LLM Provider API Keys
export ANTHROPIC_API_KEY="your-key"
export OPENAI_API_KEY="your-key"
export GROQ_API_KEY="your-key"

# Tool API Keys
export GOOGLE_SEARCH_API_KEY="your-key"
export GOOGLE_SEARCH_CX="your-key"
export FIRECRAWL_API_KEY="your-key"
```

### As a Library

To get started with Dive as a library, use go get:

```
go get github.com/diveagents/dive
```

Here's a simple example of creating a chat agent:

```go
provider := anthropic.New()
googleClient, _ := google.New()

agent, err := agent.New(agent.Options{
    Name:         "Assistant",
    Backstory:    "You are a helpful assistant.",
    LLM:          provider,
    Tools:        []llm.Tool{toolkit.NewGoogleSearch(googleClient)},
    CacheControl: "ephemeral",
})

if err := agent.Start(ctx); err != nil {
    log.Fatal(err)
}
defer agent.Stop(ctx)

// Start chatting with the agent
iterator, err := agent.Stream(ctx, llm.NewUserMessage("Hello!"))
// Handle the streaming response...
```

### Using Workflows

Dive supports defining complex AI tasks as workflows. Here's an example workflow in YAML:

```yaml
Name: Research
Description: Research a Topic

Config:
  LLM:
    DefaultProvider: anthropic
    DefaultModel: claude-3-7-sonnet-20250219

Agents:
  - Name: Research Analyst
    Description: Research Analyst who specializes in topic research
    Tools:
      - Google.Search
      - Firecrawl.Scrape

Workflows:
  - Name: Research
    Inputs:
      - Name: topic
        Type: string
    Steps:
      - Name: Historical Research
        Agent: Research Analyst
        Prompt:
          Text: "Research the history of: ${inputs.topic}"
          Output: A historical overview
          OutputFormat: Markdown
        Store: historical_research
```

Run a workflow using the simple runner:

```bash
dive run workflow.yaml --vars "topic=history of the internet"
```

### Scripting and Variables

Each workflow execution maintains its own scripting environment with variables that can be read and written. Variables can be used in:

1. **Step Prompts**: Use `${variable_name}` syntax to include variables in prompts
2. **Action Parameters**: Parameters can reference variables using the same syntax
3. **Conditional Logic**: Use variables in edge conditions to control workflow branching

Variables can come from several sources:

- **Workflow Inputs**: Available as `${inputs.name}`
- **Step Outputs**: Use `Store: variable_name` to save a step's output
- **Action Results**: Some actions may store their results in variables

Example of variable usage:

```yaml
Steps:
  - Name: Get Current Time
    Action: Time.Now
    Store: current_time

  - Name: Analyze Files
    Agent: Analyst
    Prompt:
      Text: |
        The current time is: ${current_time}
        
        Respond with the current wall clock time.
```

### Available Actions

Actions are pre-defined operations that can be used in workflow steps. The core actions include:

#### Document.Write
Writes content to a document in the document repository.

Parameters:
- `Path`: Target path for the document
- `Content`: Content to write (supports variable templates)

Example:
```yaml
- Name: Save Report
  Action: Document.Write
  Parameters:
    Path: reports/analysis.md
    Content: ${analysis_result}
```

#### Document.Read
Reads content from a document in the document repository.

Parameters:
- `Path`: Path of the document to read

Example:
```yaml
- Name: Load Previous Report
  Action: Document.Read
  Parameters:
    Path: reports/previous.md
  Store: previous_report
```

Actions can be extended by registering custom implementations in the environment. Each action:
- Has a unique name
- Accepts a set of parameters
- Can read from and write to the execution's variable environment
- May interact with external systems or resources

## LLM Integration

Dive provides a unified interface for working with different LLM providers:

* **Anthropic** (Claude 3)
* **OpenAI** (GPT-4)
* **Groq** (Llama, DeepSeek)

Each provider implementation handles API communication, token counting, and streaming:

```go
provider := anthropic.New(
    anthropic.WithModel("claude-3-7-sonnet-20250219"),
)

provider := openai.New(
    openai.WithModel("gpt-4"),
)

provider := groq.New(
    groq.WithModel("deepseek-r1-distill-llama-70b"),
)
```

### Tested Models

These are the models that have been tested with Dive:

| Provider  | Model                           |
| --------- | ------------------------------- |
| Anthropic | `claude-3-7-sonnet-20250219`    |
| OpenAI    | `gpt-4`                         |
| Groq      | `deepseek-r1-distill-llama-70b` |
| Groq      | `llama-3.3-70b-versatile`       |

### Tool Use

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

## Contributing

We welcome contributions to Dive! Whether you're fixing bugs, adding features,
improving documentation, or spreading the word, your help is appreciated.

At this early stage, we're particularly interested in feedback on the workflow system,
API design, and any use cases you'd like to see supported.

## Roadmap

- Enhanced workflow capabilities
- More built-in tools
- Agent memory systems
- Workflow persistence
- Integration with popular services (Slack, Google Drive, etc.)
- Expanded testing coverage
- CLI improvements

## FAQ

### What makes Dive different from other agent frameworks?

Dive is meant to be a highly practical, batteries-included agent framework.
Key differentiators include:

- Workflow-first approach for complex AI tasks
- Simple but powerful configuration system
- Strong streaming support for real-time updates
- Easy integration with existing Go applications
- Built-in support for popular LLM providers
- Flexible tool system for extending capabilities

### How do I handle LLM rate limits?

Dive includes built-in retry mechanisms for handling rate limits. This includes
exponential backoff and jitter.

### Should I use Dive in production?

No, Dive is not recommended for production use at this time. As mentioned in the
Project Status section, Dive is in its early development stages and breaking
changes will occur as the API matures.

We recommend using it for experimentation, prototyping, and providing feedback
during this early stage. Once the project reaches a more stable state, we'll
provide clear guidance on production readiness.

### How can I extend or customize Dive?

Dive is designed to be highly extensible:

- Create custom tools by implementing the `llm.Tool` interface
- Add support for new LLM providers by implementing the `llm.Provider` interface
- Create custom workflow actions
- Define your own agent behaviors

### Is there a hosted or managed version available?

Not at this time. Dive is provided as an open-source framework that you can
self-host and integrate into your own applications.

## Who is Behind Dive?

Dive is developed by [Stingrai](https://www.getstingrai.com).
