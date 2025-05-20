package toolkit

import (
	"context"
	"encoding/json"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/schema"
	"github.com/diveagents/dive/web"
)

var _ dive.TypedTool[*SearchInput] = &SearchTool{}

type SearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type SearchTool struct {
	searcher web.Searcher
}

func NewSearchTool(searcher web.Searcher) *SearchTool {
	return &SearchTool{searcher: searcher}
}

func (t *SearchTool) Name() string {
	return "search"
}

func (t *SearchTool) Description() string {
	return "Searches the web using the given query. The response includes the url, title, and description of each webpage in the search results."
}

func (t *SearchTool) Schema() schema.Schema {
	return schema.Schema{
		Type:     "object",
		Required: []string{"query"},
		Properties: map[string]*schema.Property{
			"query": {
				Type:        "string",
				Description: "The search query, e.g. 'cloud security companies'",
			},
			"limit": {
				Type:        "number",
				Description: "The maximum number of results to return (Default: 10, Min: 10, Max: 30)",
			},
		},
	}
}

func (t *SearchTool) Call(ctx context.Context, input *SearchInput) (*dive.ToolResult, error) {
	limit := 10
	if limit <= 0 {
		limit = 10
	}
	if limit > 30 {
		limit = 30
	}
	results, err := t.searcher.Search(ctx, &web.SearchInput{
		Query: input.Query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	if len(results.Items) == 0 {
		return NewToolResultError("No search results found"), nil
	}
	if len(results.Items) > limit {
		results.Items = results.Items[:limit]
	}
	data, err := json.Marshal(results.Items)
	if err != nil {
		return nil, err
	}
	return NewToolResultText(string(data)), nil
}

func (t *SearchTool) Annotations() dive.ToolAnnotations {
	return dive.ToolAnnotations{
		Title:           "Search",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   true,
	}
}
