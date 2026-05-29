package grok

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

// toolParamMap marshals a ResponsesToolParam to a generic map so tests can
// assert on the exact JSON sent to the xAI Responses API.
func toolParamMap(t *testing.T, p responses.ToolUnionParam) map[string]any {
	t.Helper()
	data, err := json.Marshal(p)
	assert.NoError(t, err)
	var m map[string]any
	assert.NoError(t, json.Unmarshal(data, &m))
	return m
}

func TestWebSearchTool_EnableImageSearch(t *testing.T) {
	tool, err := NewWebSearchTool(WebSearchToolOptions{
		AllowedDomains:           []string{"grokipedia.com"},
		EnableImageUnderstanding: true,
		EnableImageSearch:        true,
	})
	assert.NoError(t, err)

	m := toolParamMap(t, tool.ResponsesToolParam())
	assert.Equal(t, "web_search", m["type"])
	assert.Equal(t, true, m["enable_image_understanding"])
	assert.Equal(t, true, m["enable_image_search"])
}

func TestWebSearchTool_ImageSearchOmittedByDefault(t *testing.T) {
	tool, err := NewWebSearchTool(WebSearchToolOptions{})
	assert.NoError(t, err)
	m := toolParamMap(t, tool.ResponsesToolParam())
	_, hasImageSearch := m["enable_image_search"]
	assert.False(t, hasImageSearch)
}

func TestWebSearchTool_Validation(t *testing.T) {
	_, err := NewWebSearchTool(WebSearchToolOptions{
		AllowedDomains:  []string{"a.com"},
		ExcludedDomains: []string{"b.com"},
	})
	assert.Error(t, err)

	_, err = NewWebSearchTool(WebSearchToolOptions{
		AllowedDomains: []string{"1", "2", "3", "4", "5", "6"},
	})
	assert.Error(t, err)
}

func TestXSearchTool_HandleLimit(t *testing.T) {
	handles := make([]string, 20)
	for i := range handles {
		handles[i] = "handle"
	}
	// 20 handles is allowed.
	_, err := NewXSearchTool(XSearchToolOptions{AllowedXHandles: handles})
	assert.NoError(t, err)

	// 21 handles exceeds the maximum.
	_, err = NewXSearchTool(XSearchToolOptions{AllowedXHandles: append(handles, "extra")})
	assert.Error(t, err)
}

func TestXSearchTool_Param(t *testing.T) {
	tool, err := NewXSearchTool(XSearchToolOptions{
		AllowedXHandles:          []string{"elonmusk"},
		FromDate:                 "2025-10-01",
		ToDate:                   "2025-10-10",
		EnableImageUnderstanding: true,
		EnableVideoUnderstanding: true,
	})
	assert.NoError(t, err)

	m := toolParamMap(t, tool.ResponsesToolParam())
	assert.Equal(t, "x_search", m["type"])
	assert.Equal(t, "2025-10-01", m["from_date"])
	assert.Equal(t, "2025-10-10", m["to_date"])
	assert.Equal(t, true, m["enable_image_understanding"])
	assert.Equal(t, true, m["enable_video_understanding"])
}

func TestCodeExecutionTool_Param(t *testing.T) {
	tool := NewCodeExecutionTool(CodeExecutionToolOptions{})
	m := toolParamMap(t, tool.ResponsesToolParam())
	assert.Equal(t, "code_interpreter", m["type"])
	assert.Empty(t, tool.ResponsesIncludes())
}

func TestCodeExecutionTool_IncludeOutputs(t *testing.T) {
	tool := NewCodeExecutionTool(CodeExecutionToolOptions{IncludeOutputs: true})
	includes := tool.ResponsesIncludes()
	assert.Len(t, includes, 1)
	assert.Equal(t, responses.ResponseIncludable("code_interpreter_call.outputs"), includes[0])
}

func TestCollectionsSearchTool_Param(t *testing.T) {
	tool, err := NewCollectionsSearchTool(CollectionsSearchToolOptions{
		CollectionIDs: []string{"collection_123"},
		MaxNumResults: 10,
	})
	assert.NoError(t, err)

	m := toolParamMap(t, tool.ResponsesToolParam())
	assert.Equal(t, "file_search", m["type"])
	ids, ok := m["vector_store_ids"].([]any)
	assert.True(t, ok)
	assert.Len(t, ids, 1)
	assert.Equal(t, "collection_123", ids[0])
	assert.Equal(t, float64(10), m["max_num_results"])
}

func TestCollectionsSearchTool_Validation(t *testing.T) {
	_, err := NewCollectionsSearchTool(CollectionsSearchToolOptions{})
	assert.Error(t, err)

	_, err = NewCollectionsSearchTool(CollectionsSearchToolOptions{
		CollectionIDs: []string{"c1"},
		MaxNumResults: 51,
	})
	assert.Error(t, err)
}

func TestCollectionsSearchTool_IncludeResults(t *testing.T) {
	tool, err := NewCollectionsSearchTool(CollectionsSearchToolOptions{
		CollectionIDs:  []string{"c1"},
		IncludeResults: true,
	})
	assert.NoError(t, err)
	includes := tool.ResponsesIncludes()
	assert.Len(t, includes, 1)
	assert.Equal(t, responses.ResponseIncludable("file_search_call.results"), includes[0])

	// Default: no includes requested.
	tool, err = NewCollectionsSearchTool(CollectionsSearchToolOptions{CollectionIDs: []string{"c1"}})
	assert.NoError(t, err)
	assert.Empty(t, tool.ResponsesIncludes())
}
