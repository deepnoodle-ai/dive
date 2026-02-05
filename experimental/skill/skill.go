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

	// FilePath is the source file path for debugging and reference.
	FilePath string
}

// SkillConfig represents the YAML frontmatter structure in a SKILL.md file.
type SkillConfig struct {
	// Name is the skill identifier (lowercase letters, numbers, hyphens).
	Name string `yaml:"name"`

	// Description explains what the skill does and when to use it.
	Description string `yaml:"description"`

}

