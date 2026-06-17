package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

type memorySaveInput struct {
	Content string   `json:"content" jsonschema:"description=The fact or information to remember,required"`
	Tags    []string `json:"tags,omitempty" jsonschema:"description=Optional tags for categorization"`
}

type memorySearchInput struct {
	Query string `json:"query" jsonschema:"description=The query to search memories for,required"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (default 5)"`
}

// MemoryTools returns the memory_save and memory_search tools backed by svc.
// Include both in AgentOptions.Tools to let the model explicitly manage memories.
func MemoryTools(svc Service) []dive.Tool {
	return []dive.Tool{
		memorySaveTool(svc),
		memorySearchTool(svc),
	}
}

func memorySaveTool(svc Service) dive.Tool {
	return dive.FuncTool("memory_save", "Save a fact or piece of information to long-term memory.",
		func(ctx context.Context, input *memorySaveInput) (*dive.ToolResult, error) {
			entry := Entry{
				Content: input.Content,
				Tags:    input.Tags,
			}
			if err := svc.Save(ctx, entry); err != nil {
				return dive.NewToolResultError(fmt.Sprintf("Failed to save memory: %v", err)), nil
			}
			return dive.NewToolResultText("Memory saved."), nil
		})
}

func memorySearchTool(svc Service) dive.Tool {
	return dive.FuncTool("memory_search", "Search long-term memory for relevant information.",
		func(ctx context.Context, input *memorySearchInput) (*dive.ToolResult, error) {
			limit := input.Limit
			if limit <= 0 {
				limit = 5
			}
			entries, err := svc.Search(ctx, input.Query, limit)
			if err != nil {
				return dive.NewToolResultError(fmt.Sprintf("Failed to search memories: %v", err)), nil
			}
			if len(entries) == 0 {
				return dive.NewToolResultText("No relevant memories found."), nil
			}
			var sb strings.Builder
			for i, e := range entries {
				if i > 0 {
					sb.WriteString("\n")
				}
				tags := ""
				if len(e.Tags) > 0 {
					tags = " [" + strings.Join(e.Tags, ", ") + "]"
				}
				sb.WriteString(fmt.Sprintf("%d. [%s]%s %s", i+1, e.ID, tags, e.Content))
			}
			return dive.NewToolResultText(sb.String()), nil
		})
}
