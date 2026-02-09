package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

type weatherInput struct {
	City  string `json:"city" description:"City name"`
	Units string `json:"units,omitempty" description:"Temperature units" enum:"celsius,fahrenheit"`
}

func TestFuncTool(t *testing.T) {
	t.Run("creates tool with auto-generated schema", func(t *testing.T) {
		tool := FuncTool("get_weather", "Get current weather",
			func(ctx context.Context, input *weatherInput) (*ToolResult, error) {
				return NewToolResultText(input.City + " is sunny"), nil
			},
		)

		assert.Equal(t, tool.Name(), "get_weather")
		assert.Equal(t, tool.Description(), "Get current weather")

		s := tool.Schema()
		assert.NotNil(t, s)
		assert.Equal(t, s.Type, Object)
		assert.NotNil(t, s.Properties["city"])
		assert.Equal(t, s.Properties["city"].Description, "City name")
		assert.NotNil(t, s.Properties["units"])
		assert.Equal(t, s.Properties["units"].Description, "Temperature units")

		// city is required (no omitempty), units is optional
		assert.Contains(t, s.Required, "city")
	})

	t.Run("executes function correctly", func(t *testing.T) {
		tool := FuncTool("get_weather", "Get current weather",
			func(ctx context.Context, input *weatherInput) (*ToolResult, error) {
				return NewToolResultText(input.City + " is sunny"), nil
			},
		)

		result, err := tool.Call(context.Background(), []byte(`{"city":"Paris"}`))
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, result.Content[0].Text, "Paris is sunny")
	})

	t.Run("with annotations option", func(t *testing.T) {
		tool := FuncTool("get_weather", "Get current weather",
			func(ctx context.Context, input *weatherInput) (*ToolResult, error) {
				return NewToolResultText("ok"), nil
			},
			WithFuncToolAnnotations(&ToolAnnotations{
				ReadOnlyHint:  true,
				OpenWorldHint: true,
				Title:         "Weather",
			}),
		)

		a := tool.Annotations()
		assert.NotNil(t, a)
		assert.True(t, a.ReadOnlyHint)
		assert.True(t, a.OpenWorldHint)
		assert.Equal(t, a.Title, "Weather")
	})

	t.Run("with schema override", func(t *testing.T) {
		customSchema := &Schema{
			Type: Object,
			Properties: map[string]*SchemaProperty{
				"custom_field": {Type: String, Description: "Custom field"},
			},
			Required: []string{"custom_field"},
		}

		tool := FuncTool("custom", "Custom tool",
			func(ctx context.Context, input *weatherInput) (*ToolResult, error) {
				return NewToolResultText("ok"), nil
			},
			WithFuncToolSchema(customSchema),
		)

		s := tool.Schema()
		assert.NotNil(t, s)
		assert.NotNil(t, s.Properties["custom_field"])
		assert.Equal(t, s.Properties["custom_field"].Description, "Custom field")
	})

	t.Run("nil annotations by default", func(t *testing.T) {
		tool := FuncTool("test", "Test tool",
			func(ctx context.Context, input *weatherInput) (*ToolResult, error) {
				return NewToolResultText("ok"), nil
			},
		)
		assert.Nil(t, tool.Annotations())
	})
}

func TestFuncToolWithStructInput(t *testing.T) {
	type searchInput struct {
		Query   string   `json:"query" description:"Search query string"`
		Limit   int      `json:"limit,omitempty" description:"Max results" default:"10"`
		Filters []string `json:"filters,omitempty" description:"Filter expressions"`
	}

	tool := FuncTool("search", "Search for things",
		func(ctx context.Context, input searchInput) (*ToolResult, error) {
			return NewToolResultText(input.Query), nil
		},
	)

	s := tool.Schema()
	assert.NotNil(t, s)
	assert.NotNil(t, s.Properties["query"])
	assert.NotNil(t, s.Properties["limit"])
	assert.NotNil(t, s.Properties["filters"])
	assert.Contains(t, s.Required, "query")

	// Execute it
	result, err := tool.Call(context.Background(), []byte(`{"query":"hello"}`))
	assert.NoError(t, err)
	assert.Equal(t, result.Content[0].Text, "hello")
}
