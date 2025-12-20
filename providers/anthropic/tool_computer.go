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

// Tool versions:
//   - computer_20241022 - Claude 3.5 Sonnet (legacy)
//   - computer_20250124 - Claude Sonnet 4, Sonnet 4.5, Haiku 4.5, Opus 4, Opus 4.1, Sonnet 3.7
//   - computer_20251124 - Claude Opus 4.5 (adds zoom action)
//
// Beta headers required (use llm.WithFeatures):
//   - FeatureComputerUse ("computer-use-2025-01-24") for computer_20250124
//   - FeatureComputerUseOpus45 ("computer-use-2025-11-24") for computer_20251124
//
// Example tool definition:
//
//	{
//	  "type": "computer_20250124",
//	  "name": "computer",
//	  "display_width_px": 1024,
//	  "display_height_px": 768,
//	  "display_number": 1,
//	  "enable_zoom": true  // Optional, only for computer_20251124
//	}

// ComputerToolOptions are the options used to configure a ComputerTool.
type ComputerToolOptions struct {
	Type            string
	DisplayWidthPx  int
	DisplayHeightPx int
	DisplayNumber   int
	EnableZoom      bool // Only for computer_20251124 (Opus 4.5)
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
		enableZoom:      opts.EnableZoom,
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
	enableZoom      bool
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
	config := map[string]any{
		"type":              t.typeString,
		"name":              t.name,
		"display_width_px":  t.displayWidthPx,
		"display_height_px": t.displayHeightPx,
		"display_number":    t.displayNumber,
	}
	// enable_zoom is only valid for computer_20251124 (Opus 4.5)
	if t.enableZoom {
		config["enable_zoom"] = true
	}
	return config
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
