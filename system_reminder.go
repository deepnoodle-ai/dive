package dive

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

// patternCache memoizes compiled regexps for named system-reminder blocks.
var patternCache sync.Map // string -> *regexp.Regexp

// SetSystemReminder inserts or replaces a named <system-reminder> block in
// the first user message's text content. If the block already exists (matched
// by name), it is replaced in place. Otherwise it is appended.
//
// This provides a stable, cache-friendly location for injecting context that
// persists across generations. Placing reminders in the first user message
// ensures they sit right after the system prompt in the conversation, forming
// a stable prefix for prompt caching.
//
// If there is no user message in the conversation, one is created and prepended.
//
// The block format is:
//
//	<system-reminder name="skills">
//	...content...
//	</system-reminder>
func SetSystemReminder(messages []*llm.Message, name, content string) []*llm.Message {
	block := formatBlock(name, content)

	// Find the first user message
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		// Find the first text content block
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			// Try to replace existing block with same name
			if replaced := replaceBlock(tc, name, block); replaced {
				return messages
			}
			// Append new block
			tc.Text = strings.TrimRight(tc.Text, "\n") + "\n" + block
			return messages
		}
		// No text content — add one
		msg.Content = append(msg.Content, &llm.TextContent{Text: block})
		return messages
	}

	// No user message — create one
	return append([]*llm.Message{
		llm.NewUserTextMessage(block),
	}, messages...)
}

// RemoveSystemReminder removes a named <system-reminder> block from the
// first user message. Returns the messages unchanged if the block is not found.
func RemoveSystemReminder(messages []*llm.Message, name string) []*llm.Message {
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			tc.Text = removeBlock(tc.Text, name)
			return messages
		}
	}
	return messages
}

// HasSystemReminder returns true if a named <system-reminder> block exists
// in the first user message.
func HasSystemReminder(messages []*llm.Message, name string) bool {
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			return blockPattern(name).MatchString(tc.Text)
		}
	}
	return false
}

func formatBlock(name, content string) string {
	return fmt.Sprintf("<system-reminder name=%q>\n%s\n</system-reminder>", name, content)
}

func blockPattern(name string) *regexp.Regexp {
	if cached, ok := patternCache.Load(name); ok {
		return cached.(*regexp.Regexp)
	}
	escaped := regexp.QuoteMeta(name)
	re := regexp.MustCompile(
		`(?s)\n?<system-reminder name="` + escaped + `">\n.*?\n</system-reminder>\n?`,
	)
	patternCache.Store(name, re)
	return re
}

func replaceBlock(tc *llm.TextContent, name, newBlock string) bool {
	pat := blockPattern(name)
	if !pat.MatchString(tc.Text) {
		return false
	}
	tc.Text = pat.ReplaceAllString(tc.Text, "\n"+newBlock+"\n")
	tc.Text = strings.TrimLeft(tc.Text, "\n")
	return true
}

func removeBlock(text, name string) string {
	pat := blockPattern(name)
	result := pat.ReplaceAllString(text, "")
	return strings.TrimRight(result, "\n")
}
