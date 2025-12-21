package dive

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// SubagentDefinition defines a specialized subagent that can be spawned
// by a parent agent via the Task tool.
//
// Subagents are separate agent instances that handle focused subtasks.
// They maintain separate context from the main agent, can run in parallel,
// and can have specialized instructions and restricted tool access.
type SubagentDefinition struct {
	// Description explains when this subagent should be used.
	// Claude uses this to decide whether to invoke the subagent.
	// Write clear descriptions so Claude can match tasks appropriately.
	Description string

	// Prompt is the system prompt for the subagent, defining its role and behavior.
	Prompt string

	// Tools lists the tool names this subagent is allowed to use.
	// If nil or empty, the subagent inherits all tools from the parent
	// (except the Task tool, which is never available to subagents).
	Tools []string

	// Model overrides the LLM model for this subagent.
	// Valid values: "sonnet", "opus", "haiku", or "" to inherit from parent.
	Model string
}

// GeneralPurposeSubagent is the default subagent available to all agents.
// It can be used for complex, multi-step tasks when no specialized subagent matches.
var GeneralPurposeSubagent = &SubagentDefinition{
	Description: "General-purpose agent for complex, multi-step tasks. Use when no specialized agent matches the task.",
	Prompt:      "You are a helpful assistant that can handle complex multi-step tasks autonomously. Work through the task step by step and provide a clear summary of your findings or results.",
	Tools:       nil, // Inherit all (except Task)
	Model:       "",  // Inherit from parent
}

// SubagentRegistry manages subagent definitions for an agent.
// It provides lookup, listing, and description generation for the Task tool.
type SubagentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*SubagentDefinition
}

// NewSubagentRegistry creates a new SubagentRegistry.
// If includeGeneralPurpose is true, the GeneralPurposeSubagent is registered
// as "general-purpose".
func NewSubagentRegistry(includeGeneralPurpose bool) *SubagentRegistry {
	r := &SubagentRegistry{
		agents: make(map[string]*SubagentDefinition),
	}
	if includeGeneralPurpose {
		r.agents["general-purpose"] = GeneralPurposeSubagent
	}
	return r
}

// Register adds or updates a subagent definition in the registry.
func (r *SubagentRegistry) Register(name string, def *SubagentDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = def
}

// RegisterAll adds multiple subagent definitions to the registry.
// Existing definitions with the same name are overwritten.
func (r *SubagentRegistry) RegisterAll(defs map[string]*SubagentDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, def := range defs {
		r.agents[name] = def
	}
}

// Get retrieves a subagent definition by name.
func (r *SubagentRegistry) Get(name string) (*SubagentDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.agents[name]
	return def, ok
}

// List returns all registered subagent names in sorted order.
func (r *SubagentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len returns the number of registered subagents.
func (r *SubagentRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// GenerateToolDescription generates a description of available subagents
// suitable for inclusion in the Task tool's description.
func (r *SubagentRegistry) GenerateToolDescription() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.agents) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Available subagent types:\n")

	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		def := r.agents[name]
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, def.Description))
	}

	return sb.String()
}

// FilterTools filters a list of tools based on the subagent definition's allowed tools.
// If def.Tools is nil or empty, all tools are returned except the Task tool.
// The Task tool is never included to prevent subagents from spawning their own subagents.
func FilterTools(def *SubagentDefinition, allTools []Tool) []Tool {
	// Build set of allowed tool names
	var allowedSet map[string]bool
	if len(def.Tools) > 0 {
		allowedSet = make(map[string]bool, len(def.Tools))
		for _, name := range def.Tools {
			allowedSet[name] = true
		}
	}

	result := make([]Tool, 0, len(allTools))
	for _, tool := range allTools {
		name := tool.Name()

		// Never allow Task tool in subagents
		if name == "Task" {
			continue
		}

		// If Tools is specified, only include allowed tools
		if allowedSet != nil {
			if !allowedSet[name] {
				continue
			}
		}

		result = append(result, tool)
	}
	return result
}
