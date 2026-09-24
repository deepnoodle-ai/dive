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

func testHookContext(command string) *dive.HookContext {
	input, _ := json.Marshal(map[string]string{"command": command})
	return &dive.HookContext{
		Call: &llm.ToolUseContent{
			Name:  "run_shell",
			Input: input,
		},
	}
}

func TestHOLGuardPreToolUseAllowsAuthoritativeAllow(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"policy_action":"allow","reason_code":"native.explicit-benign","hookSpecificOutput":{"permissionDecision":"allow"}}`), nil
	})
	if err := hook(context.Background(), testHookContext("go test ./...")); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
}

func TestHOLGuardPreToolUseAllowsNativePolicyWarning(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"policy_action":"warn","reason_code":"native_policy_warning","hookSpecificOutput":{"permissionDecision":"allow"}}`), nil
	})
	if err := hook(context.Background(), testHookContext("go test ./...")); err != nil {
		t.Fatalf("expected allow-with-warning, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksUnavailableAuthority(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"policy_action":"warn","reason_code":"native_pre_tool_unavailable","reason":"native review unavailable","hookSpecificOutput":{"permissionDecision":"allow"}}`), nil
	})
	err := hook(context.Background(), testHookContext("go test ./..."))
	if err == nil || !strings.Contains(err.Error(), "native review unavailable") {
		t.Fatalf("expected fail-closed unavailable result, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksDeny(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"policy_action":"block","reason_code":"native_policy_block","reason":"blocked by policy","hookSpecificOutput":{"permissionDecision":"deny"}}`), nil
	})
	err := hook(context.Background(), testHookContext("rm -rf /tmp/demo"))
	if err == nil || !strings.Contains(err.Error(), "blocked by policy") {
		t.Fatalf("expected policy block, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksRunnerFailure(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return nil, errors.New("binary missing")
	})
	if err := hook(context.Background(), testHookContext("go test ./...")); err == nil {
		t.Fatal("expected runner failure to block")
	}
}

func TestHOLGuardPreToolUseBlocksMalformedResponse(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, []byte) ([]byte, error) {
		return []byte("not json"), nil
	})
	if err := hook(context.Background(), testHookContext("go test ./...")); err == nil {
		t.Fatal("expected malformed response to block")
	}
}
