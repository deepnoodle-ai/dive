// Package interactor provides user interaction interfaces for Dive agents.
//
// This package contains the Interactor interface and implementations for
// handling user interactions such as confirmation prompts, selections, and
// text input.
//
// # Migration from AgentOptions.Interactor
//
// Previously, interactors were passed via AgentOptions.Interactor. With the
// new architecture, interactors should be passed directly to tools that need
// them at construction time (e.g., AskUserTool).
//
// Old approach:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:      model,
//	    Interactor: dive.NewTerminalInteractor(...),
//	})
//
// New approach:
//
//	interactor := interactor.NewTerminal(...)
//	askUserTool := toolkit.NewAskUserTool(toolkit.AskUserToolOptions{
//	    Interactor: interactor,
//	})
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model: model,
//	    Tools: []dive.Tool{askUserTool, ...},
//	})
package interactor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// Interactor handles user interactions during agent execution.
type Interactor interface {
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
	Tool    dive.Tool           // The tool requesting confirmation (optional)
	Call    *llm.ToolUseContent // The tool call being confirmed (optional)
	Title   string              // Short title
	Message string              // Longer description
	Default bool                // Default value
}

// SelectOption represents a single option in a selection.
type SelectOption struct {
	Value       string // Machine-readable value
	Label       string // Human-readable label
	Description string // Optional description
	Default     bool   // Whether this is the default
}

// SelectRequest contains information for a single-selection prompt.
type SelectRequest struct {
	Title   string
	Message string
	Options []SelectOption
}

// SelectResponse contains the user's selection.
type SelectResponse struct {
	Value     string
	OtherText string
	Canceled  bool
}

// MultiSelectRequest contains information for a multi-selection prompt.
type MultiSelectRequest struct {
	Title     string
	Message   string
	Options   []SelectOption
	MinSelect int
	MaxSelect int
}

// MultiSelectResponse contains the user's selections.
type MultiSelectResponse struct {
	Values   []string
	Canceled bool
}

// InputRequest contains information for a text input prompt.
type InputRequest struct {
	Title       string
	Message     string
	Placeholder string
	Default     string
	Multiline   bool
	Validate    func(string) error
}

// InputResponse contains the user's text input.
type InputResponse struct {
	Value    string
	Canceled bool
}

// AutoApprove automatically approves all confirmations and selects defaults.
type AutoApprove struct{}

var _ Interactor = &AutoApprove{}

func (a *AutoApprove) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
	return true, nil
}

func (a *AutoApprove) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
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

func (a *AutoApprove) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
	var values []string
	for _, opt := range req.Options {
		if opt.Default {
			values = append(values, opt.Value)
		}
	}
	if len(values) == 0 && req.MinSelect == 0 {
		return &MultiSelectResponse{Values: []string{}}, nil
	}
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

func (a *AutoApprove) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
	return &InputResponse{Value: req.Default}, nil
}

// DenyAll always denies confirmations and cancels selections.
type DenyAll struct{}

var _ Interactor = &DenyAll{}

func (d *DenyAll) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
	return false, nil
}

func (d *DenyAll) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
	return &SelectResponse{Canceled: true}, nil
}

func (d *DenyAll) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
	return &MultiSelectResponse{Canceled: true}, nil
}

func (d *DenyAll) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
	return &InputResponse{Canceled: true}, nil
}

// Mode determines when user interaction is required.
type Mode string

const (
	// ModeAlways always prompts the user for confirmation.
	ModeAlways Mode = "always"

	// ModeNever never prompts the user, auto-approves everything.
	ModeNever Mode = "never"

	// ModeIfDestructive only prompts for destructive operations.
	ModeIfDestructive Mode = "if_destructive"

	// ModeIfNotReadOnly prompts unless the operation is read-only.
	ModeIfNotReadOnly Mode = "if_not_read_only"
)

// Terminal provides terminal-based UI for user interactions.
type Terminal struct {
	Mode Mode
}

var _ Interactor = &Terminal{}

// TerminalOptions configures a Terminal interactor.
type TerminalOptions struct {
	Mode Mode
}

// NewTerminal creates a new Terminal interactor.
func NewTerminal(opts TerminalOptions) *Terminal {
	mode := ModeIfNotReadOnly
	if opts.Mode != "" {
		mode = opts.Mode
	}
	return &Terminal{Mode: mode}
}

// ShouldInteract determines if interaction is needed based on the mode and tool.
func (t *Terminal) ShouldInteract(tool dive.Tool) bool {
	if t.Mode == ModeNever {
		return false
	}
	if t.Mode == ModeAlways {
		return true
	}
	if tool == nil {
		return true
	}
	annotations := tool.Annotations()
	if annotations == nil {
		return true
	}
	if t.Mode == ModeIfDestructive {
		return annotations.DestructiveHint
	}
	if t.Mode == ModeIfNotReadOnly {
		return !annotations.ReadOnlyHint
	}
	return true
}

func (t *Terminal) Confirm(ctx context.Context, req *ConfirmRequest) (bool, error) {
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

func (t *Terminal) Select(ctx context.Context, req *SelectRequest) (*SelectResponse, error) {
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
		return t.Select(ctx, req)
	}

	return &SelectResponse{Value: req.Options[num-1].Value}, nil
}

func (t *Terminal) MultiSelect(ctx context.Context, req *MultiSelectRequest) (*MultiSelectResponse, error) {
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

	var values []string
	if input == "" {
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

	if len(values) < req.MinSelect {
		fmt.Printf("Please select at least %d options.\n", req.MinSelect)
		return t.MultiSelect(ctx, req)
	}
	if req.MaxSelect > 0 && len(values) > req.MaxSelect {
		fmt.Printf("Please select at most %d options.\n", req.MaxSelect)
		return t.MultiSelect(ctx, req)
	}

	return &MultiSelectResponse{Values: values}, nil
}

func (t *Terminal) Input(ctx context.Context, req *InputRequest) (*InputResponse, error) {
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
				return t.Input(ctx, req)
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
			return t.Input(ctx, req)
		}
	}

	return &InputResponse{Value: input}, nil
}

