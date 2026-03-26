package grok

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
	"github.com/openai/openai-go/v3/option"
	_ "golang.org/x/image/webp"

	"github.com/deepnoodle-ai/dive/media"
)

// MediaProvider generates images and videos using the xAI API.
type MediaProvider struct {
	client *openai.Client
}

// NewMediaProvider creates a new Grok MediaProvider.
// Reads XAI_API_KEY or GROK_API_KEY from the environment.
func NewMediaProvider() *MediaProvider {
	apiKey := getAPIKey()
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(DefaultEndpoint),
	)
	return &MediaProvider{client: &client}
}

// GenerateImage implements media.ImageProvider.
func (p *MediaProvider) GenerateImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error) {
	count := config.Count
	if count < 1 {
		count = 1
	}

	model := config.Model
	if model == "" {
		model = ModelImagineImage
	}

	params := openai.ImageGenerateParams{
		Prompt: prompt,
		Model:  openai.ImageModel(model),
		N:      openai.Opt[int64](int64(count)),
	}

	resp, err := p.client.Images.Generate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("grok image generation: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no images in response")
	}

	return decodeGrokImageResults(resp.Data, model)
}

// GenerateVideo implements media.VideoProvider.
func (p *MediaProvider) GenerateVideo(ctx context.Context, prompt string, config *media.Config) (*media.VideoResult, error) {
	model := config.Model
	if model == "" {
		model = ModelImagineVideo
	}

	seconds := grokDurationToSeconds(config.Duration)

	video, err := p.client.Videos.NewAndPoll(ctx, openai.VideoNewParams{
		Model:   openai.VideoModel(model),
		Prompt:  prompt,
		Seconds: openai.VideoSeconds(seconds),
	}, 5000)
	if err != nil {
		return nil, fmt.Errorf("grok video generation: %w", err)
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

	ar := config.AspectRatio
	if ar == "" {
		ar = media.Aspect16x9
	}
	width, height := media.StandardVideoDimensions(ar)
	result := &media.VideoResult{
		Data:        videoData,
		Model:       model,
		Width:       width,
		Height:      height,
		Duration:    config.Duration,
		AspectRatio: ar,
		Metadata: map[string]any{
			"provider": "grok",
			"video_id": video.ID,
		},
	}
	result.SetVideoFormat("video/mp4")
	return result, nil
}

// decodeGrokImageResults extracts image data from xAI response items.
func decodeGrokImageResults(data []openai.Image, model string) ([]*media.ImageResult, error) {
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
			Metadata: map[string]any{"provider": "grok"},
		}
		results = append(results, img)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no images in response")
	}
	return results, nil
}

// grokDurationToSeconds maps a time.Duration to a Grok-compatible seconds string.
func grokDurationToSeconds(d time.Duration) string {
	sec := int(d.Seconds())
	if sec >= 10 {
		return "10"
	}
	return "5"
}

// Compile-time interface checks.
var (
	_ media.ImageProvider = (*MediaProvider)(nil)
	_ media.VideoProvider = (*MediaProvider)(nil)
)
