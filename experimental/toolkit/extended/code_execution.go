// Package toolkit provides tools for AI agents.
package extended

import (
	"github.com/deepnoodle-ai/dive/providers/anthropic"
)

// CodeExecutionTool is an alias for the Anthropic code execution tool.
// This is a server-side tool that allows Claude to execute code in a secure,
// sandboxed environment provided by Anthropic.
//
// The current version (code_execution_20250825) provides Claude with:
//   - bash_code_execution: Execute shell commands
//   - text_editor_code_execution: View, create, and edit files
//
// The sandboxed environment includes:
//   - Python 3.11.12 with common data science libraries (pandas, numpy, scipy, etc.)
//   - 5GiB RAM and disk space
//   - No internet access (for security)
//   - 30-day container expiration
//
// Usage:
//
//	agent, err := dive.NewAgent(dive.AgentOptions{
//	    Name:  "Data Analyst",
//	    Model: anthropic.New(),
//	    Tools: []dive.Tool{
//	        toolkit.NewCodeExecutionTool(),
//	    },
//	})
//
// Note: You must enable the code-execution-2025-08-25 beta feature when using
// this tool with the Anthropic API. This is done automatically when the tool
// is detected by the provider.
//
// Learn more: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool
type CodeExecutionTool = anthropic.CodeExecutionTool

// CodeExecutionToolOptions configures the code execution tool.
type CodeExecutionToolOptions = anthropic.CodeExecutionToolOptions

// Tool type versions for the code execution tool.
const (
	// CodeExecutionToolType is the current version supporting bash and text_editor.
	CodeExecutionToolType = anthropic.CodeExecutionToolType
	// CodeExecutionToolTypeLegacy is the legacy version supporting Python only.
	CodeExecutionToolTypeLegacy = anthropic.CodeExecutionToolTypeLegacy
)

// NewCodeExecutionTool creates a new CodeExecutionTool.
//
// This is a server-side tool provided by Anthropic that allows Claude to
// execute code in a secure, sandboxed environment. The tool supports:
//
//   - Bash commands: Execute shell commands for system operations
//   - File operations: Create, view, and edit files directly
//
// Example:
//
//	// Default configuration (current version)
//	tool := toolkit.NewCodeExecutionTool()
//
//	// Use legacy Python-only version
//	tool := toolkit.NewCodeExecutionTool(toolkit.CodeExecutionToolOptions{
//	    Type: toolkit.CodeExecutionToolTypeLegacy,
//	})
//
// Response Handling:
//
// When Claude uses this tool, the response will contain content blocks of
// different types depending on the operation:
//
//   - llm.ContentTypeServerToolUse: Claude's tool invocation (name: "bash_code_execution" or "text_editor_code_execution")
//   - llm.ContentTypeBashCodeExecutionToolResult: Result of bash command execution
//   - llm.ContentTypeTextEditorCodeExecutionToolResult: Result of file operations
//
// Error codes that may be returned:
//   - unavailable: The tool is temporarily unavailable
//   - execution_time_exceeded: Execution exceeded maximum time limit
//   - container_expired: Container expired and is no longer available
//   - invalid_tool_input: Invalid parameters provided to the tool
//   - too_many_requests: Rate limit exceeded for tool usage
//   - file_not_found: File doesn't exist (for view/edit operations)
//   - string_not_found: The old_str not found in file (for str_replace)
func NewCodeExecutionTool(opts ...CodeExecutionToolOptions) *CodeExecutionTool {
	return anthropic.NewCodeExecutionTool(opts...)
}
