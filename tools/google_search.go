package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/tools/google"
)

var _ llm.Tool = &GoogleSearch{}

type GoogleSearchInput struct {
	Query string      `json:"query"`
	Limit interface{} `json:"limit"`
}

type GoogleSearch struct {
	client *google.Client
}

func NewGoogleSearch(client *google.Client) *GoogleSearch {
	return &GoogleSearch{client: client}
}

func (t *GoogleSearch) Name() string {
	return "GoogleSearch"
}

func (t *GoogleSearch) Description() string {
	return "Returns search results from Google for the given search query. The response includes the url, title, and description of each webpage in the search results."
}

func (t *GoogleSearch) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
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

func (t *GoogleSearch) Call(ctx context.Context, input string) (string, error) {
	var s GoogleSearchInput
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		return "", err
	}
	limit := 10
	if value, ok := s.Limit.(int); ok {
		limit = value
	} else if value, ok := s.Limit.(float64); ok {
		limit = int(value)
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 30 {
		limit = 30
	}
	results, err := t.client.Search(ctx, &google.Query{
		Text:  s.Query,
		Limit: limit,
	})
	if err != nil {
		return "", err
	}
	if len(results.Items) == 0 {
		return "ERROR: no results found", nil
	}
	if len(results.Items) > limit {
		results.Items = results.Items[:limit]
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Found %d search results:", len(results.Items)))
	lines = append(lines, "")

	for i, r := range results.Items {
		lines = append(lines, fmt.Sprintf("## Search Result %d", i+1))
		lines = append(lines, fmt.Sprintf("- Title: %s", r.Title))
		lines = append(lines, fmt.Sprintf("- URL: %s", r.URL))
		lines = append(lines, fmt.Sprintf("- Description: %s", r.Description))
		lines = append(lines, "")
	}

	text := strings.Join(lines, "\n")
	text = strings.ReplaceAll(text, "```", "")
	return text, nil
}
