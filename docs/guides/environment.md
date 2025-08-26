# Environment Guide

Environments in Dive serve as runtime containers that orchestrate agents, manage shared resources, and provide coordination between different components of your AI system. They act as the foundation for multi-agent systems and workflow execution.

## 📋 Table of Contents

- [What is an Environment?](#what-is-an-environment)
- [Creating Environments](#creating-environments)
- [Agent Management](#agent-management)
- [Shared Resources](#shared-resources)
- [Workflow Execution](#workflow-execution)
- [Document Repository](#document-repository)
- [Thread Management](#thread-management)
- [Actions and Tools](#actions-and-tools)
- [MCP Integration](#mcp-integration)
- [Event Monitoring](#event-monitoring)
- [Best Practices](#best-practices)

## What is an Environment?

An Environment in Dive provides:

- **Agent Registry** - Central location to register and discover agents
- **Resource Sharing** - Shared document storage, thread management, and tool access
- **Workflow Orchestration** - Runtime for executing multi-step processes
- **Communication Hub** - Enables agent-to-agent interaction
- **State Management** - Centralized state and checkpoint handling
- **External Integration** - MCP server connections and custom actions

Think of an Environment as a workspace where multiple AI agents can collaborate, share information, and work together on complex tasks.

## Creating Environments

### Basic Environment

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/diveagents/dive/environment"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
    // Create a basic environment
    env, err := environment.New(environment.Options{
        Name:        "AI Research Lab",
        Description: "Environment for AI research and development",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Created environment: %s\n", env.Name())
}
```

### Environment with Configuration

```go
import (
    "github.com/diveagents/dive/objects"
    "github.com/diveagents/dive/mcp"
)

func createAdvancedEnvironment() (*environment.Environment, error) {
    // Set up document repository
    docRepo := objects.NewInMemoryDocumentRepository()
    
    // Set up thread repository for conversation persistence
    threadRepo := objects.NewInMemoryThreadRepository()
    
    // Set up MCP manager for external tools
    mcpManager := mcp.NewManager()
    
    // Create environment with full configuration
    env, err := environment.New(environment.Options{
        Name:        "Production Environment",
        Description: "Production AI system environment",
        
        // Resource management
        DocumentRepository: docRepo,
        ThreadRepository:   threadRepo,
        
        // External integrations
        MCPManager: mcpManager,
        MCPServers: []*mcp.ServerConfig{
            {
                Name: "github",
                Type: "url", 
                URL:  "https://mcp.github.com/sse",
            },
        },
        
        // Auto-start the environment
        AutoStart: true,
    })
    
    return env, err
}
```

### Environment from Configuration

```yaml
# environment.yaml
Name: Team Workspace
Description: Collaborative AI workspace

Config:
  LogLevel: info
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514

DocumentRepository:
  Type: file
  Path: ./documents

ThreadRepository:
  Type: file  
  Path: ./threads

MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}
    
Agents:
  - Name: Developer
    Instructions: You are a senior developer who writes high-quality code
    Tools: [read_file, write_file, command]
    
  - Name: Reviewer
    Instructions: You are a code reviewer focused on quality and security
    Tools: [read_file, write_file]
    
Workflows:
  - Name: Code Review
    # ... workflow definition
```

```go
// Load environment from YAML
func loadEnvironmentFromConfig() (*environment.Environment, error) {
    cfg, err := config.LoadFromFile("environment.yaml")
    if err != nil {
        return nil, err
    }
    
    env, err := config.BuildEnvironment(cfg)
    if err != nil {
        return nil, err
    }
    
    return env, nil
}
```

## Agent Management

### Adding Agents to Environment

```go
func setupTeamEnvironment() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Development Team",
    })
    if err != nil {
        return nil, err
    }
    
    // Create individual agents
    developer, err := agent.New(agent.Options{
        Name:         "Senior Developer",
        Instructions: "You write clean, efficient code and follow best practices.",
        Model:        anthropic.New(),
        Environment:  env, // Agent automatically registers with environment
    })
    if err != nil {
        return nil, err
    }
    
    reviewer, err := agent.New(agent.Options{
        Name:         "Code Reviewer", 
        Instructions: "You review code for quality, security, and maintainability.",
        Model:        anthropic.New(),
        Environment:  env,
    })
    if err != nil {
        return nil, err
    }
    
    // Or add agents manually
    tester, err := agent.New(agent.Options{
        Name:         "QA Tester",
        Instructions: "You create comprehensive tests and find bugs.",
        Model:        anthropic.New(),
    })
    if err != nil {
        return nil, err
    }
    
    err = env.AddAgent(tester)
    if err != nil {
        return nil, err
    }
    
    return env, nil
}
```

### Agent Discovery

```go
func demonstrateAgentDiscovery(env *environment.Environment) {
    // List all agents in environment
    agents := env.Agents()
    fmt.Printf("Environment has %d agents:\n", len(agents))
    
    for _, agent := range agents {
        fmt.Printf("- %s (Supervisor: %v)\n", agent.Name(), agent.IsSupervisor())
    }
    
    // Get specific agent
    developer, err := env.GetAgent("Senior Developer")
    if err != nil {
        log.Printf("Agent not found: %v", err)
        return
    }
    
    // Use the agent
    response, err := developer.CreateResponse(
        context.Background(),
        dive.WithInput("Review the latest code changes"),
    )
    if err != nil {
        log.Printf("Agent response error: %v", err)
        return
    }
    
    fmt.Println("Developer response:", response.Text())
}
```

### Supervisor Hierarchies

```go
func createSupervisorHierarchy() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Hierarchical Team",
    })
    if err != nil {
        return nil, err
    }
    
    // Create supervisor agent
    manager, err := agent.New(agent.Options{
        Name:         "Project Manager",
        Instructions: "You coordinate work between team members and ensure project success.",
        IsSupervisor: true,
        Subordinates: []string{"Developer", "Designer", "Tester"}, // Optional: specify subordinates
        Model:        anthropic.New(),
        Environment:  env,
    })
    if err != nil {
        return nil, err
    }
    
    // Create subordinate agents
    developer, err := agent.New(agent.Options{
        Name:        "Developer",
        Instructions: "You implement features and fix bugs.",
        Model:       anthropic.New(),
        Environment: env,
    })
    if err != nil {
        return nil, err
    }
    
    designer, err := agent.New(agent.Options{
        Name:        "Designer", 
        Instructions: "You create user interfaces and user experiences.",
        Model:       anthropic.New(),
        Environment: env,
    })
    if err != nil {
        return nil, err
    }
    
    // Manager can now assign work to subordinates
    response, err := manager.CreateResponse(
        context.Background(),
        dive.WithInput("We need to implement a user login feature. Please coordinate the team."),
    )
    if err != nil {
        return nil, err
    }
    
    fmt.Println("Manager coordination:", response.Text())
    
    return env, nil
}
```

## Shared Resources

### Document Repository Integration

```go
import "github.com/diveagents/dive/objects"

func setupDocumentSharing() (*environment.Environment, error) {
    // Create document repository
    docRepo := objects.NewInMemoryDocumentRepository()
    
    // Pre-populate with some documents
    projectSpec := &dive.Document{
        Path:        "specs/project-requirements.md",
        Content:     "# Project Requirements\n\n...",
        ContentType: "text/markdown",
    }
    
    err := docRepo.PutDocument(context.Background(), projectSpec)
    if err != nil {
        return nil, err
    }
    
    // Create environment with shared documents
    env, err := environment.New(environment.Options{
        Name:               "Document-Enabled Environment",
        DocumentRepository: docRepo,
    })
    if err != nil {
        return nil, err
    }
    
    // Create agents that can access shared documents
    analyst, err := agent.New(agent.Options{
        Name:         "Business Analyst",
        Instructions: "You analyze requirements and create specifications.",
        Model:        anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
        DocumentRepository: docRepo, // Agent can access documents
        Environment:        env,
    })
    if err != nil {
        return nil, err
    }
    
    // Agent can now read and write shared documents
    response, err := analyst.CreateResponse(
        context.Background(),
        dive.WithInput("Read the project requirements and create a technical specification document"),
    )
    if err != nil {
        return nil, err
    }
    
    return env, nil
}
```

### Thread Repository for Conversations

```go
func setupConversationManagement() (*environment.Environment, error) {
    // Create thread repository for persistent conversations
    threadRepo := objects.NewInMemoryThreadRepository()
    
    env, err := environment.New(environment.Options{
        Name:             "Conversation Environment",
        ThreadRepository: threadRepo,
    })
    if err != nil {
        return nil, err
    }
    
    // Create agents with conversation memory
    assistant, err := agent.New(agent.Options{
        Name:             "Assistant",
        Instructions:     "You are a helpful assistant with memory of our conversations.",
        Model:            anthropic.New(),
        ThreadRepository: threadRepo,
        Environment:      env,
    })
    if err != nil {
        return nil, err
    }
    
    // First conversation
    response1, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID("user-123"),
        dive.WithUserID("alice"),
        dive.WithInput("My name is Alice and I'm working on a Go project"),
    )
    if err != nil {
        return nil, err
    }
    
    // Later conversation - assistant remembers Alice
    response2, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID("user-123"), 
        dive.WithInput("Can you help me debug my Go code?"),
    )
    if err != nil {
        return nil, err
    }
    
    return env, nil
}
```

## Workflow Execution

### Running Workflows in Environment

```go
func executeWorkflowInEnvironment() error {
    // Create environment
    env, err := environment.New(environment.Options{
        Name: "Workflow Environment",
        DocumentRepository: objects.NewInMemoryDocumentRepository(),
    })
    if err != nil {
        return err
    }
    
    // Add agents
    researcher, err := agent.New(agent.Options{
        Name:        "Researcher",
        Instructions: "You conduct thorough research on any topic.",
        Model:       anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
        },
        Environment: env,
    })
    if err != nil {
        return err
    }
    
    writer, err := agent.New(agent.Options{
        Name:        "Technical Writer",
        Instructions: "You create clear, comprehensive documentation.",
        Model:       anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
        Environment: env,
    })
    if err != nil {
        return err
    }
    
    // Define workflow
    workflow := &workflow.Workflow{
        Name: "Research and Document",
        Steps: []*workflow.Step{
            {
                Name:   "Research Phase",
                Agent:  "Researcher",
                Prompt: "Research the topic: ${inputs.topic}",
                Store:  "research_data",
            },
            {
                Name:   "Documentation Phase", 
                Agent:  "Technical Writer",
                Prompt: "Create documentation based on: ${research_data}",
                Store:  "documentation",
            },
            {
                Name:   "Save Documentation",
                Action: "Document.Write",
                Parameters: map[string]interface{}{
                    "Path":    "docs/${inputs.topic}.md",
                    "Content": "${documentation}",
                },
            },
        },
    }
    
    // Execute workflow
    inputs := map[string]interface{}{
        "topic": "microservices architecture",
    }
    
    execution, err := env.RunWorkflow(context.Background(), workflow, inputs)
    if err != nil {
        return err
    }
    
    // Wait for completion
    result, err := execution.Wait(context.Background())
    if err != nil {
        return err
    }
    
    fmt.Printf("Workflow completed with status: %s\n", result.Status)
    return nil
}
```

### Monitoring Workflow Progress

```go
func monitorWorkflowExecution(env *environment.Environment) error {
    inputs := map[string]interface{}{
        "topic": "artificial intelligence trends",
    }
    
    // Start workflow with event streaming
    execution, err := env.RunWorkflow(
        context.Background(),
        "Research and Document",
        inputs,
        environment.WithEventCallback(func(ctx context.Context, event *environment.ExecutionEvent) error {
            switch event.Type {
            case "execution.started":
                fmt.Printf("🚀 Workflow started: %s\n", event.ExecutionID)
                
            case "step.started":
                fmt.Printf("▶️  Step started: %s\n", event.StepName)
                
            case "step.completed":
                fmt.Printf("✅ Step completed: %s\n", event.StepName)
                if event.StepOutput != nil {
                    fmt.Printf("   Output: %v\n", event.StepOutput)
                }
                
            case "agent.response":
                fmt.Printf("🤖 Agent response: %s\n", event.AgentName)
                
            case "tool.called":
                fmt.Printf("🔧 Tool called: %s\n", event.ToolName)
                
            case "execution.completed":
                fmt.Printf("🎉 Workflow completed: %s\n", event.ExecutionID)
                
            case "execution.failed":
                fmt.Printf("❌ Workflow failed: %s (Error: %v)\n", event.ExecutionID, event.Error)
            }
            
            return nil
        }),
    )
    if err != nil {
        return err
    }
    
    // Wait for completion
    _, err = execution.Wait(context.Background())
    return err
}
```

## Document Repository

### Custom Document Repository

```go
import (
    "database/sql"
    "encoding/json"
    _ "github.com/lib/pq"
)

// Custom database-backed document repository
type PostgresDocumentRepository struct {
    db *sql.DB
}

func NewPostgresDocumentRepository(connectionString string) (*PostgresDocumentRepository, error) {
    db, err := sql.Open("postgres", connectionString)
    if err != nil {
        return nil, err
    }
    
    // Create table if not exists
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS documents (
            path VARCHAR PRIMARY KEY,
            content TEXT NOT NULL,
            content_type VARCHAR NOT NULL,
            metadata JSONB,
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        )
    `)
    if err != nil {
        return nil, err
    }
    
    return &PostgresDocumentRepository{db: db}, nil
}

func (r *PostgresDocumentRepository) GetDocument(ctx context.Context, path string) (*dive.Document, error) {
    var doc dive.Document
    var metadataJSON []byte
    
    err := r.db.QueryRowContext(ctx, `
        SELECT path, content, content_type, metadata, created_at, updated_at 
        FROM documents WHERE path = $1
    `, path).Scan(
        &doc.Path, &doc.Content, &doc.ContentType, 
        &metadataJSON, &doc.CreatedAt, &doc.UpdatedAt,
    )
    
    if err == sql.ErrNoRows {
        return nil, dive.ErrDocumentNotFound
    }
    if err != nil {
        return nil, err
    }
    
    if len(metadataJSON) > 0 {
        err = json.Unmarshal(metadataJSON, &doc.Metadata)
        if err != nil {
            return nil, err
        }
    }
    
    return &doc, nil
}

func (r *PostgresDocumentRepository) PutDocument(ctx context.Context, doc *dive.Document) error {
    metadataJSON, err := json.Marshal(doc.Metadata)
    if err != nil {
        return err
    }
    
    _, err = r.db.ExecContext(ctx, `
        INSERT INTO documents (path, content, content_type, metadata, updated_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (path) DO UPDATE SET
            content = EXCLUDED.content,
            content_type = EXCLUDED.content_type,
            metadata = EXCLUDED.metadata,
            updated_at = NOW()
    `, doc.Path, doc.Content, doc.ContentType, metadataJSON)
    
    return err
}

func (r *PostgresDocumentRepository) ListDocuments(ctx context.Context, prefix string) ([]*dive.Document, error) {
    rows, err := r.db.QueryContext(ctx, `
        SELECT path, content, content_type, metadata, created_at, updated_at
        FROM documents WHERE path LIKE $1 || '%'
        ORDER BY path
    `, prefix)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var documents []*dive.Document
    for rows.Next() {
        var doc dive.Document
        var metadataJSON []byte
        
        err := rows.Scan(
            &doc.Path, &doc.Content, &doc.ContentType,
            &metadataJSON, &doc.CreatedAt, &doc.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        
        if len(metadataJSON) > 0 {
            err = json.Unmarshal(metadataJSON, &doc.Metadata)
            if err != nil {
                return nil, err
            }
        }
        
        documents = append(documents, &doc)
    }
    
    return documents, rows.Err()
}

func (r *PostgresDocumentRepository) DeleteDocument(ctx context.Context, path string) error {
    _, err := r.db.ExecContext(ctx, "DELETE FROM documents WHERE path = $1", path)
    return err
}

// Use custom repository
func useCustomDocumentRepository() (*environment.Environment, error) {
    docRepo, err := NewPostgresDocumentRepository("postgres://user:pass@localhost/dive?sslmode=disable")
    if err != nil {
        return nil, err
    }
    
    env, err := environment.New(environment.Options{
        Name:               "Production Environment",
        DocumentRepository: docRepo,
    })
    
    return env, err
}
```

## Actions and Tools

### Custom Actions

```go
// Custom action for sending notifications
type SlackNotificationAction struct {
    WebhookURL string
}

func (a *SlackNotificationAction) Name() string {
    return "Slack.Notify"
}

func (a *SlackNotificationAction) Execute(ctx context.Context, params map[string]interface{}) (*environment.ActionResult, error) {
    message, ok := params["message"].(string)
    if !ok {
        return nil, fmt.Errorf("message parameter required")
    }
    
    channel, _ := params["channel"].(string)
    if channel == "" {
        channel = "#general"
    }
    
    payload := map[string]interface{}{
        "text":    message,
        "channel": channel,
    }
    
    // Send to Slack webhook
    err := sendSlackMessage(a.WebhookURL, payload)
    if err != nil {
        return &environment.ActionResult{
            Success: false,
            Error:   err.Error(),
        }, nil
    }
    
    return &environment.ActionResult{
        Success: true,
        Data: map[string]interface{}{
            "message": "Notification sent successfully",
        },
    }, nil
}

func createEnvironmentWithCustomActions() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Notification Environment",
        Actions: []environment.Action{
            &SlackNotificationAction{
                WebhookURL: os.Getenv("SLACK_WEBHOOK_URL"),
            },
        },
    })
    
    return env, err
}
```

### Environment-Wide Tools

```go
func setupEnvironmentTools() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Tool-Rich Environment",
    })
    if err != nil {
        return nil, err
    }
    
    // Create shared tools that all agents can use
    commonTools := []dive.Tool{
        dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
            Provider: "google",
        })),
        dive.ToolAdapter(toolkit.NewFetchTool()),
        dive.ToolAdapter(toolkit.NewCalculatorTool()),
    }
    
    // Create multiple agents with shared tools
    agents := []struct {
        name         string
        instructions string
    }{
        {"Researcher", "You conduct thorough research using web search and fetch tools."},
        {"Analyst", "You analyze data and perform calculations."},
        {"Writer", "You create content based on research and analysis."},
    }
    
    for _, agentConfig := range agents {
        _, err := agent.New(agent.Options{
            Name:         agentConfig.name,
            Instructions: agentConfig.instructions,
            Model:        anthropic.New(),
            Tools:        commonTools, // Shared tools
            Environment:  env,
        })
        if err != nil {
            return nil, err
        }
    }
    
    return env, nil
}
```

## MCP Integration

### Setting up MCP Servers

```go
import "github.com/diveagents/dive/mcp"

func setupMCPIntegration() (*environment.Environment, error) {
    // Create MCP manager
    mcpManager := mcp.NewManager()
    
    // Configure MCP servers
    mcpServers := []*mcp.ServerConfig{
        {
            Name: "github",
            Type: "url",
            URL:  "https://mcp.github.com/sse",
            AuthorizationToken: os.Getenv("GITHUB_TOKEN"),
        },
        {
            Name: "linear", 
            Type: "url",
            URL:  "https://mcp.linear.app/sse",
            AuthorizationToken: os.Getenv("LINEAR_API_KEY"),
        },
        {
            Name: "filesystem",
            Type: "stdio",
            Command: "npx",
            Args:    []string{"@modelcontextprotocol/server-filesystem", "/path/to/allowed/directory"},
        },
    }
    
    // Create environment with MCP integration
    env, err := environment.New(environment.Options{
        Name:       "MCP Environment",
        MCPManager: mcpManager,
        MCPServers: mcpServers,
        AutoStart:  true, // Automatically connect to MCP servers
    })
    if err != nil {
        return nil, err
    }
    
    return env, nil
}

func useMCPTools(env *environment.Environment) error {
    // Get all available MCP tools
    allTools := env.GetMCPTools()
    fmt.Printf("Available MCP tools: %d\n", len(allTools))
    
    for name, tool := range allTools {
        fmt.Printf("- %s: %s\n", name, tool.Description())
    }
    
    // Get tools from specific server
    githubTools := env.GetMCPToolsByServer("github")
    fmt.Printf("GitHub tools: %d\n", len(githubTools))
    
    // Use specific tool
    createIssueTool := env.GetMCPTool("github:create_issue")
    if createIssueTool != nil {
        fmt.Println("GitHub create_issue tool available")
    }
    
    // Create agent with MCP tools
    developer, err := agent.New(agent.Options{
        Name:         "GitHub Developer",
        Instructions: "You can create issues and manage repositories using GitHub tools.",
        Model:        anthropic.New(),
        Tools: append([]dive.Tool{createIssueTool}, 
                     env.GetMCPToolsByServer("github")...),
        Environment: env,
    })
    if err != nil {
        return err
    }
    
    // Agent can now use GitHub MCP tools
    response, err := developer.CreateResponse(
        context.Background(),
        dive.WithInput("Create a bug report issue for the authentication problem we discussed"),
    )
    if err != nil {
        return err
    }
    
    fmt.Println("Developer response:", response.Text())
    return nil
}
```

## Event Monitoring

### Environment-Level Event Handling

```go
func monitorEnvironmentEvents(env *environment.Environment) error {
    // Set up event monitoring
    eventChan := make(chan *environment.Event, 100)
    
    // Subscribe to environment events
    env.Subscribe(eventChan)
    defer env.Unsubscribe(eventChan)
    
    // Monitor events in background
    go func() {
        for event := range eventChan {
            switch event.Type {
            case "agent.added":
                fmt.Printf("🤖 New agent added: %s\n", event.AgentName)
                
            case "workflow.started":
                fmt.Printf("🚀 Workflow started: %s\n", event.WorkflowName)
                
            case "workflow.completed":
                fmt.Printf("✅ Workflow completed: %s\n", event.WorkflowName)
                
            case "document.created":
                fmt.Printf("📄 Document created: %s\n", event.DocumentPath)
                
            case "document.updated":
                fmt.Printf("📝 Document updated: %s\n", event.DocumentPath)
                
            case "mcp.connected":
                fmt.Printf("🔌 MCP server connected: %s\n", event.ServerName)
                
            case "mcp.disconnected":
                fmt.Printf("🔌 MCP server disconnected: %s\n", event.ServerName)
                
            case "error":
                fmt.Printf("❌ Error: %v\n", event.Error)
            }
        }
    }()
    
    return nil
}
```

## Best Practices

### 1. Resource Management

```go
func properResourceManagement() {
    env, err := environment.New(environment.Options{
        Name: "Managed Environment",
        // Configure with proper resource limits
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Start environment
    ctx := context.Background()
    err = env.Start(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    // Always clean up resources
    defer func() {
        if err := env.Stop(ctx); err != nil {
            log.Printf("Error stopping environment: %v", err)
        }
    }()
    
    // Use the environment
    // ...
}
```

### 2. Error Handling

```go
func robustErrorHandling(env *environment.Environment) {
    // Create agent with error handling
    agent, err := env.GetAgent("Assistant")
    if err != nil {
        if errors.Is(err, dive.ErrAgentNotFound) {
            log.Printf("Agent not found, creating new one...")
            // Create and add agent
        } else {
            log.Printf("Unexpected error: %v", err)
            return
        }
    }
    
    // Handle workflow errors
    execution, err := env.RunWorkflow(ctx, "MyWorkflow", inputs)
    if err != nil {
        log.Printf("Failed to start workflow: %v", err)
        return
    }
    
    result, err := execution.Wait(ctx)
    if err != nil {
        log.Printf("Workflow execution failed: %v", err)
        return
    }
    
    if result.Status == "failed" {
        log.Printf("Workflow failed: %s", result.Error)
        // Handle failure appropriately
    }
}
```

### 3. Configuration Management

```go
// Use configuration objects for reusable setups
type EnvironmentConfig struct {
    Name                string
    DocumentStoragePath string
    ThreadStoragePath   string
    MCPServers          []MCPServerConfig
    DefaultModel        string
    LogLevel            string
}

func createStandardEnvironment(config EnvironmentConfig) (*environment.Environment, error) {
    // Set up repositories based on config
    var docRepo dive.DocumentRepository
    if config.DocumentStoragePath != "" {
        docRepo = objects.NewFileDocumentRepository(config.DocumentStoragePath)
    } else {
        docRepo = objects.NewInMemoryDocumentRepository()
    }
    
    var threadRepo dive.ThreadRepository
    if config.ThreadStoragePath != "" {
        threadRepo = objects.NewFileThreadRepository(config.ThreadStoragePath)
    } else {
        threadRepo = objects.NewInMemoryThreadRepository()
    }
    
    // Create environment
    env, err := environment.New(environment.Options{
        Name:               config.Name,
        DocumentRepository: docRepo,
        ThreadRepository:   threadRepo,
        MCPServers:         convertMCPServers(config.MCPServers),
        AutoStart:          true,
    })
    
    return env, err
}

// Usage
func setupProductionEnvironment() (*environment.Environment, error) {
    config := EnvironmentConfig{
        Name:                "Production AI System",
        DocumentStoragePath: "/var/lib/dive/documents",
        ThreadStoragePath:   "/var/lib/dive/threads",
        MCPServers: []MCPServerConfig{
            {Name: "github", URL: "https://mcp.github.com/sse"},
            {Name: "linear", URL: "https://mcp.linear.app/sse"},
        },
        DefaultModel: "claude-sonnet-4-20250514",
        LogLevel:     "info",
    }
    
    return createStandardEnvironment(config)
}
```

### 4. Testing Environments

```go
func createTestEnvironment() (*environment.Environment, error) {
    // Use in-memory repositories for testing
    env, err := environment.New(environment.Options{
        Name:               "Test Environment",
        DocumentRepository: objects.NewInMemoryDocumentRepository(),
        ThreadRepository:   objects.NewInMemoryThreadRepository(),
        // Don't auto-start for testing
        AutoStart: false,
    })
    if err != nil {
        return nil, err
    }
    
    // Add test agents
    testAgent, err := agent.New(agent.Options{
        Name:        "Test Agent",
        Instructions: "You are a test agent for unit testing.",
        Model:       &MockLLM{}, // Use mock LLM for testing
        Environment: env,
    })
    if err != nil {
        return nil, err
    }
    
    return env, nil
}

func TestEnvironmentWorkflow(t *testing.T) {
    env, err := createTestEnvironment()
    require.NoError(t, err)
    
    err = env.Start(context.Background())
    require.NoError(t, err)
    defer env.Stop(context.Background())
    
    // Test workflow execution
    inputs := map[string]interface{}{
        "test_input": "test_value",
    }
    
    execution, err := env.RunWorkflow(context.Background(), "TestWorkflow", inputs)
    require.NoError(t, err)
    
    result, err := execution.Wait(context.Background())
    require.NoError(t, err)
    
    assert.Equal(t, "completed", result.Status)
}
```

## Next Steps

- [Agent Guide](agents.md) - Learn about creating and managing agents
- [Workflow Guide](workflows.md) - Master workflow orchestration
- [MCP Integration](mcp-integration.md) - Connect external tool servers
- [API Reference](../api/core.md) - Environment interface details