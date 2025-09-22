# Config API Reference

Complete API reference for the Dive config package, covering configuration management, YAML/JSON parsing, and environment building.

## 📋 Table of Contents

- [Configuration Types](#configuration-types)
- [Loading Configuration](#loading-configuration)
- [Environment Building](#environment-building)
- [Agent Configuration](#agent-configuration)
- [Workflow Configuration](#workflow-configuration)
- [MCP Server Configuration](#mcp-server-configuration)
- [Provider Configuration](#provider-configuration)
- [Tool Configuration](#tool-configuration)
- [Context Building](#context-building)
- [Examples](#examples)

## Configuration Types

### Main Configuration Structure

```go
// Top-level configuration structure
type Config struct {
    DefaultProvider  string     `yaml:"DefaultProvider,omitempty" json:"DefaultProvider,omitempty"`
    DefaultModel     string     `yaml:"DefaultModel,omitempty" json:"DefaultModel,omitempty"`
    DefaultWorkflow  string     `yaml:"DefaultWorkflow,omitempty" json:"DefaultWorkflow,omitempty"`
    ConfirmationMode string     `yaml:"ConfirmationMode,omitempty" json:"ConfirmationMode,omitempty"`
    LogLevel         string     `yaml:"LogLevel,omitempty" json:"LogLevel,omitempty"`
    Providers        []Provider `yaml:"Providers,omitempty" json:"Providers,omitempty"`
}
```

### Provider Configuration

```go
type Provider struct {
    Name           string            `yaml:"Name" json:"Name"`
    Caching        *bool             `yaml:"Caching,omitempty" json:"Caching,omitempty"`
    Features       []string          `yaml:"Features,omitempty" json:"Features,omitempty"`
    RequestHeaders map[string]string `yaml:"RequestHeaders,omitempty" json:"RequestHeaders,omitempty"`
}
```

## Agent Configuration

### Agent Structure

```go
type Agent struct {
    Name               string         `yaml:"Name,omitempty" json:"Name,omitempty"`
    Goal               string         `yaml:"Goal,omitempty" json:"Goal,omitempty"`
    Instructions       string         `yaml:"Instructions,omitempty" json:"Instructions,omitempty"`
    IsSupervisor       bool           `yaml:"IsSupervisor,omitempty" json:"IsSupervisor,omitempty"`
    Subordinates       []string       `yaml:"Subordinates,omitempty" json:"Subordinates,omitempty"`
    Provider           string         `yaml:"Provider,omitempty" json:"Provider,omitempty"`
    Model              string         `yaml:"Model,omitempty" json:"Model,omitempty"`
    Tools              []string       `yaml:"Tools,omitempty" json:"Tools,omitempty"`
    ResponseTimeout    any            `yaml:"ResponseTimeout,omitempty" json:"ResponseTimeout,omitempty"`
    ToolConfig         map[string]any `yaml:"ToolConfig,omitempty" json:"ToolConfig,omitempty"`
    ToolIterationLimit int            `yaml:"ToolIterationLimit,omitempty" json:"ToolIterationLimit,omitempty"`
    DateAwareness      *bool          `yaml:"DateAwareness,omitempty" json:"DateAwareness,omitempty"`
    SystemPrompt       string         `yaml:"SystemPrompt,omitempty" json:"SystemPrompt,omitempty"`
    ModelSettings      *ModelSettings `yaml:"ModelSettings,omitempty" json:"ModelSettings,omitempty"`
    Context            []Content      `yaml:"Context,omitempty" json:"Context,omitempty"`
}
```

### Model Settings

```go
type ModelSettings struct {
    Temperature       *float64            `yaml:"Temperature,omitempty" json:"Temperature,omitempty"`
    PresencePenalty   *float64            `yaml:"PresencePenalty,omitempty" json:"PresencePenalty,omitempty"`
    FrequencyPenalty  *float64            `yaml:"FrequencyPenalty,omitempty" json:"FrequencyPenalty,omitempty"`
    ReasoningBudget   *int                `yaml:"ReasoningBudget,omitempty" json:"ReasoningBudget,omitempty"`
    ReasoningEffort   llm.ReasoningEffort `yaml:"ReasoningEffort,omitempty" json:"ReasoningEffort,omitempty"`
    MaxTokens         *int                `yaml:"MaxTokens,omitempty" json:"MaxTokens,omitempty"`
    ToolChoice        *llm.ToolChoice     `yaml:"ToolChoice,omitempty" json:"ToolChoice,omitempty"`
    ParallelToolCalls *bool               `yaml:"ParallelToolCalls,omitempty" json:"ParallelToolCalls,omitempty"`
    Features          []string            `yaml:"Features,omitempty" json:"Features,omitempty"`
    RequestHeaders    map[string]string   `yaml:"RequestHeaders,omitempty" json:"RequestHeaders,omitempty"`
    Caching           *bool               `yaml:"Caching,omitempty" json:"Caching,omitempty"`
}
```

## Workflow Configuration

### Workflow Structure

```go
type Workflow struct {
    Name        string    `yaml:"Name,omitempty" json:"Name,omitempty"`
    Description string    `yaml:"Description,omitempty" json:"Description,omitempty"`
    Inputs      []Input   `yaml:"Inputs,omitempty" json:"Inputs,omitempty"`
    Output      *Output   `yaml:"Output,omitempty" json:"Output,omitempty"`
    Triggers    []Trigger `yaml:"Triggers,omitempty" json:"Triggers,omitempty"`
    Steps       []Step    `yaml:"Steps,omitempty" json:"Steps,omitempty"`
    Path        string    `yaml:"-" json:"-"`
}
```

### Input/Output Configuration

```go
type Input struct {
    Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
    Type        string `yaml:"Type,omitempty" json:"Type,omitempty"`
    Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
    Required    bool   `yaml:"Required,omitempty" json:"Required,omitempty"`
    Default     any    `yaml:"Default,omitempty" json:"Default,omitempty"`
    As          string `yaml:"As,omitempty" json:"As,omitempty"`
}

type Output struct {
    Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
    Type        string `yaml:"Type,omitempty" json:"Type,omitempty"`
    Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
    Format      string `yaml:"Format,omitempty" json:"Format,omitempty"`
    Default     any    `yaml:"Default,omitempty" json:"Default,omitempty"`
    Document    string `yaml:"Document,omitempty" json:"Document,omitempty"`
}
```

### Step Configuration

```go
type Step struct {
    Type       string         `yaml:"Type,omitempty" json:"Type,omitempty"`
    Name       string         `yaml:"Name,omitempty" json:"Name,omitempty"`
    Agent      string         `yaml:"Agent,omitempty" json:"Agent,omitempty"`
    Prompt     string         `yaml:"Prompt,omitempty" json:"Prompt,omitempty"`
    Script     string         `yaml:"Script,omitempty" json:"Script,omitempty"`
    Store      string         `yaml:"Store,omitempty" json:"Store,omitempty"`
    Action     string         `yaml:"Action,omitempty" json:"Action,omitempty"`
    Parameters map[string]any `yaml:"Parameters,omitempty" json:"Parameters,omitempty"`
    Each       *EachBlock     `yaml:"Each,omitempty" json:"Each,omitempty"`
    Next       []NextStep     `yaml:"Next,omitempty" json:"Next,omitempty"`
    Seconds    float64        `yaml:"Seconds,omitempty" json:"Seconds,omitempty"`
    End        bool           `yaml:"End,omitempty" json:"End,omitempty"`
    Content    []Content      `yaml:"Content,omitempty" json:"Content,omitempty"`
}

type EachBlock struct {
    Items any    `yaml:"Items,omitempty" json:"Items,omitempty"`
    As    string `yaml:"As,omitempty" json:"As,omitempty"`
}

type NextStep struct {
    Step      string `yaml:"Step,omitempty" json:"Step,omitempty"`
    Condition string `yaml:"Condition,omitempty" json:"Condition,omitempty"`
}
```

### Trigger Configuration

```go
type Trigger struct {
    Name   string                 `yaml:"Name,omitempty" json:"Name,omitempty"`
    Type   string                 `yaml:"Type,omitempty" json:"Type,omitempty"`
    Config map[string]interface{} `yaml:"Config,omitempty" json:"Config,omitempty"`
}
```

## MCP Server Configuration

### MCP Server Structure

```go
type MCPServer struct {
    Type               string                `yaml:"Type" json:"Type"`
    Name               string                `yaml:"Name" json:"Name"`
    Command            string                `yaml:"Command,omitempty" json:"Command,omitempty"`
    URL                string                `yaml:"URL,omitempty" json:"URL,omitempty"`
    Env                map[string]string     `yaml:"Env,omitempty" json:"Env,omitempty"`
    Args               []string              `yaml:"Args,omitempty" json:"Args,omitempty"`
    AuthorizationToken string                `yaml:"AuthorizationToken,omitempty" json:"AuthorizationToken,omitempty"`
    OAuth              *MCPOAuthConfig       `yaml:"OAuth,omitempty" json:"OAuth,omitempty"`
    ToolConfiguration  *MCPToolConfiguration `yaml:"ToolConfiguration,omitempty" json:"ToolConfiguration,omitempty"`
    Headers            map[string]string     `yaml:"Headers,omitempty" json:"Headers,omitempty"`
}
```

### OAuth Configuration

```go
type MCPOAuthConfig struct {
    ClientID     string            `yaml:"ClientID" json:"ClientID"`
    ClientSecret string            `yaml:"ClientSecret,omitempty" json:"ClientSecret,omitempty"`
    RedirectURI  string            `yaml:"RedirectURI" json:"RedirectURI"`
    Scopes       []string          `yaml:"Scopes,omitempty" json:"Scopes,omitempty"`
    PKCEEnabled  *bool             `yaml:"PKCEEnabled,omitempty" json:"PKCEEnabled,omitempty"`
    TokenStore   *MCPTokenStore    `yaml:"TokenStore,omitempty" json:"TokenStore,omitempty"`
    ExtraParams  map[string]string `yaml:"ExtraParams,omitempty" json:"ExtraParams,omitempty"`
}

type MCPTokenStore struct {
    Type string `yaml:"Type" json:"Type"`                     // "memory", "file", "keychain"
    Path string `yaml:"Path,omitempty" json:"Path,omitempty"` // For file storage
}
```

### Tool Configuration

```go
type MCPToolConfiguration struct {
    Enabled        *bool                  `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
    AllowedTools   []string               `yaml:"AllowedTools" json:"AllowedTools"`
    ApprovalMode   string                 `yaml:"ApprovalMode,omitempty" json:"ApprovalMode,omitempty"`
    ApprovalFilter *MCPToolApprovalFilter `yaml:"ApprovalFilter,omitempty" json:"ApprovalFilter,omitempty"`
}

type MCPToolApprovalFilter struct {
    Always []string `yaml:"Always,omitempty" json:"Always,omitempty"`
    Never  []string `yaml:"Never,omitempty" json:"Never,omitempty"`
}
```

### MCP Server Methods

```go
// ToLLMConfig converts config.MCPServer to llm.MCPServerConfig
func (s MCPServer) ToLLMConfig() *llm.MCPServerConfig

// ToMCPConfig converts config.MCPServer to mcp.ServerConfig
func (s MCPServer) ToMCPConfig() *mcp.ServerConfig

// IsOAuthEnabled returns true if OAuth is configured for this server
func (s MCPServer) IsOAuthEnabled() bool
```

## Tool Configuration

### Tool Structure

```go
type Tool struct {
    Name       string         `yaml:"Name,omitempty" json:"Name,omitempty"`
    Enabled    *bool          `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
    Parameters map[string]any `yaml:"Parameters,omitempty" json:"Parameters,omitempty"`
}
```

### Variable Configuration

```go
type Variable struct {
    Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
    Type        string `yaml:"Type,omitempty" json:"Type,omitempty"`
    Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
    Default     string `yaml:"Default,omitempty" json:"Default,omitempty"`
}
```

## Content Configuration

### Content Structure

```go
type Content struct {
    Text        string `yaml:"Text,omitempty" json:"Text,omitempty"`
    Path        string `yaml:"Path,omitempty" json:"Path,omitempty"`
    URL         string `yaml:"URL,omitempty" json:"URL,omitempty"`
    Document    string `yaml:"Document,omitempty" json:"Document,omitempty"`
    Dynamic     string `yaml:"Dynamic,omitempty" json:"Dynamic,omitempty"`
    DynamicFrom string `yaml:"DynamicFrom,omitempty" json:"DynamicFrom,omitempty"`
}
```

## Document Configuration

### Document Structure

```go
type Document struct {
    ID          string `yaml:"ID,omitempty" json:"ID,omitempty"`
    Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
    Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
    Path        string `yaml:"Path,omitempty" json:"Path,omitempty"`
    Content     string `yaml:"Content,omitempty" json:"Content,omitempty"`
    ContentType string `yaml:"ContentType,omitempty" json:"ContentType,omitempty"`
}
```

### Schedule Configuration

```go
type Schedule struct {
    Name     string `yaml:"Name,omitempty" json:"Name,omitempty"`
    Cron     string `yaml:"Cron,omitempty" json:"Cron,omitempty"`
    Workflow string `yaml:"Workflow,omitempty" json:"Workflow,omitempty"`
    Enabled  *bool  `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
}
```

## Loading Configuration

### Loading Functions

```go
package config

// Load configuration from file
func LoadFromFile(path string) (*Config, error)

// Load configuration from directory
func LoadFromDirectory(dir string) (*Config, error)

// Load configuration from YAML bytes
func LoadFromYAML(data []byte) (*Config, error)

// Load configuration from JSON bytes
func LoadFromJSON(data []byte) (*Config, error)

// Parse configuration from reader
func Parse(reader io.Reader) (*Config, error)
```

### File Discovery

Configuration files are discovered in this order:

1. File specified with `--config` flag
2. `dive.yaml` in current directory
3. `dive.yml` in current directory
4. `.dive/config.yaml` in current directory
5. `.dive/config.yml` in current directory
6. Home directory: `~/.dive/config.yaml`

### Environment Variables

Configuration values can reference environment variables:

```yaml
Config:
  DefaultProvider: anthropic
  
Agents:
  - Name: Assistant
    Provider: ${DIVE_PROVIDER:-anthropic}
    Model: ${DIVE_MODEL:-claude-sonnet-4-20250514}

MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    Headers:
      Authorization: Bearer ${GITHUB_TOKEN}
```

## Environment Building

### Build Functions

```go
package config

// BuildEnvironment creates an environment from configuration
func BuildEnvironment(cfg *Config) (*environment.Environment, error)

// BuildAgent creates an agent from configuration
func BuildAgent(cfg *Agent) (*agent.Agent, error)

// BuildWorkflow creates a workflow from configuration
func BuildWorkflow(cfg *Workflow) (*workflow.Workflow, error)

// BuildMCPServer creates an MCP server from configuration
func BuildMCPServer(cfg *MCPServer) (*mcp.ServerConfig, error)
```

### Build Options

```go
type BuildOptions struct {
    // Override default providers
    Providers map[string]llm.LLM
    
    // Override default tools
    Tools map[string]dive.Tool
    
    // Custom repositories
    DocumentRepository dive.DocumentRepository
    ThreadRepository   dive.ThreadRepository
    
    // Custom confirmer
    Confirmer dive.Confirmer
    
    // Custom logger
    Logger slogger.Logger
    
    // Auto-start environment
    AutoStart bool
}

func BuildEnvironmentWithOptions(cfg *Config, opts BuildOptions) (*environment.Environment, error)
```

## Context Building

### Context Builder

```go
type ContextBuilder struct {
    documents map[string]*Document
    logger    slogger.Logger
}

func NewContextBuilder(documents map[string]*Document, logger slogger.Logger) *ContextBuilder

// BuildContext builds LLM context from configuration
func (cb *ContextBuilder) BuildContext(ctx context.Context, contents []Content) ([]llm.Content, error)
```

### Content Types

```go
// Text content
content := Content{
    Text: "This is direct text content",
}

// File content
content := Content{
    Path: "/path/to/file.txt",
}

// URL content
content := Content{
    URL: "https://example.com/content",
}

// Document reference
content := Content{
    Document: "document_id",
}

// Dynamic content
content := Content{
    Dynamic: "Get current weather for ${location}",
}

// Dynamic from step result
content := Content{
    DynamicFrom: "previous_step_result",
}
```

## Examples

### Basic Configuration

```yaml
# dive.yaml
Name: Development Environment
Description: Local development setup

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info

Agents:
  - Name: Assistant
    Instructions: You are a helpful AI assistant.
    Tools:
      - web_search
      - read_file
      - write_file

Workflows:
  - Name: Simple Chat
    Inputs:
      - Name: message
        Type: string
        Required: true
    Steps:
      - Name: Process Message
        Agent: Assistant
        Prompt: "Respond to: ${inputs.message}"
        End: true
```

### Advanced Configuration

```yaml
Name: Production Environment
Description: Full-featured production setup

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info
  ConfirmationMode: auto
  
  Providers:
    - Name: anthropic
      Caching: true
      Features:
        - prompt_caching
        - computer_use
      RequestHeaders:
        User-Agent: MyApp/1.0
    - Name: openai
      Features:
        - function_calling
        - vision

Agents:
  - Name: Research Assistant
    Goal: Conduct thorough research and analysis
    Instructions: |
      You are a research assistant who provides comprehensive,
      well-sourced information with proper citations.
    Provider: anthropic
    Model: claude-sonnet-4-20250514
    Tools:
      - web_search
      - academic_search
      - read_file
    ModelSettings:
      Temperature: 0.3
      MaxTokens: 4000
      Caching: true
    DateAwareness: true
    Context:
      - Text: "Today's research session context"
      - Path: "./research_guidelines.md"
      
  - Name: Code Reviewer
    Goal: Review code for quality and security
    Instructions: |
      You are a senior software engineer who reviews code for:
      - Code quality and maintainability
      - Security vulnerabilities
      - Performance issues
      - Best practice adherence
    Provider: openai
    Model: gpt-4o
    Tools:
      - read_file
      - static_analysis
      - security_scan
    ModelSettings:
      Temperature: 0.1
      MaxTokens: 6000
      
  - Name: Project Manager
    Goal: Coordinate team activities
    Instructions: |
      You manage projects by coordinating between team members,
      tracking progress, and ensuring deadlines are met.
    IsSupervisor: true
    Subordinates:
      - Research Assistant
      - Code Reviewer
    Tools:
      - assign_work
      - project_tracking
      - calendar_integration

Workflows:
  - Name: Research and Analysis
    Description: Comprehensive research workflow
    Inputs:
      - Name: topic
        Type: string
        Description: Research topic
        Required: true
      - Name: depth
        Type: string
        Description: Research depth
        Default: standard
        Enum:
          - quick
          - standard
          - comprehensive
    
    Output:
      Name: research_report
      Type: string
      Format: markdown
      Document: "reports/research-${workflow.id}.md"
    
    Steps:
      - Name: Initial Research
        Agent: Research Assistant
        Prompt: |
          Research this topic: ${inputs.topic}
          Depth level: ${inputs.depth}
          
          Provide a comprehensive overview with sources.
        Store: initial_research
        
      - Name: Analysis
        Agent: Research Assistant
        Prompt: |
          Analyze the research findings: ${initial_research}
          
          Provide insights, patterns, and conclusions.
        Store: analysis_results
        
      - Name: Generate Report
        Agent: Research Assistant
        Prompt: |
          Create a comprehensive report from:
          Research: ${initial_research}
          Analysis: ${analysis_results}
          
          Format as professional markdown report.
        Store: final_report
        End: true

MCPServers:
  - Name: filesystem
    Type: stdio
    Command: npx
    Args:
      - "@modelcontextprotocol/server-filesystem"
      - "./workspace"
    
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    Headers:
      Authorization: "Bearer ${GITHUB_TOKEN}"
    ToolConfiguration:
      Enabled: true
      AllowedTools:
        - create_issue
        - create_pr
        - get_repository
      ApprovalMode: auto
      
  - Name: database
    Type: stdio
    Command: npx
    Args:
      - "@modelcontextprotocol/server-postgres"
    Env:
      DATABASE_URL: "${DATABASE_URL}"
    ToolConfiguration:
      Enabled: true
      ApprovalMode: manual
      ApprovalFilter:
        Always:
          - read_query
        Never:
          - delete_table
          - drop_database

Documents:
  - ID: guidelines
    Name: Research Guidelines
    Path: ./docs/research-guidelines.md
    Description: Guidelines for conducting research
    
  - ID: style_guide
    Name: Writing Style Guide
    Path: ./docs/style-guide.md
    Description: Style guide for reports and documentation

Schedules:
  - Name: daily_summary
    Cron: "0 17 * * MON-FRI"
    Workflow: Daily Summary Report
    Enabled: true
    
  - Name: weekly_analysis
    Cron: "0 9 * * MON"
    Workflow: Weekly Analysis
    Enabled: true
```

### Loading and Building

```go
package main

import (
    "context"
    "log"
    
    "github.com/deepnoodle-ai/dive/config"
    "github.com/deepnoodle-ai/dive/objects"
)

func main() {
    // Load configuration
    cfg, err := config.LoadFromFile("dive.yaml")
    if err != nil {
        log.Fatal("Failed to load config:", err)
    }
    
    // Build environment with custom options
    env, err := config.BuildEnvironmentWithOptions(cfg, config.BuildOptions{
        DocumentRepository: objects.NewFileDocumentRepository("./data"),
        ThreadRepository:   objects.NewFileThreadRepository("./threads"),
        AutoStart:          true,
    })
    if err != nil {
        log.Fatal("Failed to build environment:", err)
    }
    defer env.Stop(context.Background())
    
    log.Printf("Environment '%s' started with %d agents", 
        env.Name(), len(env.Agents()))
    
    // Run a workflow
    inputs := map[string]interface{}{
        "topic": "quantum computing",
        "depth": "comprehensive",
    }
    
    execution, err := env.RunWorkflow(context.Background(), 
        "Research and Analysis", inputs)
    if err != nil {
        log.Fatal("Failed to run workflow:", err)
    }
    
    result, err := execution.Wait(context.Background())
    if err != nil {
        log.Fatal("Workflow failed:", err)
    }
    
    log.Printf("Research completed: %v", result.Outputs)
}
```

### Configuration Validation

```go
func validateConfig(cfg *config.Config) error {
    // Validate agents
    for _, agent := range cfg.Agents {
        if agent.Name == "" {
            return fmt.Errorf("agent name is required")
        }
        if agent.Instructions == "" {
            return fmt.Errorf("agent %s: instructions are required", agent.Name)
        }
    }
    
    // Validate workflows
    for _, workflow := range cfg.Workflows {
        if workflow.Name == "" {
            return fmt.Errorf("workflow name is required")
        }
        if len(workflow.Steps) == 0 {
            return fmt.Errorf("workflow %s: steps are required", workflow.Name)
        }
    }
    
    // Validate MCP servers
    for _, server := range cfg.MCPServers {
        if server.Name == "" {
            return fmt.Errorf("MCP server name is required")
        }
        if server.Type != "stdio" && server.Type != "url" {
            return fmt.Errorf("MCP server %s: invalid type %s", server.Name, server.Type)
        }
    }
    
    return nil
}
```

### Dynamic Configuration

```go
func buildDynamicConfig() *config.Config {
    return &config.Config{
        Config: config.Config{
            DefaultProvider: getEnvOrDefault("DIVE_PROVIDER", "anthropic"),
            DefaultModel:    getEnvOrDefault("DIVE_MODEL", "claude-sonnet-4-20250514"),
            LogLevel:        getEnvOrDefault("LOG_LEVEL", "info"),
        },
        Agents: []config.Agent{
            {
                Name:         "Dynamic Assistant",
                Instructions: loadInstructionsFromFile("./prompts/assistant.txt"),
                Provider:     getEnvOrDefault("DIVE_PROVIDER", "anthropic"),
                Model:        getEnvOrDefault("DIVE_MODEL", "claude-sonnet-4-20250514"),
                Tools:        getEnabledTools(),
                ModelSettings: &config.ModelSettings{
                    Temperature: &[]float64{parseFloatOrDefault(os.Getenv("TEMPERATURE"), 0.7)}[0],
                    MaxTokens:   &[]int{parseIntOrDefault(os.Getenv("MAX_TOKENS"), 4000)}[0],
                },
            },
        },
    }
}

func getEnabledTools() []string {
    tools := []string{"read_file", "write_file"}
    
    if os.Getenv("ENABLE_WEB_SEARCH") == "true" {
        tools = append(tools, "web_search")
    }
    
    if os.Getenv("ENABLE_CODE_EXECUTION") == "true" {
        tools = append(tools, "code_execution")
    }
    
    return tools
}
```

This comprehensive API reference covers all aspects of the config package, enabling developers to create sophisticated configuration-driven AI systems with agents, workflows, and external service integrations.