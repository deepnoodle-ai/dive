package toolkit

import (
	"container/heap"
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

type globFileEntry struct {
	path    string
	modTime time.Time
}

const maxGlobOutputBytes = 1 << 20

// globOldestFirst keeps the least useful result at the root. A path tie
// breaker makes the selected set stable when files have equal timestamps.
type globOldestFirst []globFileEntry

func (h globOldestFirst) Len() int { return len(h) }
func (h globOldestFirst) Less(i, j int) bool {
	if h[i].modTime.Equal(h[j].modTime) {
		return h[i].path > h[j].path
	}
	return h[i].modTime.Before(h[j].modTime)
}
func (h globOldestFirst) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *globOldestFirst) Push(x any)   { *h = append(*h, x.(globFileEntry)) }
func (h *globOldestFirst) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// compileGlobVariants gives **/ its usual zero-or-more-directories meaning.
// gobwas/glob requires a separator for **/, so the zero-directory variants
// must be compiled separately. Bound expansion for pathological patterns.
func compileGlobVariants(pattern string) ([]*glob.Pattern, error) {
	variants := []string{pattern}
	for i := 0; i < len(variants); i++ {
		if len(variants) > 32 {
			return nil, fmt.Errorf("too many recursive segments in glob pattern")
		}
		for pos := 0; pos < len(variants[i]); {
			idx := strings.Index(variants[i][pos:], "**/")
			if idx < 0 {
				break
			}
			idx += pos
			candidate := variants[i][:idx] + variants[i][idx+3:]
			found := false
			for _, existing := range variants {
				if existing == candidate {
					found = true
					break
				}
			}
			if !found {
				variants = append(variants, candidate)
			}
			pos = idx + 3
		}
	}
	compiled := make([]*glob.Pattern, 0, len(variants))
	for _, variant := range variants {
		g, err := glob.Compile(variant, '/')
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, g)
	}
	return compiled, nil
}

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
	// If empty, no workspace restriction is applied (access to the entire
	// filesystem). Ignored when Validator is set.
	WorkspaceDir string

	// Validator is an optional shared PathValidator. When set, it is used
	// instead of creating one from WorkspaceDir.
	Validator *PathValidator
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
// The tool only matches regular files, not directories or symlinks.
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
	return dive.ToolAdapter(&GlobTool{
		defaultExcludes: resolvedOpts.DefaultExcludes,
		maxResults:      resolvedOpts.MaxResults,
		pathValidator:   pathValidator,
		workspaceDir:    resolvedOpts.WorkspaceDir,
		configErr:       configErr,
	})
}

