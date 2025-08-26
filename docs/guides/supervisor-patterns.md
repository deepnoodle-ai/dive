# Supervisor Patterns Guide

Supervisor patterns in Dive enable hierarchical agent systems where supervisor agents coordinate and delegate work to subordinate agents. This guide covers how to design, implement, and manage multi-agent systems with clear organizational structures.

## 📋 Table of Contents

- [What are Supervisor Patterns?](#what-are-supervisor-patterns)
- [Basic Supervisor Setup](#basic-supervisor-setup)
- [Delegation Patterns](#delegation-patterns)
- [Multi-Level Hierarchies](#multi-level-hierarchies)
- [Specialized Teams](#specialized-teams)
- [Communication Patterns](#communication-patterns)
- [Coordination Strategies](#coordination-strategies)
- [Best Practices](#best-practices)

## What are Supervisor Patterns?

Supervisor patterns organize agents in hierarchical structures where:

- **Supervisor Agents** coordinate work, make high-level decisions, and delegate tasks
- **Subordinate Agents** execute specific tasks and report results back
- **Work Distribution** is managed through intelligent task assignment
- **Results Aggregation** combines outputs from multiple agents

### Benefits

1. **Scalability** - Distribute work across multiple specialized agents
2. **Specialization** - Each agent can focus on specific domain expertise
3. **Coordination** - Central oversight ensures consistent execution
4. **Fault Tolerance** - Work can be redistributed if agents fail
5. **Resource Management** - Optimize workload across available agents

### Common Patterns

```
CEO Agent (Strategy)
├── Product Manager (Planning)
│   ├── Designer (UI/UX)
│   ├── Developer (Implementation)
│   └── Tester (Quality Assurance)
├── Marketing Manager (Promotion)
│   ├── Content Writer (Content)
│   └── Social Media Manager (Distribution)
└── Operations Manager (Infrastructure)
    ├── DevOps Engineer (Deployment)
    └── Support Engineer (Maintenance)
```

## Basic Supervisor Setup

### Simple Supervisor-Subordinate Pair

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/environment"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

func createBasicSupervisorSystem() (*environment.Environment, error) {
    // Create environment for the team
    env, err := environment.New(environment.Options{
        Name: "Development Team",
    })
    if err != nil {
        return nil, err
    }

    // Create subordinate agent (specialist)
    developer, err := agent.New(agent.Options{
        Name:         "Senior Developer",
        Instructions: `You are a senior software developer. You write clean, efficient code,
                      follow best practices, and provide detailed explanations of your work.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewTextEditorTool()),
        },
        Environment: env,
    })
    if err != nil {
        return nil, err
    }

    // Create supervisor agent
    manager, err := agent.New(agent.Options{
        Name:         "Technical Manager",
        Instructions: `You are a technical manager who coordinates development work.
                      You break down complex projects into tasks and assign them to developers.
                      Always provide clear requirements and context for assigned work.`,
        IsSupervisor: true,
        Subordinates: []string{"Senior Developer"}, // Explicit subordinate assignment
        Model:        anthropic.New(),
        Environment:  env,
    })
    if err != nil {
        return nil, err
    }

    return env, nil
}

func demonstrateBasicDelegation() {
    env, err := createBasicSupervisorSystem()
    if err != nil {
        log.Fatal(err)
    }

    // Get the manager
    manager, err := env.GetAgent("Technical Manager")
    if err != nil {
        log.Fatal(err)
    }

    // Manager coordinates work
    response, err := manager.CreateResponse(
        context.Background(),
        dive.WithInput(`I need you to create a user authentication system.
                       Please coordinate this development work.`),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Manager Response:")
    fmt.Println(response.Text())
}
```

### YAML Configuration

```yaml
# supervisor-team.yaml
Name: Software Development Team
Description: Hierarchical development team with supervisor pattern

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514

Agents:
  # Supervisor Agent
  - Name: Technical Manager
    Instructions: |
      You are a technical manager who coordinates development work.
      Break down complex projects into clear, actionable tasks.
      Assign work to the most appropriate team member based on their expertise.
      Always provide context and requirements when delegating tasks.
    IsSupervisor: true
    Subordinates:
      - Senior Developer
      - UI Designer
      - QA Engineer
    
  # Subordinate Agents
  - Name: Senior Developer
    Instructions: |
      You are a senior software developer specializing in backend systems.
      Write clean, maintainable code with proper error handling and documentation.
    Tools:
      - read_file
      - write_file
      - text_editor
      - command

  - Name: UI Designer
    Instructions: |
      You are a UI/UX designer who creates intuitive user interfaces.
      Focus on user experience, accessibility, and visual design principles.
    Tools:
      - read_file
      - write_file
      - generate_image

  - Name: QA Engineer
    Instructions: |
      You are a quality assurance engineer who ensures software reliability.
      Create comprehensive test plans and identify potential issues.
    Tools:
      - read_file
      - write_file
      - command

Workflows:
  - Name: Feature Development
    Inputs:
      - Name: feature_description
        Type: string
        Required: true
    Steps:
      - Name: Plan Development
        Agent: Technical Manager
        Prompt: |
          Plan the development of this feature: ${inputs.feature_description}
          
          Break it down into specific tasks and coordinate the team.
        Store: development_plan
        
      - Name: Execute Plan
        Agent: Technical Manager
        Prompt: |
          Execute the development plan:
          ${development_plan}
          
          Coordinate with your team to complete all tasks.
```

### Loading Supervisor Configuration

```go
func loadSupervisorTeam() (*environment.Environment, error) {
    cfg, err := config.LoadFromFile("supervisor-team.yaml")
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

## Delegation Patterns

### Automatic Subordinate Discovery

```go
func createAutoDiscoveryTeam() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Auto Discovery Team",
    })
    if err != nil {
        return nil, err
    }

    // Create multiple specialists first
    specialists := []struct {
        name         string
        instructions string
        tools        []dive.Tool
    }{
        {
            "Database Specialist",
            "You are expert in database design, optimization, and management.",
            []dive.Tool{
                dive.ToolAdapter(toolkit.NewReadFileTool()),
                dive.ToolAdapter(toolkit.NewWriteFileTool()),
            },
        },
        {
            "Frontend Specialist", 
            "You specialize in user interface development and user experience.",
            []dive.Tool{
                dive.ToolAdapter(toolkit.NewReadFileTool()),
                dive.ToolAdapter(toolkit.NewWriteFileTool()),
                dive.ToolAdapter(toolkit.NewGenerateImageTool()),
            },
        },
        {
            "Security Specialist",
            "You focus on application security, vulnerability assessment, and secure coding practices.",
            []dive.Tool{
                dive.ToolAdapter(toolkit.NewReadFileTool()),
                dive.ToolAdapter(toolkit.NewCommandTool()),
            },
        },
    }

    // Create specialist agents
    for _, spec := range specialists {
        _, err := agent.New(agent.Options{
            Name:         spec.name,
            Instructions: spec.instructions,
            Model:        anthropic.New(),
            Tools:        spec.tools,
            Environment:  env,
        })
        if err != nil {
            return nil, err
        }
    }

    // Create supervisor that automatically discovers subordinates
    supervisor, err := agent.New(agent.Options{
        Name: "Architect",
        Instructions: `You are a senior architect who coordinates technical teams.
                      You have access to specialists in database, frontend, and security.
                      Delegate tasks based on each specialist's expertise.`,
        IsSupervisor: true,
        // Don't specify subordinates - they'll be auto-discovered
        Model:       anthropic.New(),
        Environment: env,
    })
    if err != nil {
        return nil, err
    }

    return env, nil
}
```

### Task-Based Delegation

```go
func demonstrateTaskDelegation() error {
    env, err := createAutoDiscoveryTeam()
    if err != nil {
        return err
    }

    architect, err := env.GetAgent("Architect")
    if err != nil {
        return err
    }

    // Complex task requiring multiple specialists
    response, err := architect.CreateResponse(
        context.Background(),
        dive.WithInput(`We need to build a secure e-commerce platform.
                       
                       Requirements:
                       - User authentication and authorization
                       - Product catalog with search functionality  
                       - Shopping cart and checkout process
                       - Payment processing integration
                       - Admin dashboard for inventory management
                       
                       Please coordinate the team to design and implement this system.`),
    )
    if err != nil {
        return err
    }

    fmt.Println("Architect Coordination:")
    fmt.Println(response.Text())

    // Show tool calls (delegation actions)
    toolCalls := response.ToolCalls()
    fmt.Printf("\nDelegation actions: %d\n", len(toolCalls))
    for _, call := range toolCalls {
        if call.Name == "assign_work" {
            fmt.Printf("- Delegated task: %s\n", call.Name)
        }
    }

    return nil
}
```

### Custom Delegation Logic

```go
// Custom assign_work tool with intelligent routing
type IntelligentAssignWorkTool struct {
    supervisor  dive.Agent
    environment dive.Environment
    taskRouter  *TaskRouter
}

type TaskRouter struct {
    specializations map[string][]string
}

func NewTaskRouter() *TaskRouter {
    return &TaskRouter{
        specializations: map[string][]string{
            "database": {"Database Specialist", "Backend Developer"},
            "frontend": {"Frontend Specialist", "UI Designer"},
            "security": {"Security Specialist", "DevOps Engineer"},
            "testing":  {"QA Engineer", "Test Automation Specialist"},
        },
    }
}

func (t *TaskRouter) AssignTask(taskDescription string, availableAgents []string) string {
    taskLower := strings.ToLower(taskDescription)
    
    // Simple keyword matching (could be enhanced with ML)
    for domain, agents := range t.specializations {
        if strings.Contains(taskLower, domain) {
            for _, agent := range agents {
                for _, available := range availableAgents {
                    if agent == available {
                        return agent
                    }
                }
            }
        }
    }
    
    // Default to first available agent
    if len(availableAgents) > 0 {
        return availableAgents[0]
    }
    
    return ""
}

func (tool *IntelligentAssignWorkTool) Call(ctx context.Context, input *AssignWorkInput) (*dive.ToolResult, error) {
    // Get available subordinates
    subordinates := tool.supervisor.Subordinates()
    
    // Use intelligent routing
    assignedAgent := tool.taskRouter.AssignTask(input.Task, subordinates)
    if assignedAgent == "" {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "No suitable agent available for this task",
            }},
            IsError: true,
        }, nil
    }
    
    // Get the assigned agent
    agent, err := tool.environment.GetAgent(assignedAgent)
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Agent %s not found", assignedAgent),
            }},
            IsError: true,
        }, nil
    }
    
    // Execute the task
    response, err := agent.CreateResponse(ctx, dive.WithInput(input.Task))
    if err != nil {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: fmt.Sprintf("Task execution failed: %v", err),
            }},
            IsError: true,
        }, nil
    }
    
    result := fmt.Sprintf("Task assigned to %s:\n\nTask: %s\n\nResult:\n%s",
                          assignedAgent, input.Task, response.Text())
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: result,
        }},
    }, nil
}
```

## Multi-Level Hierarchies

### Three-Tier Organization

```go
func createMultiTierOrganization() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Enterprise Organization",
    })
    if err != nil {
        return nil, err
    }

    // Tier 3: Individual Contributors
    contributors := []struct {
        name         string
        instructions string
    }{
        {"Junior Developer", "You write code according to specifications provided by senior developers."},
        {"Junior Designer", "You create UI components and visual assets under senior designer guidance."},
        {"Junior Tester", "You execute test plans and report bugs under QA leadership."},
    }

    for _, contrib := range contributors {
        _, err := agent.New(agent.Options{
            Name:         contrib.name,
            Instructions: contrib.instructions,
            Model:        anthropic.New(),
            Environment:  env,
        })
        if err != nil {
            return nil, err
        }
    }

    // Tier 2: Team Leads
    teamLeads := []struct {
        name         string
        instructions string
        subordinates []string
    }{
        {
            "Senior Developer",
            "You lead development work and mentor junior developers.",
            []string{"Junior Developer"},
        },
        {
            "Senior Designer", 
            "You lead design work and guide junior designers.",
            []string{"Junior Designer"},
        },
        {
            "QA Lead",
            "You oversee quality assurance and guide testing activities.",
            []string{"Junior Tester"},
        },
    }

    for _, lead := range teamLeads {
        _, err := agent.New(agent.Options{
            Name:         lead.name,
            Instructions: lead.instructions,
            IsSupervisor: true,
            Subordinates: lead.subordinates,
            Model:        anthropic.New(),
            Environment:  env,
        })
        if err != nil {
            return nil, err
        }
    }

    // Tier 1: Executive Leadership
    _, err = agent.New(agent.Options{
        Name: "Technical Director",
        Instructions: `You provide technical leadership and strategic direction.
                      Coordinate work across development, design, and QA teams.
                      Make high-level architectural and process decisions.`,
        IsSupervisor: true,
        Subordinates: []string{"Senior Developer", "Senior Designer", "QA Lead"},
        Model:        anthropic.New(),
        Environment:  env,
    })
    if err != nil {
        return nil, err
    }

    return env, nil
}
```

### Department-Based Organization

```yaml
# enterprise-org.yaml
Name: Enterprise Organization
Description: Multi-department organization with clear hierarchies

