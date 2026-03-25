package grok

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

var (
	_ llm.Tool                             = &XSearchTool{}
	_ openaiProvider.ResponsesToolProvider = &XSearchTool{}
)

// XSearchToolOptions configures the Grok X (Twitter) search tool.
type XSearchToolOptions struct {
	// AllowedXHandles restricts search to posts from these handles (max 10).
	// Cannot be used with ExcludedXHandles.
	AllowedXHandles []string

	// ExcludedXHandles excludes posts from these handles (max 10).
	// Cannot be used with AllowedXHandles.
	ExcludedXHandles []string

	// FromDate is the start date for the search range (ISO8601 "YYYY-MM-DD").
	FromDate string

	// ToDate is the end date for the search range (ISO8601 "YYYY-MM-DD").
	ToDate string

	// EnableImageUnderstanding allows analysis of images in posts.
	EnableImageUnderstanding bool

	// EnableVideoUnderstanding allows analysis of videos in posts.
	EnableVideoUnderstanding bool
}

// NewXSearchTool creates a new Grok XSearchTool with the given options.
func NewXSearchTool(opts XSearchToolOptions) *XSearchTool {
	return &XSearchTool{
		allowedXHandles:          opts.AllowedXHandles,
		excludedXHandles:         opts.ExcludedXHandles,
		fromDate:                 opts.FromDate,
		toDate:                   opts.ToDate,
		enableImageUnderstanding: opts.EnableImageUnderstanding,
		enableVideoUnderstanding: opts.EnableVideoUnderstanding,
	}
}

// XSearchTool is a server-side tool that enables Grok to search X (Twitter).
// This tool is only available via the xAI Responses API.
type XSearchTool struct {
	allowedXHandles          []string
	excludedXHandles         []string
	fromDate                 string
	toDate                   string
	enableImageUnderstanding bool
	enableVideoUnderstanding bool
}

func (t *XSearchTool) Name() string {
	return "x_search"
}

func (t *XSearchTool) Description() string {
	return "Uses Grok's X search to access real-time social media content from X (Twitter)."
}

func (t *XSearchTool) Schema() *schema.Schema {
	return nil
}

func (t *XSearchTool) ResponsesToolParam() responses.ToolUnionParam {
	// x_search is a Grok-specific tool type not in the OpenAI SDK,
	// so we use param.Override to pass raw JSON.
	config := map[string]any{
		"type": "x_search",
	}
	if len(t.allowedXHandles) > 0 {
		config["allowed_x_handles"] = t.allowedXHandles
	} else if len(t.excludedXHandles) > 0 {
		config["excluded_x_handles"] = t.excludedXHandles
	}
	if t.fromDate != "" {
		config["from_date"] = t.fromDate
	}
	if t.toDate != "" {
		config["to_date"] = t.toDate
	}
	if t.enableImageUnderstanding {
		config["enable_image_understanding"] = true
	}
	if t.enableVideoUnderstanding {
		config["enable_video_understanding"] = true
	}
	return param.Override[responses.ToolUnionParam](config)
}

func (t *XSearchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "X Search",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *XSearchTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
