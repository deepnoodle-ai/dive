package openairesponses

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
)

var (
	_ llm.Tool              = &ImageGenerationTool{}
	_ llm.ToolConfiguration = &ImageGenerationTool{}
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

// ImageGenerationToolOptions are the options used to configure an ImageGenerationTool.
type ImageGenerationToolOptions struct {
	Size          string // "1024x1024", "1024x1536", etc. or "auto"
	Quality       string // "low", "medium", "high", or "auto"
	Background    string // "transparent", "opaque", or "auto"
	Compression   *int   // 0-100 for JPEG/WebP formats
	PartialImages *int   // 1-3 for streaming partial images
}

// NewImageGenerationTool creates a new ImageGenerationTool with the given options.
func NewImageGenerationTool(opts ImageGenerationToolOptions) *ImageGenerationTool {
	return &ImageGenerationTool{
		size:          opts.Size,
		quality:       opts.Quality,
		background:    opts.Background,
		compression:   opts.Compression,
		partialImages: opts.PartialImages,
	}
}

// ImageGenerationTool is a tool that allows models to generate images. This is
// provided by OpenAI as a server-side tool. Learn more:
// https://platform.openai.com/docs/guides/image-generation
type ImageGenerationTool struct {
	size          string
	quality       string
	background    string
	compression   *int
	partialImages *int
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

func (t *ImageGenerationTool) ToolConfiguration(providerName string) map[string]any {
	config := map[string]any{
		"type": "image_generation",
	}

	if t.size != "" {
		config["size"] = t.size
	}
	if t.quality != "" {
		config["quality"] = t.quality
	}
	if t.background != "" {
		config["background"] = t.background
	}
	if t.compression != nil {
		config["compression"] = *t.compression
	}
	if t.partialImages != nil {
		config["partial_images"] = *t.partialImages
	}

	return config
}
