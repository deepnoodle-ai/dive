package toolkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/deepnoodle-ai/dive"
	"github.com/gobwas/glob"
)

const maxGrepFileBytes = 64 << 20
const maxGrepJSONRecordBytes = 2*maxGrepFileBytes + 1024
const maxGrepOutputBytes = 1 << 20
const maxGrepLineBytes = 16 << 10
const maxMultilineMatchesPerFile = 100000

var errGrepFileTooLarge = errors.New("file exceeds search limit")

type boundedGrepOutput struct {
	text      strings.Builder
	truncated bool
}

func (b *boundedGrepOutput) Write(p []byte) (int, error) {
	length := len(p)
	if b.text.Len() >= maxGrepOutputBytes {
		b.truncated = true
		return length, nil
	}
	remaining := maxGrepOutputBytes - b.text.Len()
	if length > remaining {
		_, _ = b.text.Write(validUTF8Prefix(p, remaining))
		b.truncated = true
		return length, nil
	}
	_, _ = b.text.Write(p)
	return length, nil
}

func (b *boundedGrepOutput) String() string { return b.text.String() }

func validUTF8Prefix(data []byte, limit int) []byte {
	if len(data) <= limit {
		return data
	}
	data = data[:limit]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return data
}

var grepTypeExtensions = map[string][]string{
	"go": {".go"}, "ts": {".ts", ".tsx"}, "js": {".js", ".jsx"},
	"py": {".py"}, "rust": {".rs"}, "java": {".java"},
	"c": {".c", ".h"}, "cpp": {".cpp", ".cc", ".cxx", ".hpp", ".hh"},
	"rb": {".rb"}, "php": {".php"}, "swift": {".swift"},
	"kotlin": {".kt", ".kts"}, "scala": {".scala"},
	"md": {".md", ".markdown"}, "json": {".json"},
	"yaml": {".yaml", ".yml"}, "xml": {".xml"},
	"html": {".html", ".htm"}, "css": {".css", ".scss", ".sass", ".less"},
	"sql": {".sql"}, "sh": {".sh", ".bash"},
}

type grepFileCount struct {
	path  string
	count int
}

type grepMatch struct {
	file       string
	lineNumber int
	endLine    int
	line       string
}

type grepFilters struct {
	include      []*glob.Pattern
	exclude      []*glob.Pattern
	inputPattern string
	typ          string
}