Agents:
  # C-Level Executive
  - Name: CTO
    Instructions: |
      You are the Chief Technology Officer responsible for overall technology strategy.
      Coordinate between department heads and make strategic technical decisions.
    IsSupervisor: true
    Subordinates:
      - Engineering Director
      - Product Director
      - Operations Director

  # Department Directors
  - Name: Engineering Director
    Instructions: |
      You oversee all engineering activities and manage engineering teams.
      Coordinate between different engineering disciplines.
    IsSupervisor: true
    Subordinates:
      - Backend Team Lead
      - Frontend Team Lead
      - DevOps Team Lead

  - Name: Product Director
    Instructions: |
      You manage product strategy and coordinate product development.
      Work closely with engineering and design teams.
    IsSupervisor: true
    Subordinates:
      - Product Manager
      - UX Designer
      - Product Analyst

  - Name: Operations Director
    Instructions: |
      You oversee operational activities including infrastructure and support.
    IsSupervisor: true
    Subordinates:
      - Infrastructure Engineer
      - Support Manager

  # Team Leads
  - Name: Backend Team Lead
    Instructions: You lead backend development and coordinate server-side work.
    IsSupervisor: true
    Subordinates: [Senior Backend Developer, Junior Backend Developer]

  - Name: Frontend Team Lead
    Instructions: You lead frontend development and coordinate client-side work.
    IsSupervisor: true
    Subordinates: [Senior Frontend Developer, Junior Frontend Developer]

  # Individual Contributors
  - Name: Senior Backend Developer
    Instructions: You develop backend services and mentor junior developers.

  - Name: Junior Backend Developer
    Instructions: You implement backend features under senior developer guidance.

  # ... (additional agents)
