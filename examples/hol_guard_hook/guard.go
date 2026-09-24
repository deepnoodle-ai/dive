package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
)

const guardTimeout = 9 * time.Second

type guardRunner func(context.Context, []byte) ([]byte, error)

type guardHookResponse struct {
	SystemMessage      string `json:"systemMessage"`
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

func holGuardPreToolUse(run guardRunner) dive.PreToolUseHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		if hctx == nil || hctx.Call == nil {
			return errors.New("HOL Guard: missing Dive tool call")
		}

		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(hctx.Call.Input, &input); err != nil {
			return fmt.Errorf("HOL Guard: invalid run_shell input: %w", err)
		}
		command := strings.TrimSpace(input.Command)
		if command == "" {
			return errors.New("HOL Guard: run_shell command is empty")
		}

		payload, err := guardPreToolPayload(command)
		if err != nil {
			return fmt.Errorf("HOL Guard: could not build hook payload: %w", err)
		}
		out, err := run(ctx, payload)
		if err != nil {
			return fmt.Errorf("HOL Guard: policy check failed: %w", err)
		}
		response, err := parseGuardHookResponse(out)
		if err != nil {
			return fmt.Errorf("HOL Guard: %w", err)
		}
		if response.HookSpecificOutput.PermissionDecision == "allow" {
			return nil
		}

		reason := strings.TrimSpace(response.HookSpecificOutput.PermissionDecisionReason)
		if reason == "" {
			reason = strings.TrimSpace(response.SystemMessage)
		}
		if reason == "" {
			reason = fmt.Sprintf("policy decision was %q", response.HookSpecificOutput.PermissionDecision)
		}
		return fmt.Errorf("HOL Guard: %s", reason)
	}
}

func guardPreToolPayload(command string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input": map[string]string{
			"command": command,
		},
	})
}

func runHOLGuard(ctx context.Context, payload []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hol-guard", "hook", "--harness", "claude-code", "--json")
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func parseGuardHookResponse(out []byte) (guardHookResponse, error) {
	var response guardHookResponse
	payload := bytes.TrimSpace(out)
	if len(payload) == 0 {
		return guardHookResponse{}, errors.New("empty hook response")
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return guardHookResponse{}, fmt.Errorf("invalid hook JSON: %w", err)
	}
	if response.HookSpecificOutput.HookEventName != "PreToolUse" {
		return guardHookResponse{}, errors.New("unexpected hook response event")
	}
	if strings.TrimSpace(response.HookSpecificOutput.PermissionDecision) == "" {
		return guardHookResponse{}, errors.New("hook response has no permission decision")
	}
	return response, nil
}
