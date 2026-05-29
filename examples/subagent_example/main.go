// Subagent example demonstrates a parent agent spawning a built-in read-only
// subagent (Explore) through the Agent tool.
//
// The subagent package ships ready-made Definitions (Explore, Plan,
// GeneralPurpose). Explore and Plan are read-only: their DisallowedTools strip
// Edit/Write/Bash so they can locate and reason about code but never change it.
//
// Wiring (the standard production pattern, mirroring experimental/cmd/dive):
//   - A map[string]*subagent.Definition holds the spawnable definitions (Explore, Plan).
//   - An AgentFactory turns a definition into a concrete agent on demand,
//     giving it the read-only tool set via subagent.FilterTools.
//   - orchestration.NewAgentTool exposes an "Agent" tool to the parent agent.
//   - The parent is given ONLY the Agent tool, so to search code it must
//     delegate to the Explore subagent.
//
// To keep the run visible, the parent logs every Agent call via an event
// callback, and each spawned subagent logs its own tool calls via a hook.
//
// Usage:
//
//	cd examples
//	ANTHROPIC_API_KEY=... go run ./subagent_example
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/subagent"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/dive/toolkit/orchestration"
)

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Fatal("ANTHROPIC_API_KEY is not set")
	}
	ctx := context.Background()

	// Create a small, throwaway workspace and work inside it, so the read-only
	// subagent's searches resolve to our files and can't wander the real machine.
	workspace, err := writeWorkspace()
	if err != nil {
		log.Fatalf("failed to create workspace: %v", err)
	}
	defer os.RemoveAll(workspace)
	if err := os.Chdir(workspace); err != nil {
		log.Fatalf("failed to enter workspace: %v", err)
	}

	validator, err := toolkit.NewPathValidator(workspace)
	if err != nil {
		log.Fatalf("failed to create path validator: %v", err)
	}

	model := anthropic.New(anthropic.WithModel(anthropic.ModelClaudeHaiku45))

	// The standard toolkit. Subagents inherit a filtered view of these; the
	// parent itself is given only the Agent tool (below) so it must delegate.
	subagentTools := []dive.Tool{
		toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{Validator: validator}),
		toolkit.NewGlobTool(toolkit.GlobToolOptions{Validator: validator}),
		toolkit.NewGrepTool(toolkit.GrepToolOptions{Validator: validator}),
		toolkit.NewListDirectoryTool(toolkit.ListDirectoryToolOptions{Validator: validator}),
		toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{Validator: validator}),
		toolkit.NewEditTool(toolkit.EditToolOptions{Validator: validator}),
		toolkit.NewBashTool(toolkit.BashToolOptions{Validator: validator}),
	}

	fmt.Printf("=== Dive subagent example ===\n\n")
	fmt.Printf("Workspace: %s\n\n", workspace)

	// Both read-only built-ins drop Edit/Write/Bash via DisallowedTools.
	printFiltered("Explore", subagent.Explore, subagentTools)
	printFiltered("Plan", subagent.Plan, subagentTools)

	// The spawnable subagents, keyed by type name.
	subagents := map[string]*subagent.Definition{
		"Explore": subagent.Explore,
		"Plan":    subagent.Plan,
	}

	// The factory builds a fresh agent each time the parent spawns a subagent:
	// the definition's prompt, the read-only tool set, and a hook that logs the
	// subagent's own tool calls so its work is visible from the outside.
	factory := func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
		return dive.NewAgent(dive.AgentOptions{
			Name:         name,
			SystemPrompt: def.Prompt,
			Model:        model,
			Tools:        subagent.FilterTools(def, parentTools),
			Hooks:        dive.Hooks{PreToolUse: []dive.PreToolUseHook{logSubagentTool(name)}},
		})
	}

	agentTool := orchestration.NewAgentTool(orchestration.AgentToolOptions{
		Subagents:    subagents,
		AgentFactory: factory,
		ParentTools:  subagentTools,
	})

	// The parent holds ONLY the Agent tool, so it cannot read files itself — it
	// must delegate exploration to a subagent.
	parent, err := dive.NewAgent(dive.AgentOptions{
		Name: "Orchestrator",
		SystemPrompt: "You are an orchestrator with no file access of your own. To answer " +
			"questions about code in the workspace, delegate to the \"Explore\" subagent " +
			"using the Agent tool. Wait for the subagent to finish (do not run it in the " +
			"background), then answer the user using its findings.",
		Model: model,
		Tools: []dive.Tool{agentTool},
	})
	if err != nil {
		log.Fatalf("failed to create parent agent: %v", err)
	}

	task := "Where is the `Add` function defined in this workspace, and what does it do? " +
		"Also list the other exported functions."

	fmt.Printf("\nTask for parent: %s\n\n--- Activity ([parent] delegates; [Explore] does the work) ---\n", task)

	response, err := parent.CreateResponse(ctx,
		dive.WithInput(task),
		dive.WithEventCallback(logParent),
	)
	if err != nil {
		log.Fatalf("parent agent error: %v", err)
	}

	fmt.Printf("\n--- Parent's answer ---\n%s\n", response.OutputText())
}

// logParent prints the parent agent's tool calls (i.e. its Agent delegations).
func logParent(ctx context.Context, item *dive.ResponseItem) error {
	switch item.Type {
	case dive.ResponseItemTypeToolCall:
		if tc := item.ToolCall; tc != nil {
			fmt.Printf("[parent] → %s %s\n", tc.Name, truncate(string(tc.Input), 160))
		}
	case dive.ResponseItemTypeToolCallResult:
		if r := item.ToolCallResult; r != nil {
			fmt.Printf("[parent] ← %s returned\n", r.Name)
		}
	}
	return nil
}

// logSubagentTool returns a PreToolUse hook that logs each tool call a spawned
// subagent makes, indented under the parent's delegation.
func logSubagentTool(name string) dive.PreToolUseHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		if hctx.Call != nil {
			fmt.Printf("    [%s] → %s %s\n", name, hctx.Call.Name, truncate(string(hctx.Call.Input), 120))
		}
		return nil
	}
}

// printFiltered shows which of allTools a read-only definition keeps vs removes.
func printFiltered(name string, def *subagent.Definition, allTools []dive.Tool) {
	kept := map[string]bool{}
	for _, t := range subagent.FilterTools(def, allTools) {
		kept[t.Name()] = true
	}
	var allowed, removed []string
	for _, t := range allTools {
		if kept[t.Name()] {
			allowed = append(allowed, t.Name())
		} else {
			removed = append(removed, t.Name())
		}
	}
	fmt.Printf("%-8s allowed: %-45s removed: %s\n",
		name, strings.Join(allowed, ", "), strings.Join(removed, ", "))
}

func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace for one-line logs
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// writeWorkspace creates a temp directory with a couple of tiny Go files for the
// Explore subagent to search.
func writeWorkspace() (string, error) {
	dir, err := os.MkdirTemp("", "dive-explore-*")
	if err != nil {
		return "", err
	}
	files := map[string]string{
		"math.go": `package calc

// Add returns the sum of two integers.
func Add(a, b int) int { return a + b }

// Multiply returns the product of two integers.
func Multiply(a, b int) int { return a * b }
`,
		"text.go": `package calc

import "strings"

// Shout uppercases s and appends an exclamation mark.
func Shout(s string) string { return strings.ToUpper(s) + "!" }
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}
