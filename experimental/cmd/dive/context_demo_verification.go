package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/permission"
)

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
		if len(update.checkedPaths) > 0 || update.checkedOmitted > 0 {
			content := fmt.Sprintf(
				"Verification checkpoint observed: %q completed successfully after changes to %s. This is evidence that the command passed, not proof of complete coverage; inspect its scope before declaring completion.",
				update.checkCommand,
				formatTrackedItems(update.checkedPaths, update.checkedOmitted),
			)
			reminder, err := dive.NewOperatorReminder("verification-checkpoint", content)
			if err != nil {
				return err
			}
			if err := hctx.AppendReminder(reminder, dive.ModelOnly); err != nil {
				return err
			}
		}
		if update.emitDebt && (len(update.unverified) > 0 || update.unverifiedOmitted > 0) {
			content := "Unverified changes observed during this response:\n- " + strings.Join(update.unverified, "\n- ")
			if update.unverifiedOmitted > 0 {
				content += fmt.Sprintf("\n- ... %d additional changed-path observation%s omitted", update.unverifiedOmitted, pluralSuffix(update.unverifiedOmitted))
			}
			content +=
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

func formatTrackedItems(items []string, omitted int) string {
	tracked := strings.Join(items, ", ")
	if omitted == 0 {
		return tracked
	}
	if tracked == "" {
		return fmt.Sprintf("%d omitted path observation%s", omitted, pluralSuffix(omitted))
	}
	return fmt.Sprintf("%s, and %d omitted path observation%s", tracked, omitted, pluralSuffix(omitted))
}

func isVerificationCommand(command string) bool {
	segments, hasSubstitution := permission.SplitCommand(strings.ToLower(command))
	if hasSubstitution || len(segments) == 0 {
		return false
	}
	// The shell reports the final segment's status. Requiring that segment to
	// be the verifier avoids treating "go test || true" or
	// "go test; echo done" as evidence that the test itself passed.
	fields := strings.Fields(segments[len(segments)-1])
	for len(fields) > 0 && isShellAssignment(fields[0]) {
		fields = fields[1:]
	}
	return isDirectVerificationInvocation(fields)
}

func isDirectVerificationInvocation(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	executable := filepath.Base(fields[0])
	arg := func(index int) string {
		if index >= len(fields) {
			return ""
		}
		return strings.Trim(fields[index], `"'`)
	}
	switch executable {
	case "go":
		return arg(1) == "test" || arg(1) == "vet"
	case "pytest", "mypy", "tsc", "golangci-lint":
		return true
	case "python", "python3":
		return arg(1) == "-m" && arg(2) == "pytest"
	case "cargo":
		return arg(1) == "test" || arg(1) == "clippy"
	case "swift":
		return arg(1) == "test"
	case "dotnet", "mvn", "gradle", "gradlew":
		return arg(1) == "test"
	case "ruff":
		return arg(1) == "check"
	case "make":
		return isVerificationTarget(arg(1))
	case "npm", "pnpm", "yarn":
		if isVerificationTarget(arg(1)) {
			return true
		}
		return arg(1) == "run" && isVerificationTarget(arg(2))
	case "xcodebuild":
		return arg(1) == "test" || arg(1) == "test-without-building"
	default:
		return false
	}
}

func isVerificationTarget(target string) bool {
	for _, name := range []string{"test", "check", "lint"} {
		if target == name || strings.HasPrefix(target, name+":") || strings.HasPrefix(target, name+"-") ||
			strings.HasSuffix(target, ":"+name) || strings.HasSuffix(target, "-"+name) {
			return true
		}
	}
	return false
}

func isShellAssignment(value string) bool {
	name, _, ok := strings.Cut(value, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}
