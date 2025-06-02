package anthropic

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
       "type": "web_search_20250305",
       "name": "web_search",
       "max_uses": 5
   }]
*/

// UserLocation conveys the user's location to the web search tool.
type UserLocation struct {
	Type     string `json:"type"`     // "approximate"
	City     string `json:"city"`     // "San Francisco"
	Region   string `json:"region"`   // "California"
	Country  string `json:"country"`  // "US"
	Timezone string `json:"timezone"` // "America/Los_Angeles"
}

// WebSearchToolOptions are the options used to configure a WebSearchTool.
type WebSearchToolOptions struct {
	Type           string
	MaxUses        int
	AllowedDomains []string
	BlockedDomains []string
	UserLocation   *UserLocation
}

// NewWebSearchTool creates a new WebSearchTool with the given options.
func NewWebSearchTool(opts WebSearchToolOptions) *WebSearchTool {
	if opts.Type == "" {
		opts.Type = "web_search_20250305"
	}
	if opts.MaxUses <= 0 {
		opts.MaxUses = 5
	}
	return &WebSearchTool{
		typeString:     opts.Type,
		name:           "web_search",
		maxUses:        opts.MaxUses,
		allowedDomains: opts.AllowedDomains,
		blockedDomains: opts.BlockedDomains,
		userLocation:   opts.UserLocation,
	}
}

// WebSearchTool is a tool that allows Claude to search the web. This is
// provided by Anthropic as a server-side tool. Learn more:
// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool
type WebSearchTool struct {
	typeString     string
	name           string
	maxUses        int
	allowedDomains []string
	blockedDomains []string
	userLocation   *UserLocation
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Uses Anthropic's web search feature to give Claude direct access to real-time web content."
}

func (t *WebSearchTool) Schema() *schema.Schema {
	return nil // Empty for server-side tools
}

func (t *WebSearchTool) ToolConfiguration(providerName string) map[string]any {
	config := map[string]any{
		"type":     t.typeString,
		"name":     t.name,
		"max_uses": t.maxUses,
	}
	if t.allowedDomains != nil {
		config["allowed_domains"] = t.allowedDomains
	}
	if t.blockedDomains != nil {
		config["blocked_domains"] = t.blockedDomains
	}
	if t.userLocation != nil {
		config["user_location"] = t.userLocation
	}
	return config
}
