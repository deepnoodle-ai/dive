package toolkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/gobwas/glob"
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
	ShowLines bool `json:"-n,omitempty"`

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

	// HeadLimit restricts output to the first N entries.
	// Defaults to 0 (unlimited, up to MaxResults).
	HeadLimit int `json:"head_limit,omitempty"`

	// Offset skips the first N entries before applying HeadLimit.
	Offset int `json:"offset,omitempty"`
}

// GrepToolOptions configures the behavior of [GrepTool].
type GrepToolOptions struct {
	// DefaultExcludes are glob patterns to exclude from searches.
	// Common defaults include node_modules, .git, vendor, etc.
	DefaultExcludes []string

	// MaxResults limits the total number of matches returned.
	// Defaults to 1000 if not specified.
	MaxResults int

	// UseRipgrep enables using ripgrep (rg) when available.
	// Ripgrep is significantly faster for large codebases.
	// Falls back to Go regexp if ripgrep is not installed.
	UseRipgrep bool

	// WorkspaceDir restricts searches to paths within this directory.
	// Defaults to the current working directory if empty.
	WorkspaceDir string
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
	if len(resolvedOpts.DefaultExcludes) == 0 {
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
	if resolvedOpts.WorkspaceDir != "" {
		pathValidator, _ = NewPathValidator(resolvedOpts.WorkspaceDir)
	}

	return dive.ToolAdapter(&GrepTool{
		defaultExcludes: resolvedOpts.DefaultExcludes,
		maxResults:      resolvedOpts.MaxResults,
		useRipgrep:      resolvedOpts.UseRipgrep,
		ripgrepPath:     ripgrepPath,
		pathValidator:   pathValidator,
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
				Description: "Limit output to first N lines/entries. Works across all output modes. Defaults to 0 (unlimited).",
			},
			"offset": {
				Type:        "integer",
				Description: "Skip first N lines/entries before applying head_limit. Defaults to 0.",
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
	// Use ripgrep if available
	if t.ripgrepPath != "" {
		return t.callRipgrep(ctx, input)
	}
	return t.callPureGo(ctx, input)
}

// callRipgrep uses ripgrep for searching
func (t *GrepTool) callRipgrep(ctx context.Context, input *GrepInput) (*dive.ToolResult, error) {
	searchPath := input.Path
	if searchPath == "" {
		var err error
		searchPath, err = os.Getwd()
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error getting current directory: %v", err)), nil
		}
	}

	// Validate path is within workspace (skip validation if no validator configured)
	if t.pathValidator != nil {
		if err := t.pathValidator.ValidateRead(searchPath); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
	}

	args := []string{"--json"}

	// Case sensitivity
	if input.CaseInsens {
		args = append(args, "--ignore-case")
	}

	// Multiline
	if input.Multiline {
		args = append(args, "--multiline", "--multiline-dotall")
	}

	// Context
	if input.Context > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", input.Context))
	}
	if input.Before > 0 {
		args = append(args, "-B", fmt.Sprintf("%d", input.Before))
	}
	if input.After > 0 {
		args = append(args, "-A", fmt.Sprintf("%d", input.After))
	}

	// File filtering
	if input.Glob != "" {
		args = append(args, "--glob", input.Glob)
	}
	if input.Type != "" {
		args = append(args, "--type", input.Type)
	}

	// Add default excludes
	for _, exclude := range t.defaultExcludes {
		args = append(args, "--glob", "!"+exclude)
	}

	// Pattern and path
	args = append(args, "--regexp", input.Pattern)
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, t.ripgrepPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means no matches, which is fine
			if exitError.ExitCode() == 1 {
				return t.formatNoMatches(input), nil
			}
		}
		return dive.NewToolResultError(fmt.Sprintf("ripgrep error: %v\n%s", err, stderr.String())), nil
	}

	return t.parseRipgrepOutput(stdout.String(), searchPath, input)
}

// ripgrepMatch represents a single match from ripgrep's JSON output format.
// This struct maps to ripgrep's --json output for parsing results.
type ripgrepMatch struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func (t *GrepTool) parseRipgrepOutput(output, basePath string, input *GrepInput) (*dive.ToolResult, error) {
	var matches []grepMatch
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var m ripgrepMatch
		if err := json.Unmarshal([]byte(scanner.Text()), &m); err != nil {
			continue
		}
		if m.Type != "match" {
			continue
		}
		relPath, _ := filepath.Rel(basePath, m.Data.Path.Text)
		if relPath == "" {
			relPath = m.Data.Path.Text
		}
		matches = append(matches, grepMatch{
			file:       relPath,
			lineNumber: m.Data.LineNumber,
			line:       strings.TrimRight(m.Data.Lines.Text, "\n\r"),
		})

		limit := input.HeadLimit
		if limit == 0 {
			limit = t.maxResults
		}
		if len(matches) >= limit {
			break
		}
	}

	return t.formatResults(matches, input)
}

