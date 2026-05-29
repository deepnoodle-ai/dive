// Package subagent defines specialized subagents that a parent agent can spawn
// via the Agent tool (see toolkit/orchestration).
//
// A subagent is described by a Definition (system prompt, allowed/disallowed
// tools, optional model). Definitions are organized in a plain map keyed by type
// name and handed to the Agent tool:
//
//	subagents := map[string]*subagent.Definition{
//	    "GeneralPurpose": subagent.GeneralPurpose,
//	    "Explore":        subagent.Explore,
//	    "Plan":           subagent.Plan,
//	}
//
//	agentTool := orchestration.NewAgentTool(orchestration.AgentToolOptions{
//	    Subagents:    subagents,
//	    AgentFactory: myAgentFactory,
//	    ParentTools:  tools,
//	})
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model: model,
//	    Tools: []dive.Tool{agentTool},
//	})
//
// Definitions can also be loaded from markdown files with YAML frontmatter via a
// Loader (see FileLoader); Load returns the same map type.
package subagent

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// Built-in subagent system prompts. The canonical, human-facing copies live in
// docs/prompts/; these are byte-identical copies embedded for use at runtime
// (go:embed cannot reference paths outside the package directory). Keep the two
// in sync when editing.
//
//go:embed prompts/subagent.md
var generalPurposePrompt string

//go:embed prompts/explore.md
var explorePrompt string

//go:embed prompts/plan.md
var planPrompt string

// Definition defines a specialized subagent that can be spawned
// by a parent agent via the Agent tool.
type Definition struct {
	// Description explains when this subagent should be used.
	// Claude uses this to decide whether to invoke the subagent.
	Description string

	// Prompt is the system prompt for the subagent.
	Prompt string

	// Tools lists the tool names this subagent is allowed to use.
	// If nil or empty, the subagent inherits all tools from the parent
	// (except the Agent tool, which is never available to subagents).
	Tools []string

	// DisallowedTools lists tool names to exclude from the subagent's tool set.
	// When Tools is empty, starts from all parent tools and removes these.
	// When Tools is set, starts from that allowlist and removes these.
	// Names are matched case-insensitively.
	DisallowedTools []string

	// Model overrides the LLM model for this subagent.
	// Valid values: "sonnet", "opus", "haiku", or "" to inherit from parent.
	Model string
}

// GeneralPurpose is the default subagent available to all agents.
var GeneralPurpose = &Definition{
	Description: "General-purpose agent for complex, multi-step tasks. Use when no specialized agent matches the task.",
	Prompt:      generalPurposePrompt,
	Tools:       nil,
	Model:       "",
}

// Explore is a read-only subagent optimized for file search, code reading, and summarization.
// Clone and modify to override the model or adjust the tool set:
//
//	myExplore := *subagent.Explore
//	myExplore.Model = "haiku"
var Explore = &Definition{
	Description:     "Fast read-only search agent for locating code. Use to find files, grep for symbols, or answer where-is-X questions.",
	DisallowedTools: []string{"Edit", "Write", "Bash"},
	Prompt:          explorePrompt,
}

// Plan is a read-only subagent optimized for architectural analysis and structured planning.
// Clone and modify to override the model or adjust the tool set.
var Plan = &Definition{
	Description:     "Software architect agent for designing implementation plans. Use when you need to plan an implementation strategy.",
	DisallowedTools: []string{"Edit", "Write", "Bash"},
	Prompt:          planPrompt,
}

// DescribeTypes renders a subagent catalog as a description suitable for
// inclusion in the Agent tool's description. Types are listed in sorted order;
// an empty catalog yields an empty string.
func DescribeTypes(types map[string]*Definition) string {
	if len(types) == 0 {
		return ""
	}

	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString("Available subagent types:\n")
	for _, name := range names {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, types[name].Description))
	}
	return sb.String()
}

// FilterTools filters a list of tools based on the subagent definition.
// If def.Tools is non-empty, only those tools are kept; otherwise all parent
// tools are kept. Any tool named in def.DisallowedTools is then removed
// (matched case-insensitively), and the Agent tool is never included.
func FilterTools(def *Definition, allTools []dive.Tool) []dive.Tool {
	var allowedSet map[string]bool
	if len(def.Tools) > 0 {
		allowedSet = make(map[string]bool, len(def.Tools))
		for _, name := range def.Tools {
			allowedSet[name] = true
		}
	}

	var disallowedSet map[string]bool
	if len(def.DisallowedTools) > 0 {
		disallowedSet = make(map[string]bool, len(def.DisallowedTools))
		for _, name := range def.DisallowedTools {
			disallowedSet[strings.ToLower(name)] = true
		}
	}

	result := make([]dive.Tool, 0, len(allTools))
	for _, tool := range allTools {
		name := tool.Name()

		// Never allow the Agent tool in subagents
		if name == "Agent" {
			continue
		}

		if allowedSet != nil && !allowedSet[name] {
			continue
		}

		if disallowedSet != nil && disallowedSet[strings.ToLower(name)] {
			continue
		}

		result = append(result, tool)
	}
	return result
}
