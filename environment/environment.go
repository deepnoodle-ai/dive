package environment

import (
	"github.com/diveagents/dive"
	"github.com/diveagents/dive/mcp"
	"github.com/diveagents/dive/slogger"
)

// Options configures the environment.
type Options struct {
	Name               string
	Description        string
	Agents             []dive.Agent
	Logger             slogger.Logger
	Confirmer          dive.Confirmer
	DocumentRepository dive.DocumentRepository
	ThreadRepository   dive.ThreadRepository
	MCPServers         []*mcp.ServerConfig
	MCPManager         *mcp.Manager
}

type Environment struct {
	name               string
	description        string
	agents             []dive.Agent
	logger             slogger.Logger
	confirmer          dive.Confirmer
	documentRepository dive.DocumentRepository
	threadRepository   dive.ThreadRepository
	mcpServers         []*mcp.ServerConfig
	mcpManager         *mcp.Manager
}

func New(opts Options) (*Environment, error) {
	return &Environment{
		name:               opts.Name,
		description:        opts.Description,
		agents:             opts.Agents,
		logger:             opts.Logger,
		confirmer:          opts.Confirmer,
		documentRepository: opts.DocumentRepository,
		threadRepository:   opts.ThreadRepository,
		mcpServers:         opts.MCPServers,
		mcpManager:         opts.MCPManager,
	}, nil
}

func (e *Environment) Name() string {
	return e.name
}

func (e *Environment) Description() string {
	return e.description
}

func (e *Environment) Agents() []dive.Agent {
	return e.agents
}

func (e *Environment) Logger() slogger.Logger {
	return e.logger
}

func (e *Environment) Confirmer() dive.Confirmer {
	return e.confirmer
}

func (e *Environment) DocumentRepository() dive.DocumentRepository {
	return e.documentRepository
}

func (e *Environment) ThreadRepository() dive.ThreadRepository {
	return e.threadRepository
}

func (e *Environment) MCPServers() []*mcp.ServerConfig {
	return e.mcpServers
}

func (e *Environment) MCPManager() *mcp.Manager {
	return e.mcpManager
}
