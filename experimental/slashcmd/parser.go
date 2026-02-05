package slashcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterDelimiter is the YAML frontmatter delimiter used in command files.
const frontmatterDelimiter = "---"

// ParseCommandFile parses a command markdown file and returns a Command.
//
// The file may optionally contain YAML frontmatter delimited by "---" markers.
// If no frontmatter is present, the entire file content is treated as instructions.
//
// Example file format:
//
//	---
//	description: Review code changes
//	allowed-tools:
//	  - Read
//	  - Grep
//	argument-hint: [file-pattern]
//	---
//
//	# Code Review
//	Review the specified files for issues...
func ParseCommandFile(filePath string) (*Command, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading command file: %w", err)
	}
	return ParseCommandContent(content, filePath)
}

// ParseCommandContent parses command content from bytes and returns a Command.
//
// The content may optionally begin with YAML frontmatter delimited by "---" markers.
// If no frontmatter is present, the entire content is treated as instructions.
//
// The filePath parameter serves two purposes:
//   - It is stored in the returned Command for debugging and error messages
//   - The command name is derived from the file path using [deriveCommandName]
func ParseCommandContent(content []byte, filePath string) (*Command, error) {
	// Trim leading whitespace/newlines
	content = bytes.TrimLeft(content, " \t\r\n")

	var config CommandConfig
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
			return nil, fmt.Errorf("parsing command frontmatter: %w", err)
		}
	} else {
		// No frontmatter, entire content is instructions
		body = content
	}

	// Derive name from filename if not specified in frontmatter
	name := config.Name
	if name == "" {
		name = deriveCommandName(filePath)
	}

	if name == "" {
		return nil, fmt.Errorf("command name is required")
	}

	return &Command{
		Name:         name,
		Description:  config.Description,
		Instructions: strings.TrimSpace(string(body)),
		AllowedTools: config.AllowedTools,
		Model:        config.Model,
		ArgumentHint: config.ArgumentHint,
		FilePath:     filePath,
	}, nil
}

// deriveCommandName extracts a command name from the file path.
//
// The derivation rules are:
//   - For files named "COMMAND.md" (case-insensitive): use the parent directory name
//   - For other .md files: use the filename without the .md extension
//
// This allows two command organization patterns:
//
// Directory-based:
//
//	.dive/commands/
//	└── review/
//	    └── COMMAND.md    -> command name: "review"
//
// File-based (recommended for simple commands):
//
//	.dive/commands/
//	└── review.md         -> command name: "review"
func deriveCommandName(filePath string) string {
	base := filepath.Base(filePath)

	// If it's a COMMAND.md file, use the parent directory name
	if strings.EqualFold(base, "COMMAND.md") {
		dir := filepath.Dir(filePath)
		return filepath.Base(dir)
	}

	// Otherwise, use the filename without .md extension
	return strings.TrimSuffix(base, ".md")
}