```

## Specialized Teams

### Agile Development Team

```go
func createAgileTeam() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Agile Development Team",
    })
    if err != nil {
        return nil, err
    }

    // Scrum Master (Process Facilitator)
    scrumMaster, err := agent.New(agent.Options{
        Name: "Scrum Master",
        Instructions: `You facilitate agile processes and remove blockers.
                      Help the team follow scrum practices and coordinate sprint activities.`,
        IsSupervisor: true,
        Model:        anthropic.New(),
        Environment:  env,
    })
    if err != nil {
        return nil, err
    }

    // Product Owner (Requirements)
    productOwner, err := agent.New(agent.Options{
        Name: "Product Owner",
        Instructions: `You define product requirements and prioritize features.
                      Create user stories and acceptance criteria.`,
        Model:       anthropic.New(),
        Environment: env,
    })
    if err != nil {
        return nil, err
    }

    // Development Team Members
    teamMembers := []string{
        "Full Stack Developer",
        "Frontend Specialist", 
        "Backend Specialist",
        "QA Automation Engineer",
        "DevOps Engineer",
    }

    for _, member := range teamMembers {
        _, err := agent.New(agent.Options{
            Name:        member,
            Instructions: fmt.Sprintf("You are a %s contributing to sprint goals.", strings.ToLower(member)),
            Model:       anthropic.New(),
            Environment: env,
        })
        if err != nil {
            return nil, err
        }
    }

    return env, nil
}
```

### Research Team Structure

```go
func createResearchTeam() (*environment.Environment, error) {
    env, err := environment.New(environment.Options{
        Name: "Research Team",
    })
    if err != nil {
        return nil, err
    }

    // Principal Investigator
    pi, err := agent.New(agent.Options{
        Name: "Principal Investigator",
        Instructions: `You lead research initiatives and coordinate research activities.
                      Define research questions, allocate resources, and synthesize findings.`,
        IsSupervisor: true,
        Model:        anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewFetchTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
        Environment: env,
    })
    if err != nil {
        return nil, err
    }

    // Research Specialists
    specialists := []struct {
        name         string
        instructions string
    }{
        {
            "Literature Reviewer",
            "You conduct comprehensive literature reviews and identify research gaps.",
        },
        {
            "Data Analyst",
            "You analyze research data and create statistical models.",
        },
        {
            "Technical Writer",
            "You write research papers, reports, and documentation.",
        },
        {
            "Methodology Expert",
            "You design research methodologies and experimental protocols.",
        },
    }

    for _, spec := range specialists {
        _, err := agent.New(agent.Options{
            Name:         spec.name,
            Instructions: spec.instructions,
            Model:        anthropic.New(),
            Tools: []dive.Tool{
                dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                    Provider: "google",
                })),
                dive.ToolAdapter(toolkit.NewFetchTool()),
                dive.ToolAdapter(toolkit.NewReadFileTool()),
                dive.ToolAdapter(toolkit.NewWriteFileTool()),
            },
            Environment: env,
        })
        if err != nil {
            return nil, err
        }
    }

    return env, nil
}
```

## Communication Patterns

### Status Reporting

```go
// Custom tool for status reporting
type StatusReportTool struct {
    environment dive.Environment
}

