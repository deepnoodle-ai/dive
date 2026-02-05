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

// WebSearchToolOptions configures the behavior of [WebSearchTool].
type WebSearchToolOptions struct {
	// Searcher is the underlying search implementation (e.g., Google, Kagi).
	// Required - the tool will fail at call time if not provided.
	Searcher web.Searcher
}

// SearchInput represents the input parameters for the WebSearch tool.
type SearchInput struct {
	// Query is the search query string. Required.
	Query string `json:"query"`

	// Limit is the maximum number of results to return.
	// Valid range: 10-30. Defaults to 10 if not specified or out of range.
	Limit int `json:"limit"`
}

// WebSearchTool searches the web using a configured search provider.
//
// This tool enables LLMs to access current information beyond their
// training data by performing web searches and returning structured
// results including URLs, titles, and descriptions.
//
// Features:
//   - Configurable search provider (Google, Kagi, etc.)
//   - Result limit control (10-30 results)
//   - JSON output with URL, title, and description per result
//
// The tool requires a [web.Searcher] implementation to be provided
// via options. Without a searcher, the tool cannot function.
type WebSearchTool struct {
	searcher web.Searcher
}

// NewWebSearchTool creates a new WebSearchTool with the given options.
func NewWebSearchTool(options WebSearchToolOptions) *dive.TypedToolAdapter[*SearchInput] {
	return dive.ToolAdapter(&WebSearchTool{
		searcher: options.Searcher,
	})
}

// Name returns "WebSearch" as the tool identifier.
func (t *WebSearchTool) Name() string {
	return "WebSearch"
}

// Description returns usage instructions for the LLM.
func (t *WebSearchTool) Description() string {
	return "Searches the web using the given query. The response includes the url, title, and description of each webpage in the search results."
}

// Schema returns the JSON schema describing the tool's input parameters.
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

// Call performs the web search and returns results as JSON.
//
// Results include the URL, title, and description for each matching
// web page. If no results are found, an error result is returned.
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

// Annotations returns metadata hints about the tool's behavior.
// WebSearch is marked as read-only, idempotent, and open-world
// (accesses external systems).
func (t *WebSearchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "WebSearch",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   true,
	}
}