func compileGrepFilters(input *GrepInput, excludes []string) (grepFilters, error) {
	filters := grepFilters{inputPattern: input.Glob, typ: input.Type}
	var err error
	if input.Glob != "" {
		filters.include, err = compileGlobVariants(input.Glob)
		if err != nil {
			return filters, fmt.Errorf("invalid glob pattern: %w", err)
		}
	}
	for _, pattern := range excludes {
		compiled, err := compileGlobVariants(pattern)
		if err != nil {
			return filters, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		filters.exclude = append(filters.exclude, compiled...)
	}
	return filters, nil
}

func (f grepFilters) eligible(path, rel string) bool {
	for _, exclude := range f.exclude {
		if matchesExclude(exclude, rel) {
			return false
		}
	}
	return grepMatchesGlobs(f.include, rel, f.inputPattern) && grepMatchesType(path, f.typ)
}

// grepCollector stores one bounded page, while counting all matches so the
// result can tell the model whether more entries exist. Backends emit matches
// in path order (filepath.Walk and rg --sort path).
type grepCollector struct {
	mode         GrepOutputMode
	offset       int
	limit        int
	totalMatches int
	totalFiles   int
	matches      []grepMatch
	files        []grepFileCount
	currentFile  string
	currentCount int
	warnings     []string
	warningCount int
}

func newGrepCollector(input *GrepInput, maxResults int) *grepCollector {
	mode := input.OutputMode
	if mode == "" {
		mode = GrepOutputFilesWithMatches
	}
	limit := maxResults
	if input.HeadLimit > 0 && input.HeadLimit < limit {
		limit = input.HeadLimit
	}
	return &grepCollector{mode: mode, offset: input.Offset, limit: limit}
}

func (c *grepCollector) add(m grepMatch) {
	c.totalMatches++
	if c.mode == GrepOutputContent {
		if c.totalMatches > c.offset && len(c.matches) < c.limit {
			if m.endLine == 0 {
				m.endLine = m.lineNumber + strings.Count(m.line, "\n")
			}
			m.line = strings.ToValidUTF8(m.line, "�")
			if len(m.line) > maxGrepLineBytes {
				m.line = string(validUTF8Prefix([]byte(m.line), maxGrepLineBytes)) + "... [line truncated]"
			}
			c.matches = append(c.matches, m)
		}
		return
	}
	if c.currentFile != "" && c.currentFile != m.file {
		c.finishFile()
	}
	if c.currentFile == "" {
		c.currentFile = m.file
	}
	c.currentCount++
}

func (c *grepCollector) needsLine() bool {
	return c.mode == GrepOutputContent && c.totalMatches >= c.offset && len(c.matches) < c.limit
}

func (c *grepCollector) finishFile() {
	if c.currentFile == "" {
		return
	}
	c.totalFiles++
	if c.totalFiles > c.offset && len(c.files) < c.limit {
		c.files = append(c.files, grepFileCount{path: c.currentFile, count: c.currentCount})
	}
	c.currentFile = ""
	c.currentCount = 0
}

func (c *grepCollector) warn(message string) {
	c.warningCount++
	if len(c.warnings) < 3 {
		c.warnings = append(c.warnings, message)
	}
}

func (t *GrepTool) search(ctx context.Context, input *GrepInput) (*dive.ToolResult, error) {
	if input == nil {
		return dive.NewToolResultError("grep input is required"), nil
	}
	if input.Pattern == "" {
		return dive.NewToolResultError("pattern must not be empty"), nil
	}
	if input.OutputMode != "" && input.OutputMode != GrepOutputContent && input.OutputMode != GrepOutputFilesWithMatches && input.OutputMode != GrepOutputCount {
		return dive.NewToolResultError(fmt.Sprintf("invalid output_mode %q", input.OutputMode)), nil
	}
	if input.Offset < 0 || input.HeadLimit < 0 || input.Context < 0 || input.Before < 0 || input.After < 0 {
		return dive.NewToolResultError("offset, head_limit, -C, -B, and -A must be non-negative"), nil
	}
	if input.Context > 50 || input.Before > 50 || input.After > 50 {
		return dive.NewToolResultError("-C, -B, and -A may each be at most 50 lines"), nil
	}
	if input.Type != "" {
		if _, ok := grepTypeExtensions[input.Type]; !ok {
			return dive.NewToolResultError(fmt.Sprintf("unsupported file type %q", input.Type)), nil
		}
	}
	if _, err := grepRegex(input); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Invalid regex pattern: %v", err)), nil
	}
	filters, err := compileGrepFilters(input, t.defaultExcludes)
	if err != nil {
		return dive.NewToolResultError(err.Error()), nil
	}
	searchPath := input.Path
	if searchPath == "" {
		searchPath = "."
	}
	searchPath, err = filepath.Abs(searchPath)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("cannot resolve search path: %v", err)), nil
	}
	if t.pathValidator != nil {
		if err := t.pathValidator.ValidateRead(searchPath); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
		}
	}
	rootInfo, err := os.Stat(searchPath)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("cannot access search path %s: %v", searchPath, err)), nil
	}
	if !rootInfo.IsDir() && !rootInfo.Mode().IsRegular() {
		return dive.NewToolResultError(fmt.Sprintf("search path is not a regular file or directory: %s", searchPath)), nil
	}
	if !rootInfo.IsDir() && !filters.eligible(searchPath, filepath.Base(searchPath)) {
		return t.formatNoMatches(input), nil
	}
	collector := newGrepCollector(input, t.maxResults)
	if t.ripgrepPath != "" {
		err = t.searchRipgrep(ctx, input, searchPath, filters, collector)
	} else {
		err = t.searchPureGo(ctx, input, searchPath, rootInfo, filters, collector)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return dive.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}
	collector.finishFile()
	return t.formatSearchResults(ctx, input, searchPath, collector)
}

func grepRegex(input *GrepInput) (*regexp.Regexp, error) {
	flags := ""
	if input.CaseInsens {
		flags += "i"
	}
	if input.Multiline {
		flags += "s"
	}
	pattern := input.Pattern
	if flags != "" {
		pattern = "(?" + flags + ")" + pattern
	}
	return regexp.Compile(pattern)
}

