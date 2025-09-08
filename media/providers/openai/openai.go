package openai

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/deepnoodle-ai/dive/media"
	openaiapi "github.com/openai/openai-go"
)

// Provider implements the media.ImageGenerator interface for OpenAI
type Provider struct {
	client *openaiapi.Client
}

// NewProvider creates a new OpenAI media provider
func NewProvider(client *openaiapi.Client) *Provider {
	return &Provider{
		client: client,
	}
}

// ProviderName returns the name of this provider
func (p *Provider) ProviderName() string {
	return "openai"
}

// SupportedModels returns the list of supported models for image generation
func (p *Provider) SupportedModels() []string {
	return []string{"dall-e-2", "dall-e-3", "gpt-image-1"}
}

// GenerateImage generates an image from a text prompt
func (p *Provider) GenerateImage(ctx context.Context, req *media.ImageGenerationRequest) (*media.ImageGenerationResponse, error) {
	if err := media.ValidateImageGenerationRequest(req); err != nil {
		return nil, err
	}

	// Set up parameters with defaults
	params := openaiapi.ImageGenerateParams{
		Prompt: req.Prompt,
		Model:  openaiapi.ImageModelGPTImage1, // Default to gpt-image-1
	}

	// Set model
	if req.Model != "" {
		switch req.Model {
		case "dall-e-2":
			params.Model = openaiapi.ImageModelDallE2
		case "dall-e-3":
			params.Model = openaiapi.ImageModelDallE3
		case "gpt-image-1":
			params.Model = openaiapi.ImageModelGPTImage1
		default:
			return nil, fmt.Errorf("unsupported model: %s", req.Model)
		}
	}

	// Set size
	if req.Size != "" {
		params.Size = openaiapi.ImageGenerateParamsSize(req.Size)
	} else {
		// Set default size based on model
		if params.Model == openaiapi.ImageModelGPTImage1 {
			params.Size = openaiapi.ImageGenerateParamsSizeAuto
		} else {
			params.Size = openaiapi.ImageGenerateParamsSize1024x1024
		}
	}

	// Set quality
	if req.Quality != "" {
		switch params.Model {
		case openaiapi.ImageModelGPTImage1:
			switch req.Quality {
			case "high":
				params.Quality = openaiapi.ImageGenerateParamsQualityHigh
			case "medium":
				params.Quality = openaiapi.ImageGenerateParamsQualityMedium
			case "low":
				params.Quality = openaiapi.ImageGenerateParamsQualityLow
			case "auto":
				params.Quality = openaiapi.ImageGenerateParamsQualityAuto
			default:
				params.Quality = openaiapi.ImageGenerateParamsQualityAuto
			}
		case openaiapi.ImageModelDallE3:
			switch req.Quality {
			case "standard":
				params.Quality = openaiapi.ImageGenerateParamsQualityStandard
			case "hd":
				params.Quality = openaiapi.ImageGenerateParamsQualityHD
			default:
				params.Quality = openaiapi.ImageGenerateParamsQualityHD
			}
		}
	}

	// Set count
	if req.Count > 0 {
		params.N = openaiapi.Int(int64(req.Count))
	} else {
		params.N = openaiapi.Int(1)
	}

	// Set response format - use b64_json for non-gpt-image-1 models
	if params.Model != openaiapi.ImageModelGPTImage1 {
		params.ResponseFormat = openaiapi.ImageGenerateParamsResponseFormatB64JSON
	}

	// Handle provider-specific parameters
	if req.ProviderSpecific != nil {
		if moderation, ok := req.ProviderSpecific["moderation"].(string); ok {
			params.Moderation = openaiapi.ImageGenerateParamsModeration(moderation)
		}
		if outputFormat, ok := req.ProviderSpecific["output_format"].(string); ok && params.Model == openaiapi.ImageModelGPTImage1 {
			params.OutputFormat = openaiapi.ImageGenerateParamsOutputFormat(outputFormat)
		}
		if outputCompression, ok := req.ProviderSpecific["output_compression"]; ok && params.Model == openaiapi.ImageModelGPTImage1 {
			if compression, ok := outputCompression.(int); ok {
				params.OutputCompression = openaiapi.Int(int64(compression))
			} else if compressionStr, ok := outputCompression.(string); ok {
				if compression, err := strconv.Atoi(compressionStr); err == nil {
					params.OutputCompression = openaiapi.Int(int64(compression))
				}
			}
		}
	}

	// Make the API call
	response, err := p.client.Images.Generate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error generating image: %w", err)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("no images were generated")
	}

	// Convert response to our format
	images := make([]media.GeneratedImage, len(response.Data))
	for i, imageData := range response.Data {
		images[i] = media.GeneratedImage{
			URL:           imageData.URL,
			B64JSON:       imageData.B64JSON,
			RevisedPrompt: imageData.RevisedPrompt,
		}
	}

	return &media.ImageGenerationResponse{
		Images: images,
	}, nil
}

