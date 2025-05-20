package toolkit

import (
	"context"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/schema"
)

var _ dive.TypedTool[*TextEditorToolInput] = &TextEditorTool{}

// text_editor_20241022 - Claude 3.5 Sonnet
// text_editor_20250124 - Claude 3.7 Sonnet

/* A tool definition must be added in the request that looks like this:
{
  "type": "text_editor_20250124",
  "name": "text_editor"
}
*/

type TextEditorToolInput struct {
	Text string `json:"text"`
}

// TextEditorToolOptions are the options used to configure a TextEditorTool.
type TextEditorToolOptions struct {
	Type string
}

// NewTextEditorTool creates a new TextEditorTool with the given options.
func NewTextEditorTool(opts TextEditorToolOptions) *dive.TypedToolAdapter[*TextEditorToolInput] {
	if opts.Type == "" {
		opts.Type = "text_editor_20250124"
	}
	return dive.ToolAdapter(&TextEditorTool{typeString: opts.Type})
}

// TextEditorTool is a tool that allows Claude to edit files.
// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/text-editor-tool
type TextEditorTool struct {
	typeString string
}

func (t *TextEditorTool) Name() string {
	return "str_replace_editor"
}

func (t *TextEditorTool) Description() string {
	return "Uses Anthropic's text editor feature to give Claude the ability to edit text."
}

func (t *TextEditorTool) Schema() schema.Schema {
	return schema.Schema{} // Empty for server-side tools
}

func (t *TextEditorTool) Annotations() dive.ToolAnnotations {
	return dive.ToolAnnotations{
		Title:           "Text Editor",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *TextEditorTool) Call(ctx context.Context, input *TextEditorToolInput) (*dive.ToolResult, error) {
	return nil, nil
}
