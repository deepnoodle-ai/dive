package dive

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// TerminalDialog implements Dialog using stdin/stdout.
type TerminalDialog struct {
	in  io.Reader
	out io.Writer
}

var _ Dialog = &TerminalDialog{}

// NewTerminalDialog creates a Dialog that prompts via stdin/stdout.
func NewTerminalDialog() *TerminalDialog {
	return &TerminalDialog{
		in:  os.Stdin,
		out: os.Stdout,
	}
}

// TerminalDialogOptions configures a TerminalDialog.
type TerminalDialogOptions struct {
	In  io.Reader
	Out io.Writer
}

// NewTerminalDialogWithOptions creates a Dialog with custom input/output.
func NewTerminalDialogWithOptions(opts TerminalDialogOptions) *TerminalDialog {
	d := &TerminalDialog{
		in:  opts.In,
		out: opts.Out,
	}
	if d.in == nil {
		d.in = os.Stdin
	}
	if d.out == nil {
		d.out = os.Stdout
	}
	return d
}

func (d *TerminalDialog) Show(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	if in.Confirm {
		return d.showConfirm(ctx, in)
	}
	if len(in.Options) > 0 {
		if in.MultiSelect {
			return d.showMultiSelect(ctx, in)
		}
		return d.showSelect(ctx, in)
	}
	return d.showInput(ctx, in)
}

func (d *TerminalDialog) showConfirm(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	d.printHeader(in)

	defaultHint := "y/n"
	if in.Default == "true" {
		defaultHint = "Y/n"
	} else if in.Default == "false" {
		defaultHint = "y/N"
	}

	fmt.Fprintf(d.out, "Confirm? [%s]: ", defaultHint)

	line, err := d.readLine()
	if err != nil {
		return nil, err
	}

	line = strings.ToLower(strings.TrimSpace(line))

	// Handle default
	if line == "" {
		confirmed := in.Default == "true"
		return &DialogOutput{Confirmed: confirmed}, nil
	}

	confirmed := line == "y" || line == "yes"
	return &DialogOutput{Confirmed: confirmed}, nil
}

func (d *TerminalDialog) showSelect(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	d.printHeader(in)

	// Find default index
	defaultIdx := -1
	for i, opt := range in.Options {
		marker := "  "
		if opt.Value == in.Default {
			marker = "> "
			defaultIdx = i
		}
		if opt.Description != "" {
			fmt.Fprintf(d.out, "%s%d. %s - %s\n", marker, i+1, opt.Label, opt.Description)
		} else {
			fmt.Fprintf(d.out, "%s%d. %s\n", marker, i+1, opt.Label)
		}
	}

	prompt := "Select [1-%d]: "
	if defaultIdx >= 0 {
		prompt = fmt.Sprintf("Select [1-%d, default=%d]: ", len(in.Options), defaultIdx+1)
	}
	fmt.Fprintf(d.out, prompt, len(in.Options))

	line, err := d.readLine()
	if err != nil {
		return nil, err
	}

	line = strings.TrimSpace(line)

	// Handle default
	if line == "" && defaultIdx >= 0 {
		return &DialogOutput{Values: []string{in.Options[defaultIdx].Value}}, nil
	}

	// Parse selection
	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(in.Options) {
		return &DialogOutput{Canceled: true}, nil
	}

	return &DialogOutput{Values: []string{in.Options[idx-1].Value}}, nil
}

func (d *TerminalDialog) showMultiSelect(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	d.printHeader(in)

	for i, opt := range in.Options {
		if opt.Description != "" {
			fmt.Fprintf(d.out, "  %d. %s - %s\n", i+1, opt.Label, opt.Description)
		} else {
			fmt.Fprintf(d.out, "  %d. %s\n", i+1, opt.Label)
		}
	}

	fmt.Fprintf(d.out, "Select (comma-separated, e.g., 1,3,4): ")

	line, err := d.readLine()
	if err != nil {
		return nil, err
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return &DialogOutput{Values: []string{}}, nil
	}

	// Parse selections
	parts := strings.Split(line, ",")
	var values []string
	seen := make(map[string]bool)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		idx, err := strconv.Atoi(part)
		if err != nil || idx < 1 || idx > len(in.Options) {
			continue
		}
		value := in.Options[idx-1].Value
		if !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}

	return &DialogOutput{Values: values}, nil
}

func (d *TerminalDialog) showInput(ctx context.Context, in *DialogInput) (*DialogOutput, error) {
	d.printHeader(in)

	prompt := "> "
	if in.Default != "" {
		prompt = fmt.Sprintf("[%s] > ", in.Default)
	}
	fmt.Fprint(d.out, prompt)

	line, err := d.readLine()
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(line)

	// Handle default
	if text == "" && in.Default != "" {
		text = in.Default
	}

	// Validate if provided
	if in.Validate != nil {
		if err := in.Validate(text); err != nil {
			fmt.Fprintf(d.out, "Invalid: %v\n", err)
			return d.showInput(ctx, in) // Retry
		}
	}

	return &DialogOutput{Text: text}, nil
}

func (d *TerminalDialog) printHeader(in *DialogInput) {
	fmt.Fprintln(d.out)
	if in.Title != "" {
		fmt.Fprintf(d.out, "=== %s ===\n", in.Title)
	}
	if in.Message != "" {
		fmt.Fprintln(d.out, in.Message)
	}
	if in.Title != "" || in.Message != "" {
		fmt.Fprintln(d.out)
	}
}

func (d *TerminalDialog) readLine() (string, error) {
	reader := bufio.NewReader(d.in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(line, "\n"), nil
}
