package skill

import (
	"context"
	"strings"
	"sync"

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
// Hooks are always returned even when no skills are loaded, because a
// resumed session may contain a stale catalog block that needs cleanup.
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
//
// The hook uses dive.SetSystemReminder, which is idempotent: it inserts the
// block on first call and replaces it in place if the catalog changes.
func catalogHook(loader *Loader) dive.PreGenerationHook {
	// lastHash is shared across invocations of the returned hook, which may
	// run concurrently when multiple CreateResponse calls execute on one
	// agent. The mutex guards it; per-call HookContext state needs no guard.
	var mu sync.Mutex
	var lastHash string

	return func(_ context.Context, hctx *dive.HookContext) error {
		mu.Lock()
		defer mu.Unlock()

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
