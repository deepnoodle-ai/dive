package anthropic

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
)

var (
	_ llm.Tool              = &WebSearchTool{}
	_ llm.ToolConfiguration = &WebSearchTool{}
)

/* A tool definition must be added in the request that looks like this:
   "tools": [{
       "type": "code_execution_20250522",
       "name": "code_execution"
   }]
*/

// CodeExecutionToolOptions are the options used to configure a CodeExecutionTool.
type CodeExecutionToolOptions struct {
	Type string
}

// NewCodeExecutionTool creates a new CodeExecutionTool with the given options.
func NewCodeExecutionTool(opts CodeExecutionToolOptions) *CodeExecutionTool {
	if opts.Type == "" {
		opts.Type = "code_execution_20250522"
	}
	return &CodeExecutionTool{
		typeString: opts.Type,
		name:       "code_execution",
	}
}

// CodeExecutionTool is a tool that allows Claude to execute code. This is
// provided by Anthropic as a server-side tool. Learn more:
// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool
type CodeExecutionTool struct {
	typeString string
	name       string
}

func (t *CodeExecutionTool) Name() string {
	return "code_execution"
}

func (t *CodeExecutionTool) Description() string {
	return "The code execution tool allows Claude to execute Python code in a secure, sandboxed environment. Claude can analyze data, create visualizations, perform complex calculations, and process uploaded files directly within the API conversation."
}

func (t *CodeExecutionTool) Schema() schema.Schema {
	return schema.Schema{} // Empty for server-side tools
}

func (t *CodeExecutionTool) ToolConfiguration(providerName string) map[string]any {
	return map[string]any{"type": t.typeString, "name": t.name}
}
