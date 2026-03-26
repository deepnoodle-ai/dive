package skill

import (
	"context"

	"github.com/deepnoodle-ai/dive"
)

// toolset implements dive.Toolset for allowed-tools filtering.
type toolset struct {
	loader *Loader
	tools  []dive.Tool
}

// NewToolset returns a Toolset that filters available tools based on the
// active skill's AllowedTools configuration.
//
// When no skill is active or the active skill has no allowed-tools,
// all tools are returned. When a skill with allowed-tools is active,
// only the listed tools (plus the Skill tool itself) are returned.
func NewToolset(loader *Loader, tools []dive.Tool) dive.Toolset {
	return &toolset{
		loader: loader,
		tools:  tools,
	}
}

func (ts *toolset) Name() string {
	return "skill-filter"
}

func (ts *toolset) Tools(_ context.Context) ([]dive.Tool, error) {
	active := ts.loader.ActiveSkill()
	if active == nil || len(active.Config.AllowedTools) == 0 {
		return ts.tools, nil
	}

	// Build allowed set
	allowed := make(map[string]bool, len(active.Config.AllowedTools)+1)
	for _, name := range active.Config.AllowedTools {
		allowed[name] = true
	}
	// The Skill tool is always included so the agent can switch skills
	allowed["Skill"] = true

	var filtered []dive.Tool
	for _, tool := range ts.tools {
		if allowed[tool.Name()] {
			filtered = append(filtered, tool)
		}
	}

	return filtered, nil
}
