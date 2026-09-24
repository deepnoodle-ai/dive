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

const commandInspectionAllowJSON = `{
  "schema_version": 2,
  "status": "no_match",
  "minimum_action": "allow",
  "classification": {
    "matched": false,
    "explicitly_benign": false,
    "reason": "No built-in command safety extension matched."
  },
  "policy_evaluation": "not_run",
  "side_effects": "none"
}`

func testHookContext(command string) *dive.HookContext {
	input, _ := json.Marshal(map[string]string{"command": command})
	return &dive.HookContext{
		Call: &llm.ToolUseContent{
			Name:  "run_shell",
			Input: input,
		},
	}
}

func TestHOLGuardPreToolUseAllowsCommandInspectionAllow(t *testing.T) {
	var inspected string
	hook := holGuardPreToolUse(func(_ context.Context, command string) ([]byte, error) {
		inspected = command
		return []byte(commandInspectionAllowJSON), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if inspected != "go version" {
		t.Fatalf("expected exact command, got %q", inspected)
	}
}

func TestHOLGuardPreToolUseBlocksReview(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, string) ([]byte, error) {
		return []byte(`{"schema_version":2,"status":"review","minimum_action":"review","classification":{"reason":"Guard review required"}}`), nil
	})
	err := hook(context.Background(), testHookContext("go install example.com/tool@latest"))
	if err == nil || !strings.Contains(err.Error(), "Guard review required") {
		t.Fatalf("expected review to block, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksNativeUnavailable(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, string) ([]byte, error) {
		return []byte(`{"schema_version":2,"status":"native_unavailable","minimum_action":"review","classification":{"reason":"Native command inspection is unavailable."}}`), nil
	})
	err := hook(context.Background(), testHookContext("go version"))
	if err == nil || !strings.Contains(err.Error(), "Native command inspection is unavailable") {
		t.Fatalf("expected unavailable inspection to block, got %v", err)
	}
}

func TestHOLGuardPreToolUseBlocksWrongSchema(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, string) ([]byte, error) {
		return []byte(`{"schema_version":1,"status":"no_match","minimum_action":"allow"}`), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected unsupported schema to block")
	}
}

func TestHOLGuardPreToolUseBlocksRunnerFailure(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, string) ([]byte, error) {
		return nil, errors.New("binary missing")
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected runner failure to block")
	}
}

func TestHOLGuardPreToolUseBlocksMalformedResponse(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, string) ([]byte, error) {
		return []byte("not json"), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected malformed response to block")
	}
}

func TestHOLGuardPreToolUseBlocksAmbiguousMultipleObjects(t *testing.T) {
	hook := holGuardPreToolUse(func(context.Context, string) ([]byte, error) {
		return []byte(`{"schema_version":2,"status":"review","minimum_action":"review"}
{"schema_version":2,"status":"no_match","minimum_action":"allow"}`), nil
	})
	if err := hook(context.Background(), testHookContext("go version")); err == nil {
		t.Fatal("expected multiple JSON objects to block")
	}
}
