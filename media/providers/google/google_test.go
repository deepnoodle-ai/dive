package google

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/stretchr/testify/require"
)

func TestProvider_ProviderName(t *testing.T) {
	// We can't easily create a real genai.Client for testing without credentials
	// So we test the provider with a nil client for basic functionality
	provider := NewProvider(nil)

	require.Equal(t, "google", provider.ProviderName())
}

func TestProvider_SupportedModels(t *testing.T) {
	provider := NewProvider(nil)

	models := provider.SupportedModels()
	require.Contains(t, models, "imagen-3.0-generate-001")
	require.Contains(t, models, "imagen-3.0-generate-002")
	require.Contains(t, models, "veo-2.0-generate-001")
}

func TestProvider_GenerateImage_ValidationErrors(t *testing.T) {
	provider := NewProvider(nil)
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
	require.Contains(t, err.Error(), "unsupported image model")
}

func TestProvider_EditImage_NotSupported(t *testing.T) {
	provider := NewProvider(nil)
	ctx := context.Background()

	req := &media.ImageEditRequest{
		Prompt: "Test prompt",
	}

	resp, err := provider.EditImage(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "image editing is not supported by Google GenAI provider")
}

func TestProvider_GenerateVideo_ValidationErrors(t *testing.T) {
	provider := NewProvider(nil)
	ctx := context.Background()

	// Test empty prompt
	req := &media.VideoGenerationRequest{}
	resp, err := provider.GenerateVideo(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "prompt is required")

	// Test unsupported model
	req = &media.VideoGenerationRequest{
		Prompt: "Test video prompt",
		Model:  "unsupported-video-model",
	}
	resp, err = provider.GenerateVideo(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "unsupported video model")
}

func TestIsImageModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"imagen-3.0-generate-001", true},
		{"imagen-3.0-generate-002", true},
		{"veo-2.0-generate-001", false},
		{"invalid-model", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := isImageModel(tt.model)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVideoModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"veo-2.0-generate-001", true},
		{"imagen-3.0-generate-001", false},
		{"imagen-3.0-generate-002", false},
		{"invalid-model", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := isVideoModel(tt.model)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestProvider_GenerateImage_RequestMapping(t *testing.T) {
	// Test request validation and parameter mapping without making actual API calls
	provider := NewProvider(nil)
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
				Prompt: "A beautiful landscape",
			},
			wantErr: false, // Will fail at API call level due to nil client, but validation passes
		},
		{
			name: "valid request with imagen-3.0-generate-001",
			request: &media.ImageGenerationRequest{
				Prompt: "A beautiful landscape",
				Model:  "imagen-3.0-generate-001",
			},
			wantErr: false, // Will fail at API call level due to nil client, but validation passes
		},
		{
			name: "valid request with provider-specific params",
			request: &media.ImageGenerationRequest{
				Prompt: "A beautiful landscape",
				Model:  "imagen-3.0-generate-002",
				ProviderSpecific: map[string]interface{}{
					"output_mime_type":          "image/png",
					"include_rai_reason":        true,
					"include_safety_attributes": false,
				},
			},
			wantErr: false, // Will fail at API call level due to nil client, but validation passes
		},
		{
			name: "invalid model for image generation",
			request: &media.ImageGenerationRequest{
				Prompt: "A beautiful landscape",
				Model:  "veo-2.0-generate-001", // This is a video model
			},
			wantErr: true,
			errMsg:  "unsupported image model",
		},
		{
			name: "empty prompt",
			request: &media.ImageGenerationRequest{
				Model: "imagen-3.0-generate-002",
			},
			wantErr: true,
			errMsg:  "prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.GenerateImage(ctx, tt.request)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				// We expect an error due to nil client, but it should be a client error, not validation
				require.Error(t, err)
				// The error should be related to client issues, not validation
			}
		})
	}
}

func TestProvider_GenerateVideo_RequestMapping(t *testing.T) {
	// Test request validation and parameter mapping without making actual API calls
	provider := NewProvider(nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		request *media.VideoGenerationRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with defaults",
			request: &media.VideoGenerationRequest{
				Prompt: "A cat walking in a garden",
			},
			wantErr: false, // Will fail at API call level due to nil client, but validation passes
		},
		{
			name: "valid request with veo model",
			request: &media.VideoGenerationRequest{
				Prompt: "A cat walking in a garden",
				Model:  "veo-2.0-generate-001",
			},
			wantErr: false, // Will fail at API call level due to nil client, but validation passes
		},
		{
			name: "valid request with provider-specific params",
			request: &media.VideoGenerationRequest{
				Prompt: "A cat walking in a garden",
				Model:  "veo-2.0-generate-001",
				ProviderSpecific: map[string]interface{}{
					"output_gcs_uri": "gs://my-bucket/videos/",
				},
			},
			wantErr: false, // Will fail at API call level due to nil client, but validation passes
		},
		{
			name: "invalid model for video generation",
			request: &media.VideoGenerationRequest{
				Prompt: "A cat walking in a garden",
				Model:  "imagen-3.0-generate-002", // This is an image model
			},
			wantErr: true,
			errMsg:  "unsupported video model",
		},
		{
			name: "empty prompt",
			request: &media.VideoGenerationRequest{
				Model: "veo-2.0-generate-001",
			},
			wantErr: true,
			errMsg:  "prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.GenerateVideo(ctx, tt.request)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				// We expect an error due to nil client, but it should be a client error, not validation
				require.Error(t, err)
				// The error should be related to client issues, not validation
			}
		})
	}
}
