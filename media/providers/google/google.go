package google

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"google.golang.org/genai"
)

// Provider implements the media.MediaGenerator interface for Google GenAI
type Provider struct {
	client *genai.Client
}

// NewProvider creates a new Google GenAI media provider
func NewProvider(client *genai.Client) *Provider {
	return &Provider{
		client: client,
	}
}

// ProviderName returns the name of this provider
func (p *Provider) ProviderName() string {
	return "google"
}

// SupportedModels returns the list of supported models for image generation
func (p *Provider) SupportedModels() []string {
	return []string{
		"imagen-3.0-generate-001",
		"imagen-3.0-generate-002",
		"imagen-4.0-generate-001",
		"imagen-4.0-ultra-generate-001",
		"imagen-4.0-fast-generate-001",
		"veo-2.0-generate-001",
		"veo-3.0-generate-preview",
		"veo-3.0-fast-generate-preview",
	}
}

// GenerateImage generates an image from a text prompt
func (p *Provider) GenerateImage(ctx context.Context, req *media.ImageGenerationRequest) (*media.ImageGenerationResponse, error) {
	if err := media.ValidateImageGenerationRequest(req); err != nil {
		return nil, err
	}

	// Set default model if not specified
	model := req.Model
	if model == "" {
		model = "imagen-4.0-generate-001"
	}

	// Validate model
	if !isImageModel(model) {
		return nil, fmt.Errorf("unsupported image model: %s", model)
	}

	if p.client == nil {
		return nil, fmt.Errorf("google genai client is not initialized")
	}

	numberOfImages := req.Count
	if numberOfImages == 0 {
		numberOfImages = 1
	}

	// Set up configuration
	config := &genai.GenerateImagesConfig{
		IncludeRAIReason:        true,
		IncludeSafetyAttributes: true,
		OutputMIMEType:          "image/jpeg", // Default to JPEG
		NumberOfImages:          int32(numberOfImages),
	}

	// Handle provider-specific parameters
	if req.ProviderSpecific != nil {
		if outputMIMEType, ok := req.ProviderSpecific["output_mime_type"].(string); ok {
			config.OutputMIMEType = outputMIMEType
		}
		if includeRAI, ok := req.ProviderSpecific["include_rai_reason"].(bool); ok {
			config.IncludeRAIReason = includeRAI
		}
		if includeSafety, ok := req.ProviderSpecific["include_safety_attributes"].(bool); ok {
			config.IncludeSafetyAttributes = includeSafety
		}
	}

	// Make the API call
	response, err := p.client.Models.GenerateImages(ctx, model, req.Prompt, config)
	if err != nil {
		return nil, fmt.Errorf("error generating image: %w", err)
	}

	if len(response.GeneratedImages) == 0 {
		return nil, fmt.Errorf("no images were generated")
	}

	// Convert response to our format
	images := make([]media.GeneratedImage, len(response.GeneratedImages))
	for i, genImage := range response.GeneratedImages {
		// Convert binary data to base64
		var b64JSON string
		if genImage.Image != nil && len(genImage.Image.ImageBytes) > 0 {
			b64JSON = base64.StdEncoding.EncodeToString(genImage.Image.ImageBytes)
		}

		// Create metadata map
		metadata := make(map[string]interface{})
		if genImage.SafetyAttributes != nil {
			metadata["safety_attributes"] = genImage.SafetyAttributes
		}
		if genImage.RAIFilteredReason != "" {
			metadata["rai_filtered_reason"] = genImage.RAIFilteredReason
		}
		if genImage.EnhancedPrompt != "" {
			metadata["enhanced_prompt"] = genImage.EnhancedPrompt
		}
		if response.PositivePromptSafetyAttributes != nil {
			metadata["positive_prompt_safety_attributes"] = response.PositivePromptSafetyAttributes
		}

		images[i] = media.GeneratedImage{
			B64JSON:       b64JSON,
			RevisedPrompt: genImage.EnhancedPrompt,
			Metadata:      metadata,
		}
	}

	// Create provider-specific response data
	providerSpecific := make(map[string]interface{})
	if response.PositivePromptSafetyAttributes != nil {
		providerSpecific["positive_prompt_safety_attributes"] = response.PositivePromptSafetyAttributes
	}

	return &media.ImageGenerationResponse{
		Images:           images,
		ProviderSpecific: providerSpecific,
	}, nil
}

// EditImage is not supported by Google GenAI
func (p *Provider) EditImage(ctx context.Context, req *media.ImageEditRequest) (*media.ImageEditResponse, error) {
	return nil, fmt.Errorf("image editing is not supported by Google GenAI provider")
}

