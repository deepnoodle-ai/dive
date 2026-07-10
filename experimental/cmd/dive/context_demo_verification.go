package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
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
	observation, ok := classifyQualityGateCommand(command)
	return ok && (observation.Kind == qualityGateTest || observation.Kind == qualityGateAnalysis)
}
