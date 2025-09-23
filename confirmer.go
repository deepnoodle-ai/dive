package dive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// ToolUseContent is a type alias for llm.ToolUseContent.
type ToolUseContent = llm.ToolUseContent

// Confirmer abstracts user confirmation prompts.
type Confirmer interface {
	// Confirm presents a request to the user and returns true if the user
	// confirms, false otherwise.
	Confirm(ctx context.Context, agent Agent, tool Tool, call *ToolUseContent) (bool, error)
}

// AutoApproveConfirmer always approves confirmation requests.
type AutoApproveConfirmer struct{}

func (a *AutoApproveConfirmer) Confirm(ctx context.Context, agent Agent, tool Tool, call *ToolUseContent) (bool, error) {
	return true, nil
}

// DenyAllConfirmer always denies confirmation requests.
type DenyAllConfirmer struct{}

func (d *DenyAllConfirmer) Confirm(ctx context.Context, agent Agent, tool Tool, call *ToolUseContent) (bool, error) {
	return false, nil
}

// NewConfirmer returns a Confirmer implementation based on the mode string.
// Supported modes: "auto", "deny"
func NewConfirmer(mode string) (Confirmer, error) {
	switch mode {
	case "auto":
		return &AutoApproveConfirmer{}, nil
	case "deny":
		return &DenyAllConfirmer{}, nil
	default:
		return nil, fmt.Errorf("invalid confirmer mode: %s", mode)
	}
}

type ConfirmationMode string

// Confirmation modes
const (
	// ConfirmAlways requires confirmation for all operations
	ConfirmAlways ConfirmationMode = "always"

	// ConfirmIfNotReadOnly requires confirmation only for operations that are not read-only
	ConfirmIfNotReadOnly ConfirmationMode = "if-not-read-only"

	// ConfirmIfDestructive requires confirmation only for operations that may be destructive
	ConfirmIfDestructive ConfirmationMode = "if-destructive"

	// ConfirmNever requires no confirmation
	ConfirmNever ConfirmationMode = "never"
)

func (c ConfirmationMode) String() string {
	return string(c)
}

func (c ConfirmationMode) IsValid() bool {
	return c == ConfirmAlways ||
		c == ConfirmIfNotReadOnly ||
		c == ConfirmIfDestructive ||
		c == ConfirmNever
}

var _ Confirmer = &TerminalConfirmer{}

type TerminalConfirmer struct {
	mode ConfirmationMode
}

type TerminalConfirmerOptions struct {
	Mode ConfirmationMode
}

func NewTerminalConfirmer(opts TerminalConfirmerOptions) *TerminalConfirmer {
	mode := ConfirmIfNotReadOnly
	if opts.Mode != "" {
		mode = ConfirmationMode(opts.Mode)
	}
	return &TerminalConfirmer{
		mode: mode,
	}
}

// ShouldConfirm determines if confirmation is needed based on the confirmer's
// mode and the request
func (c *TerminalConfirmer) ShouldConfirm(agent Agent, tool Tool, call *ToolUseContent) bool {
	if c.mode == ConfirmNever {
		return false
	}
	if c.mode == ConfirmAlways {
		return true
	}
	if call != nil {
		annotations := tool.Annotations()
		if c.mode == ConfirmIfDestructive && annotations.DestructiveHint {
			// Confirm if destructive
			return true
		}
		if c.mode == ConfirmIfNotReadOnly && !annotations.ReadOnlyHint {
			// Confirm if NOT read-only
			return true
		}
	}
	return false
}

func (c *TerminalConfirmer) Confirm(ctx context.Context, agent Agent, tool Tool, call *ToolUseContent) (bool, error) {
	if !c.ShouldConfirm(agent, tool, call) {
		return true, nil
	}

	fmt.Printf("\n=== Confirmation Required ===\n")
	fmt.Println("Would you like to allow this tool call to proceed?")
	if tool != nil {
		fmt.Printf("\nTool: %s\n", tool.Name())
	}
	if call != nil {
		input := string(call.Input)
		fmt.Printf("\nInput: %s\n", input)
	}

	fmt.Printf("\nProceed? (y/yes to confirm, anything else to deny): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("error reading confirmation: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes", nil
}
