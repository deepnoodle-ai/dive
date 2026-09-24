package llm

// ToolCallNotRunText is the result recorded for a tool call that never
// started because its turn ended first. The call had no effect.
const ToolCallNotRunText = "Not run: the turn ended before this call started. It had no effect."

// ToolCallUnknownText is the result recorded for a tool call that was
// running when its turn ended and whose result was not recorded. Whether it
// took effect is unknown.
const ToolCallUnknownText = "Unknown result: the turn ended while this call was running. Its result was not recorded; it may have taken effect, so check before repeating it."

// AnswerUnansweredToolCalls returns messages in which every client tool call
// (ToolUseContent) is answered by a tool result in the message after it. A
// missing result is added as an error result with ToolCallUnknownText: the
// history alone cannot say whether the call ran, and "unknown" is the claim
// that is safe either way. When the next message is a tool-result message (a
// user message with results for some of the calls), the missing results join
// it, after the results already there and before any other content.
// Otherwise they go into a new tool-result message right after the assistant
// message, so a user message that holds no results, such as a reminder that
// an encoder may render in a system or developer role, is left as it is.
//
// Providers reject a history with an unanswered call, so encoders apply this
// to the request as a backstop against a history saved by an older version,
// by an application with a bug, or by a process that died mid-turn. Server
// tool calls are left alone: a paused turn legitimately ends in one.
//
// The caller's messages are not modified. messages is returned as is when
// nothing is missing.
func AnswerUnansweredToolCalls(messages []*Message) []*Message {
	var out []*Message
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		calls := clientToolCalls(msg)
		if len(calls) == 0 {
			if out != nil {
				out = append(out, msg)
			}
			continue
		}

		var next *Message
		if i+1 < len(messages) && hasToolResults(messages[i+1]) {
			next = messages[i+1]
		}
		missing := missingResults(calls, next)
		if len(missing) == 0 {
			if out != nil {
				out = append(out, msg)
			}
			continue
		}

		if out == nil {
			out = make([]*Message, 0, len(messages)+1)
			out = append(out, messages[:i]...)
		}
		out = append(out, msg)
		if next == nil {
			out = append(out, &Message{Role: User, Content: missing})
			continue
		}
		out = append(out, withResults(next, missing))
		i++ // next is consumed
	}
	if out == nil {
		return messages
	}
	return out
}

// hasToolResults reports whether msg is a user message holding tool results.
func hasToolResults(msg *Message) bool {
	if msg == nil || msg.Role != User {
		return false
	}
	for _, c := range msg.Content {
		if _, ok := c.(*ToolResultContent); ok {
			return true
		}
	}
	return false
}

// clientToolCalls returns the client tool calls of an assistant message.
func clientToolCalls(msg *Message) []*ToolUseContent {
	if msg == nil || msg.Role != Assistant {
		return nil
	}
	var calls []*ToolUseContent
	for _, c := range msg.Content {
		if call, ok := c.(*ToolUseContent); ok && call.ID != "" {
			calls = append(calls, call)
		}
	}
	return calls
}

// missingResults returns an unknown-result block for each call that next
// does not answer, in call order.
func missingResults(calls []*ToolUseContent, next *Message) []Content {
	answered := map[string]bool{}
	if next != nil {
		for _, c := range next.Content {
			if result, ok := c.(*ToolResultContent); ok {
				answered[result.ToolUseID] = true
			}
		}
	}
	var missing []Content
	for _, call := range calls {
		if answered[call.ID] {
			continue
		}
		answered[call.ID] = true
		missing = append(missing, &ToolResultContent{
			ToolUseID:   call.ID,
			ToolsetName: call.ToolsetName,
			Content:     ToolCallUnknownText,
			IsError:     true,
		})
	}
	return missing
}

// withResults returns a copy of msg with results added after its existing
// tool results and before its other content.
func withResults(msg *Message, results []Content) *Message {
	content := make([]Content, 0, len(msg.Content)+len(results))
	var rest []Content
	for _, c := range msg.Content {
		if _, ok := c.(*ToolResultContent); ok {
			content = append(content, c)
		} else {
			rest = append(rest, c)
		}
	}
	content = append(content, results...)
	content = append(content, rest...)
	cp := *msg
	cp.Content = content
	return &cp
}
