package config

import "sort"

// Merge merges two DiveConfig configs, with the second one taking precedence
func Merge(base, override *Config) *Config {

	// Copy base config
	result := *base

	// Merge name and description if provided
	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Description != "" {
		result.Description = override.Description
	}

	// Merge config
	if override.Config.DefaultProvider != "" {
		result.Config.DefaultProvider = override.Config.DefaultProvider
	}
	if override.Config.DefaultModel != "" {
		result.Config.DefaultModel = override.Config.DefaultModel
	}
	if override.Config.LogLevel != "" {
		result.Config.LogLevel = override.Config.LogLevel
	}

	// Merge providers
	providersByName := make(map[string]Provider)
	for _, p := range result.Config.Providers {
		providersByName[p.Name] = p
	}
	for _, p := range override.Config.Providers {
		providersByName[p.Name] = p
	}
	providers := make([]Provider, 0, len(providersByName))
	for _, p := range providersByName {
		providers = append(providers, p)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	result.Config.Providers = providers

	// Merge tools
	toolMap := make(map[string]Tool)
	for _, t := range result.Tools {
		toolMap[t.Name] = t
	}
	for _, t := range override.Tools {
		toolMap[t.Name] = t
	}
	tools := make([]Tool, 0, len(toolMap))
	for _, t := range toolMap {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	result.Tools = tools

	// Merge agents
	agentMap := make(map[string]Agent)
	for _, agent := range result.Agents {
		agentMap[agent.Name] = agent
	}
	for _, agent := range override.Agents {
		agentMap[agent.Name] = agent
	}
	agents := make([]Agent, 0, len(agentMap))
	for _, a := range agentMap {
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})
	result.Agents = agents

	// Merge mcp servers
	mcpServerMap := make(map[string]MCPServer)
	for _, mcpServer := range result.MCPServers {
		mcpServerMap[mcpServer.Name] = mcpServer
	}
	for _, mcpServer := range override.MCPServers {
		mcpServerMap[mcpServer.Name] = mcpServer
	}
	mcpServers := make([]MCPServer, 0, len(mcpServerMap))
	for _, mcpServer := range mcpServerMap {
		mcpServers = append(mcpServers, mcpServer)
	}
	sort.Slice(mcpServers, func(i, j int) bool {
		return mcpServers[i].Name < mcpServers[j].Name
	})
	result.MCPServers = mcpServers

	return &result
}
