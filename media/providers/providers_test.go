package providers

import (
	"context"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	openaiapi "github.com/openai/openai-go"
	"github.com/stretchr/testify/require"
)

// MockImageGenerator is a mock implementation for testing
type MockImageGenerator struct{}

func (m *MockImageGenerator) GenerateImage(ctx context.Context, req *media.ImageGenerationRequest) (*media.ImageGenerationResponse, error) {
	return &media.ImageGenerationResponse{
		Images: []media.GeneratedImage{
			{B64JSON: "dGVzdA=="},
		},
	}, nil
}

func (m *MockImageGenerator) EditImage(ctx context.Context, req *media.ImageEditRequest) (*media.ImageEditResponse, error) {
	return &media.ImageEditResponse{
		Images: []media.GeneratedImage{
			{B64JSON: "ZWRpdGVk"},
		},
	}, nil
}

func (m *MockImageGenerator) SupportedModels() []string {
	return []string{"test-model-1", "test-model-2"}
}

func (m *MockImageGenerator) ProviderName() string {
	return "mock"
}

// MockVideoGenerator is a mock implementation for testing
type MockVideoGenerator struct{}

func (m *MockVideoGenerator) GenerateVideo(ctx context.Context, req *media.VideoGenerationRequest) (*media.VideoGenerationResponse, error) {
	return &media.VideoGenerationResponse{
		Videos: []media.GeneratedVideo{
			{URL: "https://example.com/video.mp4", Duration: 10.0, Format: "mp4"},
		},
		Status: "completed",
	}, nil
}

func (m *MockVideoGenerator) SupportedModels() []string {
	return []string{"test-video-model-1"}
}

func (m *MockVideoGenerator) ProviderName() string {
	return "mock"
}

// MockMediaGenerator implements both ImageGenerator and VideoGenerator for testing
type MockMediaGenerator struct {
	MockImageGenerator
	MockVideoGenerator
}

func (m *MockMediaGenerator) ProviderName() string {
	return "mock"
}

func (m *MockMediaGenerator) SupportedModels() []string {
	return []string{"test-model-1", "test-model-2", "test-video-model-1"}
}

func NewMockMediaGenerator() *MockMediaGenerator {
	return &MockMediaGenerator{
		MockImageGenerator: MockImageGenerator{},
		MockVideoGenerator: MockVideoGenerator{},
	}
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	require.NotNil(t, registry)
	require.Empty(t, registry.List())
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	// Create a mock provider
	mockProvider := NewMockMediaGenerator()
	registry.Register("mock", mockProvider)

	providers := registry.List()
	require.Contains(t, providers, "mock")
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()

	// Create a mock provider
	mockProvider := NewMockMediaGenerator()
	registry.Register("mock", mockProvider)

	// Test getting existing provider
	provider, err := registry.Get("mock")
	require.NoError(t, err)
	require.Equal(t, mockProvider, provider)

	// Test getting non-existent provider
	_, err = registry.Get("nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider nonexistent not found")
}

func TestRegistry_GetImageGenerator(t *testing.T) {
	registry := NewRegistry()

	// Create a mock provider
	mockProvider := NewMockMediaGenerator()
	registry.Register("mock", mockProvider)

	// Test getting image generator
	generator, err := registry.GetImageGenerator("mock")
	require.NoError(t, err)
	require.Equal(t, mockProvider, generator)

	// Test getting non-existent provider
	_, err = registry.GetImageGenerator("nonexistent")
	require.Error(t, err)
}

func TestRegistry_GetVideoGenerator(t *testing.T) {
	registry := NewRegistry()

	// Create a mock provider
	mockProvider := NewMockMediaGenerator()
	registry.Register("mock", mockProvider)

	// Test getting video generator
	generator, err := registry.GetVideoGenerator("mock")
	require.NoError(t, err)
	require.Equal(t, mockProvider, generator)

	// Test getting non-existent provider
	_, err = registry.GetVideoGenerator("nonexistent")
	require.Error(t, err)
}

func TestDefaultRegistry(t *testing.T) {
	// Save original env vars
	originalOpenAIKey := os.Getenv("OPENAI_API_KEY")

	// Test without OpenAI key
	os.Setenv("OPENAI_API_KEY", "")
	registry, err := DefaultRegistry()
	require.NoError(t, err)
	require.NotNil(t, registry)

	providers := registry.List()
	// Google might or might not be available depending on credentials

	// Test with OpenAI key
	os.Setenv("OPENAI_API_KEY", "test-key")
	registry, err = DefaultRegistry()
	require.NoError(t, err)
	require.NotNil(t, registry)

	providers = registry.List()
	require.Contains(t, providers, "openai")
	require.Contains(t, providers, "dalle") // Alias
	// Google might or might not be available depending on credentials

	// Restore original env var
	if originalOpenAIKey != "" {
		os.Setenv("OPENAI_API_KEY", originalOpenAIKey)
	} else {
		os.Unsetenv("OPENAI_API_KEY")
	}
}