// matchesExclude reports whether relPath matches the exclude pattern.
// Patterns are tested against both the bare relative path and a "./"-prefixed
// variant: under gobwas/glob, "**/" requires a literal separator, so a pattern
// like "**/node_modules/**" would otherwise never match entries at the search
// root (e.g. "node_modules/foo.js"). Testing the "./" form makes the pure-Go
// matching agree with ripgrep's gitignore-style globs, where "**/" also
// matches zero directories.
func matchesExclude(eg *glob.Pattern, relPath string) bool {
	return eg.Match(relPath) || eg.Match("./"+relPath)
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

Returns regular file paths sorted by modification time (most recent first).
Directory symlinks are not followed. Results state when the file limit,
inaccessible paths, or output limit make the answer incomplete.`
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
	if input == nil {
		return nil
	}
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil || input.Pattern == "" {
		return dive.NewToolResultError("pattern must not be empty"), nil
	}
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

	// Reject a symlinked search root. Walk does not follow it and would
	// otherwise report a complete search with no matches.
	info, err := os.Lstat(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return dive.NewToolResultError(fmt.Sprintf("Path does not exist: %s", searchPath)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf("Cannot access path %s: %v", searchPath, err)), nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return dive.NewToolResultError(fmt.Sprintf("Search root must not be a symlink: %s", searchPath)), nil
	}
	if !info.IsDir() {
		return dive.NewToolResultError(fmt.Sprintf("Path is not a directory: %s", searchPath)), nil
	}

	// Compile the glob pattern
	patterns, err := compileGlobVariants(input.Pattern)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Invalid glob pattern: %v", err)), nil
	}

	// Compile exclude patterns
	excludeGlobs := make([]*glob.Pattern, 0, len(t.defaultExcludes))
	for _, pattern := range t.defaultExcludes {
		compiled, err := compileGlobVariants(pattern)
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Invalid exclude pattern %q: %v", pattern, err)), nil
		}
		excludeGlobs = append(excludeGlobs, compiled...)
	}

	// Keep only the newest files while scanning the whole tree. Stopping at
	// MaxResults before sorting would return the first paths in walk order.
	matches := &globOldestFirst{}
	heap.Init(matches)
	truncated := false
	skipped := 0

	// The walk checks ctx at every entry because a broad pattern over a large
	// tree (a home directory) can run for minutes, and nothing else stops it:
	// without the check, cancelling the agent run left the walk going and held
	// the run open until it finished.
	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			skipped++
			return nil
		}

		// Get relative path for pattern matching
		relPath, err := filepath.Rel(searchPath, path)
		if err != nil {
			skipped++
			return nil
		}

		// Normalize path separators for matching
		relPath = filepath.ToSlash(relPath)

		// Skip directories but continue walking them
		if info.IsDir() {
			// Check if directory should be excluded
			for _, eg := range excludeGlobs {
				if matchesExclude(eg, relPath) || matchesExclude(eg, relPath+"/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		// Check excludes
		for _, eg := range excludeGlobs {
			if matchesExclude(eg, relPath) {
				return nil
			}
		}

		// Check if matches pattern
		matched := false
		for _, pattern := range patterns {
			if pattern.Match(relPath) {
				matched = true
				break
			}
		}
		if matched {
			entry := globFileEntry{path: relPath, modTime: info.ModTime()}
			if matches.Len() < t.maxResults {
				heap.Push(matches, entry)
			} else {
				truncated = true
				oldest := (*matches)[0]
				if entry.modTime.After(oldest.modTime) || (entry.modTime.Equal(oldest.modTime) && entry.path < oldest.path) {
					heap.Pop(matches)
					heap.Push(matches, entry)
				}
			}
		}

		return nil
	})

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil && err != filepath.SkipAll {
		return dive.NewToolResultError(fmt.Sprintf("Error walking directory: %v", err)), nil
	}

	if matches.Len() == 0 {
		if skipped > 0 {
			return dive.NewToolResultError(fmt.Sprintf("Search incomplete: skipped %d inaccessible paths; no matches were found in the paths searched", skipped)), nil
		}
		display := fmt.Sprintf("No files matching %q found in %s", input.Pattern, searchPath)
		return dive.NewToolResultText("No matching files found").WithDisplay(display), nil
	}

	// Sort by modification time (most recent first)
	sort.Slice(*matches, func(i, j int) bool {
		a, b := (*matches)[i], (*matches)[j]
		if a.modTime.Equal(b.modTime) {
			return a.path < b.path
		}
		return a.modTime.After(b.modTime)
	})

	// Build result
	var result strings.Builder
	shown := 0
	outputTruncated := false
	for _, m := range *matches {
		if result.Len()+len(m.path)+1 > maxGlobOutputBytes {
			outputTruncated = true
			break
		}
		result.WriteString(m.path)
		result.WriteString("\n")
		shown++
	}

	display := fmt.Sprintf("Found %d file(s) matching %q", shown, input.Pattern)
	if truncated {
		display += fmt.Sprintf(" (showing newest %d; more matches exist)", t.maxResults)
	}
	toolResult := dive.NewToolResultText(strings.TrimSpace(result.String())).WithDisplay(display)
	if truncated || skipped > 0 || outputTruncated {
		var note strings.Builder
		if truncated {
			fmt.Fprintf(&note, "Search truncated: showing the newest %d matching files; more matches exist.", t.maxResults)
		}
		if skipped > 0 {
			fmt.Fprintf(&note, " Search incomplete: skipped %d inaccessible paths.", skipped)
		}
		if outputTruncated {
			fmt.Fprintf(&note, " Output text capped at 1 MiB: showing %d of %d selected paths; narrow the search.", shown, matches.Len())
		}
		toolResult.Content = append(toolResult.Content, &dive.ToolResultContent{Type: dive.ToolResultContentTypeText, Text: strings.TrimSpace(note.String())})
	}
	return toolResult, nil
}