// EditImage edits an existing image based on a text prompt
func (p *Provider) EditImage(ctx context.Context, req *media.ImageEditRequest) (*media.ImageEditResponse, error) {
	if err := media.ValidateImageEditRequest(req); err != nil {
		return nil, err
	}

	// Set up parameters
	params := openaiapi.ImageEditParams{
		Image:  openaiapi.ImageEditParamsImageUnion{OfFile: req.Image},
		Prompt: req.Prompt,
		Model:  openaiapi.ImageModelDallE2, // Only DALL-E 2 supports editing
	}

	// Set model (validate it's supported for editing)
	if req.Model != "" {
		if req.Model != "dall-e-2" {
			return nil, fmt.Errorf("only dall-e-2 supports image editing, got: %s", req.Model)
		}
		params.Model = openaiapi.ImageModelDallE2
	}

	// Set size
	if req.Size != "" {
		switch req.Size {
		case "256x256":
			params.Size = openaiapi.ImageEditParamsSize256x256
		case "512x512":
			params.Size = openaiapi.ImageEditParamsSize512x512
		case "1024x1024":
			params.Size = openaiapi.ImageEditParamsSize1024x1024
		default:
			return nil, fmt.Errorf("unsupported size for editing: %s", req.Size)
		}
	} else {
		params.Size = openaiapi.ImageEditParamsSize1024x1024
	}

	// Set count
	if req.Count > 0 {
		params.N = openaiapi.Int(int64(req.Count))
	} else {
		params.N = openaiapi.Int(1)
	}

	// Add mask if provided
	if req.Mask != nil {
		params.Mask = req.Mask
	}

	// Set response format
	params.ResponseFormat = openaiapi.ImageEditParamsResponseFormatB64JSON

	// Make the API call
	response, err := p.client.Images.Edit(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error editing image: %w", err)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("no edited images were returned")
	}

	// Convert response to our format
	images := make([]media.GeneratedImage, len(response.Data))
	for i, imageData := range response.Data {
		images[i] = media.GeneratedImage{
			URL:           imageData.URL,
			B64JSON:       imageData.B64JSON,
			RevisedPrompt: imageData.RevisedPrompt,
		}
	}

	return &media.ImageEditResponse{
		Images: images,
	}, nil
}

// GenerateVideo is not supported by OpenAI
func (p *Provider) GenerateVideo(ctx context.Context, req *media.VideoGenerationRequest) (*media.VideoGenerationResponse, error) {
	return nil, fmt.Errorf("video generation is not supported by OpenAI provider")
}

// Utility functions

// DecodeBase64Image decodes a base64 image string to bytes
func DecodeBase64Image(b64Data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64Data)
}

// EncodeImageToBase64 encodes image bytes to base64 string
func EncodeImageToBase64(imageData []byte) string {
	return base64.StdEncoding.EncodeToString(imageData)
}
