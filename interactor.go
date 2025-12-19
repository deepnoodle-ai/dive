package dive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// UserInteractor handles user interactions during agent execution.
// Implement this interface to customize how the agent interacts with users.
type UserInteractor interface {
	// Confirm asks the user a yes/no question and returns their response.
	Confirm(ctx context.Context, request *ConfirmRequest) (bool, error)

	// Select presents options to the user and returns the selected option.
	Select(ctx context.Context, request *SelectRequest) (*SelectResponse, error)

	// MultiSelect presents options and returns all selected options.
	MultiSelect(ctx context.Context, request *MultiSelectRequest) (*MultiSelectResponse, error)

	// Input prompts for free-form text input from the user.
	Input(ctx context.Context, request *InputRequest) (*InputResponse, error)
}

// ConfirmRequest contains information for a confirmation prompt.
type ConfirmRequest struct {
	Tool    Tool                // The tool requesting confirmation (optional)
	Call    *llm.ToolUseContent // The tool call being confirmed (optional)
	Title   string              // Short title, e.g., "Execute command?"
	Message string              // Longer description of what will happen
	Default bool                // Default value if user doesn't choose
}

// SelectOption represents a single option in a selection.
type SelectOption struct {
	Value       string // Machine-readable value returned if selected
	Label       string // Human-readable label displayed to user
	Description string // Optional longer description
	Default     bool   // Whether this is the default selection
}

// SelectRequest contains information for a single-selection prompt.
type SelectRequest struct {
	Title   string         // Prompt title
	Message string         // Additional context
	Options []SelectOption // Available options
}

// SelectResponse contains the user's selection.
type SelectResponse struct {
	Value    string // Selected option's Value
	Canceled bool   // True if user canceled without selecting
}

// MultiSelectRequest contains information for a multi-selection prompt.
type MultiSelectRequest struct {
	Title     string         // Prompt title
	Message   string         // Additional context
	Options   []SelectOption // Available options
	MinSelect int            // Minimum selections required (0 = optional)
	MaxSelect int            // Maximum selections allowed (0 = unlimited)
}

// MultiSelectResponse contains the user's selections.
type MultiSelectResponse struct {
	Values   []string // Selected options' Values
	Canceled bool     // True if user canceled
}

// InputRequest contains information for a text input prompt.
type InputRequest struct {
	Title       string             // Prompt title
	Message     string             // Additional context/instructions
	Placeholder string             // Placeholder text in input field
	Default     string             // Default value
	Multiline   bool               // Whether to allow multiline input
	Validate    func(string) error // Optional validation function
}

// InputResponse contains the user's text input.
type InputResponse struct {
	Value    string // The entered text
	Canceled bool   // True if user canceled
}

// AutoApproveInteractor automatically approves all confirmations and selects defaults.
type AutoApproveInteractor struct{}

var _ UserInteractor = &AutoApproveInteractor{}

func (a *AutoApproveInteractor) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
	return true, nil
}

func (a *AutoApproveInteractor) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
	// Return the default option if one is set
	for _, opt := range req.Options {
		if opt.Default {
			return &SelectResponse{Value: opt.Value}, nil
		}
	}
	// Otherwise return the first option
	if len(req.Options) > 0 {
		return &SelectResponse{Value: req.Options[0].Value}, nil
	}
	return &SelectResponse{Canceled: true}, nil
}

func (a *AutoApproveInteractor) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
	// Return all default options
	var values []string
	for _, opt := range req.Options {
		if opt.Default {
			values = append(values, opt.Value)
		}
	}
	// If no defaults and min is 0, return empty
	if len(values) == 0 && req.MinSelect == 0 {
		return &MultiSelectResponse{Values: []string{}}, nil
	}
	// If we need more selections, add non-default options until we meet minimum
	if len(values) < req.MinSelect {
		for _, opt := range req.Options {
			if !opt.Default {
				values = append(values, opt.Value)
				if len(values) >= req.MinSelect {
					break
				}
			}
		}
	}
	return &MultiSelectResponse{Values: values}, nil
}

func (a *AutoApproveInteractor) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
	return &InputResponse{Value: req.Default}, nil
}

// DenyAllInteractor always denies confirmations and cancels selections.
type DenyAllInteractor struct{}

var _ UserInteractor = &DenyAllInteractor{}

func (d *DenyAllInteractor) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
	return false, nil
}

func (d *DenyAllInteractor) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
	return &SelectResponse{Canceled: true}, nil
}

func (d *DenyAllInteractor) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
	return &MultiSelectResponse{Canceled: true}, nil
}

