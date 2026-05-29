// Package orchestration provides tools for spawning subagents and controlling
// background runs, aligned with Claude Code's tool model:
//
//   - Agent    — spawn a subagent (EXECUTION axis)
//   - TaskStop — cancel a running background run by task_id (CONTROL axis)
//   - Monitor  — stream events from a long-running shell command (CONTROL axis)
//
// The Agent tool (for background spawns) and Monitor register their cancellable
// runs in a shared *Runs tracker; TaskStop cancels by id. Subagents spawned by
// the Agent tool are single-use: they run with a fresh context and return one
// final message. Background results are delivered automatically via Dive's
// background-task machinery — no polling tool is needed.
package orchestration

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/subagent"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/google/uuid"
)

// AgentFactory creates an agent for a subagent spawn. It receives the subagent
// type name, its definition, and the parent's tools so it can apply tool
// filtering (subagent.FilterTools). The factory is the seam where an application
// can layer in worktree isolation, sandboxing, a per-run session, or model
// routing before returning the agent.
type AgentFactory func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error)

// AgentToolInput is the input for the Agent tool.
type AgentToolInput struct {
	Prompt          string `json:"prompt"`
	Description     string `json:"description"`
	SubagentType    string `json:"subagent_type"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
}

// AgentToolOptions configures a new Agent tool.
type AgentToolOptions struct {
	// Subagents is the catalog of spawnable subagent definitions, keyed by type
	// name. It is copied at construction, so later mutation by the caller has no
	// effect on the tool.
	Subagents map[string]*subagent.Definition

	// AgentFactory creates the agent for a spawn. Required.
	AgentFactory AgentFactory

	// ParentTools are the parent agent's tools, passed to the factory for tool
	// filtering.
	ParentTools []dive.Tool

	// Runs, if non-nil, tracks background spawns so TaskStop can cancel them.
	// When nil, background spawns run un-cancellable.
	Runs *Runs

	// DefaultTimeout bounds synchronous spawns. Defaults to 10 minutes.
	// Background spawns are not time-bounded (stop them with TaskStop).
	DefaultTimeout time.Duration
}

type agentTool struct {
	subagents      map[string]*subagent.Definition
	factory        AgentFactory
	parentTools    []dive.Tool
	runs           *Runs
	defaultTimeout time.Duration
}

var _ dive.TypedTool[*AgentToolInput] = &agentTool{}

// NewAgentTool creates the Agent tool, which spawns subagents.
func NewAgentTool(opts AgentToolOptions) *dive.TypedToolAdapter[*AgentToolInput] {
	if opts.DefaultTimeout <= 0 {
		opts.DefaultTimeout = 10 * time.Minute
	}
	// Defensive copy: freeze the catalog so caller mutation can't race tool reads.
	subagents := make(map[string]*subagent.Definition, len(opts.Subagents))
	for name, def := range opts.Subagents {
		subagents[name] = def
	}
	return dive.ToolAdapter(&agentTool{
		subagents:      subagents,
		factory:        opts.AgentFactory,
		parentTools:    opts.ParentTools,
		runs:           opts.Runs,
		defaultTimeout: opts.DefaultTimeout,
	})
}

func (t *agentTool) Name() string { return "Agent" }

func (t *agentTool) Description() string {
	desc := `Launch a specialized agent to handle a complex, multi-step task autonomously.

Each agent type has specific capabilities and tools available to it. A spawned agent runs with a fresh context and returns a single final message when it is done.

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Run agents in the background by default (run_in_background: true) so you can continue working while they run
- Launch multiple agents concurrently whenever possible to maximize performance
- A background agent's result is delivered to you automatically when it completes
- Each agent is single-use; provide a clear, detailed prompt so it can work autonomously`

	if d := subagent.DescribeTypes(t.subagents); d != "" {
		desc += "\n\n" + d
	}
	return desc
}

func (t *agentTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"prompt", "description", "subagent_type"},
		Properties: map[string]*schema.Property{
			"prompt": {
				Type:        "string",
				Description: "The task for the agent to perform. Provide detailed instructions.",
			},
			"description": {
				Type:        "string",
				Description: "A short (3-5 word) description of the task.",
			},
			"subagent_type": {
				Type:        "string",
				Description: "The type of specialized agent to use (e.g., GeneralPurpose, Explore, Plan).",
			},
			"run_in_background": {
				Type:        "boolean",
				Description: "Run this agent in the background so you can continue working (default: true). The result is delivered automatically when complete. Set to false only when you need the result before continuing.",
			},
		},
	}
}

func (t *agentTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Agent",
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *agentTool) Call(ctx context.Context, input *AgentToolInput) (*dive.ToolResult, error) {
	if input.Prompt == "" {
		return dive.NewToolResultError("prompt is required"), nil
	}
	if input.Description == "" {
		return dive.NewToolResultError("description is required"), nil
	}
	if input.SubagentType == "" {
		return dive.NewToolResultError("subagent_type is required"), nil
	}

	def, ok := t.subagents[input.SubagentType]
	if !ok {
		return dive.NewToolResultError(fmt.Sprintf(
			"unknown subagent type %q. Available types: %v",
			input.SubagentType, t.typeNames())), nil
	}

	agent, err := t.factory(ctx, input.SubagentType, def, t.parentTools)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("failed to create agent: %s", err.Error())), nil
	}

	if input.RunInBackground {
		return t.runBackground(input, agent), nil
	}
	return t.runSync(ctx, input, agent), nil
}

// runSync runs the subagent to completion (bounded by DefaultTimeout) and
// returns its result inline. Synchronous spawns are not tracked — there is no
// id to TaskStop against while the turn is blocked on the call.
func (t *agentTool) runSync(ctx context.Context, input *AgentToolInput, agent *dive.Agent) *dive.ToolResult {
	runCtx, cancel := context.WithTimeout(ctx, t.defaultTimeout)
	defer cancel()

	response, err := agent.CreateResponse(runCtx, dive.WithMessages(llm.NewUserTextMessage(input.Prompt)))
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Subagent failed: %s", err.Error())).
			WithDisplay(fmt.Sprintf("Failed: %s", input.Description))
	}
	return dive.NewToolResultText(subagentOutput(response)).
		WithDisplay(fmt.Sprintf("Completed: %s", input.Description))
}

// runBackground dispatches the subagent on a cancellable, parent-independent
// context and returns immediately. Dive delivers the final result automatically
// when the goroutine completes. The run is registered in Runs (if configured)
// so TaskStop can cancel it by its task_id.
func (t *agentTool) runBackground(input *AgentToolInput, agent *dive.Agent) *dive.ToolResult {
	taskID := fmt.Sprintf("task_%s", uuid.New().String()[:8])
	runCtx, cancel := context.WithCancel(context.Background())
	if t.runs != nil {
		t.runs.add(taskID, input.Description, cancel)
	}
	return dive.NewBackgroundResultFull(runCtx, input.Description, func(runCtx context.Context) *dive.ToolResult {
		defer cancel()
		if t.runs != nil {
			defer t.runs.remove(taskID)
		}
		response, err := agent.CreateResponse(runCtx, dive.WithMessages(llm.NewUserTextMessage(input.Prompt)))
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Subagent failed (Task ID: %s): %s", taskID, err.Error())).
				WithDisplay(fmt.Sprintf("Failed: %s", input.Description))
		}
		return dive.NewToolResultText(fmt.Sprintf("Task ID: %s\n\n%s", taskID, subagentOutput(response))).
			WithDisplay(fmt.Sprintf("Completed: %s", input.Description))
	})
}

func (t *agentTool) typeNames() []string {
	names := make([]string, 0, len(t.subagents))
	for name := range t.subagents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// subagentOutput renders a completed subagent response as text. Subagents are
// single-use, so a subagent that suspends mid-turn cannot be resumed; surface
// its pending prompt as the (terminal) result instead.
func subagentOutput(response *dive.Response) string {
	if response != nil && response.Status == dive.ResponseStatusSuspended {
		if response.Suspension != nil && len(response.Suspension.PendingToolCalls) > 0 {
			if p := response.Suspension.PendingToolCalls[0].Prompt; p != "" {
				return fmt.Sprintf("Subagent paused awaiting input and cannot be resumed (subagents are single-use): %s", p)
			}
		}
		return "Subagent paused awaiting input and cannot be resumed (subagents are single-use)."
	}
	return response.OutputText()
}