type StatusReportInput struct {
    Agent      string `json:"agent" description:"Agent providing the status"`
    Task       string `json:"task" description:"Task being reported on"`
    Status     string `json:"status" description:"Current status (in_progress, completed, blocked)"`
    Progress   int    `json:"progress" description:"Completion percentage (0-100)"`
    Notes      string `json:"notes,omitempty" description:"Additional notes or blockers"`
}

func (t *StatusReportTool) Call(ctx context.Context, input *StatusReportInput) (*dive.ToolResult, error) {
    // Store status in environment or database
    statusReport := fmt.Sprintf(`Status Report:
Agent: %s
Task: %s  
Status: %s
Progress: %d%%
Notes: %s
Timestamp: %s`,
        input.Agent, input.Task, input.Status, input.Progress,
        input.Notes, time.Now().Format(time.RFC3339))

    // Notify supervisors if blocked
    if input.Status == "blocked" {
        // Send notification to supervisor
        notificationText := fmt.Sprintf("Agent %s is blocked on task: %s\nReason: %s",
                                       input.Agent, input.Task, input.Notes)
        
        // Could integrate with Slack, email, or other notification systems
        log.Printf("ALERT: %s", notificationText)
    }

    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: fmt.Sprintf("Status report recorded: %s - %s (%d%%)", 
                             input.Agent, input.Status, input.Progress),
        }},
    }, nil
}
```

### Inter-Agent Messaging

```go
// Message passing between agents
type InterAgentMessage struct {
    From    string      `json:"from"`
    To      string      `json:"to"`
    Subject string      `json:"subject"`
    Content string      `json:"content"`
    Sent    time.Time   `json:"sent"`
}

