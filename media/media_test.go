package media

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// MockImageGenerator is a mock implementation for testing
type MockImageGenerator struct {
	GenerateImageFunc   func(ctx context.Context, req *ImageGenerationRequest) (*ImageGenerationResponse, error)
	EditImageFunc       func(ctx context.Context, req *ImageEditRequest) (*ImageEditResponse, error)
	SupportedModelsFunc func() []string
	ProviderNameFunc    func() string
}

func (m *MockImageGenerator) GenerateImage(ctx context.Context, req *ImageGenerationRequest) (*ImageGenerationResponse, error) {
	if m.GenerateImageFunc != nil {
		return m.GenerateImageFunc(ctx, req)
	}
	return &ImageGenerationResponse{
		Images: []GeneratedImage{
			{
				B64JSON: "dGVzdA==", // base64 for "test"
			},
		},
	}, nil
}

func (m *MockImageGenerator) EditImage(ctx context.Context, req *ImageEditRequest) (*ImageEditResponse, error) {
	if m.EditImageFunc != nil {
		return m.EditImageFunc(ctx, req)
	}
	return &ImageEditResponse{
		Images: []GeneratedImage{
			{
				B64JSON: "ZWRpdGVk", // base64 for "edited"
			},
		},
	}, nil
}

func (m *MockImageGenerator) SupportedModels() []string {
	if m.SupportedModelsFunc != nil {
		return m.SupportedModelsFunc()
	}
	return []string{"test-model-1", "test-model-2"}
}

func (m *MockImageGenerator) ProviderName() string {
	if m.ProviderNameFunc != nil {
		return m.ProviderNameFunc()
	}
	return "mock"
}

// MockVideoGenerator is a mock implementation for testing
type MockVideoGenerator struct {
	GenerateVideoFunc   func(ctx context.Context, req *VideoGenerationRequest) (*VideoGenerationResponse, error)
	SupportedModelsFunc func() []string
	ProviderNameFunc    func() string
}

func (m *MockVideoGenerator) GenerateVideo(ctx context.Context, req *VideoGenerationRequest) (*VideoGenerationResponse, error) {
	if m.GenerateVideoFunc != nil {
		return m.GenerateVideoFunc(ctx, req)
	}
	return &VideoGenerationResponse{
		Videos: []GeneratedVideo{
			{
				URL:      "https://example.com/video.mp4",
				Duration: 10.0,
				Format:   "mp4",
			},
		},
		Status: "completed",
	}, nil
}

func (m *MockVideoGenerator) SupportedModels() []string {
	if m.SupportedModelsFunc != nil {
		return m.SupportedModelsFunc()
	}
	return []string{"test-video-model-1"}
}

func (m *MockVideoGenerator) ProviderName() string {
	if m.ProviderNameFunc != nil {
		return m.ProviderNameFunc()
	}
	return "mock"
}

func TestImageGenerationRequest(t *testing.T) {
	req := &ImageGenerationRequest{
		Prompt:  "A beautiful sunset",
		Model:   "test-model",
		Size:    "1024x1024",
		Quality: "high",
		Count:   1,
	}

	require.Equal(t, "A beautiful sunset", req.Prompt)
	require.Equal(t, "test-model", req.Model)
	require.Equal(t, "1024x1024", req.Size)
	require.Equal(t, "high", req.Quality)
	require.Equal(t, 1, req.Count)
}

func TestImageGenerationResponse(t *testing.T) {
	resp := &ImageGenerationResponse{
		Images: []GeneratedImage{
			{
				B64JSON:       "dGVzdA==",
				RevisedPrompt: "A beautiful sunset over mountains",
			},
		},
		Usage: &Usage{
			Tokens: 100,
			Cost:   0.02,
		},
	}

	require.Len(t, resp.Images, 1)
	require.Equal(t, "dGVzdA==", resp.Images[0].B64JSON)
	require.Equal(t, "A beautiful sunset over mountains", resp.Images[0].RevisedPrompt)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 100, resp.Usage.Tokens)
	require.Equal(t, 0.02, resp.Usage.Cost)
}

func TestVideoGenerationRequest(t *testing.T) {
	req := &VideoGenerationRequest{
		Prompt:     "A cat walking in a garden",
		Model:      "video-model",
		Duration:   10.5,
		Resolution: "1080p",
		Format:     "mp4",
	}

	require.Equal(t, "A cat walking in a garden", req.Prompt)
	require.Equal(t, "video-model", req.Model)
	require.Equal(t, 10.5, req.Duration)
	require.Equal(t, "1080p", req.Resolution)
	require.Equal(t, "mp4", req.Format)
}

