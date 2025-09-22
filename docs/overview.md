# Dive Overview

Dive is a powerful AI toolkit for Go that enables developers to create intelligent agents, automate complex workflows, and seamlessly integrate with leading LLM providers. Whether you're building autonomous assistants, orchestrating data pipelines, or prototyping AI-native applications, Dive provides the foundation you need.

## 🎯 Core Philosophy

Dive is built around four key principles:

1. **Simplicity** - Clean APIs and intuitive concepts that developers can quickly understand
2. **Flexibility** - Modular architecture that adapts to your specific use cases
3. **Reliability** - Checkpoint-based execution and robust error handling
4. **Interoperability** - Works with multiple LLM providers and external systems

## 🧱 Key Components

### Agents

Intelligent AI entities that can:

- Respond to natural language conversations
- Execute tasks autonomously using tools
- Delegate work to other agents (supervisor pattern)
- Maintain persistent conversation history

```go
agent, err := agent.New(agent.Options{
    Name:         "Research Assistant",
    Instructions: "You are an expert researcher who helps analyze topics thoroughly.",
    Model:        anthropic.New(),
    Tools:        []dive.Tool{webSearchTool, documentTool},
})
```

### Tools

Extensible capabilities that agents can use:

- Built-in: web search, file operations, command execution
- Custom: implement the `Tool` interface for domain-specific needs
- MCP: connect to Model Context Protocol servers
- Type-safe with rich annotations

## 🔄 How It Works

### 1. Agent Interaction

```
User Input → Agent → LLM Provider → Tool Calls → Results → Response
```

Agents receive input, process it through their configured LLM, potentially make tool calls to gather information or take actions, and return structured responses.

### 2. Event Flow

```
Execution Events → Streaming → Real-time Updates → UI/Monitoring
```

Agents emit detailed events that can be streamed to UIs, monitoring systems, or other downstream processes.

## 🌟 Key Features

### Multi-LLM Support

- **Anthropic** (Claude Sonnet, Haiku, Opus)
- **OpenAI** (GPT-4, o1, o3)
- **Groq** (Llama, DeepSeek, Qwen)
- **Ollama** (Local models)

### Advanced Capabilities

- **Tool Calling** - Agents can interact with external systems
- **Streaming Responses** - Real-time output for better UX
- **Thread Persistence** - Conversations that span multiple interactions
- **Supervisor Patterns** - Hierarchical agent systems
- **MCP Integration** - Connect to external tool ecosystems

### Developer Experience

- **Type Safety** - Leverages Go's type system for reliability
- **Rich Configuration** - YAML-based declarative setup
- **CLI Tools** - Command-line interface for testing and deployment
- **Event Streaming** - Observable execution with detailed events
- **Extensive Examples** - Learn from real-world use cases

## 🏗️ Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   CLI/App   │────│ Environment  │────│   Agents    │
└─────────────┘    └──────────────┘    └─────────────┘
                           │                    │
                   ┌───────────────┐    ┌─────────────┐
                   │   Workflows   │    │    LLMs     │
                   └───────────────┘    └─────────────┘
                           │                    │
                   ┌───────────────┐    ┌─────────────┐
                   │    Actions    │    │    Tools    │
                   └───────────────┘    └─────────────┘
```

## 🚀 Getting Started

1. **[Install Dive](guides/installation.md)** - Set up your development environment
2. **[Quick Start](guides/quick-start.md)** - Build your first agent in 5 minutes
3. **[Examples](examples/)** - Explore real-world use cases
4. **[Guides](guides/)** - Deep dive into specific topics

## 📈 Use Cases

### Research & Analysis

- Automated research pipelines
- Document analysis and summarization
- Market intelligence gathering
- Competitive analysis

### Development & Operations

- Code review and analysis
- Documentation generation
- System monitoring and alerts
- Deployment automation

### Content & Communication

- Content creation workflows
- Customer support automation
- Email and message processing
- Social media management

### Data Processing

- ETL pipeline orchestration
- Data validation and cleaning
- Report generation
- Analytics automation

## 🎯 Next Steps

Ready to start building with Dive? Here are some recommended paths:

- **Developers**: Start with [Agent Guide](guides/agents.md) to understand core concepts
- **DevOps**: Explore [CLI Reference](reference/cli.md) for automation tools
- **Researchers**: Explore [MCP Integration](guides/mcp-integration.md) for external tool access
- **Integrators**: Review [MCP Integration](guides/mcp-integration.md) for external connections

Need help? Join our [Discord community](https://discord.gg/yrcuURWk) or check [GitHub Discussions](https://github.com/deepnoodle-ai/dive/discussions).
