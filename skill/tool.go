package skill

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

// Compile-time verification that toolImpl implements TypedTool.
var _ dive.TypedTool[*ToolInput] = &toolImpl{}

// ToolInput is the input schema for the Skill tool.
type ToolInput struct {
	// Skill is the name of the skill to activate. Required.
	Skill string `json:"skill"`

	// Args is optional free-form arguments to pass to the skill.
	Args string `json:"args,omitempty"`
}

// ToolOption configures a Skill tool.
type ToolOption func(*toolConfig)

type toolConfig struct {
	shellExpansion bool
}

// WithToolShellExpansion enables !{command} substitution when skills are invoked.
func WithToolShellExpansion(allow bool) ToolOption {
	return func(c *toolConfig) {
		c.shellExpansion = allow
	}
}

// toolImpl is the agent tool that activates skills.
type toolImpl struct {
	loader *Loader
	config toolConfig
}

// NewTool creates a Skill tool backed by the given loader.
// Returns *dive.TypedToolAdapter[*ToolInput] so it satisfies dive.Tool.
func NewTool(loader *Loader, opts ...ToolOption) *dive.TypedToolAdapter[*ToolInput] {
	cfg := toolConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return dive.ToolAdapter(&toolImpl{
		loader: loader,
		config: cfg,
	})
}

func (t *toolImpl) Name() string {
	return "Skill"
}

func (t *toolImpl) Description() string {
	return "Execute a skill by name to receive specialized instructions for a task. " +
		"Use this tool when the available skills list includes one matching your current task."
}

func (t *toolImpl) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"skill"},
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

func (t *toolImpl) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Skill",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *toolImpl) Call(ctx context.Context, input *ToolInput) (*dive.ToolResult, error) {
	if input.Skill == "" {
		return dive.NewToolResultError("skill name is required"), nil
	}

	s, ok := t.loader.Get(input.Skill)
	if !ok {
		// Only list agent-invocable skills (not commands) in error
		skillNames := make([]string, 0)
		for _, sk := range t.loader.Skills() {
			skillNames = append(skillNames, sk.Name)
		}
		if len(skillNames) == 0 {
			return dive.NewToolResultError(fmt.Sprintf(
				"skill %q not found. No skills are currently available.",
				input.Skill,
			)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf(
			"skill %q not found. Available skills: %s",
			input.Skill, strings.Join(skillNames, ", "),
		)), nil
	}

	// Guard against re-invoking an already active skill
	if active := t.loader.ActiveSkill(); active != nil && active.Name == s.Name {
		return dive.NewToolResultText(
			fmt.Sprintf("Skill %q is already active.", s.Name),
		).WithDisplay(fmt.Sprintf("Skill already active: %s", s.Name)), nil
	}

	// Set as active skill
	t.loader.SetActiveSkill(s)

	// Always expand variables (handles !{command} even with empty args).
	// On shell expansion error, the partially expanded result is still usable.
	instructions, _ := s.Expand(ctx, input.Args, WithShellExpansion(t.config.shellExpansion))

	// Store expanded instructions for the PostToolUse hook to inject
	// as AdditionalContext (matching Claude Code's pattern where the tool
	// returns a brief acknowledgment and the content appears separately)
	t.loader.mu.Lock()
	t.loader.pendingInstructions = formatSkillContent(s, input.Args, instructions)
	t.loader.mu.Unlock()

	// Tool result is brief — the actual instructions are injected by the
	// PostToolUse hook as AdditionalContext on the tool result message
	return dive.NewToolResultText(
		fmt.Sprintf("Launching skill: %s", s.Name),
	).WithDisplay(fmt.Sprintf("Activated skill: %s", s.Name)), nil
}

// formatSkillContent builds the full skill content block including
// base directory for relative path resolution.
func formatSkillContent(s *Skill, args, instructions string) string {
	var sb strings.Builder
	// Include base directory so the agent can resolve relative paths
	// to reference files within the skill directory
	if s.FilePath != "" {
		fmt.Fprintf(&sb, "Base directory for this skill: %s\n\n", skillBaseDir(s))
	}
	fmt.Fprintf(&sb, "# %s\n\n", s.Name)
	if s.Description != "" {
		fmt.Fprintf(&sb, "%s\n\n", s.Description)
	}
	if args != "" {
		fmt.Fprintf(&sb, "**Arguments:** %s\n\n", args)
	}
	sb.WriteString(instructions)
	return sb.String()
}

// skillBaseDir returns the base directory for a skill's supporting files.
func skillBaseDir(s *Skill) string {
	if s.FilePath == "" {
		return ""
	}
	base := filepath.Base(s.FilePath)
	lower := strings.ToLower(base)
	// For SKILL.md or COMMAND.md, the base dir is the parent
	if lower == "skill.md" || lower == "command.md" {
		return filepath.Dir(s.FilePath)
	}
	// For standalone .md files, the base dir is the containing directory
	return filepath.Dir(s.FilePath)
}
