package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/permission"
)

const contextDemoStateKey = "dive-cli-context-demo-state"

var contextDemoNames = []string{"workspace", "sources", "verification", "recovery"}

type contextDemoSelection struct {
	workspace    bool
	sources      bool
	verification bool
	recovery     bool
}

func (s contextDemoSelection) empty() bool {
	return !s.workspace && !s.sources && !s.verification && !s.recovery
}

// parseContextDemoNames accepts repeatable values and comma-separated groups so
// both --context-demo workspace --context-demo sources and
// --context-demo workspace,sources are convenient at the shell.
func parseContextDemoNames(specs []string) (contextDemoSelection, error) {
	var selection contextDemoSelection
	for _, spec := range specs {
		for _, rawName := range strings.Split(spec, ",") {
			name := strings.ToLower(strings.TrimSpace(rawName))
			switch name {
			case "all":
				selection.workspace = true
				selection.sources = true
				selection.verification = true
				selection.recovery = true
			case "workspace":
				selection.workspace = true
			case "sources":
				selection.sources = true
			case "verification":
				selection.verification = true
			case "recovery":
				selection.recovery = true
			case "":
				return contextDemoSelection{}, fmt.Errorf("context demo name cannot be empty")
			default:
				return contextDemoSelection{}, fmt.Errorf(
					"unknown context demo %q: expected one of all, %s",
					name,
					strings.Join(contextDemoNames, ", "),
				)
			}
		}
	}
	return selection, nil
}

func applyContextDemoAgentOptions(agentOpts *dive.AgentOptions, workspaceDir string, specs []string) error {
	selection, err := parseContextDemoNames(specs)
	if err != nil {
		return err
	}
	if selection.empty() {
		return nil
	}

	// Install turn-local state before the first iteration. Tool hooks can run in
	// parallel, so the state object protects its own collections.
	agentOpts.Hooks.PreGeneration = append(agentOpts.Hooks.PreGeneration, func(_ context.Context, hctx *dive.HookContext) error {
		hctx.Values[contextDemoStateKey] = &contextDemoTurnState{}
		return nil
	})

	if selection.workspace {
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, workspaceContextDemoHook(workspaceDir))
	}
	if selection.sources {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, sourceLedgerCollectorHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, sourceLedgerReminderHook())
	}
	if selection.verification {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, verificationCollectorHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, verificationReminderHook())
	}
	if selection.recovery {
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, recoveryContextDemoHook())
	}
	return nil
}

type contextDemoTurnState struct {
	mu sync.Mutex

	sources    []string
	sourceSeen map[string]bool

	unverified   []string
	batchChanges []string
	batchChecks  []string
}

func contextDemoState(hctx *dive.HookContext) *contextDemoTurnState {
	if hctx == nil || hctx.Values == nil {
		return nil
	}
	state, _ := hctx.Values[contextDemoStateKey].(*contextDemoTurnState)
	return state
}

func (s *contextDemoTurnState) addSource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sourceSeen == nil {
		s.sourceSeen = make(map[string]bool)
	}
	if s.sourceSeen[source] {
		return
	}
	s.sourceSeen[source] = true
	if len(s.sources) < 12 {
		s.sources = append(s.sources, source)
	}
}

func (s *contextDemoTurnState) sourceSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sources...)
}

func (s *contextDemoTurnState) addBatchChange(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stringSliceContains(s.batchChanges, path) {
		return
	}
	s.batchChanges = append(s.batchChanges, path)
}

func (s *contextDemoTurnState) addBatchCheck(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchChecks = append(s.batchChecks, command)
}

type verificationUpdate struct {
	checkedPaths []string
	checkCommand string
	unverified   []string
	emitDebt     bool
}

// applyVerificationBatch treats a check as evidence only for debt that existed
// before its tool batch. An edit and test launched in parallel do not prove that
// the test covered the edit, regardless of which tool happens to finish first.
func (s *contextDemoTurnState) applyVerificationBatch() verificationUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()

	var update verificationUpdate
	if len(s.batchChecks) > 0 && len(s.unverified) > 0 {
		update.checkedPaths = append([]string(nil), s.unverified...)
		update.checkCommand = s.batchChecks[len(s.batchChecks)-1]
		s.unverified = nil
	}
	for _, path := range s.batchChanges {
		if !stringSliceContains(s.unverified, path) {
			s.unverified = append(s.unverified, path)
		}
	}
	update.emitDebt = len(s.batchChanges) > 0
	update.unverified = append([]string(nil), s.unverified...)
	s.batchChanges = nil
	s.batchChecks = nil
	return update
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func workspaceContextDemoHook(workspaceDir string) dive.PreIterationHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		reminder, err := dive.NewContextReminder("workspace-pulse", workspaceSnapshot(ctx, workspaceDir))
		if err != nil {
			return err
		}
		return hctx.PinReminder(reminder)
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

	lines = append(lines, fmt.Sprintf("- git status: %d changed path(s)", len(changed)))
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

func sourceLedgerCollectorHook() dive.PostToolUseHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil {
			return nil
		}
		if source, ok := toolSourceSummary(hctx.Call); ok {
			state.addSource(source)
		}
		return nil
	}
}

func sourceLedgerReminderHook() dive.PreIterationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil {
			return nil
		}
		sources := state.sourceSnapshot()
		if len(sources) == 0 {
			return nil
		}
		var content strings.Builder
		content.WriteString("Evidence consulted during this response (tool access does not establish truth):")
		for _, source := range sources {
			content.WriteString("\n- ")
			content.WriteString(source)
		}
		content.WriteString("\nDistinguish inspected evidence from assumptions, and re-check primary sources when a claim depends on freshness or authority.")
		reminder, err := dive.NewContextReminder("evidence-ledger", content.String())
		if err != nil {
			return err
		}
		return hctx.PinReminder(reminder)
	}
}

