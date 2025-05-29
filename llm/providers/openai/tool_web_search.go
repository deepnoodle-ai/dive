package openai

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
)

var (
	_ llm.Tool              = &WebSearchTool{}
	_ llm.ToolConfiguration = &WebSearchTool{}
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

// WebSearchToolOptions are the options used to configure a WebSearchTool.
type WebSearchToolOptions struct {
	Domains           []string      `json:"domains,omitempty"`
	SearchContextSize string        `json:"search_context_size,omitempty"` // "low", "medium", "high"
	UserLocation      *UserLocation `json:"user_location,omitempty"`
}

// NewWebSearchTool creates a new WebSearchTool with the given options.
func NewWebSearchTool(opts WebSearchToolOptions) *WebSearchTool {
	return &WebSearchTool{
		domains:           opts.Domains,
		searchContextSize: opts.SearchContextSize,
		userLocation:      opts.UserLocation,
	}
}

// WebSearchTool is a tool that allows models to search the web. This is
// provided by OpenAI as a server-side tool in the Responses API.
type WebSearchTool struct {
	domains           []string
	searchContextSize string
	userLocation      *UserLocation
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Uses OpenAI's web search feature to give models direct access to real-time web content."
}

func (t *WebSearchTool) Schema() schema.Schema {
	return schema.Schema{} // Empty for server-side tools
}

func (t *WebSearchTool) ToolConfiguration(providerName string) map[string]any {
	config := map[string]any{
		"type": "web_search_preview",
	}
	if len(t.domains) > 0 {
		config["domains"] = t.domains
	}
	if t.searchContextSize != "" {
		config["search_context_size"] = t.searchContextSize
	}
	if t.userLocation != nil {
		config["user_location"] = t.userLocation
	}
	return config
}