func TestGoogleProviderWrapper(t *testing.T) {
	ctx := context.Background()

	// Try to create wrapper - this might fail if no credentials are set
	wrapper, err := NewGoogleProviderWrapper(ctx)
	if err != nil {
		t.Skipf("Skipping test - Google GenAI credentials not available: %v", err)
		return
	}
	defer wrapper.Close()

	require.Equal(t, "google", wrapper.ProviderName())

	models := wrapper.SupportedModels()
	require.Contains(t, models, "imagen-3.0-generate-001")
	require.Contains(t, models, "imagen-3.0-generate-002")
	require.Contains(t, models, "veo-2.0-generate-001")

	// Test methods that require a client (these will fail without proper setup)
	// Image generation
	imageReq := &media.ImageGenerationRequest{
		Prompt: "Test prompt",
	}
	_, err = wrapper.GenerateImage(ctx, imageReq)
	// This might succeed or fail depending on the environment
	// We just check that it doesn't panic
	if err != nil {
		// If it fails, it should be due to client creation issues
		t.Logf("Expected error creating Google GenAI client: %v", err)
	}

	// Image editing (should fail as Google doesn't support it)
	editReq := &media.ImageEditRequest{
		Prompt: "Test prompt",
		Image:  nil, // This will fail validation
	}
	_, err = wrapper.EditImage(ctx, editReq)
	require.Error(t, err) // Should always fail

	// Video generation
	videoReq := &media.VideoGenerationRequest{
		Prompt: "Test video prompt",
	}
	_, err = wrapper.GenerateVideo(ctx, videoReq)
	// This might succeed or fail depending on the environment
	if err != nil {
		t.Logf("Expected error with video generation: %v", err)
	}
}

func TestCreateProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		opts         *ProviderOptions
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "openai with client",
			providerName: "openai",
			opts: &ProviderOptions{
				OpenAIClient: &openaiapi.Client{},
			},
			wantErr: false,
		},
		{
			name:         "openai without client",
			providerName: "openai",
			opts:         nil,
			wantErr:      false,
		},
		{
			name:         "dalle alias",
			providerName: "dalle",
			opts:         nil,
			wantErr:      false,
		},
		{
			name:         "google",
			providerName: "google",
			opts:         nil,
			wantErr:      false, // Might fail if no credentials, but that's ok
		},
		{
			name:         "unknown provider",
			providerName: "unknown",
			opts:         nil,
			wantErr:      true,
			errMsg:       "unknown provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := CreateProvider(tt.providerName, tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
				require.Nil(t, provider)
			} else {
				// For Google provider, we might get an error if credentials aren't set
				if tt.providerName == "google" && err != nil {
					t.Logf("Google provider creation failed (expected if no credentials): %v", err)
				} else {
					require.NoError(t, err)
					require.NotNil(t, provider)
				}
			}
		})
	}
}

func TestGetAvailableProviders(t *testing.T) {
	// Save original env vars
	originalOpenAIKey := os.Getenv("OPENAI_API_KEY")

	// Test without OpenAI key
	os.Setenv("OPENAI_API_KEY", "")
	providers := GetAvailableProviders()
	// Google might or might not be available depending on credentials
	require.NotContains(t, providers, "openai")

	// Test with OpenAI key
	os.Setenv("OPENAI_API_KEY", "test-key")
	providers = GetAvailableProviders()
	// Google might or might not be available depending on credentials
	require.Contains(t, providers, "openai")

	// Restore original env var
	if originalOpenAIKey != "" {
		os.Setenv("OPENAI_API_KEY", originalOpenAIKey)
	} else {
		os.Unsetenv("OPENAI_API_KEY")
	}
}

func TestRegistry_LifecycleManagement(t *testing.T) {
	registry := NewRegistry()

	// Test RegisterWithCloser
	called := false
	mockProvider := &MockMediaGenerator{}
	registry.RegisterWithCloser("test", mockProvider, func() error {
		called = true
		return nil
	})

	// Test IsAvailable
	require.True(t, registry.IsAvailable("test"))
	require.False(t, registry.IsAvailable("nonexistent"))

	// Test HasCloser
	require.True(t, registry.HasCloser("test"))
	require.False(t, registry.HasCloser("nonexistent"))

	// Test CloseProvider
	err := registry.CloseProvider("test")
	require.NoError(t, err)
	require.True(t, called)

	// Test CloseAll
	registry.RegisterWithCloser("test2", mockProvider, func() error {
		return nil
	})
	err = registry.CloseAll()
	require.NoError(t, err)
}

func TestRegistry_GetCapabilities(t *testing.T) {
	registry := NewRegistry()

	// Register a mock provider
	mockProvider := &MockMediaGenerator{}
	registry.Register("mock", mockProvider)

	// Test getting capabilities for non-existent provider
	_, err := registry.GetCapabilities("nonexistent")
	require.Error(t, err)

	// Test getting capabilities for existing provider
	caps, err := registry.GetCapabilities("mock")
	require.NoError(t, err)
	require.NotNil(t, caps)
	require.Equal(t, "mock", caps.Name)
	require.Equal(t, "mock", caps.ProviderName)
	require.Equal(t, []string{"test-model-1", "test-model-2", "test-video-model-1"}, caps.SupportedModels)
	require.True(t, caps.SupportsImageGeneration)
	require.True(t, caps.SupportsVideoGeneration)
}
