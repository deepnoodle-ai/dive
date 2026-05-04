// Package autotitle provides a PostGenerationHook that automatically generates
// and sets a session title after the first completed turn.
//
// Usage:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model:   anthropic.New(),
//	    Session: sess,
//	    Hooks: dive.Hooks{
//	        PostGeneration: []dive.PostGenerationHook{
//	            autotitle.AutoTitleHook(anthropic.NewModel("claude-haiku-4-5-20251001")),
//	        },
//	    },
//	})
package autotitle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
)

const (
	titleTimeout  = 5 * time.Second
	maxContextLen = 200
)

var titlePrompt = `Generate a short title (3-7 words) for a conversation that begins with:

User: %s
Assistant: %s

Reply with only the title, no punctuation.`

// AutoTitleHook returns a PostGenerationHook that generates and sets a session
// title after the first completed turn. Subsequent turns are no-ops.
//
// The provided LLM is used only for title generation — use a cheap, fast model
// such as claude-haiku-4-5-20251001. A 5-second timeout is applied to the
// title LLM call so a slow response does not delay the main hook chain.
func AutoTitleHook(titleLLM llm.LLM) dive.PostGenerationHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		sess, ok := hctx.Session.(*session.Session)
		if !ok || sess == nil {
			return nil
		}

		// Skip if the session already has a title.
		if sess.Title() != "" {
			return nil
		}

		// Extract the first user message and first response text.
		firstUserText := firstUserText(hctx.Messages)
		firstResponseText := firstResponseText(hctx.Response)
		if firstUserText == "" || firstResponseText == "" {
			return nil
		}

		prompt := fmt.Sprintf(titlePrompt,
			truncate(firstUserText, maxContextLen),
			truncate(firstResponseText, maxContextLen),
		)

		tctx, cancel := context.WithTimeout(ctx, titleTimeout)
		defer cancel()

		resp, err := titleLLM.Generate(tctx, llm.WithMessages(llm.NewUserTextMessage(prompt)))
		if err != nil {
			// Title generation failure is non-fatal — log and continue.
			return nil
		}
		title := strings.TrimSpace(resp.Message().Text())
		if title != "" {
			sess.SetTitle(title)
		}
		return nil
	}
}

func firstUserText(messages []*llm.Message) string {
	for _, msg := range messages {
		if msg.Role == llm.User {
			return msg.Text()
		}
	}
	return ""
}

func firstResponseText(resp *dive.Response) string {
	if resp == nil {
		return ""
	}
	for _, msg := range resp.OutputMessages {
		if msg.Role == llm.Assistant {
			return msg.Text()
		}
	}
	return ""
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
