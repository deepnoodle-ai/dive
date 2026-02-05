package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterDelimiter is the YAML frontmatter delimiter used in skill files.
// The frontmatter section is enclosed between two "---" lines at the start of the file.
const frontmatterDelimiter = "---"

// ParseSkillFile parses a SKILL.md file and returns a Skill.
// The file must contain YAML frontmatter delimited by "---" markers.
//
// Example file format:
//
//	---
//	name: code-reviewer
//	description: Review code for best practices.
//	---
//
//	# Code Reviewer
//	## Instructions
//	...
func ParseSkillFile(filePath string) (*Skill, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading skill file: %w", err)
	}
	return ParseSkillContent(content, filePath)
}

// ParseSkillContent parses skill content from bytes and returns a Skill.
//
// The content must begin with YAML frontmatter delimited by "---" markers.
// Any leading whitespace before the opening delimiter is ignored.
//
// The filePath parameter serves two purposes:
//   - It is stored in the returned Skill for debugging and error messages
//   - If the frontmatter does not specify a name, the skill name is derived
//     from the file path using [deriveSkillName]
//
// The function returns an error if:
//   - The content does not start with "---" (after trimming whitespace)
//   - The closing "---" delimiter is missing
//   - The YAML frontmatter is malformed
//   - No skill name can be determined (neither in frontmatter nor derivable from path)
//
// Example:
//
//	content := []byte(`---
//	name: my-skill
//	description: A helpful skill
//	---
//
//	# Instructions
//	Do something helpful.`)
//
//	skill, err := ParseSkillContent(content, "/path/to/my-skill/SKILL.md")
func ParseSkillContent(content []byte, filePath string) (*Skill, error) {
	// Trim leading whitespace/newlines
	content = bytes.TrimLeft(content, " \t\r\n")

	// Check for opening delimiter
	if !bytes.HasPrefix(content, []byte(frontmatterDelimiter)) {
		return nil, fmt.Errorf("skill file must start with YAML frontmatter (---)")
	}

	// Remove opening delimiter
	content = content[len(frontmatterDelimiter):]

	// Find closing delimiter
	idx := bytes.Index(content, []byte("\n"+frontmatterDelimiter))
	if idx == -1 {
		return nil, fmt.Errorf("missing closing frontmatter delimiter (---)")
	}

	// Extract frontmatter and body
	frontmatter := content[:idx]
	body := content[idx+len("\n"+frontmatterDelimiter):]

	// Skip any trailing newlines after closing delimiter
	body = bytes.TrimLeft(body, "\r\n")

	// Parse YAML frontmatter
	var config SkillConfig
	if err := yaml.Unmarshal(frontmatter, &config); err != nil {
		return nil, fmt.Errorf("parsing skill frontmatter: %w", err)
	}

	// Derive name from directory or filename if not specified
	if config.Name == "" {
		config.Name = deriveSkillName(filePath)
	}

	// Validate required fields
	if config.Name == "" {
		return nil, fmt.Errorf("skill name is required")
	}

	return &Skill{
		Name:         config.Name,
		Description:  config.Description,
		Instructions: strings.TrimSpace(string(body)),
		FilePath:     filePath,
	}, nil
}

// deriveSkillName extracts a skill name from the file path.
//
// The derivation rules are:
//   - For files named "SKILL.md" (case-insensitive): use the parent directory name
//   - For other .md files: use the filename without the .md extension
//
// This allows two skill organization patterns:
//
// Directory-based (recommended for skills with supporting files):
//
//	.dive/skills/
//	└── code-reviewer/
//	    ├── SKILL.md      -> skill name: "code-reviewer"
//	    └── templates/
//
// File-based (for simple, standalone skills):
//
//	.dive/skills/
//	└── helper.md         -> skill name: "helper"
//
// Examples:
//
//	deriveSkillName("/path/to/code-reviewer/SKILL.md") // "code-reviewer"
//	deriveSkillName("/path/to/skills/helper.md")       // "helper"
//	deriveSkillName("SKILL.md")                        // "." (edge case)
func deriveSkillName(filePath string) string {
	base := filepath.Base(filePath)

	// If it's a SKILL.md file, use the parent directory name
	if strings.EqualFold(base, "SKILL.md") {
		dir := filepath.Dir(filePath)
		return filepath.Base(dir)
	}

	// Otherwise, use the filename without .md extension
	return strings.TrimSuffix(base, ".md")
}
