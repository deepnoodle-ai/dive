package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
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
	Self *Agent

	// DefaultTaskTimeout is the default timeout for tasks assigned using this tool
	DefaultTaskTimeout time.Duration
}

// AssignWorkTool is a tool that can be used to assign a task to another agent.
// The tool call blocks until the work is complete. The result of the call is
// the output of the task.
type AssignWorkTool struct {
	self               *Agent
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

func (t *AssignWorkTool) Name() string {
	return "assign_work"
}

func (t *AssignWorkTool) Description() string {
	return AssignWorkToolDescription
}

func (t *AssignWorkTool) Schema() llm.Schema {
	return llm.Schema{
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
	}
}

func (t *AssignWorkTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	var params AssignWorkToolInput
	if err := json.Unmarshal([]byte(input.Input), &params); err != nil {
		return nil, err
	}

	if params.AgentName == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if params.ExpectedOutput == "" {
		return nil, fmt.Errorf("expected output is required")
	}
	if params.AgentName == t.self.Name() {
		return nil, fmt.Errorf("cannot delegate task to self")
	}
	agent, err := t.self.environment.GetAgent(params.AgentName)
	if err != nil {
		return nil, fmt.Errorf("I couldn't find an agent named %q", params.AgentName)
	}

	outputFormat := dive.OutputFormat(params.OutputFormat)
	if outputFormat == "" {
		outputFormat = dive.OutputFormatMarkdown
	}

	var promptLines []string
	if params.Context != "" {
		promptLines = append(promptLines, "Context:")
		promptLines = append(promptLines, fmt.Sprintf("<context>\n%s\n</context>", params.Context))
	}
	if params.Description != "" {
		promptLines = append(promptLines, "Please complete the following task:")
		promptLines = append(promptLines, fmt.Sprintf("<task>\n%s\n</task>", params.Description))
	}
	if params.ExpectedOutput != "" {
		promptLines = append(promptLines, "Expected output: "+params.ExpectedOutput)
	}
	if params.OutputFormat != "" {
		promptLines = append(promptLines, "Desired output format: "+params.OutputFormat)
	}
	promptLines = append(promptLines, "Please work on the task now and respond with the requested output only.")

	prompt := strings.Join(promptLines, "\n\n")

	stream, err := agent.StreamResponse(ctx, dive.WithInput(prompt))
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	for stream.Next(ctx) {
		event := stream.Event()
		if event.Error != nil {
			return nil, event.Error
		}
		if event.Type == dive.EventTypeResponseCompleted {
			for _, item := range event.Response.Items {
				if item.Type == dive.ResponseItemTypeMessage {
					return &llm.ToolCallOutput{
						Output: item.Message.Text(),
					}, nil
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	// We shouldn't reach this point. The agent should have returned the result
	// or an error instead.
	return nil, errors.New("agent did not return a result from a work assignment")
}

func (t *AssignWorkTool) ShouldReturnResult() bool {
	return true
}
