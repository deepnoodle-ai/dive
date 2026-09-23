package toolkit

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ dive.TypedTool[*GrepInput]          = &GrepTool{}
	_ dive.TypedToolPreviewer[*GrepInput] = &GrepTool{}
)

// GrepOutputMode specifies what type of output to produce from a grep search.
type GrepOutputMode string

const (
	// GrepOutputContent returns matching lines with file paths and line numbers.
	GrepOutputContent GrepOutputMode = "content"

	// GrepOutputFilesWithMatches returns only the paths of files containing matches.
	// This is the default mode.
	GrepOutputFilesWithMatches GrepOutputMode = "files_with_matches"

	// GrepOutputCount returns the count of matches per file.
	GrepOutputCount GrepOutputMode = "count"
)

// GrepInput represents the input parameters for the Grep tool.
type GrepInput struct {
	// Pattern is the regular expression to search for. Required.
	Pattern string `json:"pattern"`

	// Path is the file or directory to search in.
	// Defaults to the current working directory if empty.
	Path string `json:"path,omitempty"`

	// Glob filters files by pattern (e.g., "*.go", "*.{ts,tsx}").
	Glob string `json:"glob,omitempty"`

	// Type filters files by type (e.g., "go", "ts", "py", "js", "rust").
	// More efficient than Glob for common file types.
	Type string `json:"type,omitempty"`

	// OutputMode controls the format of results.
	// Defaults to GrepOutputFilesWithMatches.
	OutputMode GrepOutputMode `json:"output_mode,omitempty"`

	// CaseInsens enables case-insensitive matching.
	CaseInsens bool `json:"-i,omitempty"`

	// ShowLines includes line numbers in output (default true).
	// Only applies when OutputMode is GrepOutputContent.
	// A nil value means true.
	ShowLines *bool `json:"-n,omitempty"`

	// Context shows N lines before and after each match.
	// Only applies when OutputMode is GrepOutputContent.
	Context int `json:"-C,omitempty"`

	// Before shows N lines before each match.
	Before int `json:"-B,omitempty"`

	// After shows N lines after each match.
	After int `json:"-A,omitempty"`

	// Multiline enables patterns to match across line boundaries.
	// In this mode, . matches newlines and patterns can span multiple lines.
	Multiline bool `json:"multiline,omitempty"`

	// HeadLimit restricts the page to N entries, up to MaxResults.
	// Defaults to MaxResults when zero.
	HeadLimit int `json:"head_limit,omitempty"`

	// Offset skips the first N entries before applying HeadLimit.
	Offset int `json:"offset,omitempty"`
}

// GrepToolOptions configures the behavior of [GrepTool].
type GrepToolOptions struct {
	// DefaultExcludes are glob patterns to exclude from searches.
	// Common defaults include node_modules, .git, vendor, etc.
	DefaultExcludes []string

	// MaxResults limits entries returned per call. Content mode counts matching
	// lines; files_with_matches and count modes count files. Use Offset to page.
	// Defaults to 1000 if not specified.
	MaxResults int

	// UseRipgrep enables using ripgrep (rg) when available.
	// Ripgrep is significantly faster for large codebases.
	// Falls back to Go regexp if ripgrep is not installed.
	// Both paths search regular files up to 64 MiB each.
	UseRipgrep bool

	// WorkspaceDir restricts searches to paths within this directory.
	// If empty, no workspace restriction is applied (access to the entire
	// filesystem). Ignored when Validator is set.
	WorkspaceDir string

	// Validator is an optional shared PathValidator. When set, it is used
	// instead of creating one from WorkspaceDir.
	Validator *PathValidator
}

// GrepTool searches file contents using regular expressions.
//
// This tool is essential for finding code patterns, function definitions,
// usages, and other text within a codebase. It supports both ripgrep
// (when available) and a pure Go fallback implementation.
//
// Features:
//   - Full regex syntax with optional case-insensitive and multiline modes
//   - File type and glob pattern filtering
//   - Multiple output modes: content, file list, or counts
//   - Context lines around matches
//   - Automatic exclusion of non-source directories
//
// Performance: When ripgrep is available and UseRipgrep is true, searches
// are significantly faster, especially in large codebases.
type GrepTool struct {
	defaultExcludes []string
	maxResults      int
	useRipgrep      bool
	ripgrepPath     string
	pathValidator   *PathValidator
	workspaceDir    string
	configErr       error
}

// NewGrepTool creates a new GrepTool with the given options.
// If no options are provided, sensible defaults are used.
func NewGrepTool(opts ...GrepToolOptions) *dive.TypedToolAdapter[*GrepInput] {
	var resolvedOpts GrepToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.MaxResults == 0 {
		resolvedOpts.MaxResults = 1000
	}
	if resolvedOpts.DefaultExcludes == nil {
		resolvedOpts.DefaultExcludes = []string{
			"**/node_modules/**",
			"**/.git/**",
			"**/vendor/**",
			"**/__pycache__/**",
			"**/.venv/**",
			"**/dist/**",
			"**/build/**",
		}
	}

	// Check if ripgrep is available
	ripgrepPath := ""
	if resolvedOpts.UseRipgrep {
		if path, err := exec.LookPath("rg"); err == nil {
			ripgrepPath = path
		}
	}

	var pathValidator *PathValidator
	var configErr error
	if resolvedOpts.MaxResults < 0 || resolvedOpts.MaxResults > 10000 {
		configErr = fmt.Errorf("MaxResults must be between 1 and 10000")
	}
	if resolvedOpts.Validator != nil {
		pathValidator = resolvedOpts.Validator
	} else if resolvedOpts.WorkspaceDir != "" {
		var err error
		pathValidator, err = NewPathValidator(resolvedOpts.WorkspaceDir)
		if err != nil {
			configErr = fmt.Errorf("invalid workspace configuration for WorkspaceDir %q: %w", resolvedOpts.WorkspaceDir, err)
		}
	}

	return dive.ToolAdapter(&GrepTool{
		defaultExcludes: resolvedOpts.DefaultExcludes,
		maxResults:      resolvedOpts.MaxResults,
		useRipgrep:      resolvedOpts.UseRipgrep,
		ripgrepPath:     ripgrepPath,
		pathValidator:   pathValidator,
		workspaceDir:    resolvedOpts.WorkspaceDir,
		configErr:       configErr,
	})
}

