package extended

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/skill"
	"github.com/deepnoodle-ai/wonton/schema"
)

// Compile-time verification that SkillTool implements TypedTool.
var _ dive.TypedTool[*SkillToolInput] = &SkillTool{}

// SkillToolInput is the input schema for the Skill tool.
//
// When an agent invokes the Skill tool, it provides the skill name and
// optional arguments. The arguments are free-form text that may modify
// how the skill's instructions should be interpreted.
type SkillToolInput struct {
	// Skill is the name of the skill to activate. This must match a skill
	// name exactly as returned by the skill loader. Required.
	Skill string `json:"skill"`

	// Args is optional free-form arguments to pass to the skill.
	// These are included in the skill activation response and can provide
	// additional context for how the skill should be applied.
	// Example: "focus on security issues" or "use TypeScript examples"
	Args string `json:"args,omitempty"`
}

// SkillToolOptions configures a new SkillTool instance.
type SkillToolOptions struct {
	// Loader is the skill loader that provides available skills.
	// The loader must have LoadSkills() called before the tool is used.
	// Required.
	Loader *skill.Loader
}

// SkillTool is an agent tool that activates skills on demand.
//
// When invoked, the tool returns the skill's instructions as markdown text,
// which the agent uses to guide its subsequent actions. The tool also tracks
// the currently active skill for tool restriction enforcement.
//
// # Tool Restrictions
//
// Skills can optionally define an allowed-tools list. When such a skill is
// active, the SkillTool's IsToolAllowed method can be used to check whether
// a given tool is permitted. The Skill tool itself is always allowed, enabling
// agents to switch skills even when restrictions are in place.
//
// # Thread Safety
//
// The SkillTool is safe for concurrent use. The active skill state is protected
// by a read-write mutex.
//
// # Usage Example
//
//	loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
//	loader.LoadSkills()
//
//	skillTool := NewSkillTool(SkillToolOptions{Loader: loader})
//
//	// The tool can be wrapped with ToolAdapter for use with agents
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Tools: []dive.Tool{dive.ToolAdapter(skillTool)},
//	})
type SkillTool struct {
	loader *skill.Loader

	mu          sync.RWMutex
	activeSkill *skill.Skill
}

// NewSkillTool creates a new SkillTool with the given options.
//
// The options must include a Loader that has already loaded skills via
// LoadSkills(). The tool will list available skills in its description,
// which is presented to the LLM to help it decide when to invoke skills.
func NewSkillTool(opts SkillToolOptions) *SkillTool {
	return &SkillTool{
		loader: opts.Loader,
	}
}

// Name returns "Skill", the tool's identifier.
func (t *SkillTool) Name() string {
	return "Skill"
}

// Description returns a description of the tool including a list of available skills.
//
// The description is dynamically generated based on the skills loaded by the
// loader. This helps the LLM understand which skills are available and when
// to use them.
func (t *SkillTool) Description() string {
	var sb strings.Builder
	sb.WriteString("Execute a skill to receive specialized instructions for a task.\n\n")
	sb.WriteString("Skills provide focused expertise and instructions for specific types of work. ")
	sb.WriteString("Use this tool when you encounter a task that matches a skill's description.\n\n")

	skills := t.loader.ListSkills()
	if len(skills) > 0 {
		sb.WriteString("Available skills:\n")
		for _, s := range skills {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", s.Name, s.Description))
		}
	} else {
		sb.WriteString("No skills are currently available.\n")
	}

	return sb.String()
}

// Schema returns the JSON schema for the tool's input parameters.
func (t *SkillTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Required: []string{
			"skill",
		},
		Properties: map[string]*schema.Property{
			"skill": {
				Type:        "string",
				Description: "The name of the skill to activate.",
			},
			"args": {
				Type:        "string",
				Description: "Optional arguments to pass to the skill.",
			},
		},
	}
}

