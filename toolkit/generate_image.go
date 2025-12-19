package toolkit

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/dive/media/providers"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/openai/openai-go"
)

var _ dive.TypedTool[*ImageGenerationInput] = &ImageGenerationTool{}

// ImageGenerationInput represents the input parameters for image generation
type ImageGenerationInput struct {
	Prompt            string `json:"prompt"`
	Model             string `json:"model,omitempty"`
	Provider          string `json:"provider,omitempty"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	N                 int    `json:"n,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	OutputPath        string `json:"output_path,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression int    `json:"output_compression,omitempty"`
}

// ImageGenerationToolOptions are the options used to configure an ImageGenerationTool.
type ImageGenerationToolOptions struct {
	Client       *openai.Client
	FileSystem   FileSystem
	Registry     *providers.Registry
	WorkspaceDir string // Base directory for workspace validation (defaults to cwd)
}

// NewImageGenerationTool creates a new ImageGenerationTool with the given options.
func NewImageGenerationTool(opts ImageGenerationToolOptions) *dive.TypedToolAdapter[*ImageGenerationInput] {
	if opts.FileSystem == nil {
		opts.FileSystem = &RealFileSystem{}
	}
	if opts.Registry == nil {
		// Create default registry
		registry, _ := providers.DefaultRegistry()
		opts.Registry = registry
	}
	pathValidator, err := NewPathValidator(opts.WorkspaceDir)
	if err != nil {
		pathValidator = nil
	}
	return dive.ToolAdapter(&ImageGenerationTool{
		client:        opts.Client,
		fs:            opts.FileSystem,
		registry:      opts.Registry,
		pathValidator: pathValidator,
	})
}

// ImageGenerationTool is a tool that allows models to generate images using various
// providers including OpenAI DALL-E, gpt-image-1, and Google Imagen.
type ImageGenerationTool struct {
	client        *openai.Client
	fs            FileSystem
	registry      *providers.Registry
	pathValidator *PathValidator
}

func (t *ImageGenerationTool) Name() string {
	return "image_generation"
}

func (t *ImageGenerationTool) Description() string {
	return "Generate images using various AI providers (OpenAI gpt-image-1.5, Google Nano Banana). Provide a text prompt to create an image. Saves the images to the file system at the path specified in the `output_path` parameter. Specify provider and model for best results."
}

func (t *ImageGenerationTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"prompt", "output_path"},
		Properties: map[string]*schema.Property{
			"prompt": {
				Type:        "string",
				Description: "A text description of the desired image(s).",
			},
			"provider": {
				Type:        "string",
				Description: "The AI provider to use for image generation.",
				Enum:        []string{"openai", "google"},
			},
			"model": {
				Type:        "string",
				Description: "The model to use for image generation. For OpenAI: gpt-image-1.5 (recommended), gpt-image-1, gpt-image-1-mini. For Google: gemini-2.5-flash-image (Nano Banana), gemini-3-pro-image-preview (Nano Banana Pro).",
				Enum:        []string{"gpt-image-1.5", "gpt-image-1", "gpt-image-1-mini", "gemini-2.5-flash-image", "gemini-3-pro-image-preview"},
			},
			"size": {
				Type:        "string",
				Description: "The size of the generated images. For OpenAI models: `1024x1024`, `1536x1024` (landscape), `1024x1536` (portrait), or `auto` (default). For Google Nano Banana Pro: supports up to 4096px.",
				Enum:        []string{"1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792", "2048x2048", "4096x4096", "auto"},
			},
			"quality": {
				Type:        "string",
				Description: "The quality of the image that will be generated. `auto`, `high`, `medium` and `low` are supported for OpenAI gpt-image models. Defaults to `auto`.",
				Enum:        []string{"high", "medium", "low", "auto"},
			},
			"n": {
				Type:        "integer",
				Description: "The number of images to generate. Must be between 1 and 10.",
			},
			"moderation": {
				Type:        "string",
				Description: "The moderation level to use for the image generation. Must be one of `low` or `auto`. Defaults to `auto`.",
				Enum:        []string{"low", "auto"},
			},
			"output_path": {
				Type:        "string",
				Description: "Output file path to save the generated image(s). If `n` is greater than 1, this string is treated as a template and the index of the image is inserted into the template.",
			},
			"output_format": {
				Type:        "string",
				Description: "The format in which the generated images are returned. Must be one of `png`, `jpeg`, or `webp`. Defaults to `png`.",
				Enum:        []string{"png", "jpeg", "webp"},
			},
			"output_compression": {
				Type:        "integer",
				Description: "The compression level (0-100%) for the generated images. This parameter is only supported for `gpt-image-1` with the `webp` or `jpeg` output formats, and defaults to 100.",
			},
		},
	}
}

