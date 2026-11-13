package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

const guardHookAllowJSON = `{"continue":true,"policy_action":"allow","reason_code":"native_policy_allow","hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`

func testHookContext(command string) *dive.HookContext {
	input, _ := json.Marshal(map[string]string{"command": command})
	return &dive.HookContext{
		Call: &llm.ToolUseContent{
			Name:  "run_shell",
			Input: input,
		},
	}
}

func TestHOLGuardPreToolUseAllowsExplicitAuthoritativeAllow(t *testing.T) {
	var received map[string]any
	hook := holGuardPreToolUse(func(_ context.Context, payload []byte) ([]byte, error) {
		if err := json.Unmarshal(payload, &received); err != nil {
			t.Fatalf("invalid hook payload: %v", err)
		}
		return []byte(guardHookAllowJSON), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if received["hook_event_name"] != "PreToolUse" || received["tool_name"] != "Bash" {
		t.Fatalf("unexpected hook envelope: %#v", received)
	}
	toolInput, ok := received["tool_input"].(map[string]any)
	if !ok || toolInput["command"] != "go version" {
		t.Fatalf("expected exact command in private hook payload, got %#v", received["tool_input"])
	}
}

func TestHOLGuardPreToolUseBlocksReview(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"continue":true,"policy_action":"review","reason_code":"native_pre_tool_review","hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"Guard review required"}}`), nil
	})
	err := hook(context.Background(), testHookContext("go install example.com/tool@latest"))
	if err == nil || !strings.Contains(err.Error(), "Guard review required") {
		t.Fatalf("expected review to block, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksDeny(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"continue":false,"policy_action":"block","reason_code":"policy_block","hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Policy denied the command"}}`), nil
	})
	err := hook(context.Background(), testHookContext("rm -rf /tmp/example"))
	if err == nil || !strings.Contains(err.Error(), "Policy denied the command") {
		t.Fatalf("expected deny to block, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksUnavailableAllowShapedResponse(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"continue":true,"policy_action":"warn","reason_code":"native_hook_worker_exception","reason":"Native review was unavailable.","hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"Native review was unavailable."}}`), nil
	})
	err := hook(context.Background(), testHookContext("go version"))
	if err == nil || !strings.Contains(err.Error(), "Native review was unavailable") {
		t.Fatalf("expected unavailable allow-shaped response to block, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksCommandInspectionResult(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"schema_version":2,"status":"no_match","minimum_action":"allow","policy_evaluation":"not_run"}`), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected side-effect-free command inspection output to be insufficient for authorization")
	}
}

func TestHOLGuardPreToolUseBlocksWrongEvent(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"policy_action":"allow","reason_code":"native_policy_allow","hookSpecificOutput":{"hookEventName":"PostToolUse","permissionDecision":"allow"}}`), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected wrong hook event to block")
	}
}

func TestHOLGuardPreToolUseBlocksMissingAuthorityFields(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected missing policy authority fields to block")
	}
}

func TestHOLGuardPreToolUseBlocksRunnerFailure(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return nil, errors.New("binary missing")
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected runner failure to block")
	}
}

func TestHOLGuardPreToolUseBlocksMalformedResponse(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte("not json"), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected malformed response to block")
	}
}