// Annotations returns metadata hints about the tool's behavior.
//
// The Skill tool is marked as:
//   - ReadOnlyHint: true (does not modify external state)
//   - DestructiveHint: false (safe to call)
//   - IdempotentHint: true (calling with same args produces same result)
//   - OpenWorldHint: false (operates only on local skill definitions)
func (t *SkillTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Skill",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

// Call activates a skill and returns its instructions.
//
// The method performs the following:
//  1. Validates that a skill name was provided
//  2. Looks up the skill in the loader
//  3. Sets the skill as the active skill (for tool restriction checks)
//  4. Returns a formatted markdown response containing:
//     - The skill name and description
//     - Any arguments provided
//     - The skill's instructions
//     - Tool restrictions (if the skill has allowed-tools defined)
//
// If the skill is not found, an error result is returned listing available skills.
//
// The returned ToolResult includes a Display field with a brief summary
// suitable for UI display (e.g., "Activated skill: code-reviewer").
func (t *SkillTool) Call(ctx context.Context, input *SkillToolInput) (*dive.ToolResult, error) {
	if input.Skill == "" {
		return dive.NewToolResultError("skill name is required"), nil
	}

	s, ok := t.loader.GetSkill(input.Skill)
	if !ok {
		available := t.loader.ListSkillNames()
		if len(available) == 0 {
			return dive.NewToolResultError(fmt.Sprintf(
				"skill %q not found. No skills are currently available.",
				input.Skill,
			)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf(
			"skill %q not found. Available skills: %s",
			input.Skill, strings.Join(available, ", "),
		)), nil
	}

	// Set as active skill
	t.mu.Lock()
	t.activeSkill = s
	t.mu.Unlock()

	// Build skill instructions to return
	var result strings.Builder
	result.WriteString(fmt.Sprintf("# Skill Activated: %s\n\n", s.Name))

	if s.Description != "" {
		result.WriteString(fmt.Sprintf("**Description:** %s\n\n", s.Description))
	}

	if input.Args != "" {
		result.WriteString(fmt.Sprintf("**Arguments:** %s\n\n", input.Args))
	}

	result.WriteString("## Instructions\n\n")
	result.WriteString(s.Instructions)

	if len(s.AllowedTools) > 0 {
		result.WriteString(fmt.Sprintf("\n\n---\n**Tool Restrictions:** While this skill is active, you may only use these tools: %s\n",
			strings.Join(s.AllowedTools, ", ")))
	}

	return dive.NewToolResultText(result.String()).
		WithDisplay(fmt.Sprintf("Activated skill: %s", s.Name)), nil
}

// GetActiveSkill returns the currently active skill, or nil if no skill is active.
//
// The active skill is set when Call is invoked successfully. It remains active
// until another skill is activated or ClearActiveSkill is called.
//
// This method is thread-safe.
func (t *SkillTool) GetActiveSkill() *skill.Skill {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.activeSkill
}

// ClearActiveSkill deactivates the currently active skill.
//
// After calling this method, GetActiveSkill returns nil and IsToolAllowed
// returns true for all tools. This is useful when a task is complete and
// tool restrictions should be lifted.
//
// This method is thread-safe.
func (t *SkillTool) ClearActiveSkill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeSkill = nil
}

// IsToolAllowed checks if a tool is permitted by the currently active skill.
//
// This method implements the dive.ToolAllowanceChecker interface, enabling
// integration with agents that enforce tool restrictions.
//
// Returns true in the following cases:
//   - No skill is currently active
//   - The active skill has no allowed-tools restrictions
//   - The tool name matches an allowed tool (case-insensitive)
//   - The tool name is "Skill" (always allowed to enable skill switching)
//
// This method is thread-safe.
func (t *SkillTool) IsToolAllowed(toolName string) bool {
	// Always allow the Skill tool itself so agents can switch skills
	if toolName == t.Name() {
		return true
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.activeSkill == nil {
		return true
	}
	return t.activeSkill.IsToolAllowed(toolName)
}
