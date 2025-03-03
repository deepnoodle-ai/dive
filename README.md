<p align="center">
  <img src="https://getstingrai-public.s3.us-east-1.amazonaws.com/static/images/dive/dive-logo-2025-02-25-1024.jpg" width="200" height="200">
  <h1 align="center">
    <a href="https://github.com/getstingrai/dive">Dive - AI Agent Framework</a>
  </h1>
</p>

## Introduction

Dive is a flexible Go framework for building AI agent systems. Whether you need
a single specialized agent or a collaborative team of AI workers, Dive makes it
easy to accomplish tasks with AI.

Dive can be embedded into existing Go applications or run standalone.

## Project Status

**⚠️ Early Development Stage ⚠️**

Dive is in early development. While much core functionality is in place, the
project is still evolving rapidly.

- **Not recommended for production use** at this time
- **Breaking changes will happen** as the API matures
- **Feedback is highly valued** on concepts, APIs, and usability

We welcome your input! Please reach out in
[GitHub Discussions](https://github.com/getstingrai/dive/discussions) with
questions, suggestions, or feedback.

## Features

* **Flexible Agent Architecture**: Create specialized agents with different roles and capabilities
* **Team Collaboration**: Agents work together and supervisors assign tasks
* **Task Management**: Define, assign, and coordinate tasks with dependencies and expected outputs
* **Multi-Provider Support**: Unified Go interface for multiple LLM providers (Anthropic, OpenAI, Groq)
* **Tool System**: Extend agent capabilities with tools like web search, document retrieval, and more
* **Declarative Configuration**: Define teams using YAML, JSON, HCL, or programmatically in Go
* **Streaming Support**: Stream events for chats and task progress in real-time

## Quick Start

### Prerequisites

- Go 1.20 or higher
- API keys for any LLM providers you plan to use (Anthropic, OpenAI, Groq, etc.)
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
go get github.com/getstingrai/dive
```

Then mimic one of the example programs to get up and running:

- [Chat Example](examples/chat_example/main.go)
- [Tasks Example](examples/tasks_example/main.go)
- [Team Example](examples/team_example/main.go)

### As a CLI

Or use the YAML runner for a declarative approach:

```bash
git clone https://github.com/getstingrai/dive
cd dive/cmd/dive
go build
./dive run ../../examples/research_team.hcl --var "topic=history of the internet"
```

## Core Concepts

### Agents

Agents can work independently or as part of a team, and they can use tools to
interact with the outside world. They may be assigned tasks.

Dive's agents each run independently, with their own goroutine and run loop.
This means they can truly operate and make progress independently. They can
be spawned for a specific task and then stopped, or they can be run continuously
and make progress in the background.

Create an agent with the following code:

```go
agent := dive.NewAgent(dive.AgentOptions{
    Name:         "Chris",
    Description:  "Research Assistant",
    Instructions: "Use Google to research assigned topics",
    LLM:          anthropic.New(),
    IsSupervisor: false,
    Tools:        []llm.Tool{tools.NewGoogleSearch(googleClient)},
})
```

As an implementation detail, message passing is used behind the Agent interface
functions. Each agent has its own "mailbox" that accepts messages that tell the
agent to chat or work on a task. This borrows aspects of the actor model of
concurrency.

### Tasks

Tasks are the basic units of work in Dive. Each task describes the needed work
along with expected outputs and other configuration. Tasks may have dependencies
on other tasks, and Dive will automatically determine the order in which to
execute them. Currently, a single agent works on one task at a time.

Create a task as follows:

```go
task := dive.NewTask(dive.TaskOptions{
    Name:           "Research",
    Description:    "Research the current market trends for electric vehicles",
    ExpectedOutput: "A 500-word summary with 3 key insights",
    OutputFormat:   dive.OutputMarkdown,
    Timeout:        time.Minute * 5,
})
```

### Teams

Teams allow multiple agents to collaborate on tasks, where each agent has its
own role and capabilities.

Agents can be marked as supervisors, which allows them to assign tasks to other
agents. A list of subordinates may be specified for each supervisor.

Create a simple two-agent team as follows:

```go
team, err := dive.NewTeam(dive.TeamOptions{
    Name:        "Research Team",
    Description: "A team of researchers led by a supervisor.",
    Agents:      []dive.Agent{supervisor, researcher},
})
```

### Events

**Disclaimer:** This is not fully implemented yet.

Events provide a way to notify agents about something that happened in the world.
They can be used to trigger agent actions, provide new information, or coordinate
activities between agents.

Create and pass an event to an agent as follows:

```go
event := &dive.Event{
    Name:        "new_data_available",
    Description: "New market data is available for analysis",
    Parameters: map[string]any{
        "data_source": "quarterly_reports",
        "timestamp":   time.Now(),
    },
}
err := agent.HandleEvent(ctx, event)
```

## CLI

The Dive CLI lets you run teams defined in YAML, JSON, or HCL. It also provides
other subcommands for validating configurations and chatting 1:1 with an agent.

Currently available commands:

```bash
# Run a team and any defined tasks
dive run path/to/team.hcl

# Chat with an agent on a team
dive chat path/to/team.hcl

# Validate a team configuration
dive config check path/to/team.hcl
```

### Team Configurations

Team configurations defined in HCL are the most useful and flexible currently.
This supports referencing tasks and agents, basic string formatting, and dynamic
configuration of variables.

```hcl
name = "Research Team"

description = "A expert research team of agents that will research any topic"

config {
  log_level = "debug"
  default_provider = "anthropic"
  output_dir = "output"
}

variable "topic" {
  type = "string"
  description = "The topic to research"
}

tool "google_search" {
  enabled = true
}

tool "firecrawl_scrape" {
  enabled = true
}

agent "supervisor" {
  description = "Expert research supervisor. Assign research tasks to the assistant. Prepare the final reports yourself."
  is_supervisor = true
  subordinates = [agents.assistant]
}

agent "assistant" {
  description = "You are an expert research assistant. When researching, don't go too deep into the details unless specifically asked."
  tools = [
    tools.google_search,
    tools.firecrawl_scrape,
  ]
}

task "research" {
  description = format("Gather background research on %s. Don't consult more than one source. The goal is to produce about 3 paragraphs of research - that is all. Don't overdo it.", var.topic)
  output_file = "research.txt"
}

task "report" {
  description = format("Create a brief 3 paragraph report on %s", var.topic)
  expected_output = "The history, with the first word of each paragraph in ALL UPPERCASE"
  assigned_agent = agents.supervisor
  dependencies = [tasks.research]
  output_file = "report.txt"
}
```

## LLM Integration

Dive provides a unified interface for working with different
Large Language Model (LLM) providers. This abstraction allows you to switch
between providers or models without changing your application code.

The framework currently supports:

* **Anthropic**
* **OpenAI**
* **Groq**

Each provider implementation handles the specifics of API communication, token
counting, error handling, and other provider-specific details, giving you a
consistent interface regardless of which provider you're using.

All providers support tool-use and streaming and non-streaming responses.
Provider specific features are supported when possible. For example, with
Anthropic you must opt into caching.

If you want, you can use these LLMs directly, without agents:

```go
// Create an LLM instance
provider := anthropic.New(anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))