// callPureGo uses Go's built-in regex for searching
func (t *GrepTool) callPureGo(ctx context.Context, input *GrepInput) (*dive.ToolResult, error) {
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

	// Compile regex
	pattern := input.Pattern
	if input.CaseInsens {
		pattern = "(?i)" + pattern
	}
	if input.Multiline {
		pattern = "(?s)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Invalid regex pattern: %v", err)), nil
	}

	// Compile file filter
	var fileFilter glob.Glob
	if input.Glob != "" {
		fileFilter, err = glob.Compile(input.Glob, '/')
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Invalid glob pattern: %v", err)), nil
		}
	}

	// Type to extension mapping
	typeToExt := map[string][]string{
		"go":     {".go"},
		"ts":     {".ts", ".tsx"},
		"js":     {".js", ".jsx"},
		"py":     {".py"},
		"rust":   {".rs"},
		"java":   {".java"},
		"c":      {".c", ".h"},
		"cpp":    {".cpp", ".cc", ".cxx", ".hpp", ".hh"},
		"rb":     {".rb"},
		"php":    {".php"},
		"swift":  {".swift"},
		"kotlin": {".kt", ".kts"},
		"scala":  {".scala"},
		"md":     {".md", ".markdown"},
		"json":   {".json"},
		"yaml":   {".yaml", ".yml"},
		"xml":    {".xml"},
		"html":   {".html", ".htm"},
		"css":    {".css", ".scss", ".sass", ".less"},
		"sql":    {".sql"},
		"sh":     {".sh", ".bash"},
	}

	// Compile exclude patterns
	excludeGlobs := make([]glob.Glob, 0, len(t.defaultExcludes))
	for _, pattern := range t.defaultExcludes {
		if eg, err := glob.Compile(pattern, '/'); err == nil {
			excludeGlobs = append(excludeGlobs, eg)
		}
	}

	var matches []grepMatch

	limit := input.HeadLimit
	if limit == 0 {
		limit = t.maxResults
	}

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(searchPath, path)
		relPath = filepath.ToSlash(relPath)

		// Skip directories but check excludes
		if info.IsDir() {
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

		// Check file filter
		if fileFilter != nil && !fileFilter.Match(relPath) {
			return nil
		}

		// Check type filter
		if input.Type != "" {
			exts, ok := typeToExt[input.Type]
			if ok {
				ext := filepath.Ext(path)
				found := false
				for _, e := range exts {
					if strings.EqualFold(ext, e) {
						found = true
						break
					}
				}
				if !found {
					return nil
				}
			}
		}

		// Read and search file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Skip binary files
		if bytes.Contains(content[:min(len(content), 512)], []byte{0}) {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, grepMatch{
					file:       relPath,
					lineNumber: i + 1,
					line:       strings.TrimRight(line, "\r"),
				})
				if len(matches) >= limit {
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return dive.NewToolResultError(fmt.Sprintf("Error walking directory: %v", err)), nil
	}

	return t.formatResults(matches, input)
}

// grepMatch represents a single match found during a search.
type grepMatch struct {
	file       string // Relative path to the file containing the match
	lineNumber int    // 1-based line number of the match
	line       string // Content of the matching line
}

// formatResults converts matches into the output format specified by OutputMode.
func (t *GrepTool) formatResults(matches []grepMatch, input *GrepInput) (*dive.ToolResult, error) {
	if len(matches) == 0 {
		return t.formatNoMatches(input), nil
	}

	outputMode := input.OutputMode
	if outputMode == "" {
		outputMode = GrepOutputFilesWithMatches
	}

	var result strings.Builder

	switch outputMode {
	case GrepOutputFilesWithMatches:
		// Unique files only
		seen := make(map[string]bool)
		var files []string
		for _, m := range matches {
			if !seen[m.file] {
				seen[m.file] = true
				files = append(files, m.file)
			}
		}
		sort.Strings(files)
		for _, f := range files {
			result.WriteString(f)
			result.WriteString("\n")
		}

	case GrepOutputCount:
		// Count by file
		counts := make(map[string]int)
		for _, m := range matches {
			counts[m.file]++
		}
		var files []string
		for f := range counts {
			files = append(files, f)
		}
		sort.Strings(files)
		for _, f := range files {
			result.WriteString(fmt.Sprintf("%s:%d\n", f, counts[f]))
		}

	case GrepOutputContent:
		// Group by file
		byFile := make(map[string][]grepMatch)
		var files []string
		for _, m := range matches {
			if _, ok := byFile[m.file]; !ok {
				files = append(files, m.file)
			}
			byFile[m.file] = append(byFile[m.file], m)
		}
		sort.Strings(files)

		for _, f := range files {
			result.WriteString(fmt.Sprintf("## %s\n", f))
			for _, m := range byFile[f] {
				result.WriteString(fmt.Sprintf("%d: %s\n", m.lineNumber, m.line))
			}
			result.WriteString("\n")
		}
	}

	display := fmt.Sprintf("Found %d match(es) for %q", len(matches), input.Pattern)
	limit := input.HeadLimit
	if limit == 0 {
		limit = t.maxResults
	}
	if len(matches) >= limit {
		display += fmt.Sprintf(" (limited to %d)", limit)
	}

	return dive.NewToolResultText(strings.TrimSpace(result.String())).WithDisplay(display), nil
}

// formatNoMatches returns a result indicating no matches were found.
func (t *GrepTool) formatNoMatches(input *GrepInput) *dive.ToolResult {
	display := fmt.Sprintf("No matches found for %q", input.Pattern)
	return dive.NewToolResultText("No matches found").WithDisplay(display)
}
