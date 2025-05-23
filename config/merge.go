package config

import "sort"

// Merge merges two Environment configs, with the second one taking precedence
func Merge(base, override *Environment) *Environment {

	// Copy base environment
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
	if override.Config.DefaultWorkflow != "" {
		result.Config.DefaultWorkflow = override.Config.DefaultWorkflow
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

	// Merge workflows
	workflowMap := make(map[string]Workflow)
	for _, workflow := range result.Workflows {
		workflowMap[workflow.Name] = workflow
	}
	for _, workflow := range override.Workflows {
		workflowMap[workflow.Name] = workflow
	}
	workflows := make([]Workflow, 0, len(workflowMap))
	for _, w := range workflowMap {
		workflows = append(workflows, w)
	}
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})
	result.Workflows = workflows

	// Merge schedules
	scheduleMap := make(map[string]Schedule)
	for _, schedule := range result.Schedules {
		scheduleMap[schedule.Name] = schedule
	}
	for _, schedule := range override.Schedules {
		scheduleMap[schedule.Name] = schedule
	}
	schedules := make([]Schedule, 0, len(scheduleMap))
	for _, s := range scheduleMap {
		schedules = append(schedules, s)
	}
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].Name < schedules[j].Name
	})
	result.Schedules = schedules

	// Merge triggers
	triggerMap := make(map[string]Trigger)
	for _, trigger := range result.Triggers {
		triggerMap[trigger.Name] = trigger
	}
	for _, trigger := range override.Triggers {
		triggerMap[trigger.Name] = trigger
	}
	triggers := make([]Trigger, 0, len(triggerMap))
	for _, t := range triggerMap {
		triggers = append(triggers, t)
	}
	sort.Slice(triggers, func(i, j int) bool {
		return triggers[i].Name < triggers[j].Name
	})
	result.Triggers = triggers

	return &result
}
