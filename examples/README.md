# Dive YAML Definitions

This directory contains example YAML definitions for Dive agent teams. These 
files demonstrate how to define agents, tasks, and teams in a declarative way.

## Running a YAML Definition

To run a YAML definition, use the `yaml_runner` command:

```bash
go run cmd/yaml_runner/main.go -file=examples/research_team.yaml
```

Options:
- `-file`: Path to the YAML definition file (required)
- `-verbose`: Enable verbose output to see task results in the console
- `-output`: Directory to save task results (default: "output")
- `-timeout`: Timeout for the entire operation (default: "30m")

## YAML Structure

A Dive YAML definition consists of the following sections:

### Top-Level Fields

- `name`: Name of the team
- `description`: Description of the team
- `config`: Global configuration settings
- `agents`: List of agent definitions
- `tasks`: List of task definitions

### Config Section

- `default_provider`: Default LLM provider (anthropic, openai, groq)
- `default_model`: Default model to use
- `log_level`: Logging level (debug, info, warn, error)
- `cache_control`: Cache control setting (ephemeral, persistent, none)
- `enabled_tools`: List of tools to enable
- `provider_configs`: Provider-specific configuration

### Agent Definition

- `name`: Name of the agent
- `role`: Role definition
  - `description`: Description of the agent's role
  - `is_supervisor`: Whether the agent is a supervisor
  - `subordinates`: List of subordinate agent names
  - `accepts_chats`: Whether the agent accepts chat messages
  - `accepts_events`: List of event types the agent accepts
  - `accepts_work`: List of work types the agent accepts
- `provider`: LLM provider to use (overrides default)
- `model`: Model to use (overrides default)
- `tools`: List of tools the agent can use
- `cache_control`: Cache control setting (overrides default)
- `max_active_tasks`: Maximum number of active tasks
- `task_timeout`: Timeout for tasks (e.g., "5m")
- `chat_timeout`: Timeout for chat messages (e.g., "1m")
- `config`: Agent-specific configuration

### Task Definition

- `name`: Name of the task
- `description`: Description of the task
- `expected_output`: Description of the expected output
- `output_format`: Format of the output (text, json, etc.)
- `assigned_agent`: Name of the agent assigned to the task
- `dependencies`: List of task names this task depends on
- `max_iterations`: Maximum number of iterations
- `output_file`: File to save the task output to
- `timeout`: Timeout for the task (e.g., "5m")
- `context`: Additional context for the task
- `kind`: Kind of task

## Environment Variables

The following environment variables are used by various tools:

- `GOOGLE_SEARCH_CX`: Required for Google Search tool
- `FIRECRAWL_API_KEY`: Required for Firecrawl tool

## Example

See `research_team.yaml` for a complete example of a team definition. 

## Examples

### YAML Examples

- `research_team.yaml`: A team of researchers led by a supervisor, with a research assistant.

### HCL Examples

- `research_team.hcl`: A team of researchers with variable interpolation and function usage.

## YAML Configuration Format

## HCL Configuration Format

HCL (HashiCorp Configuration Language) provides a more flexible way to define teams, agents, and tasks with support for variables and functions.

### Top-Level Structure

```hcl
// Define variables
variable "team_name" {
  type = "string"
  description = "The name of the research team"
  default {
    value = "Elite Research Team"
  }
}

// Team definition
name "research_team" {
  description = format("A team of researchers focused on %s.", var.research_topic)
}

// Global configuration
config {
  default_provider = "anthropic"
  log_level = "info"
  enabled_tools = ["google_search"]
}

// Define agents
agent "Supervisor" {
  // Agent configuration
}

// Define tasks
task "Research" {
  // Task configuration
}
```

### Variables

Variables allow you to parameterize your configuration:

```hcl
variable "research_topic" {
  type = "string"
  description = "The topic to research"
  default {
    value = "artificial intelligence"
  }
}
```

You can reference variables using the `var.` prefix:

```hcl
description = format("Research the history of %s", var.research_topic)
```

### Functions

HCL supports several built-in functions:

- `format(format_string, args...)`: Format a string using printf-style formatting
- `env(name)`: Get the value of an environment variable
- `concat(list1, list2, ...)`: Concatenate multiple lists
- `replace(string, search, replace)`: Replace all occurrences of a substring

Example:

```hcl
output_file = format("%s_summary.txt", replace(var.research_topic, " ", "_"))
```

### Agent Configuration

```hcl
agent "Supervisor" {
  role {
    description = "Research Team Lead"
    is_supervisor = true
    subordinates = ["Researcher"]
  }
  provider = "anthropic"
  cache_control = "ephemeral"
  max_active_tasks = 1
  task_timeout = "10m"
  chat_timeout = "2m"
}
```

### Task Configuration

```hcl
task "Research" {
  description = format("Research the history of %s", var.research_topic)
  assigned_agent = "Researcher"
  timeout = "5m"
  output_file = format("%s_research.txt", replace(var.research_topic, " ", "_"))
}
```

## Running Examples

### YAML Examples

```bash
go run cmd/yaml_runner/main.go -file=examples/research_team.yaml -verbose
```

### HCL Examples

```bash
go run cmd/hcl_runner/main.go -file=examples/research_team.hcl -verbose
```

You can override variables from the command line:

```bash
go run cmd/hcl_runner/main.go -file=examples/research_team.hcl -vars="research_topic=quantum computing"
``` 
