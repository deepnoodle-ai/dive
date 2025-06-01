package openai

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
)

var (
	_ llm.Tool = &ImageGenerationTool{}
)

/* A tool definition must be added in the request that looks like this:
   "tools": [{
       "type": "image_generation",
       "size": "1024x1024",
       "quality": "high",
       "background": "auto",
       "partial_images": 2
   }]
*/

type ImageGenerationImageMask struct {
	FileID   string
	ImageURL string
}

// ImageGenerationToolOptions are the options used to configure an ImageGenerationTool.
type ImageGenerationToolOptions struct {
	Model             string // "gpt-image-1"
	Size              string // "1024x1024", "1024x1536", etc. or "auto"
	Quality           string // "low", "medium", "high", or "auto"
	Background        string // "transparent", "opaque", or "auto"
	OutputCompression *int   // 0-100 for JPEG/WebP formats
	OutputFormat      string // "jpeg", "webp", or "png"
	PartialImages     int    // 0-3 for streaming partial images
	Moderation        string // "auto", "low"
	ImageMask         *ImageGenerationImageMask
}

// NewImageGenerationTool creates a new ImageGenerationTool with the given options.
func NewImageGenerationTool(opts ImageGenerationToolOptions) *ImageGenerationTool {
	return &ImageGenerationTool{
		model:             opts.Model,
		size:              opts.Size,
		quality:           opts.Quality,
		background:        opts.Background,
		partialImages:     opts.PartialImages,
		outputCompression: opts.OutputCompression,
		outputFormat:      opts.OutputFormat,
		moderation:        opts.Moderation,
		imageMask:         opts.ImageMask,
	}
}

// ImageGenerationTool is a tool that allows models to generate images. This is
// provided by OpenAI as a server-side tool. Learn more:
// https://platform.openai.com/docs/guides/image-generation
type ImageGenerationTool struct {
	model             string
	size              string
	quality           string
	background        string
	outputCompression *int
	outputFormat      string
	partialImages     int
	moderation        string
	imageMask         *ImageGenerationImageMask
}

func (t *ImageGenerationTool) Name() string {
	return "image_generation"
}

func (t *ImageGenerationTool) Description() string {
	return "Uses OpenAI's image generation feature to create images from text prompts using the GPT Image model."
}

func (t *ImageGenerationTool) Schema() schema.Schema {
	return schema.Schema{} // Empty for server-side tools
}

// func (t *ImageGenerationTool) ToolConfiguration(providerName string) map[string]any {
// 	config := map[string]any{
// 		"type": "image_generation",
// 	}
// 	if t.model != "" {
// 		config["model"] = t.model
// 	}
// 	if t.size != "" {
// 		config["size"] = t.size
// 	}
// 	if t.quality != "" {
// 		config["quality"] = t.quality
// 	}
// 	if t.background != "" {
// 		config["background"] = t.background
// 	}
// 	if t.partialImages > 0 {
// 		config["partial_images"] = t.partialImages
// 	}
// 	if t.outputCompression != nil {
// 		config["output_compression"] = *t.outputCompression
// 	}
// 	if t.outputFormat != "" {
// 		config["output_format"] = t.outputFormat
// 	}
// 	return config
// }

func (t *ImageGenerationTool) Param() *responses.ToolImageGenerationParam {
	param := &responses.ToolImageGenerationParam{
		Model:        t.model,
		Size:         t.size,
		Quality:      t.quality,
		Background:   t.background,
		Moderation:   t.moderation,
		OutputFormat: t.outputFormat,
	}
	if t.partialImages > 0 {
		value := int64(t.partialImages)
		param.PartialImages = openai.Int(value)
	}
	if t.outputCompression != nil {
		value := int64(*t.outputCompression)
		param.OutputCompression = openai.Int(value)
	}
	if t.imageMask != nil {
		param.InputImageMask = responses.ToolImageGenerationInputImageMaskParam{
			ImageURL: openai.String(t.imageMask.ImageURL),
			FileID:   openai.String(t.imageMask.FileID),
		}
	}
	return param
}
