# Workflow API Reference

Complete API reference for the Dive workflow package, covering workflow creation, execution, and step configuration.

## 📋 Table of Contents

- [Core Interfaces](#core-interfaces)
- [Workflow Structure](#workflow-structure)
- [Step Types](#step-types)
- [Graph Management](#graph-management)
- [Input and Output](#input-and-output)
- [Execution Context](#execution-context)
- [Condition Evaluation](#condition-evaluation)
- [Error Handling](#error-handling)
- [Examples](#examples)

## Core Interfaces

### `workflow.Workflow`

The main workflow structure that defines a repeatable process as a graph of steps.

```go
type Workflow struct {
    name        string
    description string
    path        string
    inputs      []*Input
    output      *Output
    steps       []*Step
    graph       *Graph
    triggers    []*Trigger
}
```

### Constructor

```go
func New(opts Options) (*Workflow, error)
```

Creates a new Workflow with the specified configuration.

**Parameters:**
- `opts` - Configuration options for the workflow

**Returns:**
- `*Workflow` - Configured workflow instance
- `error` - Error if configuration is invalid

### Methods

```go
// Metadata access
func (w *Workflow) Name() string
func (w *Workflow) Description() string
func (w *Workflow) Path() string

// Structure access
func (w *Workflow) Inputs() []*Input
func (w *Workflow) Output() *Output
func (w *Workflow) Steps() []*Step
func (w *Workflow) Triggers() []*Trigger

// Execution access
func (w *Workflow) Start() *Step
func (w *Workflow) Graph() *Graph

// Validation
func (w *Workflow) Validate() error
```

## Workflow Structure

### `workflow.Options`

Configuration structure for creating new workflows.

```go
type Options struct {
    Name        string      // Workflow name (required)
    Description string      // Human-readable description
    Path        string      // File path (for loaded workflows)
    Inputs      []*Input    // Expected input parameters
    Output      *Output     // Expected output specification
    Steps       []*Step     // Workflow steps (required)
    Triggers    []*Trigger  // Event triggers
}
```

### Required Fields

- **`Name`** - Unique identifier for the workflow
- **`Steps`** - At least one step must be provided

### Input Parameters

```go
type Input struct {
    Name        string      `json:"name"`        // Parameter name
    Type        string      `json:"type,omitempty"` // Data type
    Description string      `json:"description,omitempty"` // Human description
    Required    bool        `json:"required,omitempty"` // Whether required
    Default     interface{} `json:"default,omitempty"` // Default value
}
```

**Supported Types:**
- `string` - Text values
- `number` - Numeric values
- `boolean` - True/false values
- `array` - List of values
- `object` - Complex structured data

### Output Specification

```go
type Output struct {
    Name        string      `json:"name"`        // Output name
    Type        string      `json:"type,omitempty"` // Data type
    Description string      `json:"description,omitempty"` // Human description
    Format      string      `json:"format,omitempty"` // Output format
    Default     interface{} `json:"default,omitempty"` // Default value
    Document    string      `json:"document,omitempty"` // Document path
}
```

### Triggers

```go
type Trigger struct {
    Name   string                 // Trigger name
    Type   string                 // Trigger type
    Config map[string]interface{} // Trigger configuration
}
```

**Trigger Types:**
- `webhook` - HTTP webhook trigger
- `schedule` - Cron-based scheduling
- `event` - Event-driven triggers
- `manual` - Manual execution only

## Step Types

### `workflow.Step`

Represents a single step in a workflow execution path.

```go
type Step struct {
    stepType    string
    name        string
    description string
    agent       dive.Agent
    prompt      string
    script      string
    store       string
    action      string
    parameters  map[string]any
    each        *EachBlock
    next        []*Edge
    seconds     float64
    end         bool
    content     []llm.Content
}
```

### Step Constructor

```go
func NewStep(opts StepOptions) *Step

type StepOptions struct {
    Type        string              // Step type
    Name        string              // Step name (required)
    Description string              // Human description
    Agent       dive.Agent          // Agent to execute step
    Prompt      string              // Prompt for agent
    Script      string              // Script to execute
    Store       string              // Variable to store result
    Action      string              // Action to perform
    Parameters  map[string]any      // Action parameters
    Each        *EachBlock          // Iteration configuration
    Next        []*Edge             // Next step edges
    Seconds     float64             // Wait time
    End         bool                // End workflow flag
    Content     []llm.Content       // Additional content
}
```

### Step Methods

```go
// Metadata access
func (s *Step) Type() string
func (s *Step) Name() string
func (s *Step) Description() string

// Execution configuration
func (s *Step) Agent() dive.Agent
func (s *Step) Prompt() string
func (s *Step) Script() string
func (s *Step) Action() string
func (s *Step) Parameters() map[string]any

// Flow control
func (s *Step) Next() []*Edge
func (s *Step) Each() *EachBlock

// Content and storage
func (s *Step) Store() string
func (s *Step) Content() []llm.Content

// Compilation
func (s *Step) Compile(ctx context.Context) error
```

### Step Types

#### Prompt Step
Executes an agent with a specific prompt.

```go
step := NewStep(StepOptions{
    Type:   "prompt",
    Name:   "Analyze Data",
    Agent:  analyst,
    Prompt: "Analyze this dataset: ${inputs.data_path}",
    Store:  "analysis_result",
})
```

#### Action Step
Performs a predefined action.

```go
step := NewStep(StepOptions{
    Type:   "action",
    Name:   "Save Report",
    Action: "Document.Write",
    Parameters: map[string]any{
        "Path":    "reports/analysis.md",
        "Content": "${analysis_result}",
    },
})
```

#### Script Step
Executes a script or command.

```go
step := NewStep(StepOptions{
    Type:   "script",
    Name:   "Run Tests",
    Script: "npm test",
    Store:  "test_results",
})
```

#### Wait Step
Pauses execution for a specified time.

```go
step := NewStep(StepOptions{
    Type:    "wait",
    Name:    "Cooldown",
    Seconds: 30.0,
})
```

### Iteration with Each

```go
type EachBlock struct {
    Items any    // Data to iterate over
    As    string // Variable name for current item
}

// Example: Process each file
step := NewStep(StepOptions{
    Name:   "Process Files",
    Agent:  processor,
    Prompt: "Process this file: ${item.path}",
    Each: &EachBlock{
        Items: "${inputs.file_list}",
        As:    "item",
    },
})
```

## Graph Management

### `workflow.Graph`

Manages the execution flow and validates workflow structure.

```go
type Graph struct {
    steps map[string]*Step
    start *Step
    edges map[string][]*Edge
}

func NewGraph(steps []*Step, start *Step) *Graph
```

### Graph Methods

```go
// Structure access
func (g *Graph) Steps() map[string]*Step
func (g *Graph) Start() *Step
func (g *Graph) Edges() map[string][]*Edge

// Navigation
func (g *Graph) GetStep(name string) *Step
func (g *Graph) NextSteps(stepName string) []*Step

// Validation
func (g *Graph) Validate() error
func (g *Graph) HasCycles() bool
func (g *Graph) IsReachable(stepName string) bool
```

### Edge Conditions

```go
type Edge struct {
    Step      string // Target step name
    Condition string // Condition expression
}
```

**Condition Examples:**
```go
// Unconditional transition
edge := &Edge{Step: "NextStep"}

// Conditional transition
edge := &Edge{
    Step:      "ErrorHandler",
    Condition: "${result.status} == 'error'",
}

// Numeric condition
edge := &Edge{
    Step:      "HighPriority",
    Condition: "${inputs.priority} > 7",
}

// Boolean condition
edge := &Edge{
    Step:      "ProcessData", 
    Condition: "${inputs.has_data}",
}
```

## Input and Output

### Variable Access

Variables are accessible throughout workflow execution using template syntax:

```go
// Input variables
"${inputs.parameter_name}"

// Step results
"${step_name.field_name}"
"${previous_step_result}"

// Workflow metadata
"${workflow.id}"
"${workflow.start_time}"

// Environment variables
"${env.API_KEY}"

// Current context
"${timestamp}"
"${user_id}"
```

### Data Types and Validation

```go
// String input
input := &Input{
    Name:        "user_message",
    Type:        "string",
    Description: "Message from user",
    Required:    true,
}

// Numeric input with constraints
input := &Input{
    Name:        "priority",
    Type:        "number",
    Description: "Priority level (1-10)",
    Required:    true,
    Default:     5,
}

// Array input
input := &Input{
    Name:        "file_paths",
    Type:        "array",
    Description: "List of files to process",
    Required:    false,
    Default:     []string{},
}

// Object input
input := &Input{
    Name:        "config",
    Type:        "object", 
    Description: "Configuration settings",
    Required:    false,
    Default: map[string]interface{}{
        "timeout": 30,
        "retries": 3,
    },
}
```

## Execution Context

### Execution State

During workflow execution, the following context is available:

```go
type ExecutionContext struct {
    WorkflowID   string                 // Unique execution ID
    StartTime    time.Time              // Execution start time
    Inputs       map[string]interface{} // Input parameters
    Variables    map[string]interface{} // Step results
    CurrentStep  string                 // Current step name
    StepHistory  []string               // Executed step names
    UserID       string                 // User identifier
    Environment  dive.Environment       // Runtime environment
}
```

### Variable Storage

```go
// Store step result
step := NewStep(StepOptions{
    Name:   "Calculate",
    Prompt: "Calculate: ${inputs.expression}",
    Store:  "calculation_result",
})

// Use stored result in next step
nextStep := NewStep(StepOptions{
    Name:   "Format",
    Prompt: "Format this result: ${calculation_result}",
    Store:  "formatted_result",
})
```

## Condition Evaluation

### `workflow.Condition`

Interface for custom condition evaluation.

```go
type Condition interface {
    Evaluate(ctx context.Context, inputs map[string]interface{}) (bool, error)
}
```

### Built-in Conditions

```go
// Simple equality
condition := "${result.status} == 'success'"

// Numeric comparison
condition := "${inputs.priority} >= 5"

// String operations
condition := "${user_message} contains 'urgent'"

// Boolean logic
condition := "${has_data} && ${is_valid}"
condition := "${is_error} || ${is_warning}"

// Null/empty checks
condition := "${result} != null"
condition := "${file_path} != ''"

// Array operations
condition := "${file_list} contains 'config.yaml'"
condition := "len(${results}) > 0"
```

### Complex Conditions

```go
// Multi-condition logic
condition := `
    (${inputs.priority} > 7 && ${inputs.category} == 'bug') ||
    (${inputs.priority} > 9)
`

// Nested object access
condition := "${user.permissions.admin} == true"

// Regular expressions
condition := "${filename} matches '^[a-zA-Z0-9_]+\\.json$'"
```

## Error Handling

### Error Types

```go
// Workflow validation errors
type ValidationError struct {
    WorkflowName string
    StepName     string
    Field        string
    Message      string
}

// Execution errors
type ExecutionError struct {
    WorkflowID   string
    StepName     string
    Cause        error
    Context      map[string]interface{}
}

// Condition evaluation errors
type ConditionError struct {
    Expression string
    Context    map[string]interface{}
    Cause      error
}
```

### Error Recovery

```go
// Error handling step
errorHandler := NewStep(StepOptions{
    Name:   "Handle Error",
    Type:   "action",
    Action: "Log.Error", 
    Parameters: map[string]any{
        "Message": "Workflow failed: ${error.message}",
        "Context": "${error.context}",
    },
})

// Retry logic
retryStep := NewStep(StepOptions{
    Name:   "Retry Operation",
    Agent:  agent,
    Prompt: "Retry the previous operation with: ${inputs.retry_params}",
    Next: []*Edge{
        {Step: "Success", Condition: "${result.status} == 'success'"},
        {Step: "Final_Failure", Condition: "${retry_count} >= 3"},
        {Step: "Retry Operation", Condition: "true"}, // Default retry
    },
})
```

## Examples

### Basic Workflow

```go
package main

import (
    "github.com/diveagents/dive/workflow"
)

func createBasicWorkflow() (*workflow.Workflow, error) {
    // Define inputs
    inputs := []*workflow.Input{
        {
            Name:        "message",
            Type:        "string",
            Description: "User message to process",
            Required:    true,
        },
    }
    
    // Define output
    output := &workflow.Output{
        Name:        "response",
        Type:        "string",
        Description: "Processed response",
        Format:      "text",
    }
    
    // Create steps
    processStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Process Message",
        Type:   "prompt",
        Agent:  assistant,
        Prompt: "Process this message: ${inputs.message}",
        Store:  "processed_result",
    })
    
    formatStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Format Response",
        Type:   "prompt", 
        Agent:  formatter,
        Prompt: "Format this response: ${processed_result}",
        Store:  "final_response",
        End:    true,
    })
    
    // Define step flow
    processStep.Next = []*workflow.Edge{
        {Step: "Format Response"},
    }
    
    return workflow.New(workflow.Options{
        Name:        "Message Processor",
        Description: "Processes user messages",
        Inputs:      inputs,
        Output:      output,
        Steps:       []*workflow.Step{processStep, formatStep},
    })
}
```

### Conditional Workflow

```go
func createConditionalWorkflow() (*workflow.Workflow, error) {
    // Input validation step
    validateStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Validate Input",
        Type:   "prompt",
        Agent:  validator,
        Prompt: "Validate this data: ${inputs.data}",
        Store:  "validation_result",
        Next: []*workflow.Edge{
            {
                Step:      "Process Valid Data", 
                Condition: "${validation_result.valid} == true",
            },
            {
                Step:      "Handle Invalid Data",
                Condition: "${validation_result.valid} == false",
            },
        },
    })
    
    // Valid data processing
    processValidStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Process Valid Data",
        Type:   "prompt",
        Agent:  processor,
        Prompt: "Process: ${inputs.data}",
        Store:  "process_result",
        End:    true,
    })
    
    // Invalid data handling
    handleInvalidStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Handle Invalid Data",
        Type:   "action",
        Action: "Log.Warning",
        Parameters: map[string]any{
            "Message": "Invalid data: ${validation_result.errors}",
        },
        End: true,
    })
    
    return workflow.New(workflow.Options{
        Name: "Conditional Processor",
        Steps: []*workflow.Step{
            validateStep,
            processValidStep, 
            handleInvalidStep,
        },
    })
}
```

### Iterative Workflow

```go
func createIterativeWorkflow() (*workflow.Workflow, error) {
    // Process each item
    processItemStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Process Item",
        Type:   "prompt",
        Agent:  itemProcessor,
        Prompt: "Process this item: ${item.data}",
        Store:  "item_results",
        Each: &workflow.EachBlock{
            Items: "${inputs.item_list}",
            As:    "item",
        },
    })
    
    // Aggregate results
    aggregateStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Aggregate Results",
        Type:   "prompt",
        Agent:  aggregator,
        Prompt: "Combine these results: ${item_results}",
        Store:  "final_aggregate",
        End:    true,
    })
    
    // Connect steps
    processItemStep.Next = []*workflow.Edge{
        {Step: "Aggregate Results"},
    }
    
    return workflow.New(workflow.Options{
        Name: "Batch Processor",
        Inputs: []*workflow.Input{
            {
                Name:     "item_list",
                Type:     "array",
                Required: true,
            },
        },
        Steps: []*workflow.Step{processItemStep, aggregateStep},
    })
}
```

### Multi-Agent Workflow

```go
func createMultiAgentWorkflow() (*workflow.Workflow, error) {
    // Research phase
    researchStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Research Topic",
        Agent:  researcher,
        Prompt: "Research this topic: ${inputs.topic}",
        Store:  "research_data",
    })
    
    // Analysis phase  
    analysisStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Analyze Research",
        Agent:  analyst,
        Prompt: "Analyze this research: ${research_data}",
        Store:  "analysis_results",
    })
    
    // Writing phase
    writingStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Write Report",
        Agent:  writer,
        Prompt: "Write a report based on: ${analysis_results}",
        Store:  "draft_report",
    })
    
    // Review phase
    reviewStep := workflow.NewStep(workflow.StepOptions{
        Name:   "Review Report",
        Agent:  reviewer,
        Prompt: "Review and improve: ${draft_report}",
        Store:  "final_report",
        End:    true,
    })
    
    // Connect steps sequentially
    researchStep.Next = []*workflow.Edge{{Step: "Analyze Research"}}
    analysisStep.Next = []*workflow.Edge{{Step: "Write Report"}}
    writingStep.Next = []*workflow.Edge{{Step: "Review Report"}}
    
    return workflow.New(workflow.Options{
        Name: "Research Pipeline",
        Inputs: []*workflow.Input{
            {Name: "topic", Type: "string", Required: true},
        },
        Steps: []*workflow.Step{
            researchStep,
            analysisStep,
            writingStep,
            reviewStep,
        },
    })
}
```

This comprehensive API reference covers all aspects of the workflow package, enabling developers to create sophisticated multi-step processes with conditional logic, iteration, and multi-agent collaboration.