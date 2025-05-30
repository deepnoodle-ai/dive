package openai

import (
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/assert"
)

func TestWebSearchTool_Implementation(t *testing.T) {
	// Test that WebSearchTool implements the required interfaces
	tool := NewWebSearchTool(WebSearchToolOptions{})

	// Check that it implements llm.Tool interface
	assert.Implements(t, (*llm.Tool)(nil), tool)

	// Check that it implements llm.ToolConfiguration interface
	assert.Implements(t, (*llm.ToolConfiguration)(nil), tool)
}

func TestWebSearchTool_BasicProperties(t *testing.T) {
	tool := NewWebSearchTool(WebSearchToolOptions{})

	assert.Equal(t, "web_search", tool.Name())
	assert.Equal(t, "Uses OpenAI's web search feature to give models direct access to real-time web content.", tool.Description())

	// Schema should be empty for server-side tools
	schema := tool.Schema()
	assert.Empty(t, schema.Type)
	assert.Empty(t, schema.Properties)
}

func TestWebSearchTool_ConfigurationOptions(t *testing.T) {
	tests := []struct {
		name     string
		options  WebSearchToolOptions
		expected map[string]any
	}{
		{
			name:    "default configuration",
			options: WebSearchToolOptions{},
			expected: map[string]any{
				"type": "web_search_preview",
			},
		},
		{
			name: "with domains",
			options: WebSearchToolOptions{
				Domains: []string{"arxiv.org", "openai.com"},
			},
			expected: map[string]any{
				"type":    "web_search_preview",
				"domains": []string{"arxiv.org", "openai.com"},
			},
		},
		{
			name: "with search context size",
			options: WebSearchToolOptions{
				SearchContextSize: "medium",
			},
			expected: map[string]any{
				"type":                "web_search_preview",
				"search_context_size": "medium",
			},
		},
		{
			name: "with user location",
			options: WebSearchToolOptions{
				UserLocation: &UserLocation{
					Type:    "approximate",
					Country: "US",
				},
			},
			expected: map[string]any{
				"type": "web_search_preview",
				"user_location": &UserLocation{
					Type:    "approximate",
					Country: "US",
				},
			},
		},
		{
			name: "with all options",
			options: WebSearchToolOptions{
				Domains:           []string{"arxiv.org", "openai.com", "github.com"},
				SearchContextSize: "high",
				UserLocation: &UserLocation{
					Type:     "exact",
					City:     "San Francisco",
					Country:  "US",
					Region:   "California",
					Timezone: "America/Los_Angeles",
				},
			},
			expected: map[string]any{
				"type":                "web_search_preview",
				"domains":             []string{"arxiv.org", "openai.com", "github.com"},
				"search_context_size": "high",
				"user_location": &UserLocation{
					Type:     "exact",
					City:     "San Francisco",
					Country:  "US",
					Region:   "California",
					Timezone: "America/Los_Angeles",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewWebSearchTool(tt.options)
			config := tool.ToolConfiguration("openai")

			assert.Equal(t, tt.expected, config)
		})
	}
}

func TestWebSearchTool_SearchContextSizes(t *testing.T) {
	validSizes := []string{"low", "medium", "high"}

	for _, size := range validSizes {
		t.Run(size, func(t *testing.T) {
			tool := NewWebSearchTool(WebSearchToolOptions{
				SearchContextSize: size,
			})

			config := tool.ToolConfiguration("openai")
			assert.Equal(t, size, config["search_context_size"])
		})
	}
}

func TestWebSearchTool_UserLocationTypes(t *testing.T) {
	tests := []struct {
		name     string
		location *UserLocation
	}{
		{
			name: "approximate location",
			location: &UserLocation{
				Type:    "approximate",
				Country: "US",
			},
		},
		{
			name: "exact location",
			location: &UserLocation{
				Type:     "exact",
				City:     "San Francisco",
				Country:  "US",
				Region:   "California",
				Timezone: "America/Los_Angeles",
			},
		},
		{
			name: "location with minimal fields",
			location: &UserLocation{
				Type: "approximate",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewWebSearchTool(WebSearchToolOptions{
				UserLocation: tt.location,
			})

			config := tool.ToolConfiguration("openai")
			assert.Equal(t, tt.location, config["user_location"])
		})
	}
}

func TestWebSearchTool_EmptyAndNilConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		options WebSearchToolOptions
	}{
		{
			name:    "empty options",
			options: WebSearchToolOptions{},
		},
		{
			name: "empty domains slice",
			options: WebSearchToolOptions{
				Domains: []string{},
			},
		},
		{
			name: "empty search context size",
			options: WebSearchToolOptions{
				SearchContextSize: "",
			},
		},
		{
			name: "nil user location",
			options: WebSearchToolOptions{
				UserLocation: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewWebSearchTool(tt.options)
			config := tool.ToolConfiguration("openai")

			// Should always have type
			assert.Equal(t, "web_search_preview", config["type"])

			// Empty/nil fields should not be present in config
			if len(tt.options.Domains) == 0 {
				_, hasDomains := config["domains"]
				assert.False(t, hasDomains, "empty domains should not be in config")
			}

			if tt.options.SearchContextSize == "" {
				_, hasSize := config["search_context_size"]
				assert.False(t, hasSize, "empty search context size should not be in config")
			}

			if tt.options.UserLocation == nil {
				_, hasLocation := config["user_location"]
				assert.False(t, hasLocation, "nil user location should not be in config")
			}
		})
	}
}

func TestWebSearchTool_ProviderAgnostic(t *testing.T) {
	// Test that the tool configuration works with different provider names
	tool := NewWebSearchTool(WebSearchToolOptions{
		Domains: []string{"example.com"},
	})

	providers := []string{"openai", "anthropic", "other"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			config := tool.ToolConfiguration(provider)

			// Configuration should be the same regardless of provider
			expected := map[string]any{
				"type":    "web_search_preview",
				"domains": []string{"example.com"},
			}
			assert.Equal(t, expected, config)
		})
	}
}
