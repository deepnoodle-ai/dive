// Package subagent provides subagent management for Dive agents.
//
// This package contains types for defining and managing specialized subagents
// that can be spawned by a parent agent via the Task tool.
//
// # Migration from AgentOptions.Subagents
//
// Previously, subagents were configured via AgentOptions.Subagents and
// AgentOptions.SubagentLoader. With the new architecture, subagent registries
// should be passed directly to the Task tool at construction time.
//
// Old approach:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model: model,
//	    Subagents: map[string]*dive.SubagentDefinition{
//	        "code-reviewer": {...},
//	    },
//	    SubagentLoader: dive.NewFileSubagentLoader(),
//	})
//
// New approach:
//
//	registry := subagent.NewRegistry(true) // Include general-purpose
//	registry.Register("code-reviewer", &subagent.Definition{...})
//
//	taskTool := toolkit.NewTaskTool(toolkit.TaskToolOptions{
//	    SubagentRegistry: registry,
//	    ParentTools:      tools,
//	    AgentFactory:     myAgentFactory,
//	})
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model: model,
//	    Tools: []dive.Tool{taskTool, ...},
//	})
package subagent

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

// Definition defines a specialized subagent that can be spawned
// by a parent agent via the Task tool.
type Definition struct {
	// Description explains when this subagent should be used.
	// Claude uses this to decide whether to invoke the subagent.
	Description string

	// Prompt is the system prompt for the subagent.
	Prompt string

	// Tools lists the tool names this subagent is allowed to use.
	// If nil or empty, the subagent inherits all tools from the parent
	// (except the Task tool, which is never available to subagents).
	Tools []string

	// Model overrides the LLM model for this subagent.
	// Valid values: "sonnet", "opus", "haiku", or "" to inherit from parent.
	Model string
}

// GeneralPurpose is the default subagent available to all agents.
var GeneralPurpose = &Definition{
	Description: "General-purpose agent for complex, multi-step tasks. Use when no specialized agent matches the task.",
	Prompt:      "You are a helpful assistant that can handle complex multi-step tasks autonomously. Work through the task step by step and provide a clear summary of your findings or results.",
	Tools:       nil,
	Model:       "",
}

// Registry manages subagent definitions.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Definition
}

// NewRegistry creates a new Registry.
// If includeGeneralPurpose is true, the GeneralPurpose subagent is registered
// as "general-purpose".
func NewRegistry(includeGeneralPurpose bool) *Registry {
	r := &Registry{
		agents: make(map[string]*Definition),
	}
	if includeGeneralPurpose {
		r.agents["general-purpose"] = GeneralPurpose
	}
	return r
}

// Register adds or updates a subagent definition.
func (r *Registry) Register(name string, def *Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = def
}

// RegisterAll adds multiple subagent definitions.
func (r *Registry) RegisterAll(defs map[string]*Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, def := range defs {
		r.agents[name] = def
	}
}

// Get retrieves a subagent definition by name.
func (r *Registry) Get(name string) (*Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.agents[name]
	return def, ok
}

// List returns all registered subagent names in sorted order.
func (r *Registry) List() []string {
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
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// GenerateToolDescription generates a description of available subagents
// suitable for inclusion in the Task tool's description.
func (r *Registry) GenerateToolDescription() string {
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
func FilterTools(def *Definition, allTools []dive.Tool) []dive.Tool {
	var allowedSet map[string]bool
	if len(def.Tools) > 0 {
		allowedSet = make(map[string]bool, len(def.Tools))
		for _, name := range def.Tools {
			allowedSet[name] = true
		}
	}

	result := make([]dive.Tool, 0, len(allTools))
	for _, tool := range allTools {
		name := tool.Name()

		// Never allow Task tool in subagents
		if name == "Task" {
			continue
		}

		if allowedSet != nil {
			if !allowedSet[name] {
				continue
			}
		}

		result = append(result, tool)
	}
	return result
}

