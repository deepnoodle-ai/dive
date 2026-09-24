package anthropic

import "github.com/deepnoodle-ai/dive/llm"

// dropUnpairedServerToolBlocks removes server tool calls that have no result
// in the same assistant message, and server tool results that have no call.
// Anthropic rejects either with a 400 ("`web_fetch` tool use with id ... was
// found without a corresponding `web_fetch_tool_result` block"). Dive's own
// responses keep calls and results paired, but a stored session can hold a
// call whose result an older Dive dropped while decoding a stream, and would
// otherwise fail on every later request.
//
// A call that is the very last block of the last message is kept: that is how
// a paused turn (stop reason pause_turn) ends, and sending it back is how the
// turn resumes. The messages are convertMessages' copies, so they are edited
// in place. Assistant messages left empty are removed.
func dropUnpairedServerToolBlocks(messages []*llm.Message) []*llm.Message {
	out := messages[:0]
	for i, message := range messages {
		if message.Role == llm.Assistant {
			message.Content = pairServerToolBlocks(message.Content, i == len(messages)-1)
			if len(message.Content) == 0 && message.Effort == "" {
				continue
			}
		}
		out = append(out, message)
	}
	return out
}

// pairServerToolBlocks returns content without its unpaired server tool
// calls and results (see dropUnpairedServerToolBlocks). When last is true, a
// call in the final position is kept.
func pairServerToolBlocks(content []llm.Content, last bool) []llm.Content {
	calls := map[string]bool{}
	results := map[string]bool{}
	for _, c := range content {
		if id, ok := serverToolCallID(c); ok {
			calls[id] = true
		} else if id, ok := serverToolResultID(c); ok {
			results[id] = true
		}
	}
	if len(calls) == 0 && len(results) == 0 {
		return content
	}
	kept := content[:0]
	for i, c := range content {
		if id, ok := serverToolCallID(c); ok && !results[id] && !(last && i == len(content)-1) {
			continue
		}
		if id, ok := serverToolResultID(c); ok && !calls[id] {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// serverToolCallID returns the ID of a server tool call.
func serverToolCallID(content llm.Content) (string, bool) {
	switch c := content.(type) {
	case *llm.ServerToolUseContent:
		return c.ID, true
	case *llm.MCPToolUseContent:
		return c.ID, true
	}
	return "", false
}

// serverToolResultID returns the ID of the call a server tool result answers.
func serverToolResultID(content llm.Content) (string, bool) {
	switch c := content.(type) {
	case *llm.ServerToolResultContent:
		return c.ToolUseID, true
	case *llm.WebSearchToolResultContent:
		return c.ToolUseID, true
	case *llm.CodeExecutionToolResultContent:
		return c.ToolUseID, true
	case *llm.BashCodeExecutionToolResultContent:
		return c.ToolUseID, true
	case *llm.TextEditorCodeExecutionToolResultContent:
		return c.ToolUseID, true
	case *llm.MCPToolResultContent:
		return c.ToolUseID, true
	}
	return "", false
}
