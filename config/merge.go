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
	if override.Config.LLM.CacheControl != "" {
		result.Config.LLM.CacheControl = override.Config.LLM.CacheControl
	}
	if override.Config.LLM.DefaultProvider != "" {
		result.Config.LLM.DefaultProvider = override.Config.LLM.DefaultProvider
	}
	if override.Config.LLM.DefaultModel != "" {
		result.Config.LLM.DefaultModel = override.Config.LLM.DefaultModel
	}
	if override.Config.Logging.Level != "" {
		result.Config.Logging.Level = override.Config.Logging.Level
	}
	if override.Config.Workflows.DefaultWorkflow != "" {
		result.Config.Workflows.DefaultWorkflow = override.Config.Workflows.DefaultWorkflow
	}

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

	// Merge prompts
	promptMap := make(map[string]Prompt)
	for _, prompt := range result.Prompts {
		promptMap[prompt.Name] = prompt
	}
	for _, prompt := range override.Prompts {
		promptMap[prompt.Name] = prompt
	}
	prompts := make([]Prompt, 0, len(promptMap))
	for _, p := range promptMap {
		prompts = append(prompts, p)
	}
	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].Name < prompts[j].Name
	})
	result.Prompts = prompts

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
