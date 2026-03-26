package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"time"

	// Register image decoders for DecodeConfig.
	_ "image/jpeg"
	_ "image/png"

	"github.com/openai/openai-go/v3"
	_ "golang.org/x/image/webp"

	"github.com/deepnoodle-ai/dive/media"
)

// MediaProvider generates images and videos using OpenAI APIs.
//
// Supported image models: gpt-image-1.5, gpt-image-1, gpt-image-1-mini
// Supported video models: sora-2, sora-2-pro
type MediaProvider struct {
	client *openai.Client
}

// NewMediaProvider creates a new OpenAI MediaProvider.
// Reads OPENAI_API_KEY from the environment.
func NewMediaProvider() *MediaProvider {
	client := openai.NewClient()
	return &MediaProvider{client: &client}
}

// GenerateImage implements media.ImageProvider.
func (p *MediaProvider) GenerateImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error) {
	size := aspectRatioToSize(config.AspectRatio)
	count := config.Count
	if count < 1 {
		count = 1
	}

	model := config.Model
	if model == "" {
		model = "gpt-image-1"
	}

	params := openai.ImageGenerateParams{
		Prompt:  prompt,
		Model:   openai.ImageModel(model),
		Size:    openai.ImageGenerateParamsSize(size),
		Quality: openai.ImageGenerateParamsQualityHigh,
		N:       openai.Opt[int64](int64(count)),
	}
	if config.OutputFormat != "" {
		params.OutputFormat = openai.ImageGenerateParamsOutputFormat(string(config.OutputFormat))
	}

	resp, err := p.client.Images.Generate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai image generation: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no images in response")
	}

	return decodeImageResults(resp.Data, model)
}

// EditImage implements media.ImageEditor.
// Uses the Images.Edit endpoint with the configured model.
func (p *MediaProvider) EditImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error) {
	if len(config.ReferenceImages) == 0 {
		return nil, fmt.Errorf("reference image data is required for editing")
	}

	model := config.Model
	if model == "" {
		model = "gpt-image-1"
	}
	size := aspectRatioToSize(config.AspectRatio)
	count := config.Count
	if count < 1 {
		count = 1
	}

	// Build image input from reference images.
	readers := make([]io.Reader, len(config.ReferenceImages))
	for i, imgData := range config.ReferenceImages {
		readers[i] = bytes.NewReader(imgData)
	}
	var imageInput openai.ImageEditParamsImageUnion
	if len(readers) == 1 {
		imageInput.OfFile = readers[0]
	} else {
		imageInput.OfFileArray = readers
	}

	params := openai.ImageEditParams{
		Image:  imageInput,
		Prompt: prompt,
		Model:  openai.ImageModel(model),
		Size:   openai.ImageEditParamsSize(size),
		N:      openai.Opt[int64](int64(count)),
	}

	resp, err := p.client.Images.Edit(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai image edit: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no images in edit response")
	}

	results, err := decodeImageResults(resp.Data, model)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if r.Metadata == nil {
			r.Metadata = map[string]any{}
		}
		r.Metadata["mode"] = "edit"
	}
	return results, nil
}

