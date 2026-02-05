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

// AskUserInputOption represents a selectable option for "select" and "multiselect"
// question types.
type AskUserInputOption struct {
	// Value is returned when this option is selected.
	Value string `json:"value"`

	// Label is the text displayed to the user for this option.
	Label string `json:"label"`

	// Description provides additional context about the option (optional).
	Description string `json:"description,omitempty"`

	// Default marks this option as pre-selected.
	Default bool `json:"default,omitempty"`
}

// AskUserInput represents the input parameters for the AskUserQuestion tool.
type AskUserInput struct {
	// Question is the text to display to the user. Required.
	Question string `json:"question"`

	// Type specifies the input method. Required.
	// Valid values: "confirm", "select", "multiselect", "input"
	Type string `json:"type"`

	// Options provides choices for "select" and "multiselect" types.
	// Required for those types, ignored for others.
	Options []AskUserInputOption `json:"options,omitempty"`

	// Default specifies the default value.
	// For "confirm": "true" or "false"
	// For "input": the pre-filled text
	Default string `json:"default,omitempty"`

	// MinSelect is the minimum number of selections for "multiselect".
	// Defaults to 0 (no minimum).
	MinSelect int `json:"min_select,omitempty"`

	// MaxSelect is the maximum number of selections for "multiselect".
	// Defaults to 0 (unlimited).
	MaxSelect int `json:"max_select,omitempty"`

	// Multiline enables multi-line text entry for "input" type.
	Multiline bool `json:"multiline,omitempty"`
}

// AskUserOutput contains the user's response to a question.
type AskUserOutput struct {
	// Response contains the text response for "confirm", "select", and "input" types.
	// For "confirm": "yes" or "no"
	// For "select": the selected option's Value
	// For "input": the entered text
	Response string `json:"response,omitempty"`

	// Values contains selected values for "multiselect" type.
	Values []string `json:"values,omitempty"`

	// Canceled is true if the user dismissed the dialog without responding.
	Canceled bool `json:"canceled"`
}

// AskUserTool enables the LLM to ask questions and receive responses from users.
//
// This tool bridges the gap between autonomous operation and human interaction,
// allowing agents to gather information, confirm actions, or request decisions
// when needed.
//
// Question types:
//   - "confirm": Yes/no questions, returns "yes" or "no"
//   - "select": Single choice from options, returns the selected value
//   - "multiselect": Multiple choices from options, returns all selected values
//   - "input": Free-form text entry, returns the entered text
//
// The tool requires a [dive.Dialog] implementation to display prompts and
// collect responses. Without a dialog, the tool will return an error.
type AskUserTool struct {
	dialog dive.Dialog
}

// AskUserToolOptions configures the behavior of [AskUserTool].
type AskUserToolOptions struct {
	// Dialog is the implementation used to display prompts and collect responses.
	// Required - the tool will fail at call time if not provided.
	Dialog dive.Dialog
}

// NewAskUserTool creates a new AskUserTool with the given options.
// A [dive.Dialog] must be provided via options; without one, the tool
// will return an error when called.
func NewAskUserTool(opts AskUserToolOptions) *dive.TypedToolAdapter[*AskUserInput] {
	return dive.ToolAdapter(&AskUserTool{
		dialog: opts.Dialog,
	})
}

// Name returns "AskUserQuestion" as the tool identifier.
func (t *AskUserTool) Name() string {
	return "AskUserQuestion"
}

// Description returns detailed usage instructions for the LLM.
func (t *AskUserTool) Description() string {
	return `Ask the user a question and get their response. Use this tool when you need to:
- Confirm an action before proceeding (type: "confirm")
- Get the user to choose from a list of options (type: "select")
- Get the user to select multiple options (type: "multiselect")
- Get free-form text input from the user (type: "input")

The tool will return the user's response or indicate if they canceled.`
}

// Schema returns the JSON schema describing the tool's input parameters.
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

// PreviewCall returns a summary of the question for permission prompts.
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

// Call displays the question to the user and returns their response.
//
// The response is returned as JSON containing either:
//   - Response: for confirm, select, and input types
//   - Values: for multiselect type
//   - Canceled: true if the user dismissed without answering
//
// Validation is performed on multiselect responses if MinSelect or
// MaxSelect constraints are specified.
func (t *AskUserTool) Call(ctx context.Context, input *AskUserInput) (*dive.ToolResult, error) {
	// Fail closed if no dialog configured
	if t.dialog == nil {
		return dive.NewToolResultError("Error: no dialog configured - cannot ask user questions"), nil
	}

	var output AskUserOutput

	switch input.Type {
	case "confirm":
		resp, err := t.dialog.Show(ctx, &dive.DialogInput{
			Title:   input.Question,
			Confirm: true,
			Default: input.Default,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting confirmation: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else if resp.Confirmed {
			output.Response = "yes"
		} else {
			output.Response = "no"
		}

	case "select":
		if len(input.Options) == 0 {
			return dive.NewToolResultError("No options provided for selection"), nil
		}
		options := make([]dive.DialogOption, len(input.Options))
		defaultValue := ""
		for i, opt := range input.Options {
			options[i] = dive.DialogOption{
				Value:       opt.Value,
				Label:       opt.Label,
				Description: opt.Description,
			}
			if opt.Default {
				defaultValue = opt.Value
			}
		}
		resp, err := t.dialog.Show(ctx, &dive.DialogInput{
			Title:   input.Question,
			Options: options,
			Default: defaultValue,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting selection: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else if len(resp.Values) > 0 {
			output.Response = resp.Values[0]
		} else if resp.Text != "" {
			// User typed custom text (if dialog supports it)
			output.Response = resp.Text
		}

	case "multiselect":
		if len(input.Options) == 0 {
			return dive.NewToolResultError("No options provided for multi-selection"), nil
		}
		options := make([]dive.DialogOption, len(input.Options))
		var defaults []string
		for i, opt := range input.Options {
			options[i] = dive.DialogOption{
				Value:       opt.Value,
				Label:       opt.Label,
				Description: opt.Description,
			}
			if opt.Default {
				defaults = append(defaults, opt.Value)
			}
		}
		// Pass defaults as comma-separated values (dialog implementation dependent)
		defaultVal := ""
		if len(defaults) > 0 {
			defaultVal = defaults[0] // Basic support for single default
		}
		resp, err := t.dialog.Show(ctx, &dive.DialogInput{
			Title:       input.Question,
			Options:     options,
			MultiSelect: true,
			Default:     defaultVal,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting multi-selection: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else {
			// Validate min/max selection constraints
			if input.MinSelect > 0 && len(resp.Values) < input.MinSelect {
				return dive.NewToolResultError(fmt.Sprintf("At least %d option(s) must be selected", input.MinSelect)), nil
			}
			if input.MaxSelect > 0 && len(resp.Values) > input.MaxSelect {
				return dive.NewToolResultError(fmt.Sprintf("At most %d option(s) can be selected", input.MaxSelect)), nil
			}
			output.Values = resp.Values
		}

	case "input":
		resp, err := t.dialog.Show(ctx, &dive.DialogInput{
			Title:   input.Question,
			Default: input.Default,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting input: %v", err)), nil
		}
		if resp.Canceled {
			output.Canceled = true
		} else {
			output.Response = resp.Text
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

// Annotations returns metadata hints about the tool's behavior.
// AskUserQuestion is marked as read-only (doesn't modify data) but not
// idempotent (user may give different answers to the same question).
func (t *AskUserTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "AskUserQuestion",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

// truncateString limits a string to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