// GenerateVideo generates a video from a text prompt
func (p *Provider) GenerateVideo(ctx context.Context, req *media.VideoGenerationRequest) (*media.VideoGenerationResponse, error) {
	if err := media.ValidateVideoGenerationRequest(req); err != nil {
		return nil, err
	}

	// Set default model if not specified
	model := req.Model
	if model == "" {
		model = "veo-2.0-generate-001"
	}

	// Validate model
	if !isVideoModel(model) {
		return nil, fmt.Errorf("unsupported video model: %s", model)
	}

	if p.client == nil {
		return nil, fmt.Errorf("google genai client is not initialized")
	}

	// Set up configuration
	config := &genai.GenerateVideosConfig{}

	// Handle provider-specific parameters
	if req.ProviderSpecific != nil {
		if outputGCSURI, ok := req.ProviderSpecific["output_gcs_uri"].(string); ok {
			config.OutputGCSURI = outputGCSURI
		}
	}

	// Make the API call - this returns a long-running operation
	operation, err := p.client.Models.GenerateVideos(ctx, model, req.Prompt, nil, config)
	if err != nil {
		return nil, fmt.Errorf("error generating video: %w", err)
	}

	// Return response with operation ID for async handling
	response := &media.VideoGenerationResponse{
		OperationID: operation.Name,
		Status:      "pending",
	}

	// If the operation is already done, process the results
	if operation.Done {
		response.Status = "completed"
		if operation.Response != nil && len(operation.Response.GeneratedVideos) > 0 {
			videos := make([]media.GeneratedVideo, len(operation.Response.GeneratedVideos))
			for i, video := range operation.Response.GeneratedVideos {
				genVideo := media.GeneratedVideo{
					URL:      video.Video.URI,
					Format:   video.Video.MIMEType,
					Metadata: make(map[string]interface{}),
				}

				// If video bytes are available, encode as base64
				if len(video.Video.VideoBytes) > 0 {
					genVideo.B64Data = base64.StdEncoding.EncodeToString(video.Video.VideoBytes)
				}

				// Add metadata
				if video.Video.URI != "" {
					genVideo.Metadata["uri"] = video.Video.URI
				}
				if video.Video.MIMEType != "" {
					genVideo.Metadata["mime_type"] = video.Video.MIMEType
				}

				videos[i] = genVideo
			}
			response.Videos = videos
		}
	}

	return response, nil
}

// CheckVideoOperation checks the status of a video generation operation
func (p *Provider) CheckVideoOperation(ctx context.Context, operationID string) (*media.OperationStatus, error) {
	if p.client == nil {
		return nil, fmt.Errorf("google genai client is not initialized")
	}

	// Create a GenerateVideosOperation from the operation ID
	operation := &genai.GenerateVideosOperation{
		Name: operationID,
		Done: false,
	}

	// Check the operation status
	updatedOperation, err := p.client.Operations.GetVideosOperation(ctx, operation, nil)
	if err != nil {
		return nil, fmt.Errorf("error checking video operation: %w", err)
	}

	status := &media.OperationStatus{
		ID: operationID,
	}

	if updatedOperation.Done {
		status.Status = "completed"
		status.Progress = 100

		if updatedOperation.Response != nil {
			// Process the completed result
			videos := make([]media.GeneratedVideo, len(updatedOperation.Response.GeneratedVideos))
			for i, video := range updatedOperation.Response.GeneratedVideos {
				genVideo := media.GeneratedVideo{
					URL:      video.Video.URI,
					Format:   video.Video.MIMEType,
					Metadata: make(map[string]interface{}),
				}

				// If video bytes are available, encode as base64
				if len(video.Video.VideoBytes) > 0 {
					genVideo.B64Data = base64.StdEncoding.EncodeToString(video.Video.VideoBytes)
				}

				// Add metadata
				if video.Video.URI != "" {
					genVideo.Metadata["uri"] = video.Video.URI
				}
				if video.Video.MIMEType != "" {
					genVideo.Metadata["mime_type"] = video.Video.MIMEType
				}

				videos[i] = genVideo
			}
			status.Result = &media.VideoGenerationResponse{
				Videos:      videos,
				Status:      "completed",
				OperationID: operationID,
			}
		}
	} else {
		status.Status = "running"
		// Progress is not directly available from the API, so we estimate
		status.Progress = 50 // Assume 50% if running
	}

	return status, nil
}

// DownloadVideo downloads video content from Google GenAI
func (p *Provider) DownloadVideo(ctx context.Context, video media.GeneratedVideo) ([]byte, error) {
	if video.URL == "" {
		return nil, fmt.Errorf("video URL is required for download")
	}

	// Only download if not using Vertex AI backend
	if p.client.ClientConfig().Backend != genai.BackendVertexAI {
		// Note: The exact API for downloading videos may vary based on the genai library version
		// This is a placeholder implementation
		return nil, fmt.Errorf("video download implementation depends on genai library version")
	}

	return nil, fmt.Errorf("video download not supported for Vertex AI backend")
}

// WaitForVideoGeneration waits for a video generation operation to complete
func (p *Provider) WaitForVideoGeneration(ctx context.Context, operationID string, pollInterval time.Duration) (*media.VideoGenerationResponse, error) {
	if pollInterval == 0 {
		pollInterval = 20 * time.Second // Default poll interval
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := p.CheckVideoOperation(ctx, operationID)
			if err != nil {
				return nil, err
			}

			switch status.Status {
			case "completed":
				if result, ok := status.Result.(*media.VideoGenerationResponse); ok {
					return result, nil
				}
				return nil, fmt.Errorf("unexpected result type")
			case "failed":
				return nil, fmt.Errorf("video generation failed: %s", status.Error)
			}
			// Continue polling for "pending" or "running" status
		}
	}
}

// Helper functions

func isImageModel(model string) bool {
	imageModels := []string{"imagen-3.0-generate-001", "imagen-3.0-generate-002"}
	for _, m := range imageModels {
		if m == model {
			return true
		}
	}
	return false
}

func isVideoModel(model string) bool {
	videoModels := []string{"veo-2.0-generate-001"}
	for _, m := range videoModels {
		if m == model {
			return true
		}
	}
	return false
}
