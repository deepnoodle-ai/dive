package toolkit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/diveagents/dive"
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
	expectedProps := []string{"prompt", "model", "size", "quality", "style", "response_format", "n", "save_to_file"}
	for _, prop := range expectedProps {
		require.Contains(t, schema.Properties, prop, "Schema should contain property: %s", prop)
	}

	// Verify prompt is required
	promptProp := schema.Properties["prompt"]
	require.Equal(t, "string", string(promptProp.Type))
	require.NotEmpty(t, promptProp.Description)

	// Verify model enum values
	modelProp := schema.Properties["model"]
	require.Equal(t, []string{"dall-e-2", "dall-e-3"}, modelProp.Enum)

	// Verify size enum values
	sizeProp := schema.Properties["size"]
	expectedSizes := []string{"256x256", "512x512", "1024x1024", "1792x1024", "1024x1792"}
	require.Equal(t, expectedSizes, sizeProp.Enum)

	// Verify quality enum values
	qualityProp := schema.Properties["quality"]
	require.Equal(t, []string{"standard", "hd"}, qualityProp.Enum)

	// Verify style enum values
	styleProp := schema.Properties["style"]
	require.Equal(t, []string{"vivid", "natural"}, styleProp.Enum)

	// Verify response_format enum values
	formatProp := schema.Properties["response_format"]
	require.Equal(t, []string{"url", "b64_json"}, formatProp.Enum)

	// Verify n constraints
	nProp := schema.Properties["n"]
	require.Equal(t, "integer", string(nProp.Type))
	require.NotNil(t, nProp.Minimum)
	require.Equal(t, 1.0, *nProp.Minimum)
	require.NotNil(t, nProp.Maximum)
	require.Equal(t, 10.0, *nProp.Maximum)
}

func TestImageGenerationTool_Metadata(t *testing.T) {
	client := openai.NewClient()
	tool := &ImageGenerationTool{client: &client}

	require.Equal(t, "openai_image_gen", tool.Name())
	require.Contains(t, tool.Description(), "DALL-E")

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
	require.Contains(t, result.Content[0].Text, "Prompt is required")

	// Test invalid model
	invalidModel := "invalid-model"
	result, err = tool.Call(ctx, &ImageGenerationInput{
		Prompt: "test prompt",
		Model:  invalidModel,
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "Invalid model")

	// Test invalid quality
	invalidQuality := "invalid-quality"
	result, err = tool.Call(ctx, &ImageGenerationInput{
		Prompt:  "test prompt",
		Quality: invalidQuality,
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "Invalid quality")
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
