package dive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/getstingrai/dive/llm"
)

var _ llm.Tool = &AssignWorkTool{}

// AssignWorkToolInput is the input for the AssignWorkTool.
type AssignWorkToolInput struct {
	AgentName      string `json:"agent"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ExpectedOutput string `json:"expected_output"`
	OutputFormat   string `json:"output_format"`
	Context        string `json:"context"`
}

// AssignWorkToolOptions is used to configure a new AssignWorkTool.
type AssignWorkToolOptions struct {
	// Self indicates which agent owns this tool
	Self Agent

	// DefaultTaskTimeout is the default timeout for tasks assigned using this tool
	DefaultTaskTimeout time.Duration
}

// AssignWorkTool is a tool that can be used to assign a task to another agent.
// The tool call blocks until the work is complete. The result of the call is
// the output of the task.
type AssignWorkTool struct {
	self               Agent
	defaultTaskTimeout time.Duration
}

// NewAssignWorkTool creates a new AssignWorkTool with the given agent as the
// tool's owner. This is used to make sure we don't assign work to ourselves.
// The default task timeout is set to 5 minutes if not specified.
func NewAssignWorkTool(opts AssignWorkToolOptions) *AssignWorkTool {
	if opts.DefaultTaskTimeout <= 0 {
		opts.DefaultTaskTimeout = 5 * time.Minute
	}
	return &AssignWorkTool{
		self:               opts.Self,
		defaultTaskTimeout: opts.DefaultTaskTimeout,
	}
}

var AssignWorkToolDescription = `Assigns work to another team member. Provide a complete and detailed request for the agent to fulfill. It will respond with the result of the request. You must assume the response is not visible to anyone else, so you are responsible for relaying the information in your own responses as needed. The team member you are assigning work to may have limited or no situational context, so provide them with any relevant information you have available using the "context" parameter. Keep the tasks focused and avoid asking for multiple things at once.`

func (t *AssignWorkTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "assign_work",
		Description: AssignWorkToolDescription,
		Parameters: llm.Schema{
			Type: "object",
			Required: []string{
				"agent",
				"name",
				"description",
				"expected_output",
			},
			Properties: map[string]*llm.SchemaProperty{
				"agent": {
					Type:        "string",
					Description: "The name of the agent that should do the work.",
				},
				"name": {
					Type:        "string",
					Description: "The name of the job to be done (e.g. 'Research Company Reviews').",
				},
				"description": {
					Type:        "string",
					Description: "The complete description of the job to be done (e.g. 'Find reviews for a company online').",
				},
				"expected_output": {
					Type:        "string",
					Description: "What the output of the work should look like (e.g. a list of URLs, a list of companies, etc.)",
				},
				"output_format": {
					Type:        "string",
					Description: "The desired output format: text, markdown, or json (optional).",
				},
				"context": {
					Type:        "string",
					Description: "Any additional context that may be relevant and aid the agent in completing the work (optional).",
				},
			},
		},
	}
}

func (t *AssignWorkTool) Call(ctx context.Context, input string) (string, error) {
	var params AssignWorkToolInput
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}

	if params.AgentName == "" {
		return "", fmt.Errorf("agent name is required")
	}
	if params.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if params.Description == "" {
		return "", fmt.Errorf("description is required")
	}
	if params.ExpectedOutput == "" {
		return "", fmt.Errorf("expected output is required")
	}
	if params.AgentName == t.self.Name() {
		return "", fmt.Errorf("cannot delegate task to self")
	}
	agent, ok := t.self.Team().GetAgent(params.AgentName)
	if !ok {
		return fmt.Sprintf("I couldn't find an agent named %q", params.AgentName), nil
	}
	outputFormat := OutputFormat(params.OutputFormat)
	if outputFormat == "" {
		outputFormat = OutputMarkdown
	}

	// Capture this request as a new task
	task := NewTask(TaskOptions{
		Name:           params.Name,
		Description:    params.Description,
		ExpectedOutput: params.ExpectedOutput,
		Context:        params.Context,
		OutputFormat:   outputFormat,
		AssignedAgent:  agent,
		Kind:           "assignment",
		Timeout:        t.defaultTaskTimeout,
	})

	// Tell the agent to work on the task
	stream, err := agent.Work(ctx, task)
	if err != nil {
		return fmt.Sprintf("This assignment could not be started: %s", err.Error()), nil
	}
	defer stream.Close()

	// Wait for the result or an error
	done := false
	for !done {
		select {
		case event, ok := <-stream.Channel():
			if !ok {
				done = true
				break
			}
			if event.Error != "" {
				return fmt.Sprintf("I encountered an error: %s", event.Error), nil
			}
			if event.TaskResult != nil {
				return event.TaskResult.Content, nil
			}
			fmt.Println("assign work event", event.Type)
		case <-ctx.Done():
			return fmt.Sprintf("My work timed out: %s", ctx.Err()), nil
		}
	}

	// We shouldn't reach this point. The agent should have returned the result
	// or an error instead.
	return "", errors.New("agent did not return a result from a work assignment")
}

func (t *AssignWorkTool) ShouldReturnResult() bool {
	return true
}
