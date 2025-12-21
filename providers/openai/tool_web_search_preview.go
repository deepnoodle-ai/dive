package openai

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
)

var (
	_ llm.Tool = &WebSearchPreviewTool{}
)

/* A tool definition must be added in the request that looks like this:
   "tools": [{
       "type": "web_search_preview",
       "domains": ["arxiv.org", "openai.com"],
       "search_context_size": "medium",
       "user_location": {
           "type": "approximate",
           "country": "US"
       }
   }]
*/

// WebSearchPreviewToolOptions are the options used to configure a WebSearchPreviewTool.
type WebSearchPreviewToolOptions struct {
	Domains           []string      `json:"domains,omitempty"`
	SearchContextSize string        `json:"search_context_size,omitempty"` // "low", "medium", "high"
	UserLocation      *UserLocation `json:"user_location,omitempty"`
}

// NewWebSearchPreviewTool creates a new WebSearchPreviewTool with the given options.
func NewWebSearchPreviewTool(opts WebSearchPreviewToolOptions) *WebSearchPreviewTool {
	return &WebSearchPreviewTool{
		domains:           opts.Domains,
		searchContextSize: opts.SearchContextSize,
		userLocation:      opts.UserLocation,
	}
}

// WebSearchPreviewTool is a tool that allows models to search the web. This is
// provided by OpenAI as a server-side tool in the Responses API.
type WebSearchPreviewTool struct {
	domains           []string
	searchContextSize string
	userLocation      *UserLocation
}

func (t *WebSearchPreviewTool) Name() string {
	return "web_search"
}

func (t *WebSearchPreviewTool) Description() string {
	return "Uses OpenAI's web search feature to give models direct access to real-time web content."
}

func (t *WebSearchPreviewTool) Schema() *schema.Schema {
	return nil // Empty for server-side tools
}

func (t *WebSearchPreviewTool) Param() *responses.WebSearchToolParam {
	param := &responses.WebSearchToolParam{
		Type: "web_search_preview",
	}
	switch t.searchContextSize {
	case "low":
		param.SearchContextSize = responses.WebSearchToolSearchContextSizeLow
	case "medium":
		param.SearchContextSize = responses.WebSearchToolSearchContextSizeMedium
	case "high":
		param.SearchContextSize = responses.WebSearchToolSearchContextSizeHigh
	}
	if t.userLocation != nil {
		param.UserLocation.Type = "approximate"
		if t.userLocation.City != "" {
			param.UserLocation.City = openai.String(t.userLocation.City)
		}
		if t.userLocation.Country != "" {
			param.UserLocation.Country = openai.String(t.userLocation.Country)
		}
		if t.userLocation.Region != "" {
			param.UserLocation.Region = openai.String(t.userLocation.Region)
		}
		if t.userLocation.Timezone != "" {
			param.UserLocation.Timezone = openai.String(t.userLocation.Timezone)
		}
	}
	return param
}

func (t *WebSearchPreviewTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Web Search",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *WebSearchPreviewTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
