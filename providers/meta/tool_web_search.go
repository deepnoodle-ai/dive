package meta

import (
	"context"
	"errors"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

var (
	_ llm.Tool                                = &WebSearchTool{}
	_ openaiProvider.ResponsesToolProvider    = &WebSearchTool{}
	_ openaiProvider.ResponsesIncludeProvider = &WebSearchTool{}
)

// SearchContextSize controls how much retrieved content reaches the model.
type SearchContextSize string

const (
	SearchContextLow    SearchContextSize = "low"
	SearchContextMedium SearchContextSize = "medium"
	SearchContextHigh   SearchContextSize = "high"
)

// UserLocation biases search toward a locale. Every field is optional; supply
// only what is known.
type UserLocation struct {
	// Country is a two-letter ISO 3166-1 code, such as "GB".
	Country string
	// Region is free text, such as "California".
	Region string
	// City is free text, such as "San Francisco".
	City string
	// Timezone is an IANA time zone name, such as "America/Los_Angeles".
	Timezone string
}

func (l UserLocation) isEmpty() bool {
	return l.Country == "" && l.Region == "" && l.City == "" && l.Timezone == ""
}

// WebSearchToolOptions configures Meta's search grounding tool.
//
// Meta's web_search takes no domain allow/deny list, unlike Grok's and OpenAI's.
// Options are deliberately limited to what Model API documents, since an
// unsupported field is a 400 rather than something the server ignores.
type WebSearchToolOptions struct {
	// SearchContextSize trades latency and tokens for breadth. Empty leaves the
	// server default of "medium".
	SearchContextSize SearchContextSize

	// UserLocation biases results toward a locale, for "near me" and other
	// location-sensitive queries.
	UserLocation UserLocation

	// IncludeResults asks for the raw hits behind each search — title, url, and
	// the snippet the model actually saw. Meta returns only that a search ran
	// unless this is set, and its own docs recommend checking the results when
	// an answer matters, because coverage is incomplete.
	IncludeResults bool
}

func (o WebSearchToolOptions) validate() error {
	switch o.SearchContextSize {
	case "", SearchContextLow, SearchContextMedium, SearchContextHigh:
	default:
		return fmt.Errorf("SearchContextSize must be low, medium, or high (got %q)",
			o.SearchContextSize)
	}
	return nil
}

// NewWebSearchTool creates a Meta search grounding tool. It returns an error if
// the options are invalid.
func NewWebSearchTool(opts WebSearchToolOptions) (*WebSearchTool, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid WebSearchToolOptions: %w", err)
	}
	return &WebSearchTool{
		searchContextSize: opts.SearchContextSize,
		userLocation:      opts.UserLocation,
		includeResults:    opts.IncludeResults,
	}, nil
}

// WebSearchTool is a server-side tool that grounds answers in live web results
// with inline citations. It is available on the Responses API only; Meta does
// not offer search grounding through Chat Completions.
//
// Enabling it does not force a search. The model decides per request and skips
// the search when it can answer from training data, so a response with no
// web_search_call is normal rather than a failure.
type WebSearchTool struct {
	searchContextSize SearchContextSize
	userLocation      UserLocation
	includeResults    bool
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Grounds answers in live web search results with inline citations."
}

func (t *WebSearchTool) Schema() *schema.Schema {
	return nil
}

func (t *WebSearchTool) ResponsesToolParam() responses.ToolUnionParam {
	param := &responses.WebSearchToolParam{Type: "web_search"}
	if t.searchContextSize != "" {
		param.SearchContextSize = responses.WebSearchToolSearchContextSize(t.searchContextSize)
	}
	if !t.userLocation.isEmpty() {
		location := responses.WebSearchToolUserLocationParam{Type: "approximate"}
		if t.userLocation.Country != "" {
			location.Country = openai.String(t.userLocation.Country)
		}
		if t.userLocation.Region != "" {
			location.Region = openai.String(t.userLocation.Region)
		}
		if t.userLocation.City != "" {
			location.City = openai.String(t.userLocation.City)
		}
		if t.userLocation.Timezone != "" {
			location.Timezone = openai.String(t.userLocation.Timezone)
		}
		param.UserLocation = location
	}
	return responses.ToolUnionParam{OfWebSearch: param}
}

func (t *WebSearchTool) ResponsesIncludes() []responses.ResponseIncludable {
	if !t.includeResults {
		return nil
	}
	return []responses.ResponseIncludable{"web_search_call.results"}
}

func (t *WebSearchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Web Search",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *WebSearchTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
