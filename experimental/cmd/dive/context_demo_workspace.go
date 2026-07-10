package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

func workspaceContextDemoHook(workspaceDir string, runtime contextDemoRuntime) dive.PreIterationHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		reminder, err := dive.NewContextReminder("workspace-pulse", workspaceSnapshot(ctx, workspaceDir))
		if err != nil {
			return err
		}
		return runtime.appendChangedModelOnly(hctx, reminder)
	}
}

func workspaceSnapshot(ctx context.Context, workspaceDir string) string {
	workspaceDir = filepath.Clean(workspaceDir)
	if absolute, err := filepath.Abs(workspaceDir); err == nil {
		workspaceDir = absolute
	}
	lines := []string{
		"Live workspace snapshot observed by the CLI:",
		"- working directory: " + workspaceDir,
	}

	inside, err := gitOutput(ctx, workspaceDir, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return strings.Join(append(lines, "- git: not available for this workspace"), "\n")
	}

	branch, err := gitOutput(ctx, workspaceDir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch == "" {
		if commit, commitErr := gitOutput(ctx, workspaceDir, "rev-parse", "--short", "HEAD"); commitErr == nil {
			branch = "detached at " + commit
		} else {
			branch = "unknown"
		}
	}
	lines = append(lines, "- git branch: "+branch)

	status, err := gitOutput(ctx, workspaceDir, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return strings.Join(append(lines, "- git status: unavailable"), "\n")
	}
	changed := nonEmptyLines(status)
	if len(changed) == 0 {
		return strings.Join(append(lines, "- git status: clean"), "\n")
	}

	lines = append(lines, fmt.Sprintf("- git status: %d changed path%s", len(changed), pluralSuffix(len(changed))))
	const maxChangedPaths = 8
	for i, path := range changed {
		if i == maxChangedPaths {
			lines = append(lines, fmt.Sprintf("  - ... and %d more", len(changed)-maxChangedPaths))
			break
		}
		lines = append(lines, "  - "+path)
	}
	return strings.Join(lines, "\n")
}

func gitOutput(ctx context.Context, workspaceDir string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", workspaceDir}, args...)
	out, err := exec.CommandContext(ctx, "git", commandArgs...).Output()
	return strings.TrimSpace(string(out)), err
}

func nonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
