package providers

import (
	"context"
	"fmt"
	"os"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/dive/media/providers/google"
	"github.com/deepnoodle-ai/dive/media/providers/openai"
	openaiapi "github.com/openai/openai-go"
	"google.golang.org/genai"
)

// Registry holds all available media providers
type Registry struct {
	providers map[string]media.MediaGenerator
	closers   map[string]func() error // Functions to close/cleanup providers
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]media.MediaGenerator),
		closers:   make(map[string]func() error),
	}
}

// Register registers a provider with the given name
func (r *Registry) Register(name string, provider media.MediaGenerator) {
	r.providers[name] = provider
}

// RegisterWithCloser registers a provider with the given name and a cleanup function
func (r *Registry) RegisterWithCloser(name string, provider media.MediaGenerator, closer func() error) {
	r.providers[name] = provider
	r.closers[name] = closer
}

// Get returns a provider by name
func (r *Registry) Get(name string) (media.MediaGenerator, error) {
	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}
	return provider, nil
}

// List returns all registered provider names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// IsAvailable checks if a provider is available
func (r *Registry) IsAvailable(name string) bool {
	_, exists := r.providers[name]
	return exists
}

// HasCloser checks if a provider has a cleanup function
func (r *Registry) HasCloser(name string) bool {
	_, exists := r.closers[name]
	return exists
}

// CloseProvider closes a specific provider if it has a cleanup function
func (r *Registry) CloseProvider(name string) error {
	if closer, exists := r.closers[name]; exists {
		return closer()
	}
	return nil
}

