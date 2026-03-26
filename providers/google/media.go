package google

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"

	"github.com/deepnoodle-ai/dive/media"
)

// MediaProvider generates images and videos using Google's GenAI SDK.
// Supports Gemini image models, Imagen models, and Veo video models.
type MediaProvider struct {
	apiKey         string
	vertexAI       bool
	location       string
	imagenLocation string

	mu           sync.Mutex
	client       *genai.Client
	imagenClient *genai.Client
}

// MediaOption configures a MediaProvider.
type MediaOption func(*MediaProvider)

// WithMediaAPIKey sets the API key.
func WithMediaAPIKey(key string) MediaOption {
	return func(p *MediaProvider) {
		p.apiKey = key
	}
}

// WithMediaVertexAI enables the Vertex AI backend.
func WithMediaVertexAI(location string) MediaOption {
	return func(p *MediaProvider) {
		p.vertexAI = true
		p.location = location
	}
}

// WithMediaImagenLocation sets a separate location for Imagen models.
func WithMediaImagenLocation(location string) MediaOption {
	return func(p *MediaProvider) {
		p.imagenLocation = location
	}
}

// NewMediaProvider creates a new Google MediaProvider.
func NewMediaProvider(opts ...MediaOption) *MediaProvider {
	p := &MediaProvider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *MediaProvider) ensureClient(ctx context.Context) (*genai.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	var client *genai.Client
	var err error

	if p.vertexAI {
		location := p.location
		if location == "" {
			location = "global"
		}
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Location: location,
		})
	} else {
		apiKey := p.apiKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey == "" {
			apiKey = os.Getenv("GOOGLE_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY must be set")
		}
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			Backend: genai.BackendGeminiAPI,
			APIKey:  apiKey,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}
	p.client = client

	// Create separate Imagen client if a different region is specified.
	if p.vertexAI && p.imagenLocation != "" && p.imagenLocation != p.location {
		imagenClient, err := genai.NewClient(ctx, &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Location: p.imagenLocation,
		})
		if err != nil {
			return nil, fmt.Errorf("creating imagen client (location=%s): %w", p.imagenLocation, err)
		}
		p.imagenClient = imagenClient
	}

	return p.client, nil
}

// GenerateImage implements media.ImageProvider.
func (p *MediaProvider) GenerateImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error) {
	if _, err := p.ensureClient(ctx); err != nil {
		return nil, err
	}

	model := config.Model
	aspectRatio := config.AspectRatio
	if aspectRatio == media.AspectAuto {
		aspectRatio = media.Aspect1x1
	}

	if strings.HasPrefix(model, "gemini-") {
		return p.generateImageWithGemini(ctx, prompt, model, aspectRatio, config)
	}
	return p.generateImageWithImagen(ctx, prompt, model, aspectRatio, config)
}

func (p *MediaProvider) generateImageWithGemini(ctx context.Context, prompt, model string, aspectRatio media.AspectRatio, config *media.Config) ([]*media.ImageResult, error) {
	parts := []*genai.Part{genai.NewPartFromText(prompt)}
	for _, imgData := range config.ReferenceImages {
		mimeType := media.DetectMIMEFromBytes(imgData)
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: mimeType,
				Data:     imgData,
			},
		})
	}

	genConfig := &genai.GenerateContentConfig{
		ResponseModalities: []string{"IMAGE"},
		ImageConfig: &genai.ImageConfig{
			AspectRatio: string(aspectRatio),
		},
	}

	resp, err := p.client.Models.GenerateContent(ctx, model, []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}, genConfig)
	if err != nil {
		return nil, fmt.Errorf("gemini image generation: %w", err)
	}
	if resp == nil || len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("empty response from gemini")
	}

	var results []*media.ImageResult
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.InlineData != nil && part.InlineData.Data != nil {
			if part.Thought {
				continue
			}
			mimeType := part.InlineData.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			width, height := media.StandardImageDimensions(aspectRatio)
			format := media.FormatFromMIME(mimeType)
			img := &media.ImageResult{
				Data:     part.InlineData.Data,
				Model:    model,
				Format:   format,
				MimeType: mimeType,
				Width:    width,
				Height:   height,
				Metadata: map[string]any{"provider": "google"},
			}
			results = append(results, img)
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no image in gemini response")
	}

	// Convert format if requested and different from provider output
	if config.OutputFormat != "" {
		for _, img := range results {
			if img.Format != config.OutputFormat {
				converted, err := media.ConvertImage(img.Data, config.OutputFormat)
				if err != nil {
					continue // Can't convert (e.g. webp target) — keep original
				}
				img.Data = converted
				img.Format = config.OutputFormat
				img.MimeType = config.OutputFormat.MIMEType()
			}
		}
	}
	return results, nil
}