type MessageTool struct {
    environment dive.Environment
    messages    []InterAgentMessage
    mutex       sync.RWMutex
}

func (t *MessageTool) SendMessage(from, to, subject, content string) error {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    message := InterAgentMessage{
        From:    from,
        To:      to,
        Subject: subject,
        Content: content,
        Sent:    time.Now(),
    }
    
    t.messages = append(t.messages, message)
    
    // Could trigger notification to receiving agent
    return nil
}

func (t *MessageTool) GetMessages(agentName string) []InterAgentMessage {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    var agentMessages []InterAgentMessage
    for _, msg := range t.messages {
        if msg.To == agentName {
            agentMessages = append(agentMessages, msg)
        }
    }
    
    return agentMessages
}
```

## Coordination Strategies

### Sprint Planning

```yaml
# sprint-planning.yaml
Workflows:
  - Name: Sprint Planning
    Inputs:
      - Name: sprint_goal
        Type: string
        Required: true
      - Name: sprint_duration
        Type: integer
        Default: 14
        
    Steps:
      - Name: Define Sprint Goal
        Agent: Product Owner
        Prompt: |
          Define the sprint goal and create user stories for: ${inputs.sprint_goal}
          Sprint duration: ${inputs.sprint_duration} days
        Store: user_stories
        
      - Name: Plan Sprint Tasks
        Agent: Scrum Master
        Prompt: |
          Based on these user stories: ${user_stories}
          
          Coordinate with the development team to:
          1. Break down stories into tasks
          2. Estimate effort for each task
          3. Assign tasks to team members
          4. Identify dependencies and risks
        Store: sprint_plan
        
      - Name: Review and Commit
        Agent: Scrum Master
        Prompt: |
          Review the sprint plan: ${sprint_plan}
          
          Ensure the team can commit to this work and finalize the sprint backlog.
