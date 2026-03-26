// Package skill provides production-quality support for agent skills and slash commands.
//
// Skills are modular capabilities defined in Markdown files with YAML frontmatter.
// They enable agents to activate specialized behaviors based on task requirements,
// and users to invoke commands via /name syntax.
//
// # Unified Model
//
// Skills and slash commands are unified into one type. A slash command is simply
// a skill without a description (or with empty frontmatter). The agent never sees
// commands in its tool description, but users can invoke both via /name.
//
// # Skill File Format
//
//	---
//	name: code-reviewer
//	description: Review code for best practices and potential issues.
//	allowed-tools:
//	  - Read
//	  - Grep
//	  - Glob
//	---
//
//	# Code Reviewer
//
//	## Instructions
//	1. Read the target files
//	2. Analyze for issues
//
// # Usage
//
//	loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
//	if err := loader.Load(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	skillTool := skill.NewTool(loader)
package skill

import "strings"

// Skill represents a loaded skill or slash command with metadata and instructions.
type Skill struct {
	// Name is the unique identifier for the skill, derived from YAML
	// frontmatter, the directory name, or the filename.
	Name string

	// Description is a brief description of what the skill does.
	// Used by the LLM to decide when to invoke the skill.
	// Empty for slash commands (user-invocable only).
	Description string

	// Instructions is the Markdown content after the YAML frontmatter.
	// May contain variable placeholders ($ARGUMENTS, $1, !{command}).
	Instructions string

	// FilePath is the source file path for debugging and reference.
	// Empty for skills loaded from non-file providers.
	FilePath string

	// SourceURI is the canonical URI (file://, https://, etc.).
	SourceURI string

	// Source indicates where the skill was loaded from ("project" or "user").
	Source string

	// Config holds the full parsed frontmatter.
	Config SkillConfig
}

// IsLocal returns true if the skill was loaded from the local filesystem.
// Only local skills are eligible for !{command} shell expansion.
func (s *Skill) IsLocal() bool {
	return s.SourceURI == "" || strings.HasPrefix(s.SourceURI, "file://")
}

// IsCommand returns true if this skill is a slash command (user-invocable only).
// A skill is a command if it has no description and UserInvocable is not
// explicitly set to false.
func (s *Skill) IsCommand() bool {
	if s.Config.UserInvocable != nil {
		return *s.Config.UserInvocable
	}
	return s.Description == ""
}

// SkillConfig represents the full YAML frontmatter in a SKILL.md or command file.
type SkillConfig struct {
	// Name is the skill identifier.
	Name string `yaml:"name,omitempty"`

	// Description explains what the skill does and when to use it.
	Description string `yaml:"description,omitempty"`

	// AllowedTools restricts which tools are available while this skill is active.
	// An empty list means all tools are available.
	AllowedTools []string `yaml:"allowed-tools,omitempty"`

	// Model is an optional model override for this skill.
	Model string `yaml:"model,omitempty"`

	// ArgumentHint describes expected arguments (e.g., "[file-pattern]").
	ArgumentHint string `yaml:"argument-hint,omitempty"`

	// Triggers define patterns that cause automatic skill suggestion.
	Triggers []Trigger `yaml:"triggers,omitempty"`

	// UserInvocable controls whether this skill is treated as a slash command.
	// nil = inferred from Description (empty description = command).
	// true = always a command (user-invocable only).
	// false = always a skill (agent-invocable, even without description).
	UserInvocable *bool `yaml:"user-invocable,omitempty"`
}

// Trigger defines a pattern that causes automatic skill suggestion.
type Trigger struct {
	// Keyword is a case-insensitive substring to match against user input.
	Keyword string `yaml:"keyword,omitempty"`

	// Pattern is a regular expression to match against user input.
	Pattern string `yaml:"pattern,omitempty"`
}

// Logger receives debug and warning messages during skill loading.
type Logger interface {
	// Debug logs informational messages about skill loading progress.
	Debug(msg string, args ...any)

	// Warn logs warning messages about non-fatal issues.
	Warn(msg string, args ...any)
}