// Name returns "Grep" as the tool identifier.
func (t *GrepTool) Name() string {
	return "Grep"
}

// Description returns detailed usage instructions for the LLM.
func (t *GrepTool) Description() string {
	return `Search file contents using regular expressions.

A powerful content search tool built on ripgrep (if available) with Go regexp fallback.

Parameters:
- pattern: The regex pattern to search for (required)
- path: Directory or file to search (defaults to current directory)
- glob: Glob pattern to filter files (e.g., "*.go", "*.{ts,tsx}")
- type: File type to search (e.g., "go", "ts", "py", "js", "rust")
- output_mode: "content" (matching lines), "files_with_matches" (file paths only), "count" (match counts)
- -i: Case insensitive search
- -n: Show line numbers (for output_mode: "content")
- -A: Lines to show after each match
- -B: Lines to show before each match
- -C: Lines to show before and after each match
- multiline: Enable multiline mode where . matches newlines
- head_limit: Limit output to first N entries
- offset: Skip first N entries

Searches regular text files up to 64 MiB, including hidden and ignored files
except configured exclusions. Missing paths are errors. Results state when
pagination, skipped paths, or output limits make the answer incomplete.

Examples:
- Search for function definitions: {"pattern": "func\\s+\\w+", "type": "go"}
- Find TODO comments: {"pattern": "TODO:", "glob": "*.ts", "-i": true}
- Show context around matches: {"pattern": "error", "-C": 3, "output_mode": "content"}`
}

// Schema returns the JSON schema describing the tool's input parameters.
func (t *GrepTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"pattern"},
		Properties: map[string]*schema.Property{
			"pattern": {
				Type:        "string",
				Description: "The regular expression pattern to search for in file contents",
			},
			"path": {
				Type:        "string",
				Description: "File or directory to search in. Defaults to current working directory.",
			},
			"glob": {
				Type:        "string",
				Description: "Glob pattern to filter files (e.g., \"*.js\", \"*.{ts,tsx}\")",
			},
			"type": {
				Type:        "string",
				Description: "File type to search (e.g., \"js\", \"py\", \"rust\", \"go\", \"java\")",
			},
			"output_mode": {
				Type:        "string",
				Enum:        []any{"content", "files_with_matches", "count"},
				Description: "Output mode: \"content\" shows matching lines, \"files_with_matches\" shows file paths (default), \"count\" shows match counts",
			},
			"-i": {
				Type:        "boolean",
				Description: "Case insensitive search",
			},
			"-n": {
				Type:        "boolean",
				Description: "Show line numbers in output. Requires output_mode: \"content\". Defaults to true.",
			},
			"-A": {
				Type:        "integer",
				Description: "Number of lines to show after each match. Requires output_mode: \"content\".",
			},
			"-B": {
				Type:        "integer",
				Description: "Number of lines to show before each match. Requires output_mode: \"content\".",
			},
			"-C": {
				Type:        "integer",
				Description: "Number of lines to show before and after each match. Requires output_mode: \"content\".",
			},
			"multiline": {
				Type:        "boolean",
				Description: "Enable multiline mode where . matches newlines and patterns can span lines",
			},
			"head_limit": {
				Type:        "integer",
				Description: "Limit this page to N matching lines (content mode) or files (other modes), up to the configured MaxResults. Defaults to MaxResults.",
			},
			"offset": {
				Type:        "integer",
				Description: "Skip the first N matching lines (content mode) or files (other modes) before returning a page. Defaults to 0.",
			},
		},
	}
}

// Annotations returns metadata hints about the tool's behavior.
// Grep is marked as read-only and idempotent.
func (t *GrepTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Grep",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

// PreviewCall returns a summary of the search operation for permission prompts.
func (t *GrepTool) PreviewCall(ctx context.Context, input *GrepInput) *dive.ToolCallPreview {
	if input == nil {
		return nil
	}
	searchPath := input.Path
	if searchPath == "" {
		searchPath = "."
	}
	filter := ""
	if input.Glob != "" {
		filter = fmt.Sprintf(" (%s)", input.Glob)
	} else if input.Type != "" {
		filter = fmt.Sprintf(" (*.%s)", input.Type)
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Search for %q in %s%s", input.Pattern, searchPath, filter),
	}
}

// Call searches for the pattern and returns matches in the requested format.
//
// The search is performed using ripgrep if available and enabled, otherwise
// using Go's built-in regexp package. Results are formatted according to
// the OutputMode setting.
func (t *GrepTool) Call(ctx context.Context, input *GrepInput) (*dive.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t.configErr != nil {
		return dive.NewToolResultError(fmt.Sprintf("error: %s", t.configErr.Error())), nil
	}
	if t.workspaceDir != "" && t.pathValidator == nil {
		return dive.NewToolResultError(fmt.Sprintf("error: invalid workspace configuration for WorkspaceDir %q: path validator is not initialized", t.workspaceDir)), nil
	}

	return t.search(ctx, input)
}
