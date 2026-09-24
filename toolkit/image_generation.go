package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/schema"
	"golang.org/x/image/draw"
)

// maxInlineImageBase64 caps the base64 size of the image returned to the
// model. Anthropic rejects a tool-result image over 5 MB, the tightest limit
// among the providers, so an image under it is accepted everywhere.
const maxInlineImageBase64 = 5 * 1024 * 1024

// inlineImageMaxEdge is the long edge an oversized image is scaled down to.
// Anthropic downsizes anything larger before the model sees it, so a smaller
// copy loses nothing the model could use.
const inlineImageMaxEdge = 1568

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
		"Saves the image to disk and returns both the file path and the image itself, "+
		"so you can see and check what was generated. "+
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
		resolved, err := validateOutputPath(input.OutputPath, t.workDir)
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
	text := display
	img, note := inlineImage(result)
	if note != "" {
		text += "\n" + note
	}
	content := []*dive.ToolResultContent{{Type: dive.ToolResultContentTypeText, Text: text}}
	if img != nil {
		content = append(content, img)
	}
	return dive.NewToolResult(content...).WithDisplay(display), nil
}

// inlineImage returns the image content block for the model, plus a note for
// the text block when the image is not shown as generated.
//
// An image within maxInlineImageBase64 is sent unchanged. A larger one is
// scaled to inlineImageMaxEdge and re-encoded as JPEG; if that fails or is
// still too large, the image is left out and the note says so. The file on
// disk is always the original.
func inlineImage(result *media.ImageResult) (*dive.ToolResultContent, string) {
	mimeType := result.MimeType
	if mimeType == "" {
		mimeType = result.Format.MIMEType()
	}
	if base64.StdEncoding.EncodedLen(len(result.Data)) <= maxInlineImageBase64 {
		return imageContent(result.Data, mimeType), ""
	}
	small, w, h, err := downscaleJPEG(result.Data, inlineImageMaxEdge)
	if err != nil || base64.StdEncoding.EncodedLen(len(small)) > maxInlineImageBase64 {
		return nil, fmt.Sprintf("The image is too large to show inline (%d bytes); "+
			"it is saved at the path above.", len(result.Data))
	}
	return imageContent(small, "image/jpeg"),
		fmt.Sprintf("The image shown is a %dx%d JPEG preview; the full-size file is saved at the path above.", w, h)
}

// imageContent builds a tool result image block from raw image bytes.
func imageContent(data []byte, mimeType string) *dive.ToolResultContent {
	return &dive.ToolResultContent{
		Type:     dive.ToolResultContentTypeImage,
		Data:     base64.StdEncoding.EncodeToString(data),
		MimeType: mimeType,
	}
}

// downscaleJPEG decodes data, scales it so its long edge is at most maxEdge,
// and encodes the result as JPEG.
func downscaleJPEG(data []byte, maxEdge int) ([]byte, int, int, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if long := max(w, h); long > maxEdge {
		w, h = max(1, w*maxEdge/long), max(1, h*maxEdge/long)
	}
	// JPEG has no alpha, so transparent areas are flattened onto white.
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
}

// validateOutputPath validates and resolves a user-provided output path,
// ensuring it is relative and contained within workDir. Uses PathValidator to
// resolve symlinks, preventing both path traversal and symlink-based attacks.
func validateOutputPath(outputPath, workDir string) (string, error) {
	if filepath.IsAbs(outputPath) {
		return "", fmt.Errorf("output_path must be a relative path")
	}
	validator, err := NewPathValidator(workDir)
	if err != nil {
		return "", fmt.Errorf("invalid working directory: %w", err)
	}
	resolved := filepath.Join(workDir, outputPath)
	if err := validator.ValidateWrite(resolved); err != nil {
		return "", fmt.Errorf("output_path must be within the working directory")
	}
	realPath, err := validator.ResolvePath(resolved)
	if err != nil {
		return "", fmt.Errorf("invalid output path: %w", err)
	}
	return realPath, nil
}
