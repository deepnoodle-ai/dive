package toolkit

import (
	"context"
	"encoding/json"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/web"
)

var _ llm.Tool = &SearchTool{}

type SearchTool struct {
	searcher web.Searcher
}

func NewSearchTool(searcher web.Searcher) *SearchTool {
	return &SearchTool{searcher: searcher}
}

func (t *SearchTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "search",
		Description: "Searches the web using the given query. The response includes the url, title, and description of each webpage in the search results.",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]*llm.SchemaProperty{
				"query": {
					Type:        "string",
					Description: "The search query, e.g. 'cloud security companies'",
				},
				"limit": {
					Type:        "number",
					Description: "The maximum number of results to return (Default: 10, Min: 10, Max: 30)",
				},
			},
		},
	}
}

func (t *SearchTool) Call(ctx context.Context, input string) (string, error) {
	var s web.SearchInput
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		return "", err
	}
	limit := 10
	if limit <= 0 {
		limit = 10
	}
	if limit > 30 {
		limit = 30
	}
	results, err := t.searcher.Search(ctx, &s)
	if err != nil {
		return "", err
	}
	if len(results.Items) == 0 {
		return "No search results found", nil
	}
	if len(results.Items) > limit {
		results.Items = results.Items[:limit]
	}
	data, err := json.Marshal(results.Items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *SearchTool) ShouldReturnResult() bool {
	return true
}
