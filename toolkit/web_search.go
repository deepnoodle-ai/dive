package toolkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/deepnoodle-ai/wonton/web"
)

var _ dive.TypedTool[*SearchInput] = &WebSearchTool{}

type WebSearchToolOptions struct {
	Searcher web.Searcher
}

type SearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type WebSearchTool struct {
	searcher web.Searcher
}

func NewWebSearchTool(options WebSearchToolOptions) *dive.TypedToolAdapter[*SearchInput] {
	return dive.ToolAdapter(&WebSearchTool{
		searcher: options.Searcher,
	})
}

func (t *WebSearchTool) Name() string {
	return "WebSearch"
}

func (t *WebSearchTool) Description() string {
	return "Searches the web using the given query. The response includes the url, title, and description of each webpage in the search results."
}

func (t *WebSearchTool) Schema() *schema.Schema {
	return &schema.Schema{
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

func (t *WebSearchTool) Call(ctx context.Context, input *SearchInput) (*dive.ToolResult, error) {
	limit := input.Limit
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
	display := fmt.Sprintf("Found %d results for %q", len(results.Items), input.Query)
	return NewToolResultText(string(data)).WithDisplay(display), nil
}

func (t *WebSearchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Web Search",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   true,
	}
}
