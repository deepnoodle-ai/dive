package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*ImageGenerationInput] = &imageGenerationTool{}

// ImageGenerationInput is the input schema for the image generation tool.
type ImageGenerationInput struct {
	// Prompt is the text description of the image to generate.
	Prompt string `json:"prompt"`

	// AspectRatio is the desired aspect ratio (e.g., "1:1", "16:9", "9:16").
	// Defaults to "1:1" if not specified.
	AspectRatio string `json:"aspect_ratio,omitempty"`

	// OutputPath is the file path to save the image. Auto-generated if omitted.
	OutputPath string `json:"output_path,omitempty"`

	// Format is the output format: "png", "jpeg", or "webp". Defaults to "png".
	Format string `json:"format,omitempty"`
}

// ImageGenerationToolOption configures the image generation tool.
type ImageGenerationToolOption func(*imageGenerationTool)

// WithImageToolWorkDir sets the working directory for output files.
func WithImageToolWorkDir(dir string) ImageGenerationToolOption {
	return func(t *imageGenerationTool) {
		t.workDir = dir
	}
}

type imageGenerationTool struct {
	model   string
	workDir string
}

// NewImageGenerationTool creates an image generation tool for the given model.
func NewImageGenerationTool(model string, opts ...ImageGenerationToolOption) *dive.TypedToolAdapter[*ImageGenerationInput] {
	t := &imageGenerationTool{model: model}
	for _, opt := range opts {
		opt(t)
	}
	if t.workDir == "" {
		t.workDir, _ = os.Getwd()
	}
	return dive.ToolAdapter(t)
}

func (t *imageGenerationTool) Name() string { return "ImageGeneration" }

func (t *imageGenerationTool) Description() string {
	return fmt.Sprintf("Generate an image from a text prompt using %s. "+
		"Saves the image to disk and returns the file path. "+
		"Use descriptive, detailed prompts for best results.", t.model)
}

func (t *imageGenerationTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"prompt"},
		Properties: map[string]*schema.Property{
			"prompt": {
				Type:        "string",
				Description: "Detailed text description of the image to generate",
			},
			"aspect_ratio": {
				Type:        "string",
				Description: "Aspect ratio: 1:1 (square), 16:9 (landscape), 9:16 (portrait)",
				Enum:        []any{"1:1", "16:9", "9:16"},
			},
			"output_path": {
				Type:        "string",
				Description: "File path to save the image. Auto-generated from the prompt if omitted.",
			},
			"format": {
				Type:        "string",
				Description: "Output format",
				Enum:        []any{"png", "jpeg", "webp"},
			},
		},
	}
}

func (t *imageGenerationTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "ImageGeneration",
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *imageGenerationTool) Call(ctx context.Context, input *ImageGenerationInput) (*dive.ToolResult, error) {
	if input.Prompt == "" {
		return NewToolResultError("prompt is required"), nil
	}

	var opts []media.Option
	opts = append(opts, media.WithModel(t.model))
	opts = append(opts, media.WithTimeout(5*time.Minute))

	if input.AspectRatio != "" {
		opts = append(opts, media.WithAspectRatio(media.AspectRatio(input.AspectRatio)))
	}
	if input.Format != "" {
		format := media.Format(input.Format)
		if err := media.ValidateFormat(format); err != nil {
			return NewToolResultError(err.Error()), nil
		}
		opts = append(opts, media.WithOutputFormat(format))
	}

	result, err := media.GenerateImage(ctx, input.Prompt, opts...)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("image generation failed: %v", err)), nil
	}

	// Determine output path, constrained to workDir
	outPath := input.OutputPath
	if outPath == "" {
		slug := media.SlugifyPrompt(input.Prompt, 40)
		outPath = filepath.Join(t.workDir, slug+result.Format.FileExtension())
	} else {
		resolved, err := resolveOutputPath(input.OutputPath, t.workDir)
		if err != nil {
			return NewToolResultError(err.Error()), nil
		}
		outPath = resolved
	}

	outPath, err = result.WriteTo(outPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("failed to save image: %v", err)), nil
	}

	absPath, _ := filepath.Abs(outPath)
	display := fmt.Sprintf("Generated image: %s (%dx%d %s)", absPath, result.Width, result.Height, result.Format)
	return dive.NewToolResultText(absPath).WithDisplay(display), nil
}

// resolveOutputPath validates and resolves a user-provided output path,
// ensuring it is relative and contained within workDir. This prevents
// path traversal attacks (e.g. "../../../etc/passwd") and absolute path writes.
func resolveOutputPath(outputPath, workDir string) (string, error) {
	if filepath.IsAbs(outputPath) {
		return "", fmt.Errorf("output_path must be a relative path")
	}
	resolved := filepath.Join(workDir, outputPath)
	absOut, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("invalid output path: %v", err)
	}
	absWork, _ := filepath.Abs(workDir)
	if !strings.HasPrefix(absOut, absWork+string(filepath.Separator)) && absOut != absWork {
		return "", fmt.Errorf("output_path must be within the working directory")
	}
	return absOut, nil
}