func TestVideoGenerationResponse(t *testing.T) {
	resp := &VideoGenerationResponse{
		Videos: []GeneratedVideo{
			{
				URL:        "https://example.com/video.mp4",
				Duration:   10.0,
				Format:     "mp4",
				Resolution: "1080p",
			},
		},
		Status:      "completed",
		OperationID: "op-123",
	}

	require.Len(t, resp.Videos, 1)
	require.Equal(t, "https://example.com/video.mp4", resp.Videos[0].URL)
	require.Equal(t, 10.0, resp.Videos[0].Duration)
	require.Equal(t, "mp4", resp.Videos[0].Format)
	require.Equal(t, "1080p", resp.Videos[0].Resolution)
	require.Equal(t, "completed", resp.Status)
	require.Equal(t, "op-123", resp.OperationID)
}

func TestMockImageGenerator(t *testing.T) {
	mock := &MockImageGenerator{}

	// Test provider name
	require.Equal(t, "mock", mock.ProviderName())

	// Test supported models
	models := mock.SupportedModels()
	require.Contains(t, models, "test-model-1")
	require.Contains(t, models, "test-model-2")

	// Test image generation
	ctx := context.Background()
	req := &ImageGenerationRequest{
		Prompt: "Test prompt",
		Count:  1,
	}

	resp, err := mock.GenerateImage(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Images, 1)
	require.Equal(t, "dGVzdA==", resp.Images[0].B64JSON)
}

func TestMockVideoGenerator(t *testing.T) {
	mock := &MockVideoGenerator{}

	// Test provider name
	require.Equal(t, "mock", mock.ProviderName())

	// Test supported models
	models := mock.SupportedModels()
	require.Contains(t, models, "test-video-model-1")

	// Test video generation
	ctx := context.Background()
	req := &VideoGenerationRequest{
		Prompt: "Test video prompt",
	}

	resp, err := mock.GenerateVideo(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Videos, 1)
	require.Equal(t, "https://example.com/video.mp4", resp.Videos[0].URL)
	require.Equal(t, "completed", resp.Status)
}

func TestImageEditRequest(t *testing.T) {
	imageReader := strings.NewReader("fake image data")
	maskReader := strings.NewReader("fake mask data")

	req := &ImageEditRequest{
		Image:  imageReader,
		Prompt: "Make the sky blue",
		Mask:   maskReader,
		Model:  "edit-model",
		Size:   "512x512",
		Count:  1,
	}

	require.NotNil(t, req.Image)
	require.Equal(t, "Make the sky blue", req.Prompt)
	require.NotNil(t, req.Mask)
	require.Equal(t, "edit-model", req.Model)
	require.Equal(t, "512x512", req.Size)
	require.Equal(t, 1, req.Count)
}

func TestOperationStatus(t *testing.T) {
	status := &OperationStatus{
		ID:       "op-123",
		Status:   "running",
		Progress: 50,
		Error:    "",
		Result:   nil,
	}

	require.Equal(t, "op-123", status.ID)
	require.Equal(t, "running", status.Status)
	require.Equal(t, 50, status.Progress)
	require.Empty(t, status.Error)
	require.Nil(t, status.Result)
}

func TestUsage(t *testing.T) {
	usage := &Usage{
		Tokens:   150,
		Credits:  0.5,
		Cost:     0.03,
		Currency: "USD",
	}

	require.Equal(t, 150, usage.Tokens)
	require.Equal(t, 0.5, usage.Credits)
	require.Equal(t, 0.03, usage.Cost)
	require.Equal(t, "USD", usage.Currency)
}

