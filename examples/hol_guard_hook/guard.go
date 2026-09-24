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

const (
	guardTimeout              = 9 * time.Second
	guardCommandSchemaVersion = 2
)

type guardRunner func(context.Context, string) ([]byte, error)

type guardResponse struct {
	SchemaVersion  int    `json:"schema_version"`
	Status         string `json:"status"`
	MinimumAction  string `json:"minimum_action"`
	Classification struct {
		Reason string `json:"reason"`
	} `json:"classification"`
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

		out, err := run(ctx, command)
		if err != nil {
			return fmt.Errorf("HOL Guard: command inspection failed: %w", err)
		}
		response, err := parseGuardResponse(out)
		if err != nil {
			return fmt.Errorf("HOL Guard: %w", err)
		}
		if allowedGuardResponse(response) {
			return nil
		}

		reason := strings.TrimSpace(response.Classification.Reason)
		if reason == "" && response.MinimumAction != "" {
			reason = fmt.Sprintf("command requires Guard action %q", response.MinimumAction)
		}
		if reason == "" && response.Status != "" {
			reason = fmt.Sprintf("command inspection returned status %q", response.Status)
		}
		if reason == "" {
			reason = "command did not receive a Guard allow decision"
		}
		return fmt.Errorf("HOL Guard: %s", reason)
	}
}

func runHOLGuard(ctx context.Context, command string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hol-guard", "command", "test", "--json", command)
	out, err := cmd.Output()
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
	payload := bytes.TrimSpace(out)
	if len(payload) == 0 {
		return guardResponse{}, errors.New("empty JSON response")
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return guardResponse{}, fmt.Errorf("invalid JSON response: %w", err)
	}
	if response.SchemaVersion != guardCommandSchemaVersion {
		return guardResponse{}, fmt.Errorf("unsupported command inspection schema %d", response.SchemaVersion)
	}
	if response.Status == "" || response.MinimumAction == "" {
		return guardResponse{}, errors.New("incomplete command inspection response")
	}
	return response, nil
}

func allowedGuardResponse(response guardResponse) bool {
	return response.SchemaVersion == guardCommandSchemaVersion &&
		response.Status == "no_match" &&
		response.MinimumAction == "allow"
}
