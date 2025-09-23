package enhanced

import (
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/hooks"
	"github.com/deepnoodle-ai/dive/mcp"
	"github.com/deepnoodle-ai/dive/memory"
	"github.com/deepnoodle-ai/dive/permissions"
	"github.com/deepnoodle-ai/dive/settings"
	"github.com/deepnoodle-ai/dive/subagents"
)

// BaseEnvironment is an interface to avoid circular import
type BaseEnvironment interface {
	GetAgents() []dive.Agent
	GetTools() map[string]dive.Tool
	GetMCPManager() *mcp.Manager
	GetLogger() interface{}
	GetThreads() dive.ThreadRepository
	GetConfirmer() dive.Confirmer
	GetDirectory() string
}

// Environment extends base environment with Claude Code-inspired features
type Environment struct {
	BaseEnvironment   BaseEnvironment
	MemoryManager     *memory.Memory
	HookManager       *hooks.HookManager
	PermissionManager *permissions.PermissionManager
	SubagentManager   *subagents.SubagentManager
	SettingsManager   *settings.SettingsManager
	UnifiedConfig     interface{} // Avoid circular import by using interface{}
	Agents            []dive.Agent
	Tools             map[string]dive.Tool
}

// EnhanceAgent wraps an agent with enhanced capabilities
func (ee *Environment) EnhanceAgent(agent dive.Agent) dive.Agent {
	return &Agent{
		Agent:             agent,
		Environment:       ee,
		MemoryManager:     ee.MemoryManager,
		HookManager:       ee.HookManager,
		PermissionManager: ee.PermissionManager,
		SubagentManager:   ee.SubagentManager,
	}
}