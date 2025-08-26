# Workflow Guide

Workflows in Dive enable you to create declarative, multi-step processes that orchestrate agents, execute scripts, and manage complex automation tasks. They provide a reliable, event-driven approach to building AI-powered pipelines.

## 📋 Table of Contents

- [What is a Workflow?](#what-is-a-workflow)
- [Basic Workflow Structure](#basic-workflow-structure)
- [Workflow Steps](#workflow-steps)
- [State Management](#state-management)
- [Conditional Logic](#conditional-logic)
- [Script Integration](#script-integration)
- [Actions and Operations](#actions-and-operations)
- [Error Handling](#error-handling)
- [Event Streaming](#event-streaming)
- [Best Practices](#best-practices)

## What is a Workflow?

A workflow is a declarative YAML definition that describes a sequence of steps to accomplish a goal. Workflows can:

- **Orchestrate agents** to perform research, analysis, or communication tasks
- **Execute scripts** for custom logic and data processing
- **Manage state** across multiple steps and parallel execution paths
- **Make decisions** using conditional branching
- **Interact with external systems** through actions and tools
- **Provide reliability** through checkpoint-based execution

Think of workflows as blueprints for automated processes that can be version-controlled, tested, and shared across teams.

## Basic Workflow Structure

### Minimal Workflow

```yaml
# basic-workflow.yaml
Name: Simple Research
Description: Research a topic and save results

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514

Agents:
  - Name: Researcher
    Instructions: You are a thorough researcher who finds accurate information.
    Tools:
      - web_search

Workflows:
  - Name: Research
    Inputs:
      - Name: topic
        Type: string
        Description: The topic to research
    Steps:
      - Name: Research Topic
        Agent: Researcher
        Prompt: "Research the following topic: ${inputs.topic}"
        Store: research_results
```

### Complete Workflow Example

```yaml
# advanced-workflow.yaml
Name: Content Creation Pipeline
Description: Research, write, and review content

Config:
  LogLevel: info
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  ConfirmationMode: if-destructive

Agents:
  - Name: Researcher
    Instructions: You are a thorough researcher.
    Tools: [web_search, fetch]
  
  - Name: Writer  
    Instructions: You create engaging, well-structured content.
    
  - Name: Editor
    Instructions: You review and improve written content.

Workflows:
  - Name: Create Article
    Inputs:
      - Name: topic
        Type: string
        Required: true
      - Name: word_count
        Type: integer
        Default: 1000
    
    Output:
      Name: article
      Type: string
      Format: markdown
      Document: articles/${inputs.topic}.md
    
    Steps:
      - Name: Research Phase
        Agent: Researcher
        Prompt: |
          Research the topic: ${inputs.topic}
          Focus on recent developments and key insights.
        Store: research_data
        
      - Name: Content Creation
        Agent: Writer
        Prompt: |
          Write a ${inputs.word_count}-word article about ${inputs.topic}.
          Use this research: ${research_data}
        Store: draft_article
        
      - Name: Editorial Review
        Agent: Editor
        Prompt: |
          Review and improve this article:
          ${draft_article}
          
          Ensure it's well-structured and engaging.
        Store: final_article
        
      - Name: Save Article
        Action: Document.Write
        Parameters:
          Path: articles/${inputs.topic}.md
          Content: ${final_article}
```

## Workflow Steps

### Agent Steps

Agent steps delegate tasks to configured agents:

```yaml
Steps:
  - Name: Analyze Data
    Agent: Data Analyst
    Prompt: |
      Analyze this dataset and identify trends:
      ${state.raw_data}
    Store: analysis_results
    
    # Optional: Override agent settings for this step
    ModelSettings:
      Temperature: 0.1
      ReasoningEffort: high
```

### Script Steps

Script steps execute custom logic using the Risor scripting language:

```yaml
Steps:
  - Name: Process Data
    Type: script
    Script: |
      // Transform the data
      processed_data = []
      for item in state.raw_data {
          if item.score > 0.8 {
              processed_data.append({
                  "name": item.name,
                  "category": item.category.upper(),
                  "processed_at": time.now()
              })
          }
      }
      return processed_data
    Store: processed_data
```

### Action Steps

Action steps perform system operations:

```yaml
Steps:
  - Name: Save Results
    Action: Document.Write
    Parameters:
      Path: results/${inputs.session_id}.json
      Content: ${state.final_results}
      
  - Name: Send Notification
    Action: HTTP.Post  
    Parameters:
      URL: https://api.slack.com/webhooks/...
      Body: |
        {
          "text": "Workflow completed: ${workflow.name}",
          "data": ${state.summary}
        }
```

## State Management

Workflows maintain state throughout execution:

### Input Access

```yaml
# Access workflow inputs
Prompt: "Process the file: ${inputs.filename}"

# Use inputs in conditions
Condition: inputs.mode == "production"

# Transform inputs in scripts
Script: |
  config = {
    "environment": inputs.env.upper(),
    "debug": inputs.env == "development"
  }
  return config
```

### Step Outputs

```yaml
Steps:
  - Name: Fetch Data
    Agent: Data Fetcher
    Prompt: "Get latest data for ${inputs.dataset}"
    Store: raw_data  # Available as ${state.raw_data} or ${raw_data}
    
  - Name: Process Data
    Type: script
    Script: |
      # Access previous step output
      filtered = []
      for item in state.raw_data {
          if item.status == "active" {
              filtered.append(item)
          }
      }
      return filtered
    Store: processed_data
    
  - Name: Generate Report
    Agent: Reporter
    Prompt: |
      Create a report based on:
      Raw data count: ${len(raw_data)}
      Processed data: ${processed_data}
```

### State Persistence

State is automatically checkpointed after each step:

```yaml
Config:
  # Checkpoint frequency (default: after each step)
  CheckpointMode: step
  
  # Enable state debugging
  StateLogging: true
```

## Conditional Logic

### Step Conditions

```yaml
Steps:
  - Name: Check Quality
    Agent: Quality Checker
    Prompt: "Rate the quality of: ${state.content}"
    Store: quality_score
    
  - Name: Improve Content
    Agent: Content Improver
    Prompt: "Improve this content: ${state.content}"
    Store: improved_content
    # Only run if quality is low
    Condition: state.quality_score < 7
    
  - Name: Approve Content
    Agent: Approver  
    Prompt: "Review final content: ${state.improved_content || state.content}"
    # Run if we improved it, or if original quality was good
    Condition: state.improved_content || state.quality_score >= 7
```

### Conditional Branching

```yaml
Steps:
  - Name: Route Processing
    Type: script
    Script: |
      if inputs.data_type == "image" {
          return "process_image"
      } else if inputs.data_type == "text" {
          return "process_text"  
      } else {
          return "unknown_type"
      }
    Store: processing_path
    
  - Name: Process Image
    Agent: Image Processor
    Condition: state.processing_path == "process_image"
    # ... image processing logic
    
  - Name: Process Text
    Agent: Text Processor  
    Condition: state.processing_path == "process_text"
    # ... text processing logic
```

### Loop Patterns

```yaml
Steps:
  - Name: Process Items
    Type: script
    Script: |
      results = []
      for item in inputs.items {
          # Process each item
          result = process_item(item)
          results.append(result)
      }
      return results
    Store: processed_items
```

## Script Integration

### Script Categories

Dive supports three types of scripts with different capabilities:

#### 1. Conditional Scripts (Deterministic)
```yaml
- Name: Check Status
  Condition: |
    state.completed_count >= state.target_count &&
    state.error_count < state.max_errors
```

#### 2. Template Scripts (Deterministic)  
```yaml
- Name: Generate Prompt
  Agent: Assistant
  Prompt: |
    Process ${format_item(state.current_item)}.
    Context: ${build_summary(state.previous_results)}
    Priority: ${inputs.priority.upper()}
```

#### 3. Activity Scripts (Non-deterministic)
```yaml
- Name: Fetch External Data
  Type: script
  Script: |
    // Can make HTTP calls, read files, etc.
    response = http.get("https://api.example.com/data")
    return json.parse(response.body)
  Store: external_data
```

### Available Functions

Scripts have access to built-in functions:

```yaml
Script: |
  // String operations
  text = "hello world"
  upper_text = text.upper()
  
  // JSON operations  
  data = json.parse(state.json_string)
  json_output = json.stringify(data)
  
  // HTTP operations (activity scripts only)
  response = http.get("https://api.example.com")
  post_result = http.post("https://webhook.site", {"key": "value"})
  
  // File operations (activity scripts only)  
  content = file.read("./data.txt")
  file.write("./output.txt", "processed data")
  
  // Time operations (activity scripts only)
  now = time.now()
  formatted = time.format(now, "2006-01-02")
  
  // Array operations
  items = [1, 2, 3, 4, 5]
  filtered = items.filter(func(x) { x > 3 })
  mapped = items.map(func(x) { x * 2 })
  
  // Document repository access
  doc = documents.read("analysis-report.md")
  documents.write("summary.txt", summary_text)
```

## Actions and Operations

### Built-in Actions

```yaml
Steps:
  # Document operations
  - Name: Save Report
    Action: Document.Write
    Parameters:
      Path: reports/analysis-${inputs.date}.md
      Content: ${state.final_report}
      
  - Name: Load Template
    Action: Document.Read
    Parameters:
      Path: templates/report-template.md
    Store: template_content
    
  # HTTP operations
  - Name: Post Results
    Action: HTTP.Post
    Parameters:
      URL: ${inputs.webhook_url}
      Headers:
        Content-Type: application/json
        Authorization: Bearer ${inputs.api_token}
      Body: |
        {
          "workflow": "${workflow.name}",
          "status": "completed",
          "results": ${state.final_results}
        }
        
  # Command execution
  - Name: Run Analysis
    Action: Command.Execute
    Parameters:
      Command: python
      Args: [analysis.py, --input, ${inputs.data_file}]
      WorkingDir: ./scripts
    Store: analysis_output
```

### Custom Actions

Create custom actions by implementing the Action interface:

```go
type SlackNotificationAction struct {
    WebhookURL string
}

func (a *SlackNotificationAction) Name() string {
    return "Slack.Notify"
}

func (a *SlackNotificationAction) Execute(ctx context.Context, params map[string]interface{}) (*ActionResult, error) {
    // Implementation...
}

// Register in environment
env, err := environment.New(environment.Options{
    Actions: []environment.Action{
        &SlackNotificationAction{WebhookURL: "..."},
    },
})
```

## Error Handling

### Step-Level Error Handling

```yaml
Steps:
  - Name: Risky Operation
    Agent: Data Processor
    Prompt: "Process this complex data: ${state.raw_data}"
    Store: processed_data
    
    # Continue workflow even if this step fails
    IgnoreErrors: true
    
    # Retry configuration
    Retry:
      MaxAttempts: 3
      BackoffDelay: 5s
      
  - Name: Handle Errors
    Agent: Error Handler
    Prompt: "Handle any processing errors: ${state.errors}"
    # Only run if previous step had errors
    Condition: len(state.errors) > 0
```

### Workflow-Level Error Handling

```yaml
Config:
  # How to handle step failures
  ErrorMode: continue  # continue, stop, or retry
  
  # Maximum retry attempts
  MaxRetries: 3
  
  # Confirmation for destructive operations
  ConfirmationMode: if-destructive
```

### Error Recovery

```yaml
Steps:
  - Name: Validate Data
    Type: script
    Script: |
      if !state.data || len(state.data) == 0 {
          throw "No data available for processing"
      }
      return true
    Store: validation_result
    
  - Name: Fallback Data Source
    Agent: Data Fetcher
    Prompt: "Fetch backup data from alternative source"
    Store: backup_data
    # Only run if validation failed
    Condition: !state.validation_result
```

## Event Streaming

### Monitoring Workflow Execution

```go
// Monitor workflow events
stream, err := env.StreamWorkflow(ctx, "workflow-name", inputs)
if err != nil {
    panic(err)
}

for event := range stream.Events() {
    switch event.Type {
    case "execution.started":
        fmt.Printf("Workflow started: %s\n", event.ExecutionID)
        
    case "step.started":
        fmt.Printf("Step started: %s\n", event.StepName)
        
    case "step.completed":
        fmt.Printf("Step completed: %s (output: %v)\n", 
                   event.StepName, event.StepOutput)
                   
    case "execution.completed":
        fmt.Printf("Workflow completed: %s\n", event.ExecutionID)
        
    case "execution.failed":
        fmt.Printf("Workflow failed: %s (error: %v)\n", 
                   event.ExecutionID, event.Error)
    }
}
```

### Event Types

Workflows emit detailed events throughout execution:

- `execution.started` - Workflow begins
- `step.started` - Individual step begins  
- `step.completed` - Step finishes successfully
- `step.failed` - Step encounters error
- `state.updated` - Workflow state changes
- `checkpoint.created` - State checkpoint saved
- `execution.completed` - Workflow finishes
- `execution.failed` - Workflow encounters fatal error

## Best Practices

### 1. Clear Structure

```yaml
# Good: Well-organized with clear sections
Name: Data Processing Pipeline
Description: Processes customer data and generates insights

Config:
  DefaultProvider: anthropic
  LogLevel: info

Agents:
  - Name: Data Validator
    Instructions: Validate data quality and completeness
  - Name: Data Processor  
    Instructions: Transform and analyze data

Workflows:
  - Name: Process Customer Data
    # Clear, descriptive steps
    Steps:
      - Name: Validate Input Data
      - Name: Clean and Transform
      - Name: Generate Insights
      - Name: Create Report
```

### 2. Effective State Management

```yaml
# Good: Descriptive variable names
Steps:
  - Name: Fetch User Data
    Store: user_profiles  # Clear what this contains
    
  - Name: Calculate Metrics
    Store: user_metrics   # Clear transformation
    
  - Name: Generate Report
    Store: final_report   # Clear end result

# Avoid: Generic variable names
Steps:
  - Name: Step 1
    Store: data1  # Unclear what this is
    
  - Name: Step 2  
    Store: result # Unclear what was processed
```

### 3. Error Resilience

```yaml
Steps:
  - Name: External API Call
    Type: script
    Script: |
      try {
          response = http.get("https://api.external.com/data")
          return json.parse(response.body)
      } catch (error) {
          // Graceful fallback
          return {"error": error.message, "fallback": true}
      }
    Store: external_data
    
  - Name: Handle API Failure
    Condition: state.external_data.fallback
    Agent: Fallback Handler
    Prompt: "Handle API failure: ${state.external_data.error}"
```

### 4. Conditional Logic

```yaml
# Good: Clear, readable conditions
Steps:
  - Name: Quality Check
    Condition: |
      state.quality_score >= 8 && 
      state.completeness_score >= 0.9 &&
      !state.has_errors
      
# Avoid: Complex, unclear conditions  
Steps:
  - Name: Complex Check
    Condition: |
      (state.a > 5 && state.b < 10) || (state.c == "x" && state.d != null && len(state.e) > 0)
```

### 5. Documentation

```yaml
# Document your workflows
Name: Customer Onboarding
Description: |
  Automated customer onboarding process that:
  1. Validates customer information
  2. Creates accounts across systems
  3. Sends welcome communications
  4. Schedules follow-up activities

Inputs:
  - Name: customer_data
    Type: object
    Description: Customer information from signup form
    Required: true
    
  - Name: service_tier
    Type: string
    Description: Service level (basic, premium, enterprise)
    Default: basic

Steps:
  - Name: Validate Customer Data
    Description: Ensures all required fields are present and valid
    # ... step definition
```

## Next Steps

- [CLI Reference](../reference/cli.md) - Run workflows from command line
- [Event Streaming](event-streaming.md) - Monitor workflow execution
- [Custom Actions](custom-actions.md) - Extend workflow capabilities
- [API Reference](../api/workflow.md) - Detailed API documentation