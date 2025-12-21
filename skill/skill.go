// Package skill provides support for Claude-compatible Agent Skills.
//
// Skills are modular capabilities that extend agent functionality through
// SKILL.md files containing YAML frontmatter and Markdown instructions.
// They enable agents to autonomously activate specialized behaviors based
// on task requirements.
//
// # Skill File Format
//
// Skills are defined in SKILL.md files with YAML frontmatter:
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
//	1. Read the target files using the Read tool
//	2. Analyze code for common issues
//	3. Provide actionable feedback
//
// # Skill Discovery
//
// Skills are discovered from multiple locations in priority order:
//   - ./.dive/skills/ (project-level, Dive)
//   - ./.claude/skills/ (project-level, Claude)
//   - ~/.dive/skills/ (user-level, Dive)
//   - ~/.claude/skills/ (user-level, Claude)
//
// The first skill found with a given name takes precedence over later ones.
//
// # Tool Restrictions
//
// Skills can optionally specify an allowed-tools list to restrict which
// tools the agent may use while the skill is active. This provides a
// security and focus mechanism for specialized tasks.
//
// # Usage Example
//
//	loader := skill.NewLoader(skill.LoaderOptions{
//	    ProjectDir: ".",
//	})
//	if err := loader.LoadSkills(); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get a specific skill
//	if s, ok := loader.GetSkill("code-reviewer"); ok {
//	    fmt.Println(s.Instructions)
//	}
//
//	// List all available skills
//	for _, s := range loader.ListSkills() {
//	    fmt.Printf("%s: %s\n", s.Name, s.Description)
//	}
package skill

// Skill represents a loaded skill with its metadata and content.
// Skills are modular capabilities that extend agent functionality through
// SKILL.md files with YAML frontmatter and Markdown instructions.
type Skill struct {
	// Name is the unique identifier for the skill, derived from YAML
	// frontmatter or the directory name.
	Name string

	// Description is a brief description of what the skill does and when
	// to use it, used by the LLM to decide when to invoke the skill.
	Description string

	// Instructions is the Markdown content after the YAML frontmatter,
	// containing the detailed instructions for the skill.
	Instructions string

	// AllowedTools is an optional list of tool names that are permitted
	// when this skill is active. If empty, all tools are allowed.
	AllowedTools []string

	// FilePath is the source file path for debugging and reference.
	FilePath string
}

// SkillConfig represents the YAML frontmatter structure in a SKILL.md file.
type SkillConfig struct {
	// Name is the skill identifier (lowercase letters, numbers, hyphens).
	Name string `yaml:"name"`

	// Description explains what the skill does and when to use it.
	Description string `yaml:"description"`

	// AllowedTools restricts which tools can be used when the skill is active.
	AllowedTools []string `yaml:"allowed-tools,omitempty"`
}

// IsToolAllowed checks if a tool is permitted by this skill's allowed-tools list.
//
// The method returns true in the following cases:
//   - AllowedTools is nil or empty (no restrictions apply)
//   - The tool name matches an entry in AllowedTools (case-insensitive comparison)
//
// Tool name matching is case-insensitive, so "Read", "read", and "READ" are
// all considered equivalent.
//
// Example:
//
//	skill := &Skill{AllowedTools: []string{"Read", "Grep"}}
//	skill.IsToolAllowed("Read")  // true
//	skill.IsToolAllowed("read")  // true (case-insensitive)
//	skill.IsToolAllowed("Write") // false
//
//	skillNoRestrictions := &Skill{}
//	skillNoRestrictions.IsToolAllowed("AnyTool") // true
func (s *Skill) IsToolAllowed(toolName string) bool {
	if len(s.AllowedTools) == 0 {
		return true
	}
	for _, allowed := range s.AllowedTools {
		if equalsIgnoreCase(allowed, toolName) {
			return true
		}
	}
	return false
}

// equalsIgnoreCase performs a case-insensitive string comparison.
func equalsIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
