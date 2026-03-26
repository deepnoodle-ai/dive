package skill

import (
	"context"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// ConfigureAgent sets up skill support on the given AgentOptions.
// Call this before dive.NewAgent(). It:
//   - Adds the Skill tool to opts.Tools
//   - Appends skill usage rules to opts.SystemPrompt
//   - Adds a PreGenerationHook for catalog injection into conversation context
//   - Adds a PostToolUseHook for skill content injection
//
// This follows the same pattern as setting AgentOptions.Session — one call
// wires up all the internal machinery.
//
// Example:
//
//	loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
//	loader.Load(ctx)
//
//	opts := dive.AgentOptions{
//	    Model: anthropic.New(),
//	    Tools: tools,
//	}
//	skill.ConfigureAgent(&opts, loader)
//	agent, _ := dive.NewAgent(opts)
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
//
// The hook uses dive.SetSystemReminder, which is idempotent: it inserts the
// block on first call and replaces it in place if the catalog changes.
func catalogHook(loader *Loader) dive.PreGenerationHook {
	var lastHash string

	return func(_ context.Context, hctx *dive.HookContext) error {
		hash := CatalogHash(loader)
		if hash == "" {
			// No skills — remove stale catalog block if present.
			// Check messages directly (not just lastHash) to handle
			// session resume where a previous process left a block.
			if dive.HasSystemReminder(hctx.Messages, skillReminderName) {
				hctx.Messages = dive.RemoveSystemReminder(hctx.Messages, skillReminderName)
			}
			lastHash = ""
			return nil
		}
		if hash == lastHash {
			// Catalog unchanged, but ensure the block exists in messages
			// (handles session resume where hook state is fresh but
			// messages already contain the block)
			if dive.HasSystemReminder(hctx.Messages, skillReminderName) {
				return nil
			}
		}
		lastHash = hash

		catalog := BuildCatalog(loader)
		if catalog == "" {
			return nil
		}

		hctx.Messages = dive.SetSystemReminder(hctx.Messages, skillReminderName, catalog)
		return nil
	}
}
