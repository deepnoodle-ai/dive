package main

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

func recoveryContextDemoHook(runtime contextDemoRuntime) dive.PostToolUseFailureHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		reminder, err := dive.NewOperatorReminder(
			"recovery-coach",
			"Tool failure observed: "+toolInvocationSummary(hctx.Call)+". Use the error details in the tool result and change at least one relevant variable—input, path, permissions, or approach—before retrying. Do not repeat the identical call blindly.",
		)
		if err != nil {
			return err
		}
		return runtime.appendModelOnly(hctx, reminder)
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
