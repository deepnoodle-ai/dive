package skill

import (
	"context"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// Compile-time check that Loader implements dive.Extension.
var _ dive.Extension = (*Loader)(nil)

// Tools returns the Skill tool if skills are loaded, or nil otherwise.
// Implements dive.Extension.
func (l *Loader) Tools() []dive.Tool {
	if l.Count() == 0 {
		return nil
	}
	var toolOpts []ToolOption
	if l.shellExpansion {
		toolOpts = append(toolOpts, WithToolShellExpansion(true))
	}
	return []dive.Tool{NewTool(l, toolOpts...)}
}

// Hooks returns the catalog injection and skill content hooks.
// Hooks are always returned even when no skills are loaded so a pinned empty
// catalog can mask a stale legacy block from an older session.
// Implements dive.Extension.
func (l *Loader) Hooks() dive.Hooks {
	return dive.Hooks{
		PreGeneration: []dive.PreGenerationHook{catalogHook(l)},
		PostToolUse:   []dive.PostToolUseHook{skillContentHook(l)},
	}
}

// Rules returns the skill usage rules for the system prompt, or empty
// string if no skills are loaded.
// Implements dive.Extension.
func (l *Loader) Rules() string {
	if l.Count() == 0 {
		return ""
	}
	return SkillRules()
}

// ConfigureAgent sets up skill support on the given AgentOptions.
//
// Deprecated: Use AgentOptions.Extensions instead:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:      anthropic.New(),
//	    Extensions: []dive.Extension{loader},
//	})
func ConfigureAgent(opts *dive.AgentOptions, loader *Loader, cfgOpts ...ConfigOption) {
	if loader == nil {
		return
	}

	cfg := &configOptions{}
	for _, opt := range cfgOpts {
		opt(cfg)
	}

	// Always register hooks — even with zero skills, a resumed session
	// may contain a stale catalog block that needs cleanup.
	opts.Hooks.PreGeneration = append(opts.Hooks.PreGeneration, catalogHook(loader))
	opts.Hooks.PostToolUse = append(opts.Hooks.PostToolUse, skillContentHook(loader))

	// Only add the Skill tool and system prompt rules when skills exist.
	if loader.Count() == 0 {
		return
	}

	var toolOpts []ToolOption
	if cfg.shellExpansion {
		toolOpts = append(toolOpts, WithToolShellExpansion(true))
	}
	skillTool := NewTool(loader, toolOpts...)
	opts.Tools = append(opts.Tools, skillTool)

	if opts.SystemPrompt != "" {
		opts.SystemPrompt = strings.TrimRight(opts.SystemPrompt, "\n") + "\n\n" + SkillRules()
	} else {
		opts.SystemPrompt = SkillRules()
	}
}

// ConfigOption configures skill agent integration.
type ConfigOption func(*configOptions)

type configOptions struct {
	shellExpansion bool
}

// WithConfigShellExpansion enables !{command} substitution when skills are invoked.
func WithConfigShellExpansion(allow bool) ConfigOption {
	return func(c *configOptions) {
		c.shellExpansion = allow
	}
}

// skillContentHook returns a PostToolUseHook that injects expanded skill
// instructions as AdditionalContext when the Skill tool is invoked.
// The instructions appear as a separate text block on the tool result message,
// matching Claude Code's pattern.
func skillContentHook(loader *Loader) dive.PostToolUseHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		if hctx.Tool == nil || hctx.Tool.Name() != "Skill" || hctx.Call == nil {
			return nil
		}
		callID := hctx.Call.ID
		loader.mu.Lock()
		content := loader.pendingInstructions[callID]
		delete(loader.pendingInstructions, callID)
		loader.mu.Unlock()

		if content != "" {
			hctx.AdditionalContext += content
		}
		return nil
	}
}

// skillReminderName is the system-reminder block name for the skill catalog.
const skillReminderName = "skills"

// catalogHook returns a PreGenerationHook that injects the skill catalog
// as a named <system-reminder> block in the first user message.
//
// Using the first user message (not the last) ensures the catalog is in a
// stable position for prompt caching — it sits right after the system prompt
// and doesn't move as the conversation grows.
func catalogHook(loader *Loader) dive.PreGenerationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		catalog := BuildCatalog(loader)
		// A fresh empty catalog has nothing to inject. Keep pinning an empty
		// reminder only when loaded history contains a stale catalog, so the
		// agent-owned overlay can mask it without mutating persisted messages.
		if catalog == "" && !dive.HasSystemReminder(hctx.Messages, skillReminderName) {
			return nil
		}
		reminder, err := dive.NewContextReminder(skillReminderName, catalog)
		if err != nil {
			return err
		}
		return hctx.PinReminder(reminder)
	}
}
