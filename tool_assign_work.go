package agent

import (
	"context"
	"encoding/json"

	"github.com/getstingrai/agents/llm"
)

var _ llm.Tool = &AssignWorkTool{}

type AssignWorkInput struct {
	AgentName      string `json:"agent"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ExpectedOutput string `json:"expected_output"`
	OutputFormat   string `json:"output_format"`
	Context        string `json:"context"`
}

type AssignWorkTool struct {
	team *Team
	self Agent
}

func NewAssignWorkTool(team *Team, self Agent) *AssignWorkTool {
	return &AssignWorkTool{team: team, self: self}
}

func (t *AssignWorkTool) Name() string {
	return "AssignWork"
}

func (t *AssignWorkTool) Description() string {
	return "Assigns work to another team member. Provide a complete and detailed request for the agent to fulfill. It will respond with the result of the request."
}

func (t *AssignWorkTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
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

func (t *AssignWorkTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	// var params AssignWorkInput
	// if err := json.Unmarshal(input, &params); err != nil {
	// 	return "", err
	// }
	// if params.AgentName == "" {
	// 	return "", fmt.Errorf("agent name is required")
	// }
	// if params.Name == "" {
	// 	return "", fmt.Errorf("name is required")
	// }
	// if params.Description == "" {
	// 	return "", fmt.Errorf("description is required")
	// }
	// if params.ExpectedOutput == "" {
	// 	return "", fmt.Errorf("expected output is required")
	// }
	// if params.AgentName == t.self.Name() {
	// 	return "", fmt.Errorf("cannot delegate task to self")
	// }
	// agent, ok := t.team.GetAgent(params.AgentName)
	// if !ok {
	// 	return "", fmt.Errorf("agent not found")
	// }
	// outputFormat := OutputFormat(params.OutputFormat)
	// if outputFormat == "" {
	// 	outputFormat = OutputMarkdown
	// }
	// // Generate a dynamic task for the agent
	// task := NewTask(TaskSpec{
	// 	Name:           params.Name,
	// 	Description:    params.Description,
	// 	ExpectedOutput: params.ExpectedOutput,
	// 	Context:        params.Context,
	// 	OutputFormat:   outputFormat,
	// })
	// result := agent.ExecuteTask(ctx, task, nil)
	// if result.Error != nil {
	// 	return fmt.Sprintf("I couldn't complete this work due to the following error: %s", result.Error.Error()), nil
	// }
	// return result.Output.Content, nil
	return "", nil
}
