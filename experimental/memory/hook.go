package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// InjectMemoriesHook returns a PreGenerationHook that queries svc using the
// most recent user message as the search query, then appends the top-limit
// results as a <memory> block to the system prompt. Empty results are a no-op.
func InjectMemoriesHook(svc Service, limit int) dive.PreGenerationHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		query := lastUserText(hctx.Messages)
		if query == "" {
			return nil
		}
		entries, err := svc.Search(ctx, query, limit)
		if err != nil || len(entries) == 0 {
			return nil
		}
		var sb strings.Builder
		sb.WriteString("<memory>\n")
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.ID, e.Content))
		}
		sb.WriteString("</memory>")
		if hctx.SystemPrompt != "" {
			hctx.SystemPrompt += "\n\n" + sb.String()
		} else {
			hctx.SystemPrompt = sb.String()
		}
		return nil
	}
}

func lastUserText(messages []*llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.User {
			return messages[i].Text()
		}
	}
	return ""
}
