package main

import (
	"bufio"
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

var nonAuthoritativeReasonCodes = map[string]struct{}{
	"native_hook_disabled":                                {},
	"native_shadow_diagnostic_disabled":                   {},
	"native_policy_not_ready":                             {},
	"native_hook_event_unavailable":                       {},
	"native_pre_tool_unavailable":                         {},
	"native_post_tool_unavailable":                        {},
	"native_overloaded":                                   {},
	"native_hook_worker_unavailable":                      {},
	"native_hook_worker_unavailable_before_compatibility": {},
	"native_hook_worker_unsupported":                      {},
	"native_hook_worker_exception":                        {},
	"native_hook_compatibility_disabled":                  {},
	"native_hook_edge_invalid_response":                   {},
	"native_hook_edge_unavailable":                        {},
	"python_hook_oracle_unavailable":                      {},
	"python_oracle_exception":                             {},
	"watch_recording_only":                                {},
	"daemon_hook_queue_capacity":                          {},
	"daemon_hook_deadline_exhausted":                      {},
	"daemon_hook_process_deadline_exhausted":              {},
	"daemon_hook_process_not_ready":                       {},
	"daemon_hook_process_failed":                          {},
	"daemon_hook_process_invalid_request":                 {},
	"daemon_hook_process_guard_home_mismatch":             {},
	"daemon_worker_exception":                             {},
	"harness_not_managed":                                 {},
	"native_degraded_emergency_safe":                      {},
}

type guardRunner func(context.Context, []byte) ([]byte, error)

type guardResponse struct {
	PolicyAction string `json:"policy_action"`
	ReasonCode   string `json:"reason_code"`
	Reason       string `json:"reason"`
	HookSpecificOutput struct {
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
		if strings.TrimSpace(input.Command) == "" {
			return errors.New("HOL Guard: run_shell command is empty")
		}

		payload, err := json.Marshal(map[string]any{
			"hook_event_name": "PreToolUse",
			"tool_name":       "Bash",
			"tool_input": map[string]string{
				"command": input.Command,
			},
		})
		if err != nil {
			return fmt.Errorf("HOL Guard: encode request: %w", err)
		}

		out, err := run(ctx, payload)
		if err != nil {
			return fmt.Errorf("HOL Guard: command review failed: %w", err)
		}
		response, err := parseGuardResponse(out)
		if err != nil {
			return fmt.Errorf("HOL Guard: %w", err)
		}
		if allowedGuardResponse(response) {
			return nil
		}

		reason := strings.TrimSpace(response.Reason)
		if reason == "" {
			reason = strings.TrimSpace(response.HookSpecificOutput.PermissionDecisionReason)
		}
		if reason == "" && response.ReasonCode != "" {
			reason = fmt.Sprintf("command blocked (%s)", response.ReasonCode)
		}
		if reason == "" {
			reason = "command did not receive an authoritative allow decision"
		}
		return fmt.Errorf("HOL Guard: %s", reason)
	}
}

func runHOLGuard(ctx context.Context, payload []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hol-guard", "hook", "--harness", "dive", "--json")
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func parseGuardResponse(out []byte) (guardResponse, error) {
	var response guardResponse
	if err := json.Unmarshal(bytes.TrimSpace(out), &response); err == nil {
		return response, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	var last []byte
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) > 0 {
			last = append(last[:0], line...)
		}
	}
	if err := scanner.Err(); err != nil {
		return guardResponse{}, err
	}
	if len(last) == 0 || json.Unmarshal(last, &response) != nil {
		return guardResponse{}, errors.New("invalid JSON response")
	}
	return response, nil
}

func allowedGuardResponse(response guardResponse) bool {
	if response.HookSpecificOutput.PermissionDecision != "allow" || response.ReasonCode == "" {
		return false
	}
	if _, unavailable := nonAuthoritativeReasonCodes[response.ReasonCode]; unavailable {
		return false
	}
	if response.PolicyAction == "allow" {
		return true
	}
	return response.PolicyAction == "warn" && response.ReasonCode == "native_policy_warning"
}