func (d *DenyAllInteractor) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
	return &InputResponse{Canceled: true}, nil
}

// InteractionMode determines when user interaction is required.
type InteractionMode string

const (
	// InteractAlways always prompts the user for confirmation.
	InteractAlways InteractionMode = "always"
	// InteractNever never prompts the user, auto-approves everything.
	InteractNever InteractionMode = "never"
	// InteractIfDestructive only prompts for destructive operations.
	InteractIfDestructive InteractionMode = "if_destructive"
	// InteractIfNotReadOnly prompts unless the operation is read-only.
	InteractIfNotReadOnly InteractionMode = "if_not_read_only"
)

// TerminalInteractor provides terminal-based UI for user interactions.
type TerminalInteractor struct {
	Mode InteractionMode
}

var _ UserInteractor = &TerminalInteractor{}

// TerminalInteractorOptions configures a TerminalInteractor.
type TerminalInteractorOptions struct {
	Mode InteractionMode
}

// NewTerminalInteractor creates a new TerminalInteractor.
func NewTerminalInteractor(opts TerminalInteractorOptions) *TerminalInteractor {
	mode := InteractIfNotReadOnly
	if opts.Mode != "" {
		mode = opts.Mode
	}
	return &TerminalInteractor{Mode: mode}
}

// ShouldInteract determines if interaction is needed based on the mode and tool.
func (t *TerminalInteractor) ShouldInteract(tool Tool) bool {
	if t.Mode == InteractNever {
		return false
	}
	if t.Mode == InteractAlways {
		return true
	}
	if tool == nil {
		return true
	}
	annotations := tool.Annotations()
	if annotations == nil {
		return true
	}
	if t.Mode == InteractIfDestructive {
		return annotations.DestructiveHint
	}
	if t.Mode == InteractIfNotReadOnly {
		return !annotations.ReadOnlyHint
	}
	return true
}

func (t *TerminalInteractor) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
	if !t.ShouldInteract(req.Tool) {
		return true, nil
	}

	fmt.Printf("\n=== Confirmation Required ===\n")
	if req.Title != "" {
		fmt.Printf("%s\n", req.Title)
	}
	if req.Message != "" {
		fmt.Printf("%s\n", req.Message)
	}
	if req.Tool != nil {
		fmt.Printf("Tool: %s\n", req.Tool.Name())
	}
	if req.Call != nil {
		fmt.Printf("Input: %s\n", string(req.Call.Input))
	}

	defaultStr := "n"
	if req.Default {
		defaultStr = "y"
	}
	fmt.Printf("\nProceed? [y/n] (default: %s): ", defaultStr)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return req.Default, nil
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return req.Default, nil
	}
	return input == "y" || input == "yes", nil
}

func (t *TerminalInteractor) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
	fmt.Printf("\n=== Selection Required ===\n")
	if req.Title != "" {
		fmt.Printf("%s\n", req.Title)
	}
	if req.Message != "" {
		fmt.Printf("%s\n", req.Message)
	}

	defaultIndex := -1
	for i, opt := range req.Options {
		marker := "  "
		if opt.Default {
			marker = "* "
			defaultIndex = i
		}
		fmt.Printf("%s%d) %s", marker, i+1, opt.Label)
		if opt.Description != "" {
			fmt.Printf(" - %s", opt.Description)
		}
		fmt.Println()
	}

	fmt.Print("\nEnter selection (number or 'q' to cancel): ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		if defaultIndex >= 0 {
			return &SelectResponse{Value: req.Options[defaultIndex].Value}, nil
		}
		return &SelectResponse{Canceled: true}, nil
	}
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "q" || input == "quit" || input == "cancel" {
		return &SelectResponse{Canceled: true}, nil
	}

	if input == "" && defaultIndex >= 0 {
		return &SelectResponse{Value: req.Options[defaultIndex].Value}, nil
	}

	num, err := strconv.Atoi(input)
	if err != nil || num < 1 || num > len(req.Options) {
		fmt.Printf("Invalid selection. Please enter a number between 1 and %d.\n", len(req.Options))
		return t.Select(ctx, req) // Retry
	}

	return &SelectResponse{Value: req.Options[num-1].Value}, nil
}