```

### Daily Standup Coordination

```go
func conductDailyStandup(env *environment.Environment) error {
    scrumMaster, err := env.GetAgent("Scrum Master")
    if err != nil {
        return err
    }

    // Collect status from all team members
    teamMembers := []string{
        "Full Stack Developer",
        "Frontend Specialist",
        "Backend Specialist", 
        "QA Automation Engineer",
        "DevOps Engineer",
    }

    var statusReports []string
    for _, member := range teamMembers {
        agent, err := env.GetAgent(member)
        if err != nil {
            continue
        }

        response, err := agent.CreateResponse(
            context.Background(),
            dive.WithInput(`Provide your daily standup update:
                          1. What did you complete yesterday?
                          2. What will you work on today?
                          3. Are there any blockers or impediments?`),
        )
        if err != nil {
            continue
        }

        statusReports = append(statusReports, 
            fmt.Sprintf("%s: %s", member, response.Text()))
    }

    // Scrum Master facilitates and identifies issues
    facilitation, err := scrumMaster.CreateResponse(
        context.Background(),
        dive.WithInput(fmt.Sprintf(`Review these standup updates and identify:
                                   1. Blockers that need attention
                                   2. Coordination opportunities
                                   3. Sprint risks or concerns
                                   
                                   Status Reports:
                                   %s`, strings.Join(statusReports, "\n\n"))),
    )
    if err != nil {
        return err
    }

    fmt.Println("Daily Standup Summary:")
    fmt.Println(facilitation.Text())

    return nil
}
```

### Cross-Team Collaboration

```go
func coordinateCrossTeamWork(env *environment.Environment) error {
    // Architecture decision that affects multiple teams
    architect, err := env.GetAgent("Technical Director")
    if err != nil {
        return err
    }

    decision, err := architect.CreateResponse(
        context.Background(),
        dive.WithInput(`We need to implement a new microservices architecture.
                       
                       Coordinate with team leads to:
                       1. Define service boundaries
                       2. Establish communication protocols
                       3. Plan migration strategy
                       4. Identify team responsibilities`),
    )
    if err != nil {
        return err
    }

    fmt.Println("Architecture Coordination:")
    fmt.Println(decision.Text())

    return nil
}
```

## Best Practices

### 1. Clear Role Definition

```go
// Good: Specific, clear role definitions
func createWellDefinedTeam() {
    // Supervisor with clear coordination responsibilities
    manager := agent.Options{
        Name: "Engineering Manager",
        Instructions: `You are an engineering manager responsible for:
                      - Breaking down complex projects into manageable tasks
                      - Assigning work based on team member expertise
                      - Monitoring progress and removing blockers
                      - Ensuring quality and timeline adherence
                      - Facilitating communication between team members`,
        IsSupervisor: true,
    }

    // Specialist with focused expertise
    backendDev := agent.Options{
        Name: "Backend Developer",
        Instructions: `You are a backend developer specializing in:
                      - API design and implementation
                      - Database schema and optimization
                      - Server-side business logic
                      - Performance optimization
                      - Security best practices`,
    }
}

