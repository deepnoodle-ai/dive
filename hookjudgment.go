package dive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

// Judgment-based hook helpers wrap a single LLM call inside an ordinary hook,
// letting a model make a decision that is hard to express as deterministic
// code: "is the task actually done?", "is this tool call safe in context?".
// They are constructors in the same spirit as InjectContext and MatchTool —
// the core hook types stay plain Go functions. See docs/design/hooks.md
// ("Judgment-Based Hooks").

// JudgmentDecision is the structured verdict a model returns from a judgment
// hook. OK reports whether the action may proceed; Reason explains a block and
// is surfaced back to the agent.
type JudgmentDecision struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

// judgmentToolName is the name of the synthetic tool the model is forced to
// call so its decision arrives as structured arguments rather than free text.
const judgmentToolName = "submit_decision"

var judgmentDecisionSchema = schema.NewSchema(
	map[string]*schema.Property{
		"ok":     schema.BooleanProp("true if the action may proceed; false to block it"),
		"reason": schema.StringProp("brief justification for the verdict; required when ok is false"),
	},
	"ok", "reason",
)

// askJudgment makes one LLM call that forces the model to answer via the
// submit_decision tool, guaranteeing structured output across providers. The
// prompt frames the question (used as the system prompt) and evidence supplies
// the material to judge as a single user message — deliberately rendered text
// rather than a replay of the raw conversation, so the judge call stays cheap
// and never trips provider validation of historical tool blocks.
func askJudgment(ctx context.Context, model llm.LLM, prompt, evidence string) (*JudgmentDecision, error) {
	tool := llm.NewToolDefinition().
		WithName(judgmentToolName).
		WithDescription("Record your verdict on whether the action may proceed.").
		WithSchema(judgmentDecisionSchema)

	resp, err := model.Generate(ctx,
		llm.WithSystemPrompt(prompt),
		llm.WithMessages(llm.NewUserTextMessage(evidence)),
		llm.WithTools(tool),
		llm.WithToolChoice(&llm.ToolChoice{Type: llm.ToolChoiceTypeTool, Name: judgmentToolName}),
	)
	if err != nil {
		return nil, err
	}
	for _, call := range resp.ToolCalls() {
		if call.Name != judgmentToolName {
			continue
		}
		var d JudgmentDecision
		if err := json.Unmarshal(call.Input, &d); err != nil {
			return nil, fmt.Errorf("judgment hook: decode decision: %w", err)
		}
		return &d, nil
	}
	return nil, fmt.Errorf("judgment hook: model returned no %s decision", judgmentToolName)
}

// PromptStopHook returns a StopHook that asks model whether the agent's work so
// far satisfies the task described by prompt. If the model answers ok=false,
// the agent keeps working with the model's reason as its next instruction.
//
// It fails open: on a model or transport error the error is returned (Dive logs
// it and lets the agent stop), so a flaky judge never traps a turn. It honors
// hctx.StopHookActive — after forcing one continuation it steps aside, so the
// agent cannot loop forever on the judge's say-so.
//
// Pass a cheap model (e.g. a small/fast one); this adds one call per stop.
func PromptStopHook(model llm.LLM, prompt string) StopHook {
	return func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
		if hctx.StopHookActive {
			return nil, nil
		}
		evidence := "The agent produced the following output this turn:\n\n" +
			messagesText(hctx.OutputMessages)
		d, err := askJudgment(ctx, model, prompt, evidence)
		if err != nil {
			return nil, err // fail open: logged by the agent, stop is allowed
		}
		if d.OK {
			return nil, nil
		}
		return &StopDecision{Continue: true, Reason: d.Reason}, nil
	}
}

// PromptToolGate returns a PreToolUseHook that asks model whether the pending
// tool call is acceptable per prompt. If the model answers ok=false the call is
// denied (a returned error denies a tool in Dive) and the reason is sent back
// to the agent.
//
// It fails closed: a model or transport error denies the call, because a safety
// gate that silently allows on error is worse than an occasional false deny.
// Pair it with MatchTool to scope the gate to specific tools, and pass a cheap
// model — this adds one call per matched tool invocation.
func PromptToolGate(model llm.LLM, prompt string) PreToolUseHook {
	return func(ctx context.Context, hctx *HookContext) error {
		evidence := fmt.Sprintf("The agent wants to call tool %q with input:\n%s",
			hctx.Call.Name, string(hctx.Call.Input))
		d, err := askJudgment(ctx, model, prompt, evidence)
		if err != nil {
			return fmt.Errorf("tool gate: judgment failed, denying %q: %w", hctx.Call.Name, err)
		}
		if d.OK {
			return nil
		}
		return fmt.Errorf("blocked by PromptToolGate: %s", d.Reason)
	}
}

// messagesText concatenates the text of a message slice, skipping non-text
// content (tool calls, etc.). Used to render evidence for a judgment call.
func messagesText(msgs []*llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if t := m.Text(); t != "" {
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
