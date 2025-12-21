package dive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SubagentLoader is the interface for loading subagent definitions.
// Implement this interface to provide custom subagent loading from
// different sources (filesystem, database, remote API, etc.).
type SubagentLoader interface {
	// Load loads subagent definitions from the source.
	// Returns a map of subagent name to definition.
	Load(ctx context.Context) (map[string]*SubagentDefinition, error)
}

// FileSubagentLoader loads subagent definitions from the filesystem.
// It reads markdown files with YAML frontmatter from configured directories.
type FileSubagentLoader struct {
	// Directories to search for subagent definitions (in priority order).
	// Later directories override earlier ones for definitions with the same name.
	// If empty, defaults to ".dive/agents" in the current working directory.
	Directories []string

	// IncludeClaudeAgents enables loading from .claude/agents/ directory.
	// This provides compatibility with Claude Code subagent definitions.
	// The .claude/agents/ directory is searched after .dive/agents/.
	IncludeClaudeAgents bool
}

// Verify FileSubagentLoader implements SubagentLoader
var _ SubagentLoader = (*FileSubagentLoader)(nil)

// NewFileSubagentLoader creates a new FileSubagentLoader with default settings.
// By default, it loads from .dive/agents/ in the current working directory.
func NewFileSubagentLoader() *FileSubagentLoader {
	return &FileSubagentLoader{}
}

// Load implements SubagentLoader.
func (l *FileSubagentLoader) Load(ctx context.Context) (map[string]*SubagentDefinition, error) {
	result := make(map[string]*SubagentDefinition)

	// Build list of directories to search
	dirs := l.Directories
	if len(dirs) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dirs = []string{filepath.Join(cwd, ".dive", "agents")}
	}

	// Optionally add .claude/agents/ directory
	if l.IncludeClaudeAgents {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dirs = append(dirs, filepath.Join(cwd, ".claude", "agents"))
	}

	// Load from each directory (later dirs override earlier ones)
	for _, dir := range dirs {
		loaded, err := LoadSubagentsFromDirectory(dir)
		if err != nil {
			// Skip directories that don't exist
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to load from %s: %w", dir, err)
		}
		for name, def := range loaded {
			result[name] = def
		}
	}

	return result, nil
}

// CompositeSubagentLoader combines multiple SubagentLoaders.
// Later loaders override earlier ones for definitions with the same name.
type CompositeSubagentLoader struct {
	Loaders []SubagentLoader
}

// Verify CompositeSubagentLoader implements SubagentLoader
var _ SubagentLoader = (*CompositeSubagentLoader)(nil)

// Load implements SubagentLoader by loading from all loaders in order.
func (c *CompositeSubagentLoader) Load(ctx context.Context) (map[string]*SubagentDefinition, error) {
	result := make(map[string]*SubagentDefinition)
	for _, loader := range c.Loaders {
		loaded, err := loader.Load(ctx)
		if err != nil {
			return nil, err
		}
		for name, def := range loaded {
			result[name] = def
		}
	}
	return result, nil
}

// MapSubagentLoader wraps a static map as a SubagentLoader.
// Useful for testing or providing fixed subagent definitions.
type MapSubagentLoader struct {
	Subagents map[string]*SubagentDefinition
}

// Verify MapSubagentLoader implements SubagentLoader
var _ SubagentLoader = (*MapSubagentLoader)(nil)

// Load implements SubagentLoader by returning the wrapped map.
func (m *MapSubagentLoader) Load(ctx context.Context) (map[string]*SubagentDefinition, error) {
	return m.Subagents, nil
}

// subagentFrontmatter represents the YAML frontmatter in a subagent markdown file.
type subagentFrontmatter struct {
	Description string   `yaml:"description"`
	Model       string   `yaml:"model,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
}

// LoadSubagentsFromDirectory loads subagent definitions from a single directory.
// Only .md files are processed. Returns an error if the directory doesn't exist.
func LoadSubagentsFromDirectory(dir string) (map[string]*SubagentDefinition, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	result := make(map[string]*SubagentDefinition)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		agentName := strings.TrimSuffix(name, ".md")
		filePath := filepath.Join(dir, name)

		def, err := loadSubagentFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", filePath, err)
		}

		result[agentName] = def
	}

	return result, nil
}

// loadSubagentFile parses a single subagent markdown file.
func loadSubagentFile(path string) (*SubagentDefinition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	frontmatter, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, err
	}

	var fm subagentFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	if fm.Description == "" {
		return nil, fmt.Errorf("description is required in frontmatter")
	}

	return &SubagentDefinition{
		Description: fm.Description,
		Prompt:      strings.TrimSpace(body),
		Tools:       fm.Tools,
		Model:       fm.Model,
	}, nil
}

// parseFrontmatter extracts YAML frontmatter and body from a markdown file.
// Frontmatter is delimited by --- at the start and end.
func parseFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("file must start with --- frontmatter delimiter")
	}

	// Find the closing ---
	rest := content[3:]
	rest = strings.TrimPrefix(rest, "\n")

	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return "", "", fmt.Errorf("missing closing --- frontmatter delimiter")
	}

	frontmatter = rest[:endIdx]
	body = strings.TrimPrefix(rest[endIdx+4:], "\n")

	return frontmatter, body, nil
}