func toolSourceSummary(call *llm.ToolUseContent) (string, bool) {
	if call == nil {
		return "", false
	}
	input := toolInput(call)
	path := firstString(input, "file_path", "path")
	pattern := firstString(input, "pattern")
	query := firstString(input, "query")
	url := firstString(input, "url")

	switch strings.ToLower(call.Name) {
	case "read":
		return prefixedValue("file", path)
	case "grep":
		return searchSummary("text search", pattern, path)
	case "glob":
		return searchSummary("file glob", pattern, path)
	case "listdirectory":
		return prefixedValue("directory", path)
	case "webfetch", "fetch":
		return prefixedValue("web page", url)
	case "websearch", "search":
		return prefixedValue("web search", query)
	default:
		return "", false
	}
}

func prefixedValue(prefix, value string) (string, bool) {
	if value == "" {
		return "", false
	}
	return prefix + ": " + truncateText(value, 180), true
}

func searchSummary(prefix, pattern, path string) (string, bool) {
	if pattern == "" && path == "" {
		return "", false
	}
	if path == "" {
		path = "."
	}
	return fmt.Sprintf("%s: %q in %s", prefix, truncateText(pattern, 100), truncateText(path, 100)), true
}

func verificationCollectorHook() dive.PostToolUseHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil || hctx.Call == nil {
			return nil
		}
		input := toolInput(hctx.Call)
		switch hctx.Call.Name {
		case "Write", "Edit":
			path := firstString(input, "file_path", "path")
			if path == "" {
				path = "unknown path"
			}
			state.addBatchChange(truncateText(path, 180))
		case "Bash":
			command := firstString(input, "command")
			if isVerificationCommand(command) {
				state.addBatchCheck(truncateText(command, 240))
			}
		}
		return nil
	}
}

func verificationReminderHook() dive.PreIterationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil {
			return nil
		}
		update := state.applyVerificationBatch()
		if len(update.checkedPaths) > 0 {
			content := fmt.Sprintf(
				"Verification checkpoint observed: %q completed successfully after changes to %s. This is evidence that the command passed, not proof of complete coverage; inspect its scope before declaring completion.",
				update.checkCommand,
				strings.Join(update.checkedPaths, ", "),
			)
			reminder, err := dive.NewOperatorReminder("verification-checkpoint", content)
			if err != nil {
				return err
			}
			if err := hctx.AppendReminder(reminder, dive.ModelOnly); err != nil {
				return err
			}
		}
		if update.emitDebt && len(update.unverified) > 0 {
			content := "Unverified changes observed during this response:\n- " + strings.Join(update.unverified, "\n- ") +
				"\nBefore declaring completion, run checks relevant to these changes. A check launched in the same parallel tool batch does not verify the edit."
			reminder, err := dive.NewOperatorReminder("verification-debt", content)
			if err != nil {
				return err
			}
			if err := hctx.AppendReminder(reminder, dive.ModelOnly); err != nil {
				return err
			}
		}
		return nil
	}
}

func isVerificationCommand(command string) bool {
	segments, _ := permission.SplitCommand(strings.ToLower(command))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		for _, prefix := range []string{
			"go test", "go vet", "pytest", "python -m pytest", "python3 -m pytest",
			"cargo test", "cargo clippy", "swift test", "dotnet test", "mvn test",
			"gradle test", "./gradlew test", "golangci-lint", "ruff check", "mypy", "tsc",
			"npm test", "npm run test", "npm run lint", "npm run check",
			"pnpm test", "pnpm run test", "pnpm lint", "pnpm check",
			"yarn test", "yarn lint", "yarn check",
		} {
			if commandHasPrefix(segment, prefix) {
				return true
			}
		}
		fields := strings.Fields(segment)
		if len(fields) >= 2 && fields[0] == "make" {
			target := fields[1]
			if strings.HasPrefix(target, "test") || strings.HasPrefix(target, "check") || strings.HasPrefix(target, "lint") {
				return true
			}
		}
		if len(fields) >= 2 && fields[0] == "xcodebuild" && stringSliceContains(fields[1:], "test") {
			return true
		}
	}
	return false
}

func commandHasPrefix(command, prefix string) bool {
	return command == prefix || strings.HasPrefix(command, prefix+" ") || strings.HasPrefix(command, prefix+":")
}

func recoveryContextDemoHook() dive.PostToolUseFailureHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		reminder, err := dive.NewOperatorReminder(
			"recovery-coach",
			"Tool failure observed: "+toolInvocationSummary(hctx.Call)+". Use the error details in the tool result and change at least one relevant variable—input, path, permissions, or approach—before retrying. Do not repeat the identical call blindly.",
		)
		if err != nil {
			return err
		}
		return hctx.AppendReminder(reminder, dive.ModelOnly)
	}
}

func toolInvocationSummary(call *llm.ToolUseContent) string {
	if call == nil {
		return "unknown tool call"
	}
	input := toolInput(call)
	for _, key := range []string{"file_path", "path", "command", "query", "url", "pattern"} {
		if value := firstString(input, key); value != "" {
			return fmt.Sprintf("%s (%s=%q)", call.Name, key, truncateText(value, 160))
		}
	}
	return call.Name
}

func toolInput(call *llm.ToolUseContent) map[string]any {
	if call == nil || len(call.Input) == 0 {
		return nil
	}
	var input map[string]any
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return nil
	}
	return input
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func truncateText(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes-1]) + "…"
}
