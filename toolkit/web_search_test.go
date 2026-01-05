package toolkit

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/web"
)

// mockSearcher implements web.Searcher for testing
type mockSearcher struct {
	receivedLimit int
	itemCount     int
}

func (m *mockSearcher) Search(ctx context.Context, input *web.SearchInput) (*web.SearchOutput, error) {
	m.receivedLimit = input.Limit

	// Generate items up to the requested count
	items := []*web.SearchItem{}
	for i := 0; i < m.itemCount; i++ {
		items = append(items, &web.SearchItem{
			URL:         "https://example.com",
			Title:       "Test Result",
			Description: "Test description",
		})
	}

	return &web.SearchOutput{
		Items: items,
	}, nil
}

func TestWebSearchTool_LimitParameter(t *testing.T) {
	t.Run("UsesProvidedLimit", func(t *testing.T) {
		searcher := &mockSearcher{itemCount: 15}
		tool := &WebSearchTool{searcher: searcher}

		_, err := tool.Call(context.Background(), &SearchInput{
			Query: "test query",
			Limit: 15,
		})

		assert.NoError(t, err)
		assert.Equal(t, 15, searcher.receivedLimit, "Should use the provided limit")
	})

	t.Run("DefaultsToTenWhenZero", func(t *testing.T) {
		searcher := &mockSearcher{itemCount: 10}
		tool := &WebSearchTool{searcher: searcher}

		_, err := tool.Call(context.Background(), &SearchInput{
			Query: "test query",
			Limit: 0,
		})

		assert.NoError(t, err)
		assert.Equal(t, 10, searcher.receivedLimit, "Should default to 10 when limit is 0")
	})

	t.Run("DefaultsToTenWhenNegative", func(t *testing.T) {
		searcher := &mockSearcher{itemCount: 10}
		tool := &WebSearchTool{searcher: searcher}

		_, err := tool.Call(context.Background(), &SearchInput{
			Query: "test query",
			Limit: -5,
		})

		assert.NoError(t, err)
		assert.Equal(t, 10, searcher.receivedLimit, "Should default to 10 when limit is negative")
	})

	t.Run("CapsAtThirty", func(t *testing.T) {
		searcher := &mockSearcher{itemCount: 30}
		tool := &WebSearchTool{searcher: searcher}

		_, err := tool.Call(context.Background(), &SearchInput{
			Query: "test query",
			Limit: 100,
		})

		assert.NoError(t, err)
		assert.Equal(t, 30, searcher.receivedLimit, "Should cap limit at 30")
	})

	t.Run("AcceptsValidLimitInRange", func(t *testing.T) {
		testCases := []int{1, 5, 10, 15, 20, 25, 30}

		for _, limit := range testCases {
			searcher := &mockSearcher{itemCount: limit}
			tool := &WebSearchTool{searcher: searcher}

			_, err := tool.Call(context.Background(), &SearchInput{
				Query: "test query",
				Limit: limit,
			})

			assert.NoError(t, err)
			assert.Equal(t, limit, searcher.receivedLimit, "Should accept limit %d", limit)
		}
	})
}

func TestWebSearchTool_Metadata(t *testing.T) {
	tool := &WebSearchTool{}

	assert.Equal(t, "WebSearch", tool.Name())
	assert.Contains(t, tool.Description(), "Searches the web")

	annotations := tool.Annotations()
	assert.NotNil(t, annotations)
	assert.True(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
	assert.True(t, annotations.OpenWorldHint)
}

func TestWebSearchTool_Schema(t *testing.T) {
	tool := &WebSearchTool{}
	schema := tool.Schema()

	assert.NotNil(t, schema)
	assert.Equal(t, "object", string(schema.Type))
	assert.Contains(t, schema.Required, "query")
	assert.Contains(t, schema.Properties, "query")
	assert.Contains(t, schema.Properties, "limit")
}

func TestWebSearchTool_NoResults(t *testing.T) {
	searcher := &mockSearcher{itemCount: 0}
	tool := &WebSearchTool{searcher: searcher}

	result, err := tool.Call(context.Background(), &SearchInput{
		Query: "test query",
		Limit: 10,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No search results found")
}
