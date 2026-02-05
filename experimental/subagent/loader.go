package subagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader is the interface for loading subagent definitions.
type Loader interface {
	// Load loads subagent definitions from the source.
	Load(ctx context.Context) (map[string]*Definition, error)
}

// FileLoader loads subagent definitions from the filesystem.
// It reads markdown files with YAML frontmatter from configured directories.
type FileLoader struct {
	// Directories to search for subagent definitions (in priority order).
	Directories []string

	// IncludeClaudeAgents enables loading from .claude/agents/ directory.
	IncludeClaudeAgents bool
}

var _ Loader = (*FileLoader)(nil)

// NewFileLoader creates a new FileLoader with default settings.
func NewFileLoader() *FileLoader {
	return &FileLoader{}
}

// Load implements Loader.
func (l *FileLoader) Load(ctx context.Context) (map[string]*Definition, error) {
	result := make(map[string]*Definition)

	dirs := l.Directories
	if len(dirs) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dirs = []string{filepath.Join(cwd, ".dive", "agents")}
	}

	if l.IncludeClaudeAgents {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dirs = append(dirs, filepath.Join(cwd, ".claude", "agents"))
	}

	for _, dir := range dirs {
		loaded, err := LoadFromDirectory(dir)
		if err != nil {
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

// CompositeLoader combines multiple Loaders.
type CompositeLoader struct {
	Loaders []Loader
}

var _ Loader = (*CompositeLoader)(nil)

// Load implements Loader by loading from all loaders in order.
func (c *CompositeLoader) Load(ctx context.Context) (map[string]*Definition, error) {
	result := make(map[string]*Definition)
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

// MapLoader wraps a static map as a Loader.
type MapLoader struct {
	Definitions map[string]*Definition
}

var _ Loader = (*MapLoader)(nil)

// Load implements Loader by returning the wrapped map.
func (m *MapLoader) Load(ctx context.Context) (map[string]*Definition, error) {
	return m.Definitions, nil
}

// frontmatter represents the YAML frontmatter in a subagent markdown file.
type frontmatter struct {
	Description string   `yaml:"description"`
	Model       string   `yaml:"model,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
}

// LoadFromDirectory loads subagent definitions from a single directory.
func LoadFromDirectory(dir string) (map[string]*Definition, error) {
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

	result := make(map[string]*Definition)

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

		def, err := loadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", filePath, err)
		}

		result[agentName] = def
	}

	return result, nil
}

func loadFile(path string) (*Definition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fm, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, err
	}

	var frontm frontmatter
	if err := yaml.Unmarshal([]byte(fm), &frontm); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	if frontm.Description == "" {
		return nil, fmt.Errorf("description is required in frontmatter")
	}

	return &Definition{
		Description: frontm.Description,
		Prompt:      strings.TrimSpace(body),
		Tools:       frontm.Tools,
		Model:       frontm.Model,
	}, nil
}

func parseFrontmatter(content string) (fm, body string, err error) {
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("file must start with --- frontmatter delimiter")
	}

	rest := content[3:]
	rest = strings.TrimPrefix(rest, "\n")

	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return "", "", fmt.Errorf("missing closing --- frontmatter delimiter")
	}

	fm = rest[:endIdx]
	body = strings.TrimPrefix(rest[endIdx+4:], "\n")

	return fm, body, nil
}

// LoadIntoRegistry loads definitions from a Loader and registers them in a Registry.
func LoadIntoRegistry(ctx context.Context, loader Loader, registry *Registry) error {
	defs, err := loader.Load(ctx)
	if err != nil {
		return err
	}
	registry.RegisterAll(defs)
	return nil
}
