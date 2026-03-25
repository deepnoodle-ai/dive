package grok

import (
	"context"
	"errors"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/openai/openai-go/v3/responses"
)

var (
	_ llm.Tool                             = &WebSearchTool{}
	_ openaiProvider.ResponsesToolProvider = &WebSearchTool{}
)

// WebSearchToolOptions configures the Grok web search tool.
type WebSearchToolOptions struct {
	// AllowedDomains restricts search to these domains (max 5).
	// Cannot be used with ExcludedDomains.
	AllowedDomains []string

	// ExcludedDomains excludes these domains from search (max 5).
	// Cannot be used with AllowedDomains.
	ExcludedDomains []string

	// EnableImageUnderstanding allows analysis of images found during browsing.
	EnableImageUnderstanding bool
}

func (o WebSearchToolOptions) validate() error {
	if len(o.AllowedDomains) > 0 && len(o.ExcludedDomains) > 0 {
		return fmt.Errorf("AllowedDomains and ExcludedDomains cannot both be set")
	}
	if len(o.AllowedDomains) > 5 {
		return fmt.Errorf("AllowedDomains exceeds maximum of 5 (got %d)", len(o.AllowedDomains))
	}
	if len(o.ExcludedDomains) > 5 {
		return fmt.Errorf("ExcludedDomains exceeds maximum of 5 (got %d)", len(o.ExcludedDomains))
	}
	return nil
}

// NewWebSearchTool creates a new Grok WebSearchTool with the given options.
// Returns an error if the options are invalid.
func NewWebSearchTool(opts WebSearchToolOptions) (*WebSearchTool, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid WebSearchToolOptions: %w", err)
	}
	return &WebSearchTool{
		allowedDomains:           opts.AllowedDomains,
		excludedDomains:          opts.ExcludedDomains,
		enableImageUnderstanding: opts.EnableImageUnderstanding,
	}, nil
}

// WebSearchTool is a server-side tool that enables Grok to search the web.
// This tool is only available via the xAI Responses API.
type WebSearchTool struct {
	allowedDomains           []string
	excludedDomains          []string
	enableImageUnderstanding bool
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Uses Grok's web search to access real-time web content."
}

func (t *WebSearchTool) Schema() *schema.Schema {
	return nil
}

func (t *WebSearchTool) ResponsesToolParam() responses.ToolUnionParam {
	param := &responses.WebSearchToolParam{
		Type: "web_search",
	}
	if len(t.allowedDomains) > 0 {
		param.Filters = responses.WebSearchToolFiltersParam{
			AllowedDomains: t.allowedDomains,
		}
	}
	// excluded_domains and enable_image_understanding are Grok-specific
	// extensions not in the base OpenAI SDK, so we use SetExtraFields.
	extras := map[string]any{}
	if len(t.excludedDomains) > 0 {
		extras["filters"] = map[string]any{
			"excluded_domains": t.excludedDomains,
		}
	}
	if t.enableImageUnderstanding {
		extras["enable_image_understanding"] = true
	}
	if len(extras) > 0 {
		param.SetExtraFields(extras)
	}
	return responses.ToolUnionParam{OfWebSearch: param}
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
