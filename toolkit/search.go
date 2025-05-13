package toolkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/web"
)

var _ llm.ToolWithMetadata = &SearchTool{}

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

func (t *SearchTool) Schema() llm.Schema {
	return llm.Schema{
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
	}
}

func (t *SearchTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	var s web.SearchInput
	if err := json.Unmarshal([]byte(input.Input), &s); err != nil {
		return nil, err
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
		return nil, err
	}
	if len(results.Items) == 0 {
		return llm.NewToolCallOutput("No search results found"), nil
	}
	if len(results.Items) > limit {
		results.Items = results.Items[:limit]
	}
	data, err := json.Marshal(results.Items)
	if err != nil {
		return nil, err
	}
	return llm.NewToolCallOutput(string(data)).
		WithSummary(fmt.Sprintf("Searched the web for %s and found %d results",
			s.Query, len(results.Items))), nil
}

func (t *SearchTool) Metadata() llm.ToolMetadata {
	return llm.ToolMetadata{
		Version:    "0.0.1",
		Capability: llm.ToolCapabilityReadOnly,
	}
}
