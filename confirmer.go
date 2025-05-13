package dive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/fatih/color"
)

// Confirmation modes
const (
	// ConfirmAlways requires confirmation for all operations
	ConfirmAlways = "always"
	// ConfirmReadWrite requires confirmation only for operations that modify state
	ConfirmReadWrite = "read-write"
	// ConfirmNever requires no confirmation
	ConfirmNever = "never"
)

var _ llm.Confirmer = &TerminalConfirmer{}

type TerminalConfirmer struct {
	mode string
}

type TerminalConfirmerOptions struct {
	// Mode controls when confirmation is required:
	// - "always": confirm all operations (default)
	// - "read-write": only confirm operations that modify state
	// - "never": never confirm
	Mode string
}

func NewTerminalConfirmer(opts TerminalConfirmerOptions) *TerminalConfirmer {
	mode := opts.Mode
	if mode == "" {
		mode = ConfirmAlways
	}
	return &TerminalConfirmer{
		mode: mode,
	}
}

// ShouldConfirm determines if confirmation is needed based on the confirmer's mode and the request
func (c *TerminalConfirmer) ShouldConfirm(req llm.ConfirmationRequest) bool {
	if c.mode == ConfirmNever {
		return false
	}
	if c.mode == ConfirmAlways {
		return true
	}
	if c.mode == ConfirmReadWrite && req.Tool != nil {
		if toolWithMeta, ok := req.Tool.(llm.ToolWithMetadata); ok {
			return toolWithMeta.Metadata().Capability == llm.ToolCapabilityReadWrite
		}
	}
	return false
}

func (c *TerminalConfirmer) Confirm(ctx context.Context, req llm.ConfirmationRequest) (bool, error) {
	if !c.ShouldConfirm(req) {
		return true, nil
	}

	fmt.Printf("\n=== Confirmation Required ===\n")
	fmt.Printf("%s\n", req.Prompt)

	if req.Details != "" {
		fmt.Printf("\nDetails: %s\n", yellowSprintf(req.Details))
	}
	if req.Tool != nil {
		fmt.Printf("\nTool: %s\n", req.Tool.Name())
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

func yellowSprintf(s string) string {
	return color.New(color.FgYellow).Sprintf(s)
}
