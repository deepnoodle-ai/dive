package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/gobwas/glob"
)

var (
	_ dive.TypedTool[*GlobInput]          = &GlobTool{}
	_ dive.TypedToolPreviewer[*GlobInput] = &GlobTool{}
)

// GlobInput represents the input parameters for the Glob tool.
type GlobInput struct {
	// Pattern is the glob pattern to match files against. Required.
	// Supports *, **, ?, [abc], and {a,b,c} syntax.
	Pattern string `json:"pattern"`

	// Path is the directory to search in.
	// Defaults to the current working directory if empty.
	Path string `json:"path,omitempty"`
}

// GlobToolOptions configures the behavior of [GlobTool].
type GlobToolOptions struct {
	// DefaultExcludes are glob patterns to exclude from results.
	// Common defaults include node_modules, .git, vendor, etc.
	// Set to an empty slice to disable default exclusions.
	DefaultExcludes []string

	// MaxResults limits the number of files returned.
	// Defaults to 500 if not specified.
	MaxResults int

	// WorkspaceDir restricts searches to paths within this directory.
	// Defaults to the current working directory if empty.
	WorkspaceDir string
}

// GlobTool finds files matching glob patterns.
//
// This tool is useful for discovering files in a codebase by pattern.
// Results are sorted by modification time (most recent first) to help
// identify recently changed files.
//
// Features:
//   - Full glob syntax: *, **, ?, [abc], {a,b,c}
//   - Automatic exclusion of common non-source directories
//   - Results sorted by modification time
//   - Configurable result limit
//
// The tool only matches files, not directories.
type GlobTool struct {
	defaultExcludes []string
	maxResults      int
	pathValidator   *PathValidator
	workspaceDir    string
	configErr       error
}

// NewGlobTool creates a new GlobTool with the given options.
// If no options are provided, sensible defaults are used.
func NewGlobTool(opts ...GlobToolOptions) *dive.TypedToolAdapter[*GlobInput] {
	var resolvedOpts GlobToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.MaxResults == 0 {
		resolvedOpts.MaxResults = 500
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
			"**/*.min.js",
			"**/*.min.css",
		}
	}
	var pathValidator *PathValidator
	var configErr error
	if resolvedOpts.WorkspaceDir != "" {
		pathValidator, configErr = NewPathValidator(resolvedOpts.WorkspaceDir)
		if configErr != nil {
			configErr = fmt.Errorf("invalid workspace configuration for WorkspaceDir %q: %w", resolvedOpts.WorkspaceDir, configErr)
		}
	}
	return dive.ToolAdapter(&GlobTool{
		defaultExcludes: resolvedOpts.DefaultExcludes,
		maxResults:      resolvedOpts.MaxResults,
		pathValidator:   pathValidator,
		workspaceDir:    resolvedOpts.WorkspaceDir,
		configErr:       configErr,
	})
}

// Name returns "Glob" as the tool identifier.
func (t *GlobTool) Name() string {
	return "Glob"
}

// Description returns detailed usage instructions for the LLM.
func (t *GlobTool) Description() string {
	return `Find files matching a glob pattern.

Supports standard glob patterns:
- * matches any sequence of characters (not including path separators)
- ** matches any sequence of characters (including path separators)
- ? matches any single character
- [abc] matches any character in the set
- {a,b,c} matches any of the alternatives

Examples:
- "**/*.go" - all Go files
- "src/**/*.ts" - all TypeScript files under src
- "*.{js,ts}" - all JS or TS files in current directory
- "test_*.py" - all Python test files in current directory

Returns file paths sorted by modification time (most recent first).`
}

// Schema returns the JSON schema describing the tool's input parameters.
func (t *GlobTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"pattern"},
		Properties: map[string]*schema.Property{
			"pattern": {
				Type:        "string",
				Description: "The glob pattern to match files against (e.g., \"**/*.go\", \"src/**/*.ts\")",
			},
			"path": {
				Type:        "string",
				Description: "The directory to search in. Defaults to current working directory if not specified.",
			},
		},
	}
}