func (t *ImageGenerationTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Image Generation",
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *ImageGenerationTool) Call(ctx context.Context, input *ImageGenerationInput) (*dive.ToolResult, error) {
	// Validate inputs
	if input.Prompt == "" {
		return dive.NewToolResultError("Error: prompt is required for image generation"), nil
	}
	if input.OutputPath == "" {
		return dive.NewToolResultError("Error: output_path is required for image generation"), nil
	}

	// Validate output path is within workspace
	if t.pathValidator != nil && t.pathValidator.WorkspaceDir != "" {
		if err := t.pathValidator.ValidateWrite(input.OutputPath); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
	}

	// Validate and set defaults
	provider := input.Provider
	if provider == "" {
		provider = "openai" // Default to OpenAI
	}

	// Validate provider
	if provider != "openai" && provider != "google" {
		return dive.NewToolResultError(fmt.Sprintf("Error: unsupported provider '%s', must be 'openai' or 'google'", provider)), nil
	}

	// Validate count
	if input.N < 0 {
		return dive.NewToolResultError("Error: count cannot be negative"), nil
	}
	if input.N > 10 {
		return dive.NewToolResultError("Error: count cannot exceed 10"), nil
	}

	// Get the image generator from registry
	generator, err := t.registry.GetImageGenerator(provider)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: Failed to get provider %s: %s", provider, err.Error())), nil
	}

	// Create the media request
	req := &media.ImageGenerationRequest{
		Prompt:  input.Prompt,
		Model:   input.Model,
		Size:    input.Size,
		Quality: input.Quality,
		Count:   input.N,
	}

	// Add provider-specific parameters
	if req.Count == 0 {
		req.Count = 1
	}

	providerSpecific := make(map[string]interface{})
	if input.Moderation != "" {
		providerSpecific["moderation"] = input.Moderation
	}
	if input.OutputFormat != "" {
		providerSpecific["output_format"] = input.OutputFormat
	}
	if input.OutputCompression > 0 {
		providerSpecific["output_compression"] = input.OutputCompression
	}
	if len(providerSpecific) > 0 {
		req.ProviderSpecific = providerSpecific
	}

	// Make the API call using the media package
	response, err := generator.GenerateImage(ctx, req)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error generating image: %s", err.Error())), nil
	}

	if len(response.Images) == 0 {
		return dive.NewToolResultError("Error: No images were generated"), nil
	}

	var results []string
	for i, imageData := range response.Images {
		// Decode image data
		var imageBytes []byte
		if imageData.B64JSON != "" {
			imageBytes, err = base64.StdEncoding.DecodeString(imageData.B64JSON)
			if err != nil {
				return dive.NewToolResultError(fmt.Sprintf("Error decoding base64 image: %s", err.Error())), nil
			}
		} else {
			return dive.NewToolResultError("Error: No image data in response"), nil
		}

		// Determine output path
		filePath := input.OutputPath
		if req.Count > 1 {
			ext := filepath.Ext(filePath)
			name := filePath[:len(filePath)-len(ext)]
			filePath = fmt.Sprintf("%s_%d%s", name, i+1, ext)
		}

		// Create directory and save file
		dir := filepath.Dir(filePath)
		if err := t.fs.MkdirAll(dir, 0755); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error creating directory '%s': %s", dir, err.Error())), nil
		}

		// Validate file path is writable by checking if directory exists
		if !t.fs.IsDir(dir) {
			return dive.NewToolResultError(fmt.Sprintf("Error: directory '%s' is not accessible", dir)), nil
		}

		err = t.fs.WriteFile(filePath, string(imageBytes))
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error saving image to file '%s': %s", filePath, err.Error())), nil
		}
		results = append(results, fmt.Sprintf("Image %d saved to: %s", i+1, filePath))
	}

	resultText := fmt.Sprintf("Successfully generated %d image(s) using %s:\n%s",
		len(response.Images),
		provider,
		fmt.Sprintf("• %s", results[0]))

	if len(results) > 1 {
		for _, result := range results[1:] {
			resultText += fmt.Sprintf("\n• %s", result)
		}
	}
	return dive.NewToolResultText(resultText), nil
}
