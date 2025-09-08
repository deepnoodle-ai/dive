package toolkit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/openai/openai-go"
	"github.com/stretchr/testify/require"
)

func TestImageGenerationTool_Schema(t *testing.T) {
	client := openai.NewClient()
	tool := &ImageGenerationTool{client: &client}

	schema := tool.Schema()
	require.NotNil(t, schema)
	require.Equal(t, "object", string(schema.Type))
	require.Contains(t, schema.Required, "prompt")

	// Check that all expected properties exist
	expectedProps := []string{"prompt", "model", "size", "quality", "n", "output_path"}
	for _, prop := range expectedProps {
		require.Contains(t, schema.Properties, prop, "Schema should contain property: %s", prop)
	}

	// Verify prompt is required
	promptProp := schema.Properties["prompt"]
	require.Equal(t, "string", string(promptProp.Type))
	require.NotEmpty(t, promptProp.Description)

	// Verify model enum values (should include both OpenAI and Google models)
	modelProp := schema.Properties["model"]
	require.Contains(t, modelProp.Enum, "gpt-image-1")
	require.Contains(t, modelProp.Enum, "dall-e-2")
	require.Contains(t, modelProp.Enum, "dall-e-3")
	require.Contains(t, modelProp.Enum, "imagen-3.0-generate-001")
	require.Contains(t, modelProp.Enum, "imagen-3.0-generate-002")

	// Verify quality enum values
	qualityProp := schema.Properties["quality"]
	require.Equal(t, []string{"high", "medium", "low", "auto"}, qualityProp.Enum)

	// Verify n constraints
	nProp := schema.Properties["n"]
	require.Equal(t, "integer", string(nProp.Type))
}

func TestImageGenerationTool_Metadata(t *testing.T) {
	client := openai.NewClient()
	tool := &ImageGenerationTool{client: &client}

	require.Equal(t, "image_generation", tool.Name())
	require.Contains(t, tool.Description(), "Generate images")

	annotations := tool.Annotations()
	require.NotNil(t, annotations)
	require.Equal(t, "Image Generation", annotations.Title)
	require.False(t, annotations.ReadOnlyHint)
	require.False(t, annotations.DestructiveHint)
	require.False(t, annotations.IdempotentHint)
	require.True(t, annotations.OpenWorldHint)
}

func TestImageGenerationTool_InputValidation(t *testing.T) {
	client := openai.NewClient()
	tool := &ImageGenerationTool{client: &client}
	ctx := context.Background()

	// Test missing prompt
	result, err := tool.Call(ctx, &ImageGenerationInput{})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "prompt is required")

	// Test invalid call
	result, err = tool.Call(ctx, &ImageGenerationInput{
		Prompt: "test prompt",
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "output_path is required")
}

func TestImageGenInput_JSONSerialization(t *testing.T) {
	// Test that the ImageGenInput can be properly serialized and deserialized
	input := &ImageGenerationInput{
		Prompt:     "A beautiful sunset",
		Model:      "dall-e-3",
		Size:       "1024x1024",
		Quality:    "hd",
		N:          1,
		OutputPath: "./test_image.png",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(input)
	require.NoError(t, err)

	// Unmarshal from JSON
	var unmarshaled ImageGenerationInput
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Verify all fields were preserved
	require.Equal(t, input.Prompt, unmarshaled.Prompt)
	require.Equal(t, input.Model, unmarshaled.Model)
	require.Equal(t, input.Size, unmarshaled.Size)
	require.Equal(t, input.Quality, unmarshaled.Quality)
	require.Equal(t, input.N, unmarshaled.N)
	require.Equal(t, input.OutputPath, unmarshaled.OutputPath)
}

func TestNewImageGenerationTool(t *testing.T) {
	client := openai.NewClient()
	adapter := NewImageGenerationTool(ImageGenerationToolOptions{Client: &client})

	require.NotNil(t, adapter)

	// Verify it implements the expected interface
	var _ dive.Tool = adapter
}