// Generate a response (non-streaming API)
response, err := provider.Generate(ctx, 
    []*llm.Message{llm.NewUserMessage("What is the capital of France?")},
    llm.WithSystemPrompt("You are a helpful assistant."),
)
```

Each provider has its own package but offers similar configuration options:

```go
llm := anthropic.New(
    anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
    anthropic.WithModel("claude-3-7-sonnet-20250219"),
)

llm := openai.New(
    openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
    openai.WithModel("gpt-4o"),
)

llm := groq.New(
    groq.WithAPIKey(os.Getenv("GROQ_API_KEY")),
    groq.WithModel("llama-3.3-70b-versatile"),
)
```

If unspecified, the API keys will be read from the standard environment
variables. Each provider defines its own default model as well.

### Tested Models

These are the models that I have personally tested so far. Currently, Dive does
not restrict the model names you can specify, so feel free to try others.

| Provider  | Model                           |
| --------- | ------------------------------- |
| Anthropic | `claude-3-7-sonnet-20250219`    |
| Anthropic | `claude-3-5-sonnet-20241022`    |
| Anthropic | `claude-3-5-haiku-20241022`     |
| OpenAI    | `gpt-4o`                        |
| Groq      | `llama-3.3-70b-versatile`       |
| Groq      | `deepseek-r1-distill-llama-70b` |

### Tool Use

Tools extend the capabilities of agents by allowing them to perform actions
beyond just generating text. Tools can access external APIs, search the web,
retrieve data, perform calculations, and more.

Dive provides a simple interface for defining and using tools:

```go
type Tool interface {
	Definition() *ToolDefinition
	Call(ctx context.Context, input string) (string, error)
	ShouldReturnResult() bool
}
```

The framework includes a handful of built-in tools at this time:

* **google_search**: Searches with Google and returns URL, description, and title
* **firecrawl_scrape**: Scrapes a web page and returns the content as Markdown
* **directory_list**: Lists the contents of a local directory
* **file_read**: Reads the contents of a file and returns the content
* **file_write**: Writes content to a file

Creating a custom tool is straightforward:

```go
type WeatherTool struct {
    apiKey string
}

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{apiKey: apiKey}
}

