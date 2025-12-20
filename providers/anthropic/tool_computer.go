package anthropic

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ llm.Tool              = &ComputerTool{}
	_ llm.ToolConfiguration = &ComputerTool{}
)

// computer_20241022 - Claude 3.5 Sonnet
// computer_20250124 - Claude 3.7 Sonnet

/* A tool definition must be added in the request that looks like this:
{
  "type": "computer_20250124",
  "name": "computer",
  "display_width_px": 1024,
  "display_height_px": 768,
  "display_number": 1
}
*/

// ComputerToolOptions are the options used to configure a ComputerTool.
type ComputerToolOptions struct {
	Type            string
	DisplayWidthPx  int
	DisplayHeightPx int
	DisplayNumber   int
}

// NewComputerTool creates a new ComputerTool with the given options.
func NewComputerTool(opts ComputerToolOptions) *ComputerTool {
	if opts.Type == "" {
		opts.Type = "computer_20250124"
	}
	return &ComputerTool{
		typeString:      opts.Type,
		name:            "computer",
		displayWidthPx:  opts.DisplayWidthPx,
		displayHeightPx: opts.DisplayHeightPx,
		displayNumber:   opts.DisplayNumber,
	}
}

// ComputerTool is a tool that allows Claude to use a computer.
// https://docs.anthropic.com/en/docs/agents-and-tools/computer-use
type ComputerTool struct {
	typeString      string
	name            string
	displayWidthPx  int
	displayHeightPx int
	displayNumber   int
}

func (t *ComputerTool) Name() string {
	return "computer"
}

func (t *ComputerTool) Description() string {
	return "Uses Anthropic's computer feature to give Claude the ability to use a computer."
}

func (t *ComputerTool) Schema() *schema.Schema {
	return nil // Empty for server-side tools
}

func (t *ComputerTool) ToolConfiguration(providerName string) map[string]any {
	return map[string]any{
		"type":              t.typeString,
		"name":              t.name,
		"display_width_px":  t.displayWidthPx,
		"display_height_px": t.displayHeightPx,
		"display_number":    t.displayNumber,
	}
}

func (t *ComputerTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Computer",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *ComputerTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