func (p *MediaProvider) generateImageWithImagen(ctx context.Context, prompt, model string, aspectRatio media.AspectRatio, config *media.Config) ([]*media.ImageResult, error) {
	safetyFilter := genai.SafetyFilterLevelBlockOnlyHigh
	if strings.HasPrefix(model, "imagen-4") {
		safetyFilter = genai.SafetyFilterLevelBlockLowAndAbove
	}

	outputMIME := "image/png"
	if config.OutputFormat != "" {
		outputMIME = config.OutputFormat.MIMEType()
	}

	genConfig := &genai.GenerateImagesConfig{
		OutputMIMEType:    outputMIME,
		NumberOfImages:    int32(config.Count),
		AddWatermark:      false,
		PersonGeneration:  genai.PersonGenerationAllowAll,
		SafetyFilterLevel: safetyFilter,
		AspectRatio:       string(aspectRatio),
	}

	client := p.client
	if p.imagenClient != nil {
		client = p.imagenClient
	}
	response, err := client.Models.GenerateImages(ctx, model, prompt, genConfig)
	if err != nil {
		return nil, fmt.Errorf("imagen generation: %w", err)
	}

	var results []*media.ImageResult
	for _, genImage := range response.GeneratedImages {
		if genImage.RAIFilteredReason != "" {
			continue
		}
		if genImage.Image == nil || len(genImage.Image.ImageBytes) == 0 {
			continue
		}
		width, height := media.StandardImageDimensions(aspectRatio)
		mimeType := genImage.Image.MIMEType
		if mimeType == "" {
			mimeType = "image/png"
		}
		format := media.FormatFromMIME(mimeType)
		img := &media.ImageResult{
			Data:     genImage.Image.ImageBytes,
			Model:    model,
			Format:   format,
			MimeType: mimeType,
			Width:    width,
			Height:   height,
			Metadata: map[string]any{"provider": "google"},
		}
		results = append(results, img)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no images generated (may have been filtered by safety)")
	}
	return results, nil
}

// GenerateVideo implements media.VideoProvider.
func (p *MediaProvider) GenerateVideo(ctx context.Context, prompt string, config *media.Config) (*media.VideoResult, error) {
	if _, err := p.ensureClient(ctx); err != nil {
		return nil, err
	}

	model := config.Model
	aspectRatio := config.AspectRatio
	if aspectRatio == media.AspectAuto {
		aspectRatio = media.Aspect16x9
	}

	videoConfig := &genai.GenerateVideosConfig{
		NumberOfVideos: 1,
		AspectRatio:    string(aspectRatio),
	}

	operation, err := p.client.Models.GenerateVideos(ctx, model, prompt, nil, videoConfig)
	if err != nil {
		return nil, fmt.Errorf("veo video generation start: %w", err)
	}

	// Poll until complete, respecting context deadline.
	pollInterval := 10 * time.Second
	for !operation.Done {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("video generation cancelled: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
		operation, err = p.client.Operations.GetVideosOperation(ctx, operation, nil)
		if err != nil {
			return nil, fmt.Errorf("polling video operation: %w", err)
		}
	}

	if operation.Response == nil || len(operation.Response.GeneratedVideos) == 0 {
		return nil, fmt.Errorf("no videos generated")
	}

	firstVideo := operation.Response.GeneratedVideos[0]
	if firstVideo.Video == nil {
		return nil, fmt.Errorf("no video data in response")
	}

	videoData, err := p.client.Files.Download(ctx, genai.NewDownloadURIFromGeneratedVideo(firstVideo), nil)
	if err != nil {
		return nil, fmt.Errorf("downloading video: %w", err)
	}
	if len(videoData) == 0 {
		return nil, fmt.Errorf("empty video data after download")
	}

	width, height := media.StandardVideoDimensions(aspectRatio)
	result := &media.VideoResult{
		Data:        videoData,
		Model:       model,
		Width:       width,
		Height:      height,
		Duration:    config.Duration,
		AspectRatio: aspectRatio,
		Metadata:    map[string]any{"provider": "google"},
	}
	mimeType := firstVideo.Video.MIMEType
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	result.SetVideoFormat(mimeType)
	return result, nil
}

// Compile-time interface checks.
var (
	_ media.ImageProvider = (*MediaProvider)(nil)
	_ media.VideoProvider = (*MediaProvider)(nil)
)
