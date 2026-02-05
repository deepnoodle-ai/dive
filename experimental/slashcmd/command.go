// Package slashcmd provides support for Claude-compatible slash commands.
//
// Slash commands are user-invocable actions defined in markdown files with
// optional YAML frontmatter. They enable users to trigger specialized behaviors
// directly from the CLI using /command-name syntax.
//
// # Command File Format
//
// Commands are defined in markdown files with optional YAML frontmatter:
//
//	---
//	description: Review code for best practices
//	argument-hint: [file-pattern]
//	---
//
//	# Code Review
//
//	Review the files matching the pattern: $ARGUMENTS
//
// # Command Discovery
//
// Commands are discovered from multiple locations in priority order:
//   - ./.dive/commands/ (project-level, Dive)
//   - ./.claude/commands/ (project-level, Claude)
//   - ~/.dive/commands/ (user-level, Dive)
//   - ~/.claude/commands/ (user-level, Claude)
//
// The first command found with a given name takes precedence over later ones.
//
// # Argument Expansion
//
// Commands support argument placeholders:
//   - $1, $2, etc. for positional arguments
//   - $ARGUMENTS for the full argument string
//
// # Usage Example
//
//	loader := slashcmd.NewLoader(slashcmd.LoaderOptions{
//	    ProjectDir: ".",
//	})
//	if err := loader.LoadCommands(); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get a command and expand its arguments
//	if cmd, ok := loader.GetCommand("fix-issue"); ok {
//	    expanded := cmd.ExpandArguments("123 high")
//	    fmt.Println(expanded)
//	}
//
//	// List all available commands
//	for _, cmd := range loader.ListCommands() {
//	    fmt.Printf("/%s - %s\n", cmd.Name, cmd.Description)
//	}
package slashcmd

import (
	"fmt"
	"regexp"
	"strings"
)

// Command represents a loaded slash command with its metadata and content.
type Command struct {
	// Name is the unique identifier for the command, derived from the filename.
	Name string

	// Description is a brief description of what the command does.
	Description string

	// Instructions is the Markdown content after the YAML frontmatter,
	// containing the prompt/instructions for the command.
	Instructions string

	// Model is an optional model override for this command.
	Model string

	// ArgumentHint describes expected arguments (e.g., "[issue-number] [priority]").
	ArgumentHint string

	// FilePath is the source file path for debugging and reference.
	FilePath string

	// Source indicates where the command was loaded from ("project" or "user").
	Source string
}

// CommandConfig represents the YAML frontmatter structure in a command file.
type CommandConfig struct {
	// Name is an optional explicit name for the command.
	// If not specified, the name is derived from the filename.
	Name string `yaml:"name,omitempty"`

	// Description explains what the command does.
	Description string `yaml:"description,omitempty"`

	// Model is an optional model override (e.g., "claude-sonnet-4-5-20250929").
	Model string `yaml:"model,omitempty"`

	// ArgumentHint describes expected arguments for help text.
	ArgumentHint string `yaml:"argument-hint,omitempty"`
}

// positionalArgPattern matches $1, $2, etc.
var positionalArgPattern = regexp.MustCompile(`\$(\d+)`)

// ExpandArguments replaces argument placeholders in the template.
//
// Placeholders:
//   - $1, $2, etc. are replaced with positional arguments
//   - $ARGUMENTS is replaced with the full argument string
//
// Example:
//
//	template := "Fix issue #$1 with priority $2. Full args: $ARGUMENTS"
//	result := cmd.ExpandArguments(template, "123 high")
//	// result: "Fix issue #123 with priority high. Full args: 123 high"
func (c *Command) ExpandArguments(argsString string) string {
	args := strings.Fields(argsString)
	result := c.Instructions

	// Replace positional arguments $1, $2, etc.
	result = positionalArgPattern.ReplaceAllStringFunc(result, func(match string) string {
		var num int
		fmt.Sscanf(match, "$%d", &num)
		if num > 0 && num <= len(args) {
			return args[num-1]
		}
		return match // Leave unreplaced if no matching arg
	})

	// Replace $ARGUMENTS with full argument string
	result = strings.ReplaceAll(result, "$ARGUMENTS", argsString)

	return result
}