// Annotations returns metadata hints about the tool's behavior.
// Glob is marked as read-only and idempotent.
func (t *GlobTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Glob",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

// PreviewCall returns a summary of the search operation for permission prompts.
func (t *GlobTool) PreviewCall(ctx context.Context, input *GlobInput) *dive.ToolCallPreview {
	searchPath := input.Path
	if searchPath == "" {
		searchPath = "."
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Find files matching %q in %s", input.Pattern, searchPath),
	}
}

// Call searches for files matching the glob pattern.
//
// Returns matching file paths as newline-separated text, sorted by
// modification time (most recent first). If no files match, returns
// a message indicating no matches were found.
func (t *GlobTool) Call(ctx context.Context, input *GlobInput) (*dive.ToolResult, error) {
	if t.configErr != nil {
		return dive.NewToolResultError(fmt.Sprintf("error: %s", t.configErr.Error())), nil
	}
	if t.workspaceDir != "" && t.pathValidator == nil {
		return dive.NewToolResultError(fmt.Sprintf("error: invalid workspace configuration for WorkspaceDir %q: path validator is not initialized", t.workspaceDir)), nil
	}

	searchPath := input.Path
	if searchPath == "" {
		var err error
		searchPath, err = os.Getwd()
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting current directory: %v", err)), nil
		}
	}

	// Resolve to absolute path
	if !filepath.IsAbs(searchPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting current directory: %v", err)), nil
		}
		searchPath = filepath.Join(cwd, searchPath)
	}

	// Validate path is within workspace (skip validation if no validator configured)
	if t.pathValidator != nil {
		if err := t.pathValidator.ValidateRead(searchPath); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
	}

	// Check if path exists
	info, err := os.Stat(searchPath)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Path does not exist: %s", searchPath)), nil
	}
	if !info.IsDir() {
		return dive.NewToolResultError(fmt.Sprintf("Path is not a directory: %s", searchPath)), nil
	}

	// Compile the glob pattern
	g, err := glob.Compile(input.Pattern, '/')
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Invalid glob pattern: %v", err)), nil
	}

	// Compile exclude patterns
	excludeGlobs := make([]glob.Glob, 0, len(t.defaultExcludes))
	for _, pattern := range t.defaultExcludes {
		if eg, err := glob.Compile(pattern, '/'); err == nil {
			excludeGlobs = append(excludeGlobs, eg)
		}
	}

	// Find matching files
	type fileEntry struct {
		path    string
		modTime time.Time
	}
	var matches []fileEntry

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Get relative path for pattern matching
		relPath, err := filepath.Rel(searchPath, path)
		if err != nil {
			return nil
		}

		// Normalize path separators for matching
		relPath = filepath.ToSlash(relPath)

		// Skip directories but continue walking them
		if info.IsDir() {
			// Check if directory should be excluded
			for _, eg := range excludeGlobs {
				if eg.Match(relPath) || eg.Match(relPath+"/") {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check excludes
		for _, eg := range excludeGlobs {
			if eg.Match(relPath) {
				return nil
			}
		}

		// Check if matches pattern
		if g.Match(relPath) {
			matches = append(matches, fileEntry{
				path:    relPath,
				modTime: info.ModTime(),
			})

			// Stop if we've reached max results
			if len(matches) >= t.maxResults {
				return filepath.SkipAll
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return dive.NewToolResultError(fmt.Sprintf("Error walking directory: %v", err)), nil
	}

	if len(matches) == 0 {
		display := fmt.Sprintf("No files matching %q found in %s", input.Pattern, searchPath)
		return dive.NewToolResultText("No matching files found").WithDisplay(display), nil
	}

	// Sort by modification time (most recent first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})

	// Build result
	var result strings.Builder
	for _, m := range matches {
		result.WriteString(m.path)
		result.WriteString("\n")
	}

	display := fmt.Sprintf("Found %d file(s) matching %q", len(matches), input.Pattern)
	if len(matches) >= t.maxResults {
		display += fmt.Sprintf(" (limited to %d results)", t.maxResults)
	}

	return dive.NewToolResultText(strings.TrimSpace(result.String())).WithDisplay(display), nil
}
