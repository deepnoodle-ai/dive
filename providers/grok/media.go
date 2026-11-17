package grok

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"net/http"
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

	// videoPollInterval is the wait between video job polls. Zero means
	// defaultVideoPollInterval. Tests set it to a short value.
	videoPollInterval time.Duration
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
		Prompt:         prompt,
		Model:          openai.ImageModel(model),
		N:              openai.Opt[int64](int64(count)),
		ResponseFormat: openai.ImageGenerateParamsResponseFormatB64JSON,
	}

	resp, err := p.client.Images.Generate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("grok image generation: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no images in response")
	}

	return decodeGrokImageResults(ctx, resp.Data, model)
}

// GenerateVideo implements media.VideoProvider. It uses xAI's native video
// API: it starts a job, polls it until it finishes, and downloads the video
// from the temporary URL in the result.
func (p *MediaProvider) GenerateVideo(ctx context.Context, prompt string, config *media.Config) (*media.VideoResult, error) {
	model := config.Model
	if model == "" {
		model = ModelImagineVideo
	}
	seconds := grokDurationToSeconds(config.Duration)
	ar := config.AspectRatio
	if ar == "" || ar == media.AspectAuto {
		ar = media.Aspect16x9
	}

	var started struct {
		RequestID string `json:"request_id"`
	}
	err := p.client.Post(ctx, "videos/generations", map[string]any{
		"model":        model,
		"prompt":       prompt,
		"duration":     seconds,
		"aspect_ratio": string(ar),
	}, &started)
	if err != nil {
		return nil, fmt.Errorf("grok video generation: %w", err)
	}
	if started.RequestID == "" {
		return nil, fmt.Errorf("grok video generation: response has no request_id")
	}

	job, err := p.pollVideo(ctx, started.RequestID)
	if err != nil {
		return nil, err
	}

	videoData, err := downloadURL(ctx, job.Video.URL, maxMediaSize)
	if err != nil {
		return nil, fmt.Errorf("downloading video: %w", err)
	}

	width, height := media.StandardVideoDimensions(ar)
	duration := time.Duration(seconds) * time.Second
	if job.Video.Duration > 0 {
		duration = time.Duration(job.Video.Duration * float64(time.Second))
	}
	metadata := map[string]any{
		"provider":   "grok",
		"request_id": started.RequestID,
	}
	if job.Usage.CostInUSDTicks > 0 {
		metadata["cost_usd"] = float64(job.Usage.CostInUSDTicks) / usdTicksPerDollar
	}
	if job.Model != "" {
		model = job.Model
	}
	result := &media.VideoResult{
		Data:        videoData,
		Model:       model,
		Width:       width,
		Height:      height,
		Duration:    duration,
		AspectRatio: ar,
		Metadata:    metadata,
	}
	result.SetVideoFormat("video/mp4")
	return result, nil
}

// usdTicksPerDollar converts xAI's usage.cost_in_usd_ticks to dollars.
const usdTicksPerDollar = 1e10

const defaultVideoPollInterval = 5 * time.Second

// grokVideoJob is the status of an xAI video job.
type grokVideoJob struct {
	Status string `json:"status"`
	Model  string `json:"model"`
	Video  struct {
		URL      string  `json:"url"`
		Duration float64 `json:"duration"`
	} `json:"video"`
	Usage struct {
		CostInUSDTicks int64 `json:"cost_in_usd_ticks"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// pollVideo waits until the video job is done or failed, or ctx ends.
func (p *MediaProvider) pollVideo(ctx context.Context, requestID string) (*grokVideoJob, error) {
	interval := p.videoPollInterval
	if interval == 0 {
		interval = defaultVideoPollInterval
	}
	for {
		var job grokVideoJob
		if err := p.client.Get(ctx, "videos/"+requestID, nil, &job); err != nil {
			return nil, fmt.Errorf("polling grok video %s: %w", requestID, err)
		}
		switch job.Status {
		case "done":
			if job.Video.URL == "" {
				return nil, fmt.Errorf("grok video %s is done but has no video URL", requestID)
			}
			return &job, nil
		case "failed", "expired":
			msg := job.Error.Message
			if msg == "" {
				msg = "no reason given"
			}
			return nil, fmt.Errorf("grok video %s %s: %s", requestID, job.Status, msg)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// decodeGrokImageResults extracts image data from xAI response items.
// Handles both base64 and URL response formats.
func decodeGrokImageResults(ctx context.Context, data []openai.Image, model string) ([]*media.ImageResult, error) {
	var results []*media.ImageResult
	for _, item := range data {
		var imageData []byte
		var err error

		switch {
		case item.B64JSON != "":
			imageData, err = base64.StdEncoding.DecodeString(item.B64JSON)
			if err != nil {
				return nil, fmt.Errorf("decoding image data: %w", err)
			}
		case item.URL != "":
			imageData, err = downloadURL(ctx, item.URL, maxMediaSize)
			if err != nil {
				return nil, fmt.Errorf("downloading image from URL: %w", err)
			}
		default:
			continue
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

// maxMediaSize is the largest image or video that downloadURL accepts.
const maxMediaSize = 200 * 1024 * 1024 // 200 MB

// downloadURL fetches image or video data from a URL. It returns an error,
// not truncated data, when the body is larger than limit bytes.
func downloadURL(ctx context.Context, url string, limit int64) ([]byte, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching media", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("media larger than %d MB", limit/(1024*1024))
	}
	return data, nil
}

// grokDurationToSeconds converts a time.Duration to whole seconds for the
// Grok video API. Clamps to the 1–15 second range, defaulting to 5.
func grokDurationToSeconds(d time.Duration) int {
	sec := int(d.Seconds())
	if sec < 1 {
		return 5
	}
	return min(sec, 15)
}

// Compile-time interface checks.
var (
	_ media.ImageProvider = (*MediaProvider)(nil)
	_ media.VideoProvider = (*MediaProvider)(nil)
)
