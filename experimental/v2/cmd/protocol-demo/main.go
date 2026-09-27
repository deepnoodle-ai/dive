// protocol-demo runs deterministic v2 experiments without credentials or network.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	v2 "github.com/deepnoodle-ai/dive/experimental/v2"
)

func engine() v2.Engine {
	return v2.Engine{
		Model: func(_ context.Context, request v2.Request) (v2.ModelResult, error) {
			fmt.Printf("  model call %d: input=%q\n", request.CallNumber, request.Input)
			result := v2.ModelResult{Model: "scripted-model", Usage: &v2.Usage{Input: 10, Output: 2}}
			switch request.CallNumber {
			case 1:
				result.Call = &v2.Call{Name: "echo", Arguments: `{"text":"original"}`}
			case 2:
				result.Call = &v2.Call{Name: "worker", Arguments: `{"job":"example"}`, External: true}
			default:
				result.Text, result.Stop = "All results accepted.", "end"
			}
			return result, nil
		},
		Transform: func(_ context.Context, call v2.Call) (v2.Call, error) {
			if call.Name == "echo" {
				call.Arguments = `{"text":"effective"}`
			}
			return call, nil
		},
		Authorize: func(_ context.Context, call v2.Call) error {
			fmt.Printf("  authorize %s %s\n", call.Name, call.Arguments)
			return nil
		},
		Tools: map[string]v2.Tool{"echo": func(_ context.Context, call v2.ToolInvocation) (v2.ToolResult, error) {
			fmt.Printf("  execute %s %s\n", call.Name, call.Arguments)
			return v2.ToolResult{State: v2.ToolSucceeded, Text: call.Arguments}, nil
		}},
	}
}

func start() v2.Command {
	return v2.Command{Start: &v2.Start{TurnID: "demo", Input: "Run local and external work", ModelCallLimit: 3}}
}

func resolution() v2.Command {
	return v2.Command{Resolution: &v2.Resolution{CommandID: "worker-result-1", EffectID: "demo/model/2/tool", Tool: &v2.ToolResult{State: v2.ToolSucceeded, Text: "worker finished"}}}
}

func report(out v2.Outcome, err error) v2.Outcome {
	if err != nil {
		panic(err)
	}
	fmt.Printf("  outcome=%s acknowledged_steps=%d\n", out.Reason, len(out.Records))
	if out.WriteError != nil {
		fmt.Printf("  write=%s unacknowledged_step=%d\n", out.WriteError.Disposition, out.Unacknowledged.Sequence)
	}
	return out
}

// loseAck simulates a transaction committing while its reply is lost.
type loseAck struct{ store v2.Store }

func (r loseAck) Commit(ctx context.Context, step v2.Step) error {
	if err := r.store.Commit(ctx, step); err != nil {
		return err
	}
	if step.Sequence == 3 {
		return errors.New("simulated lost acknowledgement")
	}
	return nil
}

func main() {
	ctx := context.Background()
	fmt.Println("Prototype 1: session runner, file snapshot, reopen and external resume")
	dir, err := os.MkdirTemp("", "dive-v2-demo-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	store, err := v2.NewFileJournal(dir)
	if err != nil {
		panic(err)
	}
	runner := &v2.SessionRunner{Store: store, Engine: engine()}
	report(runner.Run(ctx, "demo", start()))
	store, err = v2.NewFileJournal(dir)
	if err != nil {
		panic(err)
	}
	runner = &v2.SessionRunner{Store: store, Engine: engine()}
	report(runner.Run(ctx, "demo", v2.Command{}))
	report(runner.Run(ctx, "demo", resolution()))

	fmt.Println("Prototype 2: application transaction, lost acknowledgement, accounting")
	host := v2.NewTransactionalHost()
	host.Queue("demo", start().Start.Input)
	e := engine()
	report(e.Execute(ctx, nil, start(), loseAck{store: host}))
	loaded, err := host.Load(ctx, "demo")
	if err != nil {
		panic(err)
	}
	out := report(e.Execute(ctx, loaded, v2.Command{}, host))
	out = report(e.Execute(ctx, out.Records, resolution(), host))
	// A repeated acknowledgement retry must not duplicate metering.
	for _, step := range out.Records {
		if err := host.Commit(ctx, step); err != nil {
			panic(err)
		}
	}
	usage, unknown, inbox := host.Snapshot()
	fmt.Printf("  usage=%+v unknown_results=%d queued_inputs=%d\n", usage, unknown, len(inbox))
}
