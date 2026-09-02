package meta

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestWebSearchToolValidation(t *testing.T) {
	_, err := NewWebSearchTool(WebSearchToolOptions{SearchContextSize: "enormous"})
	assert.Error(t, err)
	assert.ErrorContains(t, err, "SearchContextSize")

	for _, size := range []SearchContextSize{"", SearchContextLow, SearchContextMedium, SearchContextHigh} {
		_, err := NewWebSearchTool(WebSearchToolOptions{SearchContextSize: size})
		assert.NoError(t, err, "size %q should be accepted", size)
	}
}

// An unset option must not reach the wire. Meta rejects unsupported fields with
// a 400 rather than ignoring them, so a zero value serialized as "" would break
// every request made with default options.
func TestWebSearchToolOmitsUnsetFields(t *testing.T) {
	tool, err := NewWebSearchTool(WebSearchToolOptions{})
	assert.NoError(t, err)

	encoded, err := json.Marshal(tool.ResponsesToolParam())
	assert.NoError(t, err)
	body := string(encoded)

	assert.Contains(t, body, `"type":"web_search"`)
	assert.NotContains(t, body, "search_context_size")
	assert.NotContains(t, body, "user_location")
	assert.Empty(t, tool.ResponsesIncludes())
}

func TestWebSearchToolEncodesSetFields(t *testing.T) {
	tool, err := NewWebSearchTool(WebSearchToolOptions{
		SearchContextSize: SearchContextHigh,
		UserLocation:      UserLocation{Country: "GB", City: "London"},
		IncludeResults:    true,
	})
	assert.NoError(t, err)

	encoded, err := json.Marshal(tool.ResponsesToolParam())
	assert.NoError(t, err)
	body := string(encoded)

	assert.Contains(t, body, `"search_context_size":"high"`)
	assert.Contains(t, body, `"country":"GB"`)
	assert.Contains(t, body, `"city":"London"`)
	// type is required on user_location whenever the object is present.
	assert.Contains(t, body, `"type":"approximate"`)
	// Fields that were not supplied stay absent rather than empty.
	assert.NotContains(t, body, `"region"`)
	assert.NotContains(t, body, `"timezone"`)

	includes := tool.ResponsesIncludes()
	assert.Len(t, includes, 1)
	assert.Equal(t, string(includes[0]), "web_search_call.results")
}