func (t *TerminalInteractor) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
	fmt.Printf("\n=== Multi-Selection Required ===\n")
	if req.Title != "" {
		fmt.Printf("%s\n", req.Title)
	}
	if req.Message != "" {
		fmt.Printf("%s\n", req.Message)
	}
	if req.MinSelect > 0 || req.MaxSelect > 0 {
		if req.MaxSelect > 0 {
			fmt.Printf("(Select %d-%d options)\n", req.MinSelect, req.MaxSelect)
		} else {
			fmt.Printf("(Select at least %d options)\n", req.MinSelect)
		}
	}

	for i, opt := range req.Options {
		marker := "[ ]"
		if opt.Default {
			marker = "[x]"
		}
		fmt.Printf("%s %d) %s", marker, i+1, opt.Label)
		if opt.Description != "" {
			fmt.Printf(" - %s", opt.Description)
		}
		fmt.Println()
	}

	fmt.Print("\nEnter selections (comma-separated numbers, or 'q' to cancel): ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return &MultiSelectResponse{Canceled: true}, nil
	}
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "q" || input == "quit" || input == "cancel" {
		return &MultiSelectResponse{Canceled: true}, nil
	}

	// Parse selections
	var values []string
	if input == "" {
		// Use defaults
		for _, opt := range req.Options {
			if opt.Default {
				values = append(values, opt.Value)
			}
		}
	} else {
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			num, err := strconv.Atoi(part)
			if err != nil || num < 1 || num > len(req.Options) {
				continue
			}
			values = append(values, req.Options[num-1].Value)
		}
	}

	// Validate min/max
	if len(values) < req.MinSelect {
		fmt.Printf("Please select at least %d options.\n", req.MinSelect)
		return t.MultiSelect(ctx, req) // Retry
	}
	if req.MaxSelect > 0 && len(values) > req.MaxSelect {
		fmt.Printf("Please select at most %d options.\n", req.MaxSelect)
		return t.MultiSelect(ctx, req) // Retry
	}

	return &MultiSelectResponse{Values: values}, nil
}

func (t *TerminalInteractor) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
	fmt.Printf("\n=== Input Required ===\n")
	if req.Title != "" {
		fmt.Printf("%s\n", req.Title)
	}
	if req.Message != "" {
		fmt.Printf("%s\n", req.Message)
	}

	prompt := "> "
	if req.Default != "" {
		prompt = fmt.Sprintf("> (default: %s) ", req.Default)
	}
	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)

	if req.Multiline {
		fmt.Println("(Enter text, then press Enter twice to finish, or 'q' to cancel)")
		var lines []string
		emptyCount := 0
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				emptyCount++
				if emptyCount >= 2 {
					break
				}
				lines = append(lines, "")
			} else if line == "q" && len(lines) == 0 {
				return &InputResponse{Canceled: true}, nil
			} else {
				emptyCount = 0
				lines = append(lines, line)
			}
		}
		value := strings.Join(lines, "\n")
		value = strings.TrimRight(value, "\n")
		if value == "" {
			value = req.Default
		}
		if req.Validate != nil {
			if err := req.Validate(value); err != nil {
				fmt.Printf("Invalid input: %v\n", err)
				return t.Input(ctx, req) // Retry
			}
		}
		return &InputResponse{Value: value}, nil
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		return &InputResponse{Value: req.Default}, nil
	}
	input = strings.TrimSpace(input)

	if input == "" {
		input = req.Default
	}

	if input == "q" || input == "quit" || input == "cancel" {
		return &InputResponse{Canceled: true}, nil
	}

	if req.Validate != nil {
		if err := req.Validate(input); err != nil {
			fmt.Printf("Invalid input: %v\n", err)
			return t.Input(ctx, req) // Retry
		}
	}

	return &InputResponse{Value: input}, nil
}

// ConfirmerAdapter wraps a legacy Confirmer as a UserInteractor.
// This provides backwards compatibility with existing Confirmer implementations.
type ConfirmerAdapter struct {
	Confirmer Confirmer
}

var _ UserInteractor = &ConfirmerAdapter{}

func (a *ConfirmerAdapter) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
	return a.Confirmer.Confirm(ctx, nil, req.Tool, req.Call)
}

func (a *ConfirmerAdapter) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
	// Fall back to returning the default or first option
	for _, opt := range req.Options {
		if opt.Default {
			return &SelectResponse{Value: opt.Value}, nil
		}
	}
	if len(req.Options) > 0 {
		return &SelectResponse{Value: req.Options[0].Value}, nil
	}
	return &SelectResponse{Canceled: true}, nil
}

func (a *ConfirmerAdapter) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
	// Fall back to returning default options
	var values []string
	for _, opt := range req.Options {
		if opt.Default {
			values = append(values, opt.Value)
		}
	}
	return &MultiSelectResponse{Values: values}, nil
}

func (a *ConfirmerAdapter) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
	return &InputResponse{Value: req.Default}, nil
}
