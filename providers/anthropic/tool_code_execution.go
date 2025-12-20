package anthropic

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ llm.Tool              = &CodeExecutionTool{}
	_ llm.ToolConfiguration = &CodeExecutionTool{}
)

// Tool type versions for the code execution tool.
const (
	// CodeExecutionToolType is the current version supporting bash and text_editor.
	CodeExecutionToolType = "code_execution_20250825"
	// CodeExecutionToolTypeLegacy is the legacy version supporting Python only.
	CodeExecutionToolTypeLegacy = "code_execution_20250522"
)

/* A tool definition must be added in the request that looks like this:
   "tools": [{
       "type": "code_execution_20250825",
       "name": "code_execution"
   }]

When this tool is provided, Claude automatically gains access to two sub-tools:
  - bash_code_execution: Run shell commands
  - text_editor_code_execution: View, create, and edit files

Response types include:
  - server_tool_use with name "bash_code_execution" or "text_editor_code_execution"
  - bash_code_execution_tool_result
  - text_editor_code_execution_tool_result
*/

// CodeExecutionToolOptions are the options used to configure a CodeExecutionTool.
type CodeExecutionToolOptions struct {
	// Type specifies the tool version. Defaults to CodeExecutionToolType (current).
	// Use CodeExecutionToolTypeLegacy for the older Python-only version.
	Type string
}

// NewCodeExecutionTool creates a new CodeExecutionTool with the given options.
func NewCodeExecutionTool(opts ...CodeExecutionToolOptions) *CodeExecutionTool {
	var resolvedOpts CodeExecutionToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.Type == "" {
		resolvedOpts.Type = CodeExecutionToolType
	}
	return &CodeExecutionTool{
		typeString: resolvedOpts.Type,
		name:       "code_execution",
	}
}

// CodeExecutionTool is a tool that allows Claude to execute code in a secure,
// sandboxed environment. This is provided by Anthropic as a server-side tool.
//
// The current version (code_execution_20250825) provides Claude with:
//   - bash_code_execution: Execute shell commands
//   - text_editor_code_execution: View, create, and edit files
//
// The sandboxed environment includes:
//   - Python 3.11.12 with common data science libraries
//   - 5GiB RAM and disk space
//   - No internet access (for security)
//   - 30-day container expiration
//
// Learn more: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool
type CodeExecutionTool struct {
	typeString string
	name       string
}

func (t *CodeExecutionTool) Name() string {
	return "code_execution"
}

func (t *CodeExecutionTool) Description() string {
	return `The code execution tool allows Claude to run Bash commands and manipulate files in a secure, sandboxed environment. Claude can analyze data, create visualizations, perform complex calculations, run system commands, create and edit files, and process uploaded files directly within the API conversation.`
}

func (t *CodeExecutionTool) Schema() *schema.Schema {
	return nil // Empty for server-side tools
}

func (t *CodeExecutionTool) ToolConfiguration(providerName string) map[string]any {
	return map[string]any{"type": t.typeString, "name": t.name}
}

func (t *CodeExecutionTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Code Execution",
		ReadOnlyHint:    false, // Can create/modify files
		DestructiveHint: false, // Sandboxed, doesn't affect user's system
		IdempotentHint:  false, // Same code can produce different results
		OpenWorldHint:   false, // No internet access in sandbox
	}
}

func (t *CodeExecutionTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}

// Type returns the tool type string (e.g., "code_execution_20250825").
func (t *CodeExecutionTool) Type() string {
	return t.typeString
}