func grepMatchesType(path, typ string) bool {
	if typ == "" {
		return true
	}
	for _, ext := range grepTypeExtensions[typ] {
		if strings.EqualFold(filepath.Ext(path), ext) {
			return true
		}
	}
	return false
}

func grepMatchesGlobs(patterns []*glob.Pattern, relPath, inputPattern string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if pattern.Match(relPath) || (!strings.Contains(inputPattern, "/") && pattern.Match(filepath.Base(relPath))) {
			return true
		}
	}
	return false
}

func readGrepFile(ctx context.Context, path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if info.Size() > maxGrepFileBytes {
		return nil, errGrepFileTooLarge
	}
	var content bytes.Buffer
	chunk := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := f.Read(chunk)
		if n > 0 {
			if content.Len()+n > maxGrepFileBytes {
				return nil, errGrepFileTooLarge
			}
			_, _ = content.Write(chunk[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return content.Bytes(), ctx.Err()
}

func (t *GrepTool) searchPureGo(ctx context.Context, input *GrepInput, root string, rootInfo os.FileInfo, filters grepFilters, c *grepCollector) error {
	re, err := grepRegex(input)
	if err != nil {
		return fmt.Errorf("Invalid regex pattern: %w", err)
	}
	searchFile := func(path, rel string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !filters.eligible(path, rel) {
			return nil
		}
		content, err := readGrepFile(ctx, path)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if errors.Is(err, errGrepFileTooLarge) {
				return nil
			}
			c.warn(fmt.Sprintf("%s: %v", rel, err))
			return nil
		}
		if bytes.IndexByte(content, 0) >= 0 {
			return nil
		}
		if input.Multiline {
			locations := re.FindAllIndex(content, maxMultilineMatchesPerFile+1)
			if len(locations) > maxMultilineMatchesPerFile {
				return fmt.Errorf("%s: more than %d multiline matches; narrow the search", rel, maxMultilineMatchesPerFile)
			}
			lineNumber, previousStart := 1, 0
			for _, loc := range locations {
				if err := ctx.Err(); err != nil {
					return err
				}
				lineNumber += bytes.Count(content[previousStart:loc[0]], []byte{'\n'})
				previousStart = loc[0]
				lineStart := bytes.LastIndexByte(content[:loc[0]], '\n') + 1
				lineEnd := bytes.IndexByte(content[loc[1]:], '\n')
				end := len(content)
				if lineEnd >= 0 {
					end = loc[1] + lineEnd
				}
				match := grepMatch{file: rel, lineNumber: lineNumber}
				if c.needsLine() {
					match.line = strings.TrimRight(string(content[lineStart:end]), "\r")
				}
				c.add(match)
			}
			return nil
		}
		lineNumber := 1
		for start := 0; start <= len(content); {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := bytes.IndexByte(content[start:], '\n')
			if end < 0 {
				end = len(content)
			} else {
				end += start
			}
			line := content[start:end]
			if re.Match(line) {
				match := grepMatch{file: rel, lineNumber: lineNumber}
				if c.needsLine() {
					match.line = strings.TrimRight(string(line), "\r")
				}
				c.add(match)
			}
			if end == len(content) {
				break
			}
			start = end + 1
			lineNumber++
		}
		return nil
	}
	if !rootInfo.IsDir() {
		return searchFile(root, filepath.Base(root))
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			c.warn(fmt.Sprintf("%s: %v", path, walkErr))
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			c.warn(fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			for _, exclude := range filters.exclude {
				if matchesExclude(exclude, rel) || matchesExclude(exclude, rel+"/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return searchFile(path, rel)
	})
}

type streamedGrepRecord struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text  string `json:"text"`
			Bytes string `json:"bytes"`
		} `json:"path"`
		Lines struct {
			Text  string `json:"text"`
			Bytes string `json:"bytes"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func (t *GrepTool) searchRipgrep(ctx context.Context, input *GrepInput, root string, filters grepFilters, c *grepCollector) error {
	args := []string{"--json", "--sort", "path", "--hidden", "--no-ignore", "--max-filesize", "64M"}
	if input.CaseInsens {
		args = append(args, "--ignore-case")
	}
	if input.Multiline {
		args = append(args, "--multiline", "--multiline-dotall")
	}
	if input.Glob != "" {
		args = append(args, "--glob", input.Glob)
	}
	for _, exclude := range t.defaultExcludes {
		args = append(args, "--glob", "!"+exclude)
	}
	args = append(args, "--regexp", input.Pattern, root)
	cmd := exec.CommandContext(ctx, t.ripgrepPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr boundedGrepOutput
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), maxGrepJSONRecordBytes)
	var parseErr error
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			parseErr = err
			break
		}
		var record streamedGrepRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			parseErr = fmt.Errorf("invalid ripgrep JSON record: %w", err)
			break
		}
		if record.Type != "match" {
			continue
		}
		path := record.Data.Path.Text
		if path == "" && record.Data.Path.Bytes != "" {
			decoded, err := base64.StdEncoding.DecodeString(record.Data.Path.Bytes)
			if err != nil {
				parseErr = fmt.Errorf("invalid ripgrep path encoding: %w", err)
				break
			}
			path = string(decoded)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			parseErr = fmt.Errorf("cannot relate ripgrep path %q to root: %w", path, err)
			break
		}
		if rel == "." {
			rel = filepath.Base(root)
		}
		if !filters.eligible(path, filepath.ToSlash(rel)) {
			continue
		}
		line := record.Data.Lines.Text
		if line == "" && record.Data.Lines.Bytes != "" {
			decoded, err := base64.StdEncoding.DecodeString(record.Data.Lines.Bytes)
			if err != nil {
				parseErr = fmt.Errorf("invalid ripgrep line encoding: %w", err)
				break
			}
			line = string(decoded)
		}
		c.add(grepMatch{file: filepath.ToSlash(rel), lineNumber: record.Data.LineNumber, line: strings.TrimRight(line, "\r\n")})
	}
	if err := scanner.Err(); err != nil && parseErr == nil {
		parseErr = fmt.Errorf("ripgrep output exceeded the supported record size or could not be read: %w", err)
	}
	if parseErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if parseErr != nil {
		return parseErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
		if c.totalMatches > 0 {
			c.warn(fmt.Sprintf("ripgrep: %s", strings.TrimSpace(stderr.String())))
			return nil
		}
		return fmt.Errorf("ripgrep: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (t *GrepTool) formatSearchResults(ctx context.Context, input *GrepInput, root string, c *grepCollector) (*dive.ToolResult, error) {
	if c.totalMatches == 0 {
		if c.warningCount > 0 {
			return dive.NewToolResultError(fmt.Sprintf("Search incomplete: %d paths skipped (%s). No matches in searched files.", c.warningCount, strings.Join(c.warnings, "; "))), nil
		}
		return t.formatNoMatches(input), nil
	}
	output := &boundedGrepOutput{}
	var selected, total int
	if c.mode == GrepOutputContent {
		selected, total = len(c.matches), c.totalMatches
		if err := t.writeGrepContent(ctx, output, root, input, c); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return dive.NewToolResultError(fmt.Sprintf("cannot format grep content: %v", err)), nil
		}
	} else {
		selected, total = len(c.files), c.totalFiles
		for _, file := range c.files {
			if c.mode == GrepOutputCount {
				fmt.Fprintf(output, "%s:%d\n", file.path, file.count)
			} else {
				fmt.Fprintln(output, file.path)
			}
		}
	}
	if selected == 0 {
		result := dive.NewToolResultText(fmt.Sprintf("No results at offset %d (%d total entries)", c.offset, total)).WithDisplay(fmt.Sprintf("No results for %q at offset %d", input.Pattern, c.offset))
		if c.warningCount > 0 {
			result.Content = append(result.Content, &dive.ToolResultContent{Type: dive.ToolResultContentTypeText, Text: fmt.Sprintf("Search incomplete: skipped %d paths (%s).", c.warningCount, strings.Join(c.warnings, "; "))})
		}
		return result, nil
	}
	display := fmt.Sprintf("Found %d match(es) for %q", c.totalMatches, input.Pattern)
	if c.offset > 0 || input.HeadLimit > 0 || total > selected {
		display += fmt.Sprintf(" (showing %d", selected)
		if total > c.offset+selected {
			display += fmt.Sprintf(", limited to %d", c.limit)
		}
		display += ")"
	}
	result := dive.NewToolResultText(strings.TrimSpace(output.String())).WithDisplay(display)
	var notes []string
	if total > c.offset+selected || c.offset > 0 {
		notes = append(notes, fmt.Sprintf("Showing entries %d-%d of %d; use offset %d to continue.", c.offset+1, c.offset+selected, total, c.offset+selected))
	}
	if c.warningCount > 0 {
		notes = append(notes, fmt.Sprintf("Search incomplete: skipped %d paths (%s).", c.warningCount, strings.Join(c.warnings, "; ")))
	}
	if output.truncated {
		notes = append(notes, "Output text was capped at 1 MiB; narrow the search or lower head_limit before paging.")
	}
	if len(notes) > 0 {
		result.Content = append(result.Content, &dive.ToolResultContent{Type: dive.ToolResultContentTypeText, Text: strings.Join(notes, " ")})
	}
	return result, nil
}

func (t *GrepTool) writeGrepContent(ctx context.Context, output io.Writer, root string, input *GrepInput, c *grepCollector) error {
	byFile := make(map[string][]grepMatch)
	var files []string
	for _, m := range c.matches {
		if _, ok := byFile[m.file]; !ok {
			files = append(files, m.file)
		}
		byFile[m.file] = append(byFile[m.file], m)
	}
	sort.Strings(files)
	before, after := input.Before, input.After
	if input.Context > before {
		before = input.Context
	}
	if input.Context > after {
		after = input.Context
	}
	showLines := input.ShowLines == nil || *input.ShowLines
	for _, file := range files {
		if bounded, ok := output.(*boundedGrepOutput); ok && bounded.truncated {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintf(output, "## %s\n", file)
		if before == 0 && after == 0 {
			for _, m := range byFile[file] {
				if showLines {
					fmt.Fprintf(output, "%d: %s\n", m.lineNumber, m.line)
				} else {
					fmt.Fprintln(output, m.line)
				}
			}
			fmt.Fprintln(output)
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(file))
		if info, err := os.Stat(root); err == nil && !info.IsDir() {
			path = root
		}
		content, err := readGrepFile(ctx, path)
		if err != nil {
			c.warn(fmt.Sprintf("%s: context unavailable: %v", file, err))
			for _, m := range byFile[file] {
				fmt.Fprintf(output, "%d: %s\n", m.lineNumber, m.line)
			}
			fmt.Fprintln(output)
			continue
		}
		lineCount := bytes.Count(content, []byte{'\n'}) + 1
		included := make(map[int]bool)
		for _, m := range byFile[file] {
			start := max(1, m.lineNumber-before)
			end := min(lineCount, m.endLine+after)
			for line := start; line <= end; line++ {
				if _, exists := included[line]; !exists {
					included[line] = false
				}
			}
			for line := m.lineNumber; line <= m.endLine; line++ {
				included[line] = true
			}
		}
		lineNumber := 1
		for len(content) > 0 {
			if bounded, ok := output.(*boundedGrepOutput); ok && bounded.truncated {
				break
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			end := bytes.IndexByte(content, '\n')
			if end < 0 {
				end = len(content)
			}
			matched, keep := included[lineNumber]
			if keep {
				raw := bytes.TrimRight(content[:end], "\r")
				text := strings.ToValidUTF8(string(validUTF8Prefix(raw, maxGrepLineBytes)), "�")
				if len(raw) > maxGrepLineBytes {
					text += "... [line truncated]"
				}
				if showLines {
					separator := "-"
					if matched {
						separator = ":"
					}
					fmt.Fprintf(output, "%d%s %s\n", lineNumber, separator, text)
				} else {
					fmt.Fprintln(output, text)
				}
			}
			if end == len(content) {
				break
			}
			content = content[end+1:]
			lineNumber++
		}
		fmt.Fprintln(output)
	}
	return nil
}

func (t *GrepTool) formatNoMatches(input *GrepInput) *dive.ToolResult {
	display := fmt.Sprintf("No matches found for %q", input.Pattern)
	return dive.NewToolResultText("No matches found in eligible text files (up to 64 MiB each)").WithDisplay(display)
}