// GenerateVideo implements media.VideoProvider.
func (p *MediaProvider) GenerateVideo(ctx context.Context, prompt string, config *media.Config) (*media.VideoResult, error) {
	model := config.Model
	if model == "" {
		model = "sora-2"
	}

	size := aspectRatioToVideoSize(config.AspectRatio, model)
	seconds := durationToSeconds(config.Duration)

	video, err := p.client.Videos.NewAndPoll(ctx, openai.VideoNewParams{
		Model:   openai.VideoModel(model),
		Prompt:  prompt,
		Size:    openai.VideoSize(size),
		Seconds: openai.VideoSeconds(seconds),
	}, 5000)
	if err != nil {
		return nil, fmt.Errorf("openai video generation: %w", err)
	}

	if video.Status != openai.VideoStatusCompleted {
		errMsg := "unknown error"
		if video.Error.Message != "" {
			errMsg = video.Error.Message
		}
		return nil, fmt.Errorf("video generation failed (status: %s): %s", video.Status, errMsg)
	}

	resp, err := p.client.Videos.DownloadContent(ctx, video.ID, openai.VideoDownloadContentParams{})
	if err != nil {
		return nil, fmt.Errorf("downloading video: %w", err)
	}
	defer resp.Body.Close()

	videoData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading video data: %w", err)
	}

	ar := videoSizeToAspectRatio(size)
	width, height := media.StandardVideoDimensions(ar)
	duration := config.Duration
	if duration == 0 {
		if parsed, err := time.ParseDuration(seconds + "s"); err == nil {
			duration = parsed
		}
	}
	result := &media.VideoResult{
		Data:        videoData,
		Model:       string(video.Model),
		Width:       width,
		Height:      height,
		Duration:    duration,
		AspectRatio: ar,
		Metadata: map[string]any{
			"provider": "openai",
			"video_id": video.ID,
		},
	}
	result.SetVideoFormat("video/mp4")
	return result, nil
}

// decodeImageResults extracts image data from OpenAI response items.
func decodeImageResults(data []openai.Image, model string) ([]*media.ImageResult, error) {
	var results []*media.ImageResult
	for _, item := range data {
		if item.B64JSON == "" {
			continue
		}
		imageData, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("decoding image data: %w", err)
		}
		format := media.DetectFormat(imageData)
		var width, height int
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(imageData)); err == nil {
			width = cfg.Width
			height = cfg.Height
		}
		img := &media.ImageResult{
			Data:     imageData,
			Model:    model,
			Format:   format,
			MimeType: format.MIMEType(),
			Width:    width,
			Height:   height,
			Metadata: map[string]any{"provider": "openai"},
		}
		results = append(results, img)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no images in response")
	}
	return results, nil
}

// aspectRatioToSize converts a media.AspectRatio to an OpenAI image size string.
// gpt-image models support: 1024x1024, 1536x1024 (landscape), 1024x1536 (portrait),
// and auto.
func aspectRatioToSize(ar media.AspectRatio) string {
	switch ar {
	case media.Aspect16x9:
		return "1536x1024"
	case media.Aspect9x16:
		return "1024x1536"
	default:
		return "1024x1024"
	}
}

// aspectRatioToVideoSize converts a media.AspectRatio to an OpenAI video size string.
// sora-2-pro supports 1080p; sora-2 supports 720p.
func aspectRatioToVideoSize(ar media.AspectRatio, model string) string {
	isPro := model == "sora-2-pro"
	switch ar {
	case media.Aspect9x16:
		if isPro {
			return "1080x1920"
		}
		return "720x1280"
	default:
		if isPro {
			return "1920x1080"
		}
		return "1280x720"
	}
}

// videoSizeToAspectRatio converts an OpenAI video size to media.AspectRatio.
func videoSizeToAspectRatio(size string) media.AspectRatio {
	switch size {
	case "720x1280", "1024x1536", "1080x1920":
		return media.Aspect9x16
	default:
		return media.Aspect16x9
	}
}

// durationToSeconds maps a time.Duration to an OpenAI Sora-compatible seconds string.
// Sora only supports discrete values: 8, 16, and 20 seconds.
// Requested durations are rounded up to the nearest supported value.
// Any duration under 16s maps to "8", 16-19s maps to "16", and 20s+ maps to "20".
func durationToSeconds(d time.Duration) string {
	sec := int(d.Seconds())
	if sec >= 20 {
		return "20"
	}
	if sec >= 16 {
		return "16"
	}
	return "8"
}

// Compile-time interface checks.
var (
	_ media.ImageProvider = (*MediaProvider)(nil)
	_ media.ImageEditor   = (*MediaProvider)(nil)
	_ media.VideoProvider = (*MediaProvider)(nil)
)
