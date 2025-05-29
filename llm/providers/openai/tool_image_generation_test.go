package openai

import (
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageGenerationTool_Interfaces(t *testing.T) {
	tool := NewImageGenerationTool(ImageGenerationToolOptions{})

	// Verify it implements the required interfaces
	var _ llm.Tool = tool
	var _ llm.ToolConfiguration = tool
}

func TestImageGenerationTool_Basic(t *testing.T) {
	tool := NewImageGenerationTool(ImageGenerationToolOptions{})

	assert.Equal(t, "image_generation", tool.Name())
	assert.Contains(t, tool.Description(), "image generation")
	assert.Equal(t, schema.Schema{}, tool.Schema()) // Empty for server-side tools
}

func TestImageGenerationTool_ToolConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		opts     ImageGenerationToolOptions
		expected map[string]any
	}{
		{
			name: "minimal configuration",
			opts: ImageGenerationToolOptions{},
			expected: map[string]any{
				"type": "image_generation",
			},
		},
		{
			name: "full configuration",
			opts: ImageGenerationToolOptions{
				Size:          "1024x1024",
				Quality:       "high",
				Background:    "auto",
				Compression:   &[]int{80}[0],
				PartialImages: &[]int{2}[0],
			},
			expected: map[string]any{
				"type":           "image_generation",
				"size":           "1024x1024",
				"quality":        "high",
				"background":     "auto",
				"compression":    80,
				"partial_images": 2,
			},
		},
		{
			name: "partial configuration",
			opts: ImageGenerationToolOptions{
				Size:    "512x512",
				Quality: "medium",
			},
			expected: map[string]any{
				"type":    "image_generation",
				"size":    "512x512",
				"quality": "medium",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewImageGenerationTool(tt.opts)
			config := tool.ToolConfiguration("openai-responses")

			for key, expectedValue := range tt.expected {
				actualValue, exists := config[key]
				require.True(t, exists, "Expected key %s to exist in config", key)
				assert.Equal(t, expectedValue, actualValue, "Mismatch for key %s", key)
			}

			// Verify no unexpected keys
			assert.Len(t, config, len(tt.expected), "Config should only contain expected keys")
		})
	}
}