// Avoid: Vague or overlapping responsibilities
func avoidVagueRoles() {
    // Too vague
    badManager := agent.Options{
        Name:         "Manager",
        Instructions: "You manage things and tell people what to do.",
    }

    // Overlapping responsibilities
    fullStackDev := agent.Options{
        Name:         "Full Stack Developer",
        Instructions: "You do frontend, backend, database, DevOps, and testing work.",
    }
}
```

### 2. Effective Delegation Patterns

```go
func demonstrateGoodDelegation() {
    // Good: Specific task with clear context
    goodDelegation := `Create a user authentication API with the following requirements:
                     
                     Requirements:
                     - JWT token-based authentication
                     - Password hashing with bcrypt
                     - Rate limiting for login attempts
                     - OAuth2 integration (Google, GitHub)
                     
                     Deliverables:
                     - API endpoints documentation
                     - Implementation with error handling
                     - Unit tests with >90% coverage
                     - Security audit checklist
                     
                     Timeline: 3 days
                     Priority: High (blocking other development)`

    // Avoid: Vague or incomplete task description
    badDelegation := "Make a login system"
}
```

### 3. Communication Protocols

```go
// Structured communication between agents
type CommunicationProtocol struct {
    ReportingFrequency time.Duration
    EscalationRules    map[string][]string
    StatusCategories   []string
}

func establishCommunicationProtocols() CommunicationProtocol {
    return CommunicationProtocol{
        ReportingFrequency: time.Hour * 24, // Daily reports
        
        EscalationRules: map[string][]string{
            "blocked":     {"immediate_supervisor", "team_lead"},
            "overdue":     {"immediate_supervisor"},
            "quality_issue": {"team_lead", "architect"},
        },
        
        StatusCategories: []string{
            "not_started",
            "in_progress", 
            "review_ready",
            "completed",
            "blocked",
            "cancelled",
        },
    }
}
```

### 4. Error Handling and Recovery

```go
func handleSupervisorFailures(env *environment.Environment) {
    // Detect if supervisor is unavailable
    supervisor, err := env.GetAgent("Technical Manager")
    if err != nil || supervisor == nil {
        log.Println("Supervisor unavailable, implementing backup coordination")
        
        // Promote senior team member to temporary supervisor role
        seniorDev, err := env.GetAgent("Senior Developer")
        if err == nil {
            // Could implement temporary supervisor promotion
            log.Println("Senior Developer taking temporary coordination role")
        }
    }
    
    // Implement work redistribution
    redistributeWork(env)
}

func redistributeWork(env *environment.Environment) {
    // Get all available agents
    agents := env.Agents()
    
    var availableAgents []dive.Agent
    for _, agent := range agents {
        // Check agent availability (could ping or check last activity)
        availableAgents = append(availableAgents, agent)
    }
    
    // Redistribute pending work among available agents
    log.Printf("Redistributing work among %d available agents", len(availableAgents))
}
```

### 5. Performance Monitoring

```go
type SupervisorMetrics struct {
    TasksAssigned     int
    TasksCompleted    int
    AverageTaskTime   time.Duration
    AgentUtilization  map[string]float64
    BlockedTasks      int
}

func monitorSupervisorPerformance(env *environment.Environment) *SupervisorMetrics {
    metrics := &SupervisorMetrics{
        AgentUtilization: make(map[string]float64),
    }
    
    // Collect metrics from all agents
    agents := env.Agents()
    for _, agent := range agents {
        if agent.IsSupervisor() {
            // Collect supervisor-specific metrics
            // Implementation would track delegation patterns
        } else {
            // Collect subordinate utilization metrics
            // Implementation would track task completion rates
        }
    }
    
    return metrics
}
```

## Next Steps

- [Agent Guide](agents.md) - Learn more about individual agent capabilities
- [Environment Guide](environment.md) - Understand multi-agent environments
- [Workflow Guide](workflows.md) - Orchestrate supervisor patterns in workflows
- [Examples](../examples/advanced.md) - See complex supervisor pattern implementations