package toolkit

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/openai/openai-go"
)

var _ dive.TypedTool[*ImageGenerationInput] = &ImageGenerationTool{}

// ImageGenerationInput represents the input parameters for image generation
type ImageGenerationInput struct {
	Prompt            string `json:"prompt"`
	Model             string `json:"model,omitempty"`
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
	Client     *openai.Client
	FileSystem FileSystem
}

// NewImageGenerationTool creates a new ImageGenerationTool with the given options.
func NewImageGenerationTool(opts ImageGenerationToolOptions) *dive.TypedToolAdapter[*ImageGenerationInput] {
	if opts.FileSystem == nil {
		opts.FileSystem = &RealFileSystem{}
	}
	return dive.ToolAdapter(&ImageGenerationTool{
		client: opts.Client,
		fs:     opts.FileSystem,
	})
}

// ImageGenerationTool is a tool that allows models to generate images using one of
// OpenAI's image generation models, including DALL-E and gpt-image-1.
type ImageGenerationTool struct {
	client *openai.Client
	fs     FileSystem
}

func (t *ImageGenerationTool) Name() string {
	return "openai_image_generation"
}

func (t *ImageGenerationTool) Description() string {
	return "Generate images using OpenAI's image generation API. Provide a text prompt to create an image. Saves the images to the file system at the path specified in the `output_path` parameter. Specify the `gpt-image-1` model for best results."
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
			"model": {
				Type:        "string",
				Description: "The model to use for image generation. Prefer `gpt-image-1` for best results.",
				Enum:        []string{"gpt-image-1", "dall-e-2", "dall-e-3"},
			},
			"size": {
				Type:        "string",
				Description: "The size of the generated images. Must be one of `1024x1024`, `1536x1024` (landscape), `1024x1536` (portrait), or `auto` (default value) for `gpt-image-1`, and one of `256x256`, `512x512`, or `1024x1024` for `dall-e-2`.",
				Enum:        []string{"256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792", "auto"},
			},
			"quality": {
				Type:        "string",
				Description: "The quality of the image that will be generated. `auto`, `high`, `medium` and `low` are only supported for `gpt-image-1`. Defaults to `auto`.",
				Enum:        []string{"high", "medium", "low", "auto"},
			},
			"n": {
				Type:        "integer",
				Description: "The number of images to generate. Must be between 1 and 10. For dall-e-3, only n=1 is supported.",
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
	if input.Prompt == "" {
		return dive.NewToolResultError("Error: prompt is required for image generation"), nil
	}
	if input.OutputPath == "" {
		return dive.NewToolResultError("Error: output_path is required for image generation"), nil
	}

	// Set up parameters with defaults
	params := openai.ImageGenerateParams{
		Prompt:     input.Prompt,
		Model:      openai.ImageModelGPTImage1,
		Size:       openai.ImageGenerateParamsSizeAuto,
		Moderation: openai.ImageGenerateParamsModerationAuto,
	}

	if input.Model != "" {
		switch input.Model {
		case "dall-e-2":
			params.Model = openai.ImageModelDallE2
		case "dall-e-3":
			params.Model = openai.ImageModelDallE3
		case "gpt-image-1":
			params.Model = openai.ImageModelGPTImage1
		default:
			return dive.NewToolResultError(fmt.Sprintf("Error: Invalid model '%s'. Must be 'dall-e-2', 'dall-e-3', or 'gpt-image-1'", input.Model)), nil
		}
	}

	// Allow certain customizations, depending on which model was selected
	if params.Model != openai.ImageModelGPTImage1 {
		params.ResponseFormat = openai.ImageGenerateParamsResponseFormatB64JSON
	} else {
		if input.OutputFormat != "" {
			params.OutputFormat = openai.ImageGenerateParamsOutputFormat(input.OutputFormat)
		}
		if input.OutputCompression > 0 {
			params.OutputCompression = openai.Int(int64(input.OutputCompression))
		}
	}
	if input.Moderation != "" {
		params.Moderation = openai.ImageGenerateParamsModeration(input.Moderation)
	}
	if input.Size != "" {
		params.Size = openai.ImageGenerateParamsSize(input.Size)
	}
	if input.N > 0 {
		if params.Model != openai.ImageModelGPTImage1 {
			return dive.NewToolResultError("Error: n > 1 is only supported for gpt-image-1"), nil
		}
		params.N = openai.Int(int64(input.N))
	}

	if input.Quality != "" {
		// Translate quality as needed, since the valid values differ by model
		if params.Model == openai.ImageModelGPTImage1 {
			switch input.Quality {
			case "high":
				params.Quality = openai.ImageGenerateParamsQualityHigh
			case "medium":
				params.Quality = openai.ImageGenerateParamsQualityMedium
			case "low":
				params.Quality = openai.ImageGenerateParamsQualityLow
			case "auto":
				params.Quality = openai.ImageGenerateParamsQualityAuto
			default: // Translate everything else to auto
				params.Quality = openai.ImageGenerateParamsQualityAuto
			}
		} else if params.Model == openai.ImageModelDallE3 {
			switch input.Quality {
			case "standard":
				params.Quality = openai.ImageGenerateParamsQualityStandard
			case "hd":
				params.Quality = openai.ImageGenerateParamsQualityHD
			default: // Translate everything else to hd
				params.Quality = openai.ImageGenerateParamsQualityHD
			}
		} else {
			params.Quality = openai.ImageGenerateParamsQualityAuto
		}
	}

	// Make the API call
	response, err := t.client.Images.Generate(ctx, params)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error generating image: %s", err.Error())), nil
	}

	if len(response.Data) == 0 {
		return dive.NewToolResultError("Error: No images were generated"), nil
	}

	var results []string
	for i, imageData := range response.Data {
		// Save base64 image to file
		imageBytes, err := base64.StdEncoding.DecodeString(imageData.B64JSON)
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error decoding base64 image: %s", err.Error())), nil
		}
		filePath := input.OutputPath
		if input.N > 1 {
			ext := filepath.Ext(filePath)
			name := filePath[:len(filePath)-len(ext)]
			filePath = fmt.Sprintf("%s_%d%s", name, i+1, ext)
		}
		dir := filepath.Dir(filePath)
		if err := t.fs.MkdirAll(dir, 0755); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error creating directory: %s", err.Error())), nil
		}
		err = t.fs.WriteFile(filePath, string(imageBytes))
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error saving image to file: %s", err.Error())), nil
		}
		results = append(results, fmt.Sprintf("Image %d saved to: %s", i+1, filePath))
	}

	resultText := fmt.Sprintf("Successfully generated %d image(s):\n%s",
		len(response.Data),
		fmt.Sprintf("• %s", results[0]))

	if len(results) > 1 {
		for _, result := range results[1:] {
			resultText += fmt.Sprintf("\n• %s", result)
		}
	}
	return dive.NewToolResultText(resultText), nil
}
