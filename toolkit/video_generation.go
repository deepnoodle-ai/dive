package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*VideoGenerationInput] = &videoGenerationTool{}

// VideoGenerationInput is the input schema for the video generation tool.
type VideoGenerationInput struct {
	// Prompt is the text description of the video to generate.
	Prompt string `json:"prompt"`

	// Duration is the target video duration (e.g., "4s", "8s", "12s").
	// Defaults to "8s" if not specified.
	Duration string `json:"duration,omitempty"`

	// AspectRatio is the desired aspect ratio (e.g., "16:9", "9:16", "1:1").
	// Defaults to "16:9" if not specified.
	AspectRatio string `json:"aspect_ratio,omitempty"`

	// OutputPath is the file path to save the video. Auto-generated if omitted.
	OutputPath string `json:"output_path,omitempty"`
}

// VideoGenerationToolOption configures the video generation tool.
type VideoGenerationToolOption func(*videoGenerationTool)

// WithVideoToolWorkDir sets the working directory for output files.
func WithVideoToolWorkDir(dir string) VideoGenerationToolOption {
	return func(t *videoGenerationTool) {
		t.workDir = dir
	}
}

type videoGenerationTool struct {
	model   string
	workDir string
}

// NewVideoGenerationTool creates a video generation tool for the given model.
func NewVideoGenerationTool(model string, opts ...VideoGenerationToolOption) *dive.TypedToolAdapter[*VideoGenerationInput] {
	t := &videoGenerationTool{model: model}
	for _, opt := range opts {
		opt(t)
	}
	if t.workDir == "" {
		t.workDir, _ = os.Getwd()
	}
	return dive.ToolAdapter(t)
}

func (t *videoGenerationTool) Name() string { return "VideoGeneration" }

func (t *videoGenerationTool) Description() string {
	return fmt.Sprintf("Generate a video from a text prompt using %s. "+
		"Saves the video to disk and returns the file path. "+
		"Video generation can take several minutes. "+
		"Use descriptive, detailed prompts for best results.", t.model)
}

func (t *videoGenerationTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"prompt"},
		Properties: map[string]*schema.Property{
			"prompt": {
				Type:        "string",
				Description: "Detailed text description of the video to generate",
			},
			"duration": {
				Type:        "string",
				Description: "Video duration as a Go duration string (e.g. 8s, 16s, 20s). Exact duration depends on the provider.",
			},
			"aspect_ratio": {
				Type:        "string",
				Description: "Aspect ratio: 16:9 (landscape), 9:16 (portrait), 1:1 (square)",
				Enum:        []any{"16:9", "9:16", "1:1"},
			},
			"output_path": {
				Type:        "string",
				Description: "File path to save the video. Auto-generated from the prompt if omitted.",
			},
		},
	}
}

func (t *videoGenerationTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "VideoGeneration",
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *videoGenerationTool) Call(ctx context.Context, input *VideoGenerationInput) (*dive.ToolResult, error) {
	if input.Prompt == "" {
		return NewToolResultError("prompt is required"), nil
	}

	var opts []media.Option
	opts = append(opts, media.WithModel(t.model))
	opts = append(opts, media.WithTimeout(15*time.Minute))

	duration := 8 * time.Second
	if input.Duration != "" {
		parsed, err := time.ParseDuration(input.Duration)
		if err != nil {
			return NewToolResultError(fmt.Sprintf("invalid duration %q: %v", input.Duration, err)), nil
		}
		duration = parsed
	}
	opts = append(opts, media.WithDuration(duration))

	if input.AspectRatio != "" {
		opts = append(opts, media.WithAspectRatio(media.AspectRatio(input.AspectRatio)))
	}

	result, err := media.GenerateVideo(ctx, input.Prompt, opts...)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("video generation failed: %v", err)), nil
	}

	// Determine output path, constrained to workDir
	outPath := input.OutputPath
	if outPath == "" {
		slug := media.SlugifyPrompt(input.Prompt, 40)
		ext := ".mp4"
		if result.Format == "webm" {
			ext = ".webm"
		}
		outPath = filepath.Join(t.workDir, slug+ext)
	} else {
		resolved, err := resolveOutputPath(input.OutputPath, t.workDir)
		if err != nil {
			return NewToolResultError(err.Error()), nil
		}
		outPath = resolved
	}

	outPath, err = result.WriteTo(outPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("failed to save video: %v", err)), nil
	}

	absPath, _ := filepath.Abs(outPath)
	display := fmt.Sprintf("Generated video: %s (%dx%d %s, %s)",
		absPath, result.Width, result.Height, result.Format, result.Duration.Round(time.Second))
	return dive.NewToolResultText(absPath).WithDisplay(display), nil
}
