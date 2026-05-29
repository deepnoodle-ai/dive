// hooks_example demonstrates three additions to Dive's hook system, printing a
// line each time a hook fires so you can watch them work:
//
//   - SessionStart hook — seeds a fresh conversation with durable project
//     context (Persist: true), so it stays in history on every later turn.
//   - PromptToolGate — a cheap model vets each shell command and denies
//     anything that would change production (fails closed).
//   - PromptStopHook — a cheap model checks whether the task is actually done
//     and nudges the agent to keep going if not (fails open).
//
// The main agent runs on a capable model; the two judgment hooks run on a small,
// fast model to keep the extra calls cheap. The gate and stop hooks are each
// wrapped in a few lines of logging purely so the demo is observable — in real
// code you would use the helper directly.
//
// Requires ANTHROPIC_API_KEY.
// Run: cd examples && go run ./hooks_example
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type ShellInput struct {
	Command string `json:"command" description:"The shell command to run"`
}

func main() {
	ctx := context.Background()

	// A small, fast model for the judgment hooks — they only return a verdict.
	judge := anthropic.New(anthropic.WithModel(anthropic.ModelClaudeHaiku45))

	// SessionStart: seed durable project context the agent should always know.
	// Persist: true saves it as a pre-turn so it survives later turns and resume.
	seedContext := func(ctx context.Context, hctx *dive.HookContext) (*dive.SessionStartResult, error) {
		fmt.Println("[SessionStart] seeding durable project context")
		return &dive.SessionStartResult{
			Persist: true,
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Project context: Acme API (Go), currently on version v1.4.0. " +
					"Builds always use the `make build` command."),
			},
		}, nil
	}

	// A stub shell tool so the agent has something to call. It does not actually
	// execute anything — PromptToolGate still vets every command first.
	shellTool := dive.FuncTool("run_shell",
		"Runs a shell command and returns its output.",
		func(ctx context.Context, in *ShellInput) (*dive.ToolResult, error) {
			return dive.NewToolResultText(fmt.Sprintf("(demo) ran: %s", in.Command)), nil
		})

	// The judgment helpers, each wrapped in a tiny logging closure for the demo.
	gate := dive.PromptToolGate(judge,
		"Is this shell command safe to run on a developer's machine? "+
			"Approve ordinary local build and inspection commands; "+
			"deny anything that changes production systems.")
	stopJudge := dive.PromptStopHook(judge,
		"Has the user's request been fully completed? If not, briefly say what remains.")

	sess := session.New("hooks-demo")

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Release Assistant",
		SystemPrompt: "You are a release assistant. Use run_shell for any shell work.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{shellTool},
		Session:      sess,
		Hooks: dive.Hooks{
			SessionStart: []dive.SessionStartHook{seedContext},
			PreToolUse: []dive.PreToolUseHook{
				dive.MatchTool("run_shell", func(ctx context.Context, hctx *dive.HookContext) error {
					err := gate(ctx, hctx)
					if err != nil {
						fmt.Printf("[PromptToolGate] denied %s\n", string(hctx.Call.Input))
					} else {
						fmt.Printf("[PromptToolGate] allowed %s\n", string(hctx.Call.Input))
					}
					return err
				}),
			},
			Stop: []dive.StopHook{
				func(ctx context.Context, hctx *dive.HookContext) (*dive.StopDecision, error) {
					dec, err := stopJudge(ctx, hctx)
					if dec != nil && dec.Continue {
						fmt.Printf("[PromptStopHook] work remains, nudging agent: %s\n", dec.Reason)
					} else {
						fmt.Println("[PromptStopHook] task complete, allowing stop")
					}
					return dec, err
				},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Explicit commands so the gate is exercised both ways: `make build` is an
	// ordinary local command (approved), while the production deploy is denied —
	// and the denial reason is fed back to the agent.
	resp, err := agent.CreateResponse(ctx, dive.WithInput(
		"What version are we on? Build the project with `make build`, then deploy it "+
			"to production with `make deploy ENV=production`."))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nAgent: %s\n", resp.OutputText())

	// Show that the durable seed was persisted: it leads the saved history and
	// would be visible to the model on every subsequent turn of this session.
	msgs, err := sess.Messages(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nSession holds %d messages; the first is the durable seed:\n  %q\n",
		len(msgs), truncate(msgs[0].Text(), 80))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
