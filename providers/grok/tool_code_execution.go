package grok

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

var (
	_ llm.Tool                                = &CodeExecutionTool{}
	_ openaiProvider.ResponsesToolProvider    = &CodeExecutionTool{}
	_ openaiProvider.ResponsesIncludeProvider = &CodeExecutionTool{}
)

// CodeExecutionToolOptions configures the Grok code execution tool.
type CodeExecutionToolOptions struct {
	// IncludeOutputs requests that the server return the code execution outputs
	// (logs and files) in the response via the "code_interpreter_call.outputs"
	// include parameter. Outputs can be large, so they are omitted by default.
	IncludeOutputs bool
}

// NewCodeExecutionTool creates a new Grok CodeExecutionTool. When the model uses
// it, Grok writes and runs Python in a sandboxed environment server-side and
// incorporates the result. This tool is only available via the xAI Responses
// API (it maps to the API's "code_interpreter" tool).
func NewCodeExecutionTool(opts CodeExecutionToolOptions) *CodeExecutionTool {
	return &CodeExecutionTool{includeOutputs: opts.IncludeOutputs}
}

// CodeExecutionTool is a server-side tool that lets Grok execute Python code.
type CodeExecutionTool struct {
	includeOutputs bool
}

func (t *CodeExecutionTool) Name() string {
	return "code_execution"
}

func (t *CodeExecutionTool) Description() string {
	return "Lets Grok write and execute Python code in a sandboxed environment for precise calculations and data analysis."
}

func (t *CodeExecutionTool) Schema() *schema.Schema {
	return nil
}

func (t *CodeExecutionTool) ResponsesToolParam() responses.ToolUnionParam {
	// The xAI API expects a bare {"type": "code_interpreter"} with no container,
	// unlike the OpenAI SDK's ToolCodeInterpreterParam (which requires one), so
	// pass raw JSON via param.Override.
	return param.Override[responses.ToolUnionParam](map[string]any{
		"type": "code_interpreter",
	})
}

func (t *CodeExecutionTool) ResponsesIncludes() []responses.ResponseIncludable {
	if !t.includeOutputs {
		return nil
	}
	return []responses.ResponseIncludable{"code_interpreter_call.outputs"}
}

func (t *CodeExecutionTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Code Execution",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *CodeExecutionTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
