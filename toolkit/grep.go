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
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/gobwas/glob"
)

var (
	_ dive.TypedTool[*GrepInput]          = &GrepTool{}
	_ dive.TypedToolPreviewer[*GrepInput] = &GrepTool{}
)

// GrepOutputMode specifies what type of output to produce
type GrepOutputMode string

const (
	GrepOutputContent          GrepOutputMode = "content"
	GrepOutputFilesWithMatches GrepOutputMode = "files_with_matches"
	GrepOutputCount            GrepOutputMode = "count"
)

// GrepInput represents the input parameters for the grep tool
type GrepInput struct {
	Pattern    string         `json:"pattern"`
	Path       string         `json:"path,omitempty"`
	Glob       string         `json:"glob,omitempty"`
	Type       string         `json:"type,omitempty"`
	OutputMode GrepOutputMode `json:"output_mode,omitempty"`
	CaseInsens bool           `json:"-i,omitempty"`          // Case insensitive search
	ShowLines  bool           `json:"-n,omitempty"`          // Show line numbers (default true)
	Context    int            `json:"-C,omitempty"`          // Lines before and after each match
	Before     int            `json:"-B,omitempty"`          // Lines before each match
	After      int            `json:"-A,omitempty"`          // Lines after each match
	Multiline  bool           `json:"multiline,omitempty"`   // Enable multiline mode
	HeadLimit  int            `json:"head_limit,omitempty"`  // Limit output to first N entries
	Offset     int            `json:"offset,omitempty"`      // Skip first N entries
}

// GrepToolOptions configures the GrepTool
type GrepToolOptions struct {
	// DefaultExcludes are glob patterns to exclude by default
	DefaultExcludes []string
	// MaxResults limits the number of results
	MaxResults int
	// UseRipgrep attempts to use ripgrep if available
	UseRipgrep bool
}

// GrepTool is a tool for searching file contents using regex patterns
type GrepTool struct {
	defaultExcludes []string
	maxResults      int
	useRipgrep      bool
	ripgrepPath     string
}

// NewGrepTool creates a new GrepTool
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

	return dive.ToolAdapter(&GrepTool{
		defaultExcludes: resolvedOpts.DefaultExcludes,
		maxResults:      resolvedOpts.MaxResults,
		useRipgrep:      resolvedOpts.UseRipgrep,
		ripgrepPath:     ripgrepPath,
	})
}

func (t *GrepTool) Name() string {
	return "grep"
}

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
				Enum:        []string{"content", "files_with_matches", "count"},
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

func (t *GrepTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Search Content",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

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

// ripgrepMatch represents a match from ripgrep's JSON output
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

type grepMatch struct {
	file       string
	lineNumber int
	line       string
}

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

func (t *GrepTool) formatNoMatches(input *GrepInput) *dive.ToolResult {
	display := fmt.Sprintf("No matches found for %q", input.Pattern)
	return dive.NewToolResultText("No matches found").WithDisplay(display)
}
