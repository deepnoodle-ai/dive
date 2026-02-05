package dive

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
)

// Dialog handles user interaction prompts during agent execution.
//
// Implementations of this interface provide the UI layer for confirmations,
// selections, and text input. The prompt type is determined by which fields
// are set in DialogInput:
//
//   - Confirm mode: set Confirm=true (yes/no question)
//   - Select mode: set Options (pick one)
//   - MultiSelect mode: set Options and MultiSelect=true (pick many)
//   - Input mode: none of the above (free-form text)
//
// Example implementation for auto-approve:
//
//	type AutoApproveDialog struct{}
//
//	func (d *AutoApproveDialog) Show(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
//	    if in.Confirm {
//	        return &DialogOutput{Confirmed: true}, nil
//	    }
//	    if len(in.Options) > 0 {
//	        return &DialogOutput{Values: []string{in.Options[0].Value}}, nil
//	    }
//	    return &DialogOutput{Text: in.Default}, nil
//	}
type Dialog interface {
	// Show presents a dialog to the user and waits for their response.
	Show(ctx context.Context, in *DialogInput) (*DialogOutput, error)
}

// DialogInput describes what to present to the user.
type DialogInput struct {
	// Title is a short heading for the dialog.
	Title string

	// Message provides additional context or instructions.
	Message string

	// Confirm indicates this is a yes/no confirmation dialog.
	// When true, the response's Confirmed field contains the answer.
	Confirm bool

	// Options provides choices for selection dialogs.
	// When non-empty, the response's Values field contains selected option values.
	Options []DialogOption

	// MultiSelect allows selecting multiple options.
	// Only applies when Options is non-empty.
	MultiSelect bool

	// Default is the pre-selected value.
	// Type depends on mode: bool (confirm), string (select/input), []string (multi-select).
	Default string

	// Validate is an optional validation function for text input.
	// Return an error to reject the input with a message.
	Validate func(string) error

	// Tool is the tool requesting this dialog (optional, for context).
	Tool Tool

	// Call is the specific tool invocation (optional, for context).
	Call *llm.ToolUseContent
}

// DialogOption represents a selectable choice.
type DialogOption struct {
	// Value is the machine-readable identifier returned when selected.
	Value string

	// Label is the human-readable text displayed to the user.
	Label string

	// Description provides additional context about this option.
	Description string
}

// DialogOutput contains the user's response.
type DialogOutput struct {
	// Confirmed is the answer for Confirm mode dialogs.
	Confirmed bool

	// Values contains selected option values.
	// For single-select, this has one element.
	// For multi-select, this may have zero or more elements.
	Values []string

	// Text is the entered text for Input mode dialogs.
	Text string

	// Canceled indicates the user dismissed the dialog without responding.
	Canceled bool
}

// AutoApproveDialog automatically approves confirmations and selects first/default options.
type AutoApproveDialog struct{}

var _ Dialog = &AutoApproveDialog{}

func (d *AutoApproveDialog) Show(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	// Confirm mode: always approve
	if in.Confirm {
		return &DialogOutput{Confirmed: true}, nil
	}

	// Select mode: pick default or first option
	if len(in.Options) > 0 {
		if in.Default != "" {
			return &DialogOutput{Values: []string{in.Default}}, nil
		}
		return &DialogOutput{Values: []string{in.Options[0].Value}}, nil
	}

	// Input mode: return default
	return &DialogOutput{Text: in.Default}, nil
}

// DenyAllDialog denies all confirmations and cancels all other dialogs.
type DenyAllDialog struct{}

var _ Dialog = &DenyAllDialog{}

func (d *DenyAllDialog) Show(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	if in.Confirm {
		return &DialogOutput{Confirmed: false}, nil
	}
	return &DialogOutput{Canceled: true}, nil
}
