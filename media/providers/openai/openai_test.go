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

func TestProvider_GenerateImage_RequestMapping(t *testing.T) {
	// This test verifies that the request is properly mapped to OpenAI parameters
	// We can't test the actual API call without mocking, but we can test the validation logic

	client := openaiapi.NewClient()
	provider := NewProvider(&client)
	ctx := context.Background()

	tests := []struct {
		name    string
		request *media.ImageGenerationRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with defaults",
			request: &media.ImageGenerationRequest{
				Prompt: "A beautiful sunset",
			},
			wantErr: false,
		},
		{
			name: "valid request with dall-e-2",
			request: &media.ImageGenerationRequest{
				Prompt: "A beautiful sunset",
				Model:  "dall-e-2",
				Size:   "512x512",
				Count:  1,
			},
			wantErr: false,
		},
		{
			name: "valid request with dall-e-3",
			request: &media.ImageGenerationRequest{
				Prompt:  "A beautiful sunset",
				Model:   "dall-e-3",
				Size:    "1024x1024",
				Quality: "hd",
				Count:   1,
			},
			wantErr: false,
		},
		{
			name: "valid request with gpt-image-1",
			request: &media.ImageGenerationRequest{
				Prompt:  "A beautiful sunset",
				Model:   "gpt-image-1",
				Size:    "auto",
				Quality: "high",
				Count:   2,
				ProviderSpecific: map[string]interface{}{
					"moderation":         "low",
					"output_format":      "webp",
					"output_compression": 80,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid model",
			request: &media.ImageGenerationRequest{
				Prompt: "A beautiful sunset",
				Model:  "invalid-model",
			},
			wantErr: true,
			errMsg:  "unsupported model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We expect these to fail at the API call level since we don't have valid credentials
			// But they should pass validation and parameter mapping
			_, err := provider.GenerateImage(ctx, tt.request)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			}
			// Note: We can't test success cases without mocking the OpenAI client
			// The validation logic is tested above
		})
	}
}