func TestValidateImageGenerationRequest_EdgeCases(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		err := ValidateImageGenerationRequest(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("empty prompt", func(t *testing.T) {
		req := &ImageGenerationRequest{
			Prompt: "",
		}
		err := ValidateImageGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("whitespace only prompt", func(t *testing.T) {
		req := &ImageGenerationRequest{
			Prompt: "   \t\n  ",
		}
		err := ValidateImageGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("negative count", func(t *testing.T) {
		req := &ImageGenerationRequest{
			Prompt: "test",
			Count:  -1,
		}
		err := ValidateImageGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be negative")
	})

	t.Run("count exceeds maximum", func(t *testing.T) {
		req := &ImageGenerationRequest{
			Prompt: "test",
			Count:  15,
		}
		err := ValidateImageGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot exceed 10")
	})

	t.Run("invalid response format", func(t *testing.T) {
		req := &ImageGenerationRequest{
			Prompt:         "test",
			ResponseFormat: "invalid",
		}
		err := ValidateImageGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid response format")
	})

	t.Run("valid request", func(t *testing.T) {
		req := &ImageGenerationRequest{
			Prompt:         "A beautiful sunset",
			Model:          "gpt-image-1",
			Size:           "1024x1024",
			Quality:        "high",
			Count:          2,
			ResponseFormat: "b64_json",
			ProviderSpecific: map[string]interface{}{
				"moderation": "auto",
			},
		}
		err := ValidateImageGenerationRequest(req)
		require.NoError(t, err)
	})
}

func TestValidateImageEditRequest_EdgeCases(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		err := ValidateImageEditRequest(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("nil image", func(t *testing.T) {
		req := &ImageEditRequest{
			Prompt: "test",
			Image:  nil,
		}
		err := ValidateImageEditRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "image is required")
	})

	t.Run("empty prompt", func(t *testing.T) {
		req := &ImageEditRequest{
			Prompt: "",
			Image:  &MockReader{},
		}
		err := ValidateImageEditRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("negative count", func(t *testing.T) {
		req := &ImageEditRequest{
			Prompt: "test",
			Image:  &MockReader{},
			Count:  -1,
		}
		err := ValidateImageEditRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be negative")
	})

	t.Run("valid request", func(t *testing.T) {
		req := &ImageEditRequest{
			Prompt: "Make it blue",
			Image:  &MockReader{},
			Mask:   &MockReader{},
			Model:  "dall-e-2",
			Size:   "512x512",
			Count:  1,
		}
		err := ValidateImageEditRequest(req)
		require.NoError(t, err)
	})
}

func TestValidateVideoGenerationRequest_EdgeCases(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		err := ValidateVideoGenerationRequest(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("empty prompt", func(t *testing.T) {
		req := &VideoGenerationRequest{
			Prompt: "",
		}
		err := ValidateVideoGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("negative duration", func(t *testing.T) {
		req := &VideoGenerationRequest{
			Prompt:   "test",
			Duration: -1.0,
		}
		err := ValidateVideoGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be negative")
	})

	t.Run("duration exceeds maximum", func(t *testing.T) {
		req := &VideoGenerationRequest{
			Prompt:   "test",
			Duration: 400.0, // Over 5 minutes
		}
		err := ValidateVideoGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot exceed 300 seconds")
	})

	t.Run("negative frame rate", func(t *testing.T) {
		req := &VideoGenerationRequest{
			Prompt:    "test",
			FrameRate: -1,
		}
		err := ValidateVideoGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be negative")
	})

	t.Run("frame rate exceeds maximum", func(t *testing.T) {
		req := &VideoGenerationRequest{
			Prompt:    "test",
			FrameRate: 70,
		}
		err := ValidateVideoGenerationRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot exceed 60 fps")
	})

	t.Run("valid request", func(t *testing.T) {
		req := &VideoGenerationRequest{
			Prompt:     "A cat walking in a garden",
			Model:      "veo-2.0-generate-001",
			Duration:   5.0,
			Resolution: "1080p",
			FrameRate:  30,
			Format:     "mp4",
		}
		err := ValidateVideoGenerationRequest(req)
		require.NoError(t, err)
	})
}

func TestGeneratedImage_Metadata(t *testing.T) {
	image := GeneratedImage{
		URL:           "https://example.com/image.jpg",
		B64JSON:       "base64data",
		RevisedPrompt: "A beautiful enhanced sunset",
		Metadata: map[string]interface{}{
			"width":  1024,
			"height": 1024,
			"format": "jpeg",
		},
	}

	require.Equal(t, "https://example.com/image.jpg", image.URL)
	require.Equal(t, "base64data", image.B64JSON)
	require.Equal(t, "A beautiful enhanced sunset", image.RevisedPrompt)
	require.NotNil(t, image.Metadata)
	require.Equal(t, 1024, image.Metadata["width"])
	require.Equal(t, 1024, image.Metadata["height"])
	require.Equal(t, "jpeg", image.Metadata["format"])
}

func TestGeneratedVideo_Metadata(t *testing.T) {
	video := GeneratedVideo{
		URL:        "https://example.com/video.mp4",
		B64Data:    "base64video",
		Duration:   10.5,
		Format:     "mp4",
		Resolution: "1920x1080",
		Metadata: map[string]interface{}{
			"bitrate": 5000000,
			"codec":   "h264",
		},
	}

	require.Equal(t, "https://example.com/video.mp4", video.URL)
	require.Equal(t, "base64video", video.B64Data)
	require.Equal(t, 10.5, video.Duration)
	require.Equal(t, "mp4", video.Format)
	require.Equal(t, "1920x1080", video.Resolution)
	require.NotNil(t, video.Metadata)
	require.Equal(t, 5000000, video.Metadata["bitrate"])
	require.Equal(t, "h264", video.Metadata["codec"])
}

func TestOperationStatus_States(t *testing.T) {
	tests := []struct {
		name     string
		status   OperationStatus
		expected string
	}{
		{
			name: "pending status",
			status: OperationStatus{
				ID:       "op-123",
				Status:   "pending",
				Progress: 0,
			},
			expected: "pending",
		},
		{
			name: "running status with progress",
			status: OperationStatus{
				ID:       "op-123",
				Status:   "running",
				Progress: 75,
			},
			expected: "running",
		},
		{
			name: "completed status",
			status: OperationStatus{
				ID:       "op-123",
				Status:   "completed",
				Progress: 100,
				Result: &VideoGenerationResponse{
					Status: "completed",
				},
			},
			expected: "completed",
		},
		{
			name: "failed status with error",
			status: OperationStatus{
				ID:     "op-123",
				Status: "failed",
				Error:  "Generation failed due to quota exceeded",
			},
			expected: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, "op-123", tt.status.ID)
			require.Equal(t, tt.expected, tt.status.Status)

			switch tt.expected {
			case "running":
				require.Equal(t, 75, tt.status.Progress)
			case "completed":
				require.Equal(t, 100, tt.status.Progress)
				require.NotNil(t, tt.status.Result)
			case "failed":
				require.Equal(t, "Generation failed due to quota exceeded", tt.status.Error)
			}
		})
	}
}

// MockReader is a simple io.Reader implementation for testing
type MockReader struct {
	data []byte
	pos  int
}

func (m *MockReader) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func NewMockReader(data string) *MockReader {
	return &MockReader{data: []byte(data)}
}

func TestGetImageEditCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantCaps *ImageEditCapabilities
		wantErr  bool
	}{
		{
			name:     "OpenAI provider",
			provider: "openai",
			wantCaps: &ImageEditCapabilities{
				SupportsMask:    true,
				SupportedModels: []string{"dall-e-2"},
				SupportedSizes:  []string{"256x256", "512x512", "1024x1024"},
				MaxImages:       10,
				ProviderName:    "openai",
			},
			wantErr: false,
		},
		{
			name:     "DALL-E alias",
			provider: "dalle",
			wantCaps: &ImageEditCapabilities{
				SupportsMask:    true,
				SupportedModels: []string{"dall-e-2"},
				SupportedSizes:  []string{"256x256", "512x512", "1024x1024"},
				MaxImages:       10,
				ProviderName:    "openai",
			},
			wantErr: false,
		},
		{
			name:     "Google provider (not supported)",
			provider: "google",
			wantCaps: nil,
			wantErr:  true,
		},
		{
			name:     "Unknown provider",
			provider: "unknown",
			wantCaps: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps, err := GetImageEditCapabilities(tt.provider)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, caps)
			} else {
				require.NoError(t, err)
				require.NotNil(t, caps)
				require.Equal(t, tt.wantCaps.SupportsMask, caps.SupportsMask)
				require.Equal(t, tt.wantCaps.SupportedModels, caps.SupportedModels)
				require.Equal(t, tt.wantCaps.SupportedSizes, caps.SupportedSizes)
				require.Equal(t, tt.wantCaps.MaxImages, caps.MaxImages)
				require.Equal(t, tt.wantCaps.ProviderName, caps.ProviderName)
			}
		})
	}
}

func TestValidateImageEditCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		req       *ImageEditRequest
		provider  string
		wantError bool
		errMsg    string
	}{
		{
			name: "valid OpenAI request",
			req: &ImageEditRequest{
				Image:  NewMockReader("fake image"),
				Prompt: "Make it blue",
				Model:  "dall-e-2",
				Size:   "512x512",
				Count:  1,
			},
			provider:  "openai",
			wantError: false,
		},
		{
			name: "unsupported model",
			req: &ImageEditRequest{
				Image:  NewMockReader("fake image"),
				Prompt: "Make it blue",
				Model:  "gpt-image-1", // Not supported for editing
				Size:   "512x512",
				Count:  1,
			},
			provider:  "openai",
			wantError: true,
			errMsg:    "not supported for image editing",
		},
		{
			name: "unsupported size",
			req: &ImageEditRequest{
				Image:  NewMockReader("fake image"),
				Prompt: "Make it blue",
				Model:  "dall-e-2",
				Size:   "1536x1024", // Not supported for editing
				Count:  1,
			},
			provider:  "openai",
			wantError: true,
			errMsg:    "not supported for image editing",
		},
		{
			name: "exceeds max count",
			req: &ImageEditRequest{
				Image:  NewMockReader("fake image"),
				Prompt: "Make it blue",
				Model:  "dall-e-2",
				Size:   "512x512",
				Count:  15, // Exceeds max of 10
			},
			provider:  "openai",
			wantError: true,
			errMsg:    "exceeds maximum",
		},
		{
			name: "mask not supported by provider",
			req: &ImageEditRequest{
				Image:  NewMockReader("fake image"),
				Prompt: "Make it blue",
				Mask:   NewMockReader("fake mask"),
				Model:  "dall-e-2",
				Size:   "512x512",
				Count:  1,
			},
			provider:  "google", // Doesn't support editing
			wantError: true,
			errMsg:    "does not support image editing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageEditCapabilities(tt.req, tt.provider)
			if tt.wantError {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