// CloseAll closes all providers that have cleanup functions
func (r *Registry) CloseAll() error {
	var errs []error
	for name, closer := range r.closers {
		if err := closer(); err != nil {
			errs = append(errs, fmt.Errorf("error closing provider %s: %w", name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multiple errors during cleanup: %v", errs)
	}
	return nil
}

// GetCapabilities returns capabilities for a provider
func (r *Registry) GetCapabilities(name string) (*ProviderCapabilities, error) {
	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}

	caps := &ProviderCapabilities{
		Name:            name,
		ProviderName:    provider.ProviderName(),
		SupportedModels: provider.SupportedModels(),
	}

	// Check if it supports image generation
	if _, ok := provider.(media.ImageGenerator); ok {
		caps.SupportsImageGeneration = true
		caps.ImageCapabilities = &ImageCapabilities{}
		// Try to get edit capabilities
		if editCaps, err := media.GetImageEditCapabilities(name); err == nil {
			caps.ImageCapabilities.SupportsEditing = true
			caps.ImageCapabilities.EditCapabilities = editCaps
		}
	}

	// Check if it supports video generation
	if _, ok := provider.(media.VideoGenerator); ok {
		caps.SupportsVideoGeneration = true
		caps.VideoCapabilities = &VideoCapabilities{}
	}

	return caps, nil
}

// ProviderCapabilities describes what a provider can do
type ProviderCapabilities struct {
	Name                    string
	ProviderName            string
	SupportedModels         []string
	SupportsImageGeneration bool
	SupportsVideoGeneration bool
	ImageCapabilities       *ImageCapabilities
	VideoCapabilities       *VideoCapabilities
}

// ImageCapabilities describes image-related capabilities
type ImageCapabilities struct {
	SupportsEditing  bool
	EditCapabilities *media.ImageEditCapabilities
}

// VideoCapabilities describes video-related capabilities
type VideoCapabilities struct {
	// Add video-specific capabilities here as needed
}

// GetImageGenerator returns an image generator by provider name
func (r *Registry) GetImageGenerator(name string) (media.ImageGenerator, error) {
	provider, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

// GetVideoGenerator returns a video generator by provider name
func (r *Registry) GetVideoGenerator(name string) (media.VideoGenerator, error) {
	provider, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

// DefaultRegistry creates a registry with default providers
func DefaultRegistry() (*Registry, error) {
	registry := NewRegistry()

	// Register OpenAI provider if API key is available
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		client := openaiapi.NewClient()
		provider := openai.NewProvider(&client)
		registry.Register("openai", provider)
		registry.Register("dalle", provider) // Alias for backward compatibility
	}

	// Register Google provider if credentials are available
	if hasGoogleCredentials() {
		wrapper, err := NewGoogleProviderWrapper(context.Background())
		if err != nil {
			// Log error but don't fail - other providers might still work
			fmt.Fprintf(os.Stderr, "Warning: Failed to create Google provider: %v\n", err)
		} else {
			registry.RegisterWithCloser("google", wrapper, wrapper.Close)
		}
	}

	return registry, nil
}

// GoogleProviderWrapper wraps the Google provider with client management
type GoogleProviderWrapper struct {
	client   *genai.Client
	provider *google.Provider
}

// NewGoogleProviderWrapper creates a new Google provider wrapper
func NewGoogleProviderWrapper(ctx context.Context) (*GoogleProviderWrapper, error) {
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Google GenAI client: %w", err)
	}
	return &GoogleProviderWrapper{
		client:   client,
		provider: google.NewProvider(client),
	}, nil
}

// Forward all MediaGenerator interface methods to the wrapped provider
func (w *GoogleProviderWrapper) ProviderName() string {
	return w.provider.ProviderName()
}

func (w *GoogleProviderWrapper) SupportedModels() []string {
	return w.provider.SupportedModels()
}

func (w *GoogleProviderWrapper) GenerateImage(ctx context.Context, req *media.ImageGenerationRequest) (*media.ImageGenerationResponse, error) {
	return w.provider.GenerateImage(ctx, req)
}

func (w *GoogleProviderWrapper) EditImage(ctx context.Context, req *media.ImageEditRequest) (*media.ImageEditResponse, error) {
	return w.provider.EditImage(ctx, req)
}

func (w *GoogleProviderWrapper) GenerateVideo(ctx context.Context, req *media.VideoGenerationRequest) (*media.VideoGenerationResponse, error) {
	return w.provider.GenerateVideo(ctx, req)
}

// CheckVideoOperation is forwarded to the wrapped provider if it supports it
func (w *GoogleProviderWrapper) CheckVideoOperation(ctx context.Context, operationID string) (*media.OperationStatus, error) {
	return w.provider.CheckVideoOperation(ctx, operationID)
}

// Close cleans up the client (no-op for Google GenAI)
func (w *GoogleProviderWrapper) Close() error {
	// Google GenAI client doesn't require explicit cleanup
	return nil
}

// ProviderOptions contains options for creating specific providers
type ProviderOptions struct {
	// OpenAI options
	OpenAIClient *openaiapi.Client

	// Google options
	GoogleClient *genai.Client
}

// CreateProvider creates a provider with the given name and options
func CreateProvider(name string, opts *ProviderOptions) (media.MediaGenerator, error) {
	switch name {
	case "openai", "dalle":
		if opts != nil && opts.OpenAIClient != nil {
			return openai.NewProvider(opts.OpenAIClient), nil
		}
		// Create default client
		client := openaiapi.NewClient()
		return openai.NewProvider(&client), nil

	case "google":
		if opts != nil && opts.GoogleClient != nil {
			return google.NewProvider(opts.GoogleClient), nil
		}
		// Create wrapper with new client
		return NewGoogleProviderWrapper(context.Background())

	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

// GetAvailableProviders returns a list of providers that can be initialized
// hasGoogleCredentials checks if Google GenAI credentials are available
func hasGoogleCredentials() bool {
	// Check for Gemini API key
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
		return true
	}

	// Check for Vertex AI credentials
	if os.Getenv("GOOGLE_GENAI_USE_VERTEXAI") != "" && os.Getenv("GOOGLE_CLOUD_PROJECT") != "" {
		return true
	}

	return false
}

// GetAvailableProviders returns a list of providers that can be initialized
func GetAvailableProviders() []string {
	var providers []string

	// Check OpenAI
	if os.Getenv("OPENAI_API_KEY") != "" {
		providers = append(providers, "openai")
	}

	// Check Google GenAI credentials
	if hasGoogleCredentials() {
		providers = append(providers, "google")
	}

	return providers
}
