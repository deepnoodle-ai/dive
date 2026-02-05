// Package toolkit provides a collection of tools for AI agents to interact with
// the local filesystem, execute shell commands, search the web, and communicate
// with users.
//
// # Tool Categories
//
// The toolkit includes tools organized into several categories:
//
// File Operations:
//   - [ReadFileTool]: Read file contents with optional line range
//   - [WriteFileTool]: Write content to files
//   - [EditTool]: Perform exact string replacements in files
//   - [GlobTool]: Find files using glob patterns
//   - [GrepTool]: Search file contents using regular expressions
//   - [ListDirectoryTool]: List directory contents with metadata
//   - [TextEditorTool]: Advanced file editor (Anthropic-compatible)
//
// Shell Execution:
//   - [BashTool]: Execute shell commands with timeout and output capture
//
// Web Operations:
//   - [FetchTool]: Fetch and extract content from web pages
//   - [WebSearchTool]: Search the web using a configured search provider
//
// User Interaction:
//   - [AskUserTool]: Ask users questions with various input types
//
// # Path Validation
//
// Tools that access the filesystem use [PathValidator] to enforce workspace
// boundaries and prevent path traversal attacks. By default, operations are
// restricted to the configured workspace directory.
//
// # Creating Tools
//
// Each tool has a constructor function (e.g., [NewBashTool], [NewEditTool])
// that accepts an options struct for configuration. Most tools return a
// [dive.TypedToolAdapter] that can be used directly with an agent:
//
//	bashTool := toolkit.NewBashTool(toolkit.BashToolOptions{
//	    WorkspaceDir: "/path/to/workspace",
//	    MaxOutputLength: 50000,
//	})
//
//	agent, err := dive.NewAgent(dive.AgentOptions{
//	    Tools: []dive.Tool{bashTool},
//	})
//
// # Filesystem Abstraction
//
// The [FileSystem] interface abstracts file operations, allowing tools to be
// tested with mock implementations. The [RealFileSystem] type provides the
// standard implementation using actual file system operations.
package toolkit

import "github.com/deepnoodle-ai/dive"

var (
	// NewToolResultError creates a tool result indicating an error occurred.
	// The error message is returned to the LLM for context about what went wrong.
	NewToolResultError = dive.NewToolResultError

	// NewToolResultText creates a successful tool result with text content.
	// This is the primary way tools return data to the LLM.
	NewToolResultText = dive.NewToolResultText
)
