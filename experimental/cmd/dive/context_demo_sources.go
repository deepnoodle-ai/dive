package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

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

func sourceLedgerReminderHook(runtime contextDemoRuntime) dive.PreIterationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil {
			return nil
		}
		ledger := state.sourceSnapshot()
		if len(ledger.sources) == 0 {
			return nil
		}
		var content strings.Builder
		content.WriteString("Evidence consulted during this response (tool access does not establish truth):")
		for _, source := range ledger.sources {
			content.WriteString("\n- ")
			content.WriteString(source)
		}
		if ledger.omitted > 0 {
			fmt.Fprintf(&content, "\n- ... %d additional source observation%s omitted", ledger.omitted, pluralSuffix(ledger.omitted))
		}
		content.WriteString("\nDistinguish inspected evidence from assumptions, and re-check primary sources when a claim depends on freshness or authority.")
		reminder, err := dive.NewContextReminder("evidence-ledger", content.String())
		if err != nil {
			return err
		}
		return runtime.pin(hctx, reminder)
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
