package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*AssignWorkToolInput] = &AssignWorkTool{}

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
	Self dive.Agent

	// Others contains the other agents that can be assigned work to.
	Others []dive.Agent

	// DefaultTaskTimeout is the default timeout for tasks assigned using this tool
	DefaultTaskTimeout time.Duration
}

// AssignWorkTool is a tool that can be used to assign a task to another agent.
// The tool call blocks until the work is complete. The result of the call is
// the output of the task.
type AssignWorkTool struct {
	self               dive.Agent
	others             []dive.Agent
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
		others:             opts.Others,
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

func (t *AssignWorkTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Required: []string{
			"agent",
			"name",
			"description",
			"expected_output",
		},
		Properties: map[string]*schema.Property{
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

func (t *AssignWorkTool) Annotations() *dive.ToolAnnotations {
	// This tool may indirectly be destructive or non-read-only, however this
	// action itself is not. Downstream tool calls will need to be checked for
	// safety.
	return &dive.ToolAnnotations{
		Title:           "Assign Work",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *AssignWorkTool) Call(ctx context.Context, input *AssignWorkToolInput) (*dive.ToolResult, error) {
	if input.AgentName == "" {
		return dive.NewToolResultError("agent name is required"), nil
	}
	if input.Name == "" {
		return dive.NewToolResultError("name is required"), nil
	}
	if input.Description == "" {
		return dive.NewToolResultError("description is required"), nil
	}
	if input.ExpectedOutput == "" {
		return dive.NewToolResultError("expected output is required"), nil
	}
	if input.AgentName == t.self.Name() {
		return dive.NewToolResultError("cannot delegate task to self"), nil
	}
	var assignee dive.Agent
	for _, other := range t.others {
		if other.Name() == input.AgentName {
			assignee = other
			break
		}
	}
	if assignee == nil {
		return dive.NewToolResultError("agent not found"), nil
	}
	prompt := strings.Builder{}
	prompt.WriteString(input.Description)
	prompt.WriteString("\n\n")
	if input.ExpectedOutput != "" {
		prompt.WriteString("<expected_output>\n")
		prompt.WriteString("Please respond with the following:\n")
		prompt.WriteString(input.ExpectedOutput)
		prompt.WriteString("</expected_output>")
		prompt.WriteString("\n\n")
	}
	if input.OutputFormat != "" {
		prompt.WriteString("<output_format>\n")
		prompt.WriteString("Please respond with the following format:\n")
		prompt.WriteString(string(input.OutputFormat))
		prompt.WriteString("</output_format>")
		prompt.WriteString("\n\n")
	}

	message := &llm.Message{Role: llm.User}
	message.Content = append(message.Content, &llm.TextContent{Text: prompt.String()})

	if input.Context != "" {
		context := fmt.Sprintf("<context>\n%s\n</context>", input.Context)
		message.Content = append(message.Content, &llm.TextContent{Text: context})
	}

	response, err := assignee.CreateResponse(ctx, dive.WithMessage(message))
	if err != nil {
		return dive.NewToolResultError("failed to create response: " + err.Error()), nil
	}
	return dive.NewToolResultText(response.OutputText()), nil
}

func (t *AssignWorkTool) ShouldReturnResult() bool {
	return true
}
