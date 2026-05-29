package grok

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
	_ llm.Tool                                = &CollectionsSearchTool{}
	_ openaiProvider.ResponsesToolProvider    = &CollectionsSearchTool{}
	_ openaiProvider.ResponsesIncludeProvider = &CollectionsSearchTool{}
)

// CollectionsSearchToolOptions configures the Grok collections search tool.
type CollectionsSearchToolOptions struct {
	// CollectionIDs are the xAI collection IDs to search (at least one required).
	CollectionIDs []string

	// MaxNumResults caps the number of results returned per search. When set it
	// must be between 1 and 50. Zero leaves the server default in place.
	MaxNumResults int

	// IncludeResults requests that the server return the matched document chunks
	// in the response via the "file_search_call.results" include parameter.
	IncludeResults bool
}

func (o CollectionsSearchToolOptions) validate() error {
	if len(o.CollectionIDs) == 0 {
		return fmt.Errorf("at least one CollectionID is required")
	}
	if o.MaxNumResults < 0 || o.MaxNumResults > 50 {
		return fmt.Errorf("MaxNumResults must be between 1 and 50 (got %d)", o.MaxNumResults)
	}
	return nil
}

// NewCollectionsSearchTool creates a new Grok CollectionsSearchTool, letting the
// model search your uploaded knowledge bases (collections) for relevant
// documents. This tool is only available via the xAI Responses API (it maps to
// the API's "file_search" tool, where collection IDs are the vector store IDs).
func NewCollectionsSearchTool(opts CollectionsSearchToolOptions) (*CollectionsSearchTool, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid CollectionsSearchToolOptions: %w", err)
	}
	return &CollectionsSearchTool{
		collectionIDs:  opts.CollectionIDs,
		maxNumResults:  opts.MaxNumResults,
		includeResults: opts.IncludeResults,
	}, nil
}

// CollectionsSearchTool is a server-side tool that enables Grok to search
// uploaded collections (knowledge bases).
type CollectionsSearchTool struct {
	collectionIDs  []string
	maxNumResults  int
	includeResults bool
}

func (t *CollectionsSearchTool) Name() string {
	return "collections_search"
}

func (t *CollectionsSearchTool) Description() string {
	return "Searches your uploaded xAI collections (knowledge bases) for relevant documents."
}

func (t *CollectionsSearchTool) Schema() *schema.Schema {
	return nil
}

func (t *CollectionsSearchTool) ResponsesToolParam() responses.ToolUnionParam {
	param := &responses.FileSearchToolParam{
		VectorStoreIDs: t.collectionIDs,
	}
	if t.maxNumResults > 0 {
		param.MaxNumResults = openai.Int(int64(t.maxNumResults))
	}
	return responses.ToolUnionParam{OfFileSearch: param}
}

func (t *CollectionsSearchTool) ResponsesIncludes() []responses.ResponseIncludable {
	if !t.includeResults {
		return nil
	}
	return []responses.ResponseIncludable{"file_search_call.results"}
}

func (t *CollectionsSearchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Collections Search",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *CollectionsSearchTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
