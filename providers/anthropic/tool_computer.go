package anthropic

import (
	"context"
	"errors"
	"slices"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ llm.Tool              = &ComputerTool{}
	_ llm.ToolConfiguration = &ComputerTool{}
	_ llm.Tool              = &ComputerToolset{}
	_ llm.ToolConfiguration = &ComputerToolset{}
	_ dive.ToolDeclarer     = &ComputerToolset{}
)

const (
	// ComputerToolsetType is the computer use toolset. It needs no beta
	// header, and it is the only form of computer use Opus 5.5 accepts on the
	// Claude API and Google Cloud.
	ComputerToolsetType = "computer_toolset_20260801"
	// ComputerToolsetName is the toolset_name on each of the toolset's calls
	// (llm.ToolUseContent.ToolsetName) and results.
	ComputerToolsetName = "computer"
	// ComputerToolsetHaltText is the result text Anthropic specifies for the
	// calls in a batch after one that failed, which the agent loop must not
	// run. It is sent with IsError set. A dive.Agent answers those calls
	// with it on its own; an application running its own loop sends it.
	ComputerToolsetHaltText = "Not executed: an earlier computer action in this turn failed."
)

// ComputerToolsetOptions configures a ComputerToolset.
type ComputerToolsetOptions struct {
	// Disabled lists members to withhold from the model, such as "zoom" for
	// an environment that can't produce zoom images. All 17 members,
	// including zoom, are enabled by default.
	Disabled []string
}

// NewComputerToolset declares the computer_toolset_20260801 toolset.
func NewComputerToolset(opts ComputerToolsetOptions) *ComputerToolset {
	return &ComputerToolset{disabled: append([]string(nil), opts.Disabled...)}
}

// ComputerToolset declares Anthropic's computer use toolset, which gives Claude
// 17 member tools such as screenshot, left_click, type, and zoom. Supported on
// Opus 4.8 and later, Sonnet 5, Fable 5 and 5.1, and Mythos 5 and 5.1.
//
// Each action is its own tool call: its Name is the member ("left_click"),
// its ToolsetName is ComputerToolsetName, and its input has no "action". Your
// application runs every call, with an ordinary dive.Tool per member, named
// for it; the agent routes each call to the tool of that name, so permission
// rules and hooks apply per member. The rest of the contract is handled for
// you when the tools run in a dive.Agent:
//   - The toolset declares its members (DeclaredTools), so the agent does not
//     send the member tools as custom tools while the toolset is among the
//     agent's tools. Leave the toolset out for a model that doesn't take it
//     and the same member tools are sent as ordinary tools.
//   - A response can hold several calls (a batch). The agent runs them in
//     order and stops at the first failure: that call keeps its error, and
//     each later one is answered with IsError and ComputerToolsetHaltText,
//     without running. See dive.ToolAnnotations.HaltsBatch, which gives the
//     member tools the same behavior when they are offered as plain tools.
//   - Each result must carry the toolset name. The Anthropic provider fills
//     it in from the matching call when ToolResultContent.ToolsetName is
//     empty.
//
// Still the application's job: report a failed action as a failure (an
// IsError result or a Go error), and resize screenshots and zoom images to
// the model's image limits, which the API enforces instead of downscaling.
//
// https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool
type ComputerToolset struct {
	disabled []string
}

// computerToolsetMembers are the toolset's member names, as the API accepts
// them in the "configs" of computer_toolset_20260801.
var computerToolsetMembers = []string{
	"screenshot", "left_click", "right_click", "middle_click", "double_click",
	"triple_click", "mouse_move", "left_click_drag", "left_mouse_down",
	"left_mouse_up", "scroll", "type", "key", "hold_key", "wait",
	"cursor_position", "zoom",
}

// DeclaredTools returns the names of all 17 members, including any listed in
// ComputerToolsetOptions.Disabled, so that an application tool named for a
// disabled member is withheld from the model along with the member. It
// implements dive.ToolDeclarer.
func (t *ComputerToolset) DeclaredTools() []string {
	return slices.Clone(computerToolsetMembers)
}

// Name returns the toolset name. The model never calls a tool by this name;
// its calls name the member.
func (t *ComputerToolset) Name() string {
	return ComputerToolsetName
}

func (t *ComputerToolset) Description() string {
	return "Uses Anthropic's computer use toolset to give Claude the ability to use a computer."
}

func (t *ComputerToolset) Schema() *schema.Schema {
	return nil // Anthropic defines the member schemas
}

func (t *ComputerToolset) ToolConfiguration(providerName string) map[string]any {
	config := map[string]any{"type": ComputerToolsetType}
	if len(t.disabled) > 0 {
		configs := make(map[string]any, len(t.disabled))
		for _, member := range t.disabled {
			configs[member] = map[string]any{"enabled": false}
		}
		config["configs"] = configs
	}
	return config
}

func (t *ComputerToolset) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Computer",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *ComputerToolset) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("computer use toolset calls must be run by the application")
}

// Tool versions:
//   - computer_toolset_20260801 - see ComputerToolset
//   - computer_20241022 - Claude 3.5 Sonnet (legacy)
//   - computer_20250124 - Claude Sonnet 4, Sonnet 4.5, Haiku 4.5, Opus 4, Opus 4.1, Sonnet 3.7
//   - computer_20251124 - Claude Opus 4.5 through 4.8, Opus 5, Sonnet 4.6 and 5,
//     Fable 5 and 5.1, Mythos 5 and 5.1 (adds zoom action). Opus 5.5 rejects
//     it with a 400 on the Claude API and Google Cloud; use ComputerToolset.
//
// Beta headers required (use llm.WithFeatures):
//   - FeatureComputerUse ("computer-use-2025-01-24") for computer_20250124
//   - FeatureComputerUse45_46 ("computer-use-2025-11-24") for computer_20251124
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
	EnableZoom      bool // Only for computer_20251124
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
	// enable_zoom is only valid for computer_20251124
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
