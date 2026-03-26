package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterDelimiter is the YAML frontmatter delimiter.
const frontmatterDelimiter = "---"

// ParseFile reads and parses a skill/command file at the given path.
func ParseFile(filePath string) (*Skill, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading skill file: %w", err)
	}
	return ParseContent(content, filePath)
}

// ParseContent parses skill/command content from bytes.
//
// If frontmatter is present (content starts with "---"), it is parsed into
// SkillConfig. If absent, the entire content becomes Instructions and the
// skill is treated as a slash command.
//
// The filePath parameter is stored in the returned Skill and used for
// name derivation when frontmatter does not specify a name.
func ParseContent(content []byte, filePath string) (*Skill, error) {
	// Trim leading whitespace/newlines
	content = bytes.TrimLeft(content, " \t\r\n")

	var config SkillConfig
	var body []byte

	// Check for optional frontmatter
	if bytes.HasPrefix(content, []byte(frontmatterDelimiter)) {
		// Remove opening delimiter
		remaining := content[len(frontmatterDelimiter):]

		// Find closing delimiter
		idx := bytes.Index(remaining, []byte("\n"+frontmatterDelimiter))
		if idx == -1 {
			return nil, fmt.Errorf("missing closing frontmatter delimiter (---)")
		}

		// Extract frontmatter and body
		frontmatter := remaining[:idx]
		body = remaining[idx+len("\n"+frontmatterDelimiter):]

		// Skip any trailing newlines after closing delimiter
		body = bytes.TrimLeft(body, "\r\n")

		// Parse YAML frontmatter
		if err := yaml.Unmarshal(frontmatter, &config); err != nil {
			return nil, fmt.Errorf("parsing skill frontmatter: %w", err)
		}
	} else {
		// No frontmatter, entire content is instructions
		body = content
	}

	// Derive name from directory or filename if not specified
	name := config.Name
	if name == "" {
		name = deriveName(filePath)
	}

	if name == "" {
		return nil, fmt.Errorf("skill name is required")
	}

	s := &Skill{
		Name:         name,
		Description:  config.Description,
		Instructions: strings.TrimSpace(string(body)),
		FilePath:     filePath,
		SourceURI:    "file://" + filePath,
		Config:       config,
	}
	// Sync top-level fields with config
	s.Config.Name = name
	s.Config.Description = config.Description

	return s, nil
}

// deriveName extracts a skill name from the file path.
//
// For files named SKILL.md or COMMAND.md (case-insensitive): use the parent
// directory name. For other .md files: use the filename without extension.
func deriveName(filePath string) string {
	base := filepath.Base(filePath)
	lower := strings.ToLower(base)

	if lower == "skill.md" || lower == "command.md" {
		dir := filepath.Dir(filePath)
		return filepath.Base(dir)
	}

	return strings.TrimSuffix(base, ".md")
}
