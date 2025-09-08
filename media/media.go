package media

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ImageGenerator provides an interface for generating images from text prompts
type ImageGenerator interface {
	// GenerateImage generates an image from a text prompt
	GenerateImage(ctx context.Context, req *ImageGenerationRequest) (*ImageGenerationResponse, error)

	// EditImage edits an existing image based on a text prompt
	EditImage(ctx context.Context, req *ImageEditRequest) (*ImageEditResponse, error)

	// SupportedModels returns a list of supported models for this provider
	SupportedModels() []string

	// ProviderName returns the name of the provider
	ProviderName() string
}

// VideoGenerator provides an interface for generating videos from text prompts
type VideoGenerator interface {
	// GenerateVideo generates a video from a text prompt
	GenerateVideo(ctx context.Context, req *VideoGenerationRequest) (*VideoGenerationResponse, error)

	// SupportedModels returns a list of supported models for this provider
	SupportedModels() []string

	// ProviderName returns the name of the provider
	ProviderName() string
}

// MediaGenerator combines both image and video generation capabilities
type MediaGenerator interface {
	ImageGenerator
	VideoGenerator
}

// ImageGenerationRequest represents a request to generate an image
type ImageGenerationRequest struct {
	// Prompt is the text description of the desired image
	Prompt string `json:"prompt"`

	// Model specifies which model to use for generation
	Model string `json:"model,omitempty"`

	// Size specifies the dimensions of the generated image
	Size string `json:"size,omitempty"`

	// Quality specifies the quality level of the generated image
	Quality string `json:"quality,omitempty"`

	// Count specifies how many images to generate
	Count int `json:"count,omitempty"`

	// ResponseFormat specifies the format of the response (url, b64_json)
	ResponseFormat string `json:"response_format,omitempty"`

	// ProviderSpecific allows passing provider-specific parameters
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// ImageGenerationResponse represents the response from image generation
type ImageGenerationResponse struct {
	// Images contains the generated image data
	Images []GeneratedImage `json:"images"`

	// Usage contains information about token/credit usage
	Usage *Usage `json:"usage,omitempty"`

	// ProviderSpecific contains provider-specific response data
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// ImageEditRequest represents a request to edit an existing image
type ImageEditRequest struct {
	// Image is the input image to edit
	Image io.Reader `json:"-"`

	// Prompt describes the desired changes to the image
	Prompt string `json:"prompt"`

	// Mask is an optional mask image for selective editing
	Mask io.Reader `json:"-"`

	// Model specifies which model to use for editing
	Model string `json:"model,omitempty"`

	// Size specifies the dimensions of the edited image
	Size string `json:"size,omitempty"`

	// Count specifies how many variations to generate
	Count int `json:"count,omitempty"`

	// ResponseFormat specifies the format of the response (url, b64_json)
	ResponseFormat string `json:"response_format,omitempty"`

	// ProviderSpecific allows passing provider-specific parameters
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// ImageEditResponse represents the response from image editing
type ImageEditResponse struct {
	// Images contains the edited image data
	Images []GeneratedImage `json:"images"`

	// Usage contains information about token/credit usage
	Usage *Usage `json:"usage,omitempty"`

	// ProviderSpecific contains provider-specific response data
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// VideoGenerationRequest represents a request to generate a video
type VideoGenerationRequest struct {
	// Prompt is the text description of the desired video
	Prompt string `json:"prompt"`

	// Model specifies which model to use for generation
	Model string `json:"model,omitempty"`

	// Duration specifies the length of the video in seconds
	Duration float64 `json:"duration,omitempty"`

	// Resolution specifies the video resolution
	Resolution string `json:"resolution,omitempty"`

	// FrameRate specifies the frames per second
	FrameRate int `json:"frame_rate,omitempty"`

	// Format specifies the output video format
	Format string `json:"format,omitempty"`

	// ProviderSpecific allows passing provider-specific parameters
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// VideoGenerationResponse represents the response from video generation
type VideoGenerationResponse struct {
	// Videos contains the generated video data
	Videos []GeneratedVideo `json:"videos"`

	// Usage contains information about token/credit usage
	Usage *Usage `json:"usage,omitempty"`

	// OperationID is used for async operations
	OperationID string `json:"operation_id,omitempty"`

	// Status indicates if the operation is complete or in progress
	Status string `json:"status,omitempty"`

	// ProviderSpecific contains provider-specific response data
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// GeneratedImage represents a single generated image
type GeneratedImage struct {
	// URL is the URL to download the image (if available)
	URL string `json:"url,omitempty"`

	// B64JSON is the base64-encoded image data
	B64JSON string `json:"b64_json,omitempty"`

	// RevisedPrompt contains any prompt revisions made by the provider
	RevisedPrompt string `json:"revised_prompt,omitempty"`

	// Metadata contains additional information about the image
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// GeneratedVideo represents a single generated video
type GeneratedVideo struct {
	// URL is the URL to download the video (if available)
	URL string `json:"url,omitempty"`

	// B64Data is the base64-encoded video data (for small videos)
	B64Data string `json:"b64_data,omitempty"`

	// Duration is the actual duration of the generated video
	Duration float64 `json:"duration,omitempty"`

	// Format is the format of the generated video
	Format string `json:"format,omitempty"`

	// Resolution is the resolution of the generated video
	Resolution string `json:"resolution,omitempty"`

	// Metadata contains additional information about the video
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Usage represents usage information from the API
type Usage struct {
	// Tokens used (if applicable)
	Tokens int `json:"tokens,omitempty"`

	// Credits used (if applicable)
	Credits float64 `json:"credits,omitempty"`

	// Cost in the provider's currency (if available)
	Cost float64 `json:"cost,omitempty"`

	// Currency of the cost
	Currency string `json:"currency,omitempty"`
}

// OperationStatus represents the status of a long-running operation
type OperationStatus struct {
	// ID of the operation
	ID string `json:"id"`

	// Status of the operation (pending, running, completed, failed)
	Status string `json:"status"`

	// Progress percentage (0-100)
	Progress int `json:"progress,omitempty"`

	// Error message if the operation failed
	Error string `json:"error,omitempty"`

	// Result contains the final result if completed
	Result interface{} `json:"result,omitempty"`
}

// VideoOperationChecker provides an interface for checking video generation status
type VideoOperationChecker interface {
	// CheckVideoOperation checks the status of a video generation operation
	CheckVideoOperation(ctx context.Context, operationID string) (*OperationStatus, error)
}

// Validation functions

// ValidateImageGenerationRequest validates an image generation request
func ValidateImageGenerationRequest(req *ImageGenerationRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if req.Count < 0 {
		return fmt.Errorf("count cannot be negative")
	}

	if req.Count > 10 {
		return fmt.Errorf("count cannot exceed 10")
	}

	// Validate response format if specified
	if req.ResponseFormat != "" {
		validFormats := []string{"url", "b64_json"}
		isValid := false
		for _, format := range validFormats {
			if req.ResponseFormat == format {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid response format: %s, must be one of: %s", req.ResponseFormat, strings.Join(validFormats, ", "))
		}
	}

	return nil
}

// ValidateImageEditRequest validates an image edit request
func ValidateImageEditRequest(req *ImageEditRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if req.Image == nil {
		return fmt.Errorf("image is required")
	}

	if req.Count < 0 {
		return fmt.Errorf("count cannot be negative")
	}

	if req.Count > 10 {
		return fmt.Errorf("count cannot exceed 10")
	}

	// Validate response format if specified
	if req.ResponseFormat != "" {
		validFormats := []string{"url", "b64_json"}
		isValid := false
		for _, format := range validFormats {
			if req.ResponseFormat == format {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid response format: %s, must be one of: %s", req.ResponseFormat, strings.Join(validFormats, ", "))
		}
	}

	return nil
}

// ValidateVideoGenerationRequest validates a video generation request
func ValidateVideoGenerationRequest(req *VideoGenerationRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if req.Duration < 0 {
		return fmt.Errorf("duration cannot be negative")
	}

	if req.Duration > 300 { // 5 minutes max
		return fmt.Errorf("duration cannot exceed 300 seconds (5 minutes)")
	}

	if req.FrameRate < 0 {
		return fmt.Errorf("frame rate cannot be negative")
	}

	if req.FrameRate > 60 {
		return fmt.Errorf("frame rate cannot exceed 60 fps")
	}

	return nil
}

// ImageEditCapabilities represents the capabilities of a provider for image editing
type ImageEditCapabilities struct {
	// SupportsMask indicates if the provider supports mask-based editing
	SupportsMask bool

	// SupportedModels lists the models that support image editing
	SupportedModels []string

	// SupportedSizes lists the supported output sizes
	SupportedSizes []string

	// MaxImages is the maximum number of images that can be generated per request
	MaxImages int

	// ProviderName is the name of the provider
	ProviderName string
}

// GetImageEditCapabilities returns the image editing capabilities for a provider
func GetImageEditCapabilities(provider string) (*ImageEditCapabilities, error) {
	switch strings.ToLower(provider) {
	case "openai", "dalle":
		return &ImageEditCapabilities{
			SupportsMask:    true,
			SupportedModels: []string{"dall-e-2"},
			SupportedSizes:  []string{"256x256", "512x512", "1024x1024"},
			MaxImages:       10,
			ProviderName:    "openai",
		}, nil
	case "google":
		return nil, fmt.Errorf("google GenAI does not support image editing")
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// ValidateImageEditCapabilities validates that the request parameters are supported by the provider
func ValidateImageEditCapabilities(req *ImageEditRequest, provider string) error {
	caps, err := GetImageEditCapabilities(provider)
	if err != nil {
		return err
	}

	// Validate model
	if req.Model != "" {
		found := false
		for _, model := range caps.SupportedModels {
			if req.Model == model {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("model '%s' is not supported for image editing by provider '%s'. Supported models: %s",
				req.Model, provider, strings.Join(caps.SupportedModels, ", "))
		}
	}

	// Validate size
	if req.Size != "" {
		found := false
		for _, size := range caps.SupportedSizes {
			if req.Size == size {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("size '%s' is not supported for image editing by provider '%s'. Supported sizes: %s",
				req.Size, provider, strings.Join(caps.SupportedSizes, ", "))
		}
	}

	// Validate count
	if req.Count > caps.MaxImages {
		return fmt.Errorf("count %d exceeds maximum of %d for provider '%s'", req.Count, caps.MaxImages, provider)
	}

	// Validate mask support
	if req.Mask != nil && !caps.SupportsMask {
		return fmt.Errorf("mask-based editing is not supported by provider '%s'", provider)
	}

	return nil
}
