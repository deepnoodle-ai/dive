package toolkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*AskUserInput] = &AskUserTool{}
var _ dive.TypedToolPreviewer[*AskUserInput] = &AskUserTool{}

// AskUserInputOption represents an option for selection-type questions.
type AskUserInputOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// AskUserInput is the input for the ask_user tool.
type AskUserInput struct {
	Question  string               `json:"question"`
	Type      string               `json:"type"` // "confirm", "select", "multiselect", "input"
	Options   []AskUserInputOption `json:"options,omitempty"`
	Default   string               `json:"default,omitempty"`
	MinSelect int                  `json:"min_select,omitempty"`
	MaxSelect int                  `json:"max_select,omitempty"`
	Multiline bool                 `json:"multiline,omitempty"`
}

// AskUserOutput is the output from the ask_user tool.
type AskUserOutput struct {
	Response string   `json:"response,omitempty"`
	Values   []string `json:"values,omitempty"`
	Canceled bool     `json:"canceled"`
}

// AskUserTool is a tool that allows the LLM to ask the user questions.
type AskUserTool struct {
	interactor dive.UserInteractor
}

// AskUserToolOptions configures the AskUserTool.
type AskUserToolOptions struct {
	Interactor dive.UserInteractor
}

// NewAskUserTool creates a new tool for asking the user questions.
// An explicit Interactor must be provided; without one, the tool will fail at call time.
func NewAskUserTool(opts AskUserToolOptions) *dive.TypedToolAdapter[*AskUserInput] {
	return dive.ToolAdapter(&AskUserTool{
		interactor: opts.Interactor,
	})
}

func (t *AskUserTool) Name() string {
	return "AskUser"
}

func (t *AskUserTool) Description() string {
	return `Ask the user a question and get their response. Use this tool when you need to:
- Confirm an action before proceeding (type: "confirm")
- Get the user to choose from a list of options (type: "select")
- Get the user to select multiple options (type: "multiselect")
- Get free-form text input from the user (type: "input")

The tool will return the user's response or indicate if they canceled.`
}

func (t *AskUserTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"question", "type"},
		Properties: map[string]*schema.Property{
			"question": {
				Type:        "string",
				Description: "The question to ask the user",
			},
			"type": {
				Type:        "string",
				Enum:        []any{"confirm", "select", "multiselect", "input"},
				Description: "The type of question: 'confirm' for yes/no, 'select' for single choice, 'multiselect' for multiple choices, 'input' for free-form text",
			},
			"options": {
				Type:        "array",
				Description: "Options for 'select' and 'multiselect' types. Each option should have 'value' (returned if selected), 'label' (displayed to user), and optionally 'description' and 'default'",
				Items: &schema.Property{
					Type: "object",
					Properties: map[string]*schema.Property{
						"value": {
							Type:        "string",
							Description: "The value returned if this option is selected",
						},
						"label": {
							Type:        "string",
							Description: "The label displayed to the user",
						},
						"description": {
							Type:        "string",
							Description: "Optional description for this option",
						},
						"default": {
							Type:        "boolean",
							Description: "Whether this option is selected by default",
						},
					},
				},
			},
			"default": {
				Type:        "string",
				Description: "Default value for 'input' type, or 'true'/'false' for 'confirm' type",
			},
			"min_select": {
				Type:        "integer",
				Description: "Minimum number of selections for 'multiselect' type (default: 0)",
			},
			"max_select": {
				Type:        "integer",
				Description: "Maximum number of selections for 'multiselect' type (default: unlimited)",
			},
			"multiline": {
				Type:        "boolean",
				Description: "Whether to allow multiline input for 'input' type",
			},
		},
	}
}

func (t *AskUserTool) PreviewCall(ctx context.Context, input *AskUserInput) *dive.ToolCallPreview {
	typeDesc := ""
	switch input.Type {
	case "confirm":
		typeDesc = "confirmation"
	case "select":
		typeDesc = "selection"
	case "multiselect":
		typeDesc = "multi-selection"
	case "input":
		typeDesc = "text input"
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Ask user (%s): %s", typeDesc, truncateString(input.Question, 50)),
	}
}

func (t *AskUserTool) Call(ctx context.Context, input *AskUserInput) (*dive.ToolResult, error) {
	// Fail closed if no interactor configured
	if t.interactor == nil {
		return dive.NewToolResultError("Error: no user interactor configured - cannot ask user questions"), nil
	}

	var output AskUserOutput

	switch input.Type {
	case "confirm":
		defaultVal := false
		if input.Default == "true" || input.Default == "yes" {
			defaultVal = true
		}
		confirmed, err := t.interactor.Confirm(ctx, &dive.ConfirmRequest{
			Title:   input.Question,
			Default: defaultVal,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting confirmation: %v", err)), nil
		}
		if confirmed {
			output.Response = "yes"
		} else {
			output.Response = "no"
		}

	case "select":
		if len(input.Options) == 0 {
			return dive.NewToolResultError("No options provided for selection"), nil
		}
		options := make([]dive.SelectOption, len(input.Options))
		for i, opt := range input.Options {
			options[i] = dive.SelectOption{
				Value:       opt.Value,
				Label:       opt.Label,
				Description: opt.Description,
				Default:     opt.Default,
			}
		}
		resp, err := t.interactor.Select(ctx, &dive.SelectRequest{
			Title:   input.Question,
			Options: options,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting selection: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else {
			output.Response = resp.Value
		}

	case "multiselect":
		if len(input.Options) == 0 {
			return dive.NewToolResultError("No options provided for multi-selection"), nil
		}
		options := make([]dive.SelectOption, len(input.Options))
		for i, opt := range input.Options {
			options[i] = dive.SelectOption{
				Value:       opt.Value,
				Label:       opt.Label,
				Description: opt.Description,
				Default:     opt.Default,
			}
		}
		resp, err := t.interactor.MultiSelect(ctx, &dive.MultiSelectRequest{
			Title:     input.Question,
			Options:   options,
			MinSelect: input.MinSelect,
			MaxSelect: input.MaxSelect,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting multi-selection: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else {
			output.Values = resp.Values
		}

	case "input":
		resp, err := t.interactor.Input(ctx, &dive.InputRequest{
			Title:     input.Question,
			Default:   input.Default,
			Multiline: input.Multiline,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting input: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else {
			output.Response = resp.Value
		}

	default:
		return dive.NewToolResultError(fmt.Sprintf("Unknown question type: %s", input.Type)), nil
	}

	// Marshal output to JSON
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error marshaling output: %v", err)), nil
	}

	display := fmt.Sprintf("Asked user: %s", truncateString(input.Question, 40))
	if output.Canceled {
		display += " (canceled)"
	} else if output.Response != "" {
		display += fmt.Sprintf(" → %s", truncateString(output.Response, 30))
	} else if len(output.Values) > 0 {
		display += fmt.Sprintf(" → %d selected", len(output.Values))
	}

	return dive.NewToolResultText(string(outputJSON)).WithDisplay(display), nil
}

func (t *AskUserTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Ask User",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
