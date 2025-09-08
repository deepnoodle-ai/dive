package openai

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	openaiapi "github.com/openai/openai-go"
	"github.com/stretchr/testify/require"
)

func TestProvider_ProviderName(t *testing.T) {
	client := openaiapi.NewClient()
	provider := NewProvider(&client)

	require.Equal(t, "openai", provider.ProviderName())
}

func TestProvider_SupportedModels(t *testing.T) {
	client := openaiapi.NewClient()
	provider := NewProvider(&client)

	models := provider.SupportedModels()
	require.Contains(t, models, "dall-e-2")
	require.Contains(t, models, "dall-e-3")
	require.Contains(t, models, "gpt-image-1")
}

func TestProvider_GenerateImage_ValidationErrors(t *testing.T) {
	client := openaiapi.NewClient()
	provider := NewProvider(&client)
	ctx := context.Background()

	// Test empty prompt
	req := &media.ImageGenerationRequest{}
	resp, err := provider.GenerateImage(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "prompt is required")

	// Test unsupported model
	req = &media.ImageGenerationRequest{
		Prompt: "Test prompt",
		Model:  "unsupported-model",
	}
	resp, err = provider.GenerateImage(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "unsupported model")
}

func TestProvider_EditImage_ValidationErrors(t *testing.T) {
	client := openaiapi.NewClient()
	provider := NewProvider(&client)
	ctx := context.Background()

	// Test empty prompt
	req := &media.ImageEditRequest{}
	resp, err := provider.EditImage(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "prompt is required")

	// Test missing image
	req = &media.ImageEditRequest{
		Prompt: "Test prompt",
	}
	resp, err = provider.EditImage(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "image is required")

	// Test unsupported model
	imageReader := strings.NewReader("fake image data")
	req = &media.ImageEditRequest{
		Image:  imageReader,
		Prompt: "Test prompt",
		Model:  "dall-e-3", // Only dall-e-2 supports editing
	}
	resp, err = provider.EditImage(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "only dall-e-2 supports image editing")
}

func TestProvider_GenerateVideo_NotSupported(t *testing.T) {
	client := openaiapi.NewClient()
	provider := NewProvider(&client)
	ctx := context.Background()

	req := &media.VideoGenerationRequest{
		Prompt: "Test video prompt",
	}

	resp, err := provider.GenerateVideo(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "video generation is not supported by OpenAI provider")
}

func TestDecodeBase64Image(t *testing.T) {
	// Test valid base64
	b64Data := "dGVzdA==" // base64 for "test"
	decoded, err := DecodeBase64Image(b64Data)
	require.NoError(t, err)
	require.Equal(t, "test", string(decoded))

	// Test invalid base64
	invalidB64 := "invalid-base64!"
	_, err = DecodeBase64Image(invalidB64)
	require.Error(t, err)
}

func TestEncodeImageToBase64(t *testing.T) {
	imageData := []byte("test image data")
	encoded := EncodeImageToBase64(imageData)
	require.NotEmpty(t, encoded)

	// Verify we can decode it back
	decoded, err := DecodeBase64Image(encoded)
	require.NoError(t, err)
	require.Equal(t, imageData, decoded)
}