func (t *WeatherTool) Definition() *llm.ToolDefinition {
    return &llm.ToolDefinition{
        Name:        "GetWeather",
        Description: "Get the current weather for a location",
        Parameters: llm.Schema{
            Type:     "object",
            Required: []string{"location"},
            Properties: map[string]*llm.SchemaProperty{
                "location": {
                    Type:        "string",
                    Description: "The city and state/country",
                },
            },
        },
    }
}

func (t *WeatherTool) Call(ctx context.Context, input string) (string, error) {
    // Parse input, call weather API, and return results
    // ...
}

func (t *WeatherTool) ShouldReturnResult() bool {
	return true
}
```

## Configuration

Dive provides flexible configuration options for agents, teams, and tasks. You
can configure these components either programmatically in Go or declaratively
using JSON, YAML, or HCL.

## Contributing

We welcome contributions to Dive! Whether you're fixing bugs, adding features,
improving documentation, or spreading the word, your help is appreciated.

At this early stage, we're particularly interested in feedback on the concepts,
API design, usability, and any use cases you'd like to see supported.

## Roadmap

- Voice interactions
- More memory systems
- Defined interfaces for RAG
- Agent state persistence
- Tool use: Slack
- Tool use: Google Drive
- Tool use: Expanded set of file I/O tools
- More tests
- More CLI subcommands
- Loads more...

## FAQ

### What makes Dive different from other agent frameworks?

Dive is meant to be a highly practical, batteries-included agent framework.
Key differentiators include:

- Simple concepts: teams, agents, and tasks
- Easy definition of tasks and dependencies
- Scalable to thousands of concurrent agents
- Independent execution of agents and teams
- Easily embeddable into existing Go programs
- Interfaces in place for customization and extension
- Emphasis on streaming support and real-time updates
- Built-in support for long-running tasks with flexible completion criteria
- Configuration via HCL provides path to flexible and robust team configurations

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
- Implement custom agents by implementing the `dive.Agent` interface
- Implement custom teams by implementing the `dive.Team` interface

### Is there a hosted or managed version available?

Not at this time. Dive is provided as an open-source framework that you can
self-host and integrate into your own applications.

## Who is Behind Dive?

Dive is developed by [Stingrai](https://www.getstingrai.com), a company building
products powered by agentic AI for competitive intelligence and product messaging.
