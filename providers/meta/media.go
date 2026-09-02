package meta

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"time"

	// Register image decoders for DecodeConfig.
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

var (
	_ media.ImageProvider = &MediaProvider{}
	_ media.ImageEditor   = &MediaProvider{}
)

// MediaProvider generates and edits images with Muse Image.
//
// Muse Image is agentic: it runs its own web and image search before rendering,
// so a prompt naming a real place, product, or current fact is grounded without
// any tool being configured. That search is part of the per-image price rather
// than billed as search grounding, and the lookups are not surfaced as separate
// tool calls — the finished image is all that comes back.
type MediaProvider struct {
	client                *openai.Client
	transcriptionMode     TranscriptionMode
	transcriptionKeywords []string
	languageBias          []string
}

// NewMediaProvider creates a Meta MediaProvider, reading MODEL_API_KEY or
// META_API_KEY from the environment.
func NewMediaProvider(opts ...Option) *MediaProvider {
	cfg := &config{apiKey: getAPIKey(), endpoint: DefaultEndpoint}
	for _, opt := range opts {
		opt(cfg)
	}
	requestOptions := []option.RequestOption{
		option.WithAPIKey(cfg.apiKey),
		option.WithBaseURL(cfg.endpoint),
	}
	if cfg.client != nil {
		requestOptions = append(requestOptions, option.WithHTTPClient(cfg.client))
	}
	client := openai.NewClient(requestOptions...)
	return &MediaProvider{
		client:                &client,
		transcriptionMode:     cfg.transcriptionMode,
		transcriptionKeywords: cfg.transcriptionKeywords,
		languageBias:          cfg.languageBias,
	}
}

// imageSize renders an aspect ratio as the "WxH" string Muse Image expects.
//
// The numbers set the aspect ratio, not the pixel dimensions: Meta renders at
// the generator's own resolution, so the returned image will not match them
// exactly. An empty ratio omits the field and takes Meta's default shape.
func imageSize(ratio media.AspectRatio) string {
	if ratio == media.AspectAuto {
		return ""
	}
	width, height := media.StandardImageDimensions(ratio)
	return fmt.Sprintf("%dx%d", width, height)
}

// outputFormat maps Dive's format to Meta's, which defaults to webp.
func outputFormat(format media.Format) string {
	switch format {
	case media.FormatPNG, media.FormatJPEG, media.FormatWebP:
		return string(format)
	default:
		return ""
	}
}

func imageCount(config *media.Config) int64 {
	if config.Count < 1 {
		return 1
	}
	return int64(config.Count)
}

func imageModel(config *media.Config) string {
	if config.Model != "" {
		return config.Model
	}
	return ModelMuseImage
}

// GenerateImage implements media.ImageProvider.
func (p *MediaProvider) GenerateImage(
	ctx context.Context,
	prompt string,
	config *media.Config,
) ([]*media.ImageResult, error) {
	model := imageModel(config)

	params := openai.ImageGenerateParams{
		Prompt:         prompt,
		Model:          openai.ImageModel(model),
		N:              openai.Opt[int64](imageCount(config)),
		ResponseFormat: openai.ImageGenerateParamsResponseFormatB64JSON,
	}
	if size := imageSize(config.AspectRatio); size != "" {
		params.Size = openai.ImageGenerateParamsSize(size)
	}
	extras := map[string]any{}
	if format := outputFormat(config.OutputFormat); format != "" {
		extras["output_format"] = format
	}
	if len(extras) > 0 {
		params.SetExtraFields(extras)
	}

	response, err := p.client.Images.Generate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("meta image generation: %w", err)
	}
	return decodeImageResults(ctx, response.Data, model)
}

// EditImage implements media.ImageEditor.
//
// The multipart body is written here rather than through the SDK's
// ImageEditParams because the two disagree on how to name the parts: the SDK
// emits "image[]" for a file array, and Meta rejects that with `image[]` is not
// a valid image key; use `image[N]` with indices numbered consecutively from 0.
// Everything else still goes through the SDK client, so base URL, auth, retries
// and transport are unchanged.
func (p *MediaProvider) EditImage(
	ctx context.Context,
	prompt string,
	config *media.Config,
) ([]*media.ImageResult, error) {
	if len(config.ReferenceImages) == 0 {
		return nil, fmt.Errorf("meta image edit: at least one reference image is required")
	}
	model := imageModel(config)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := [][2]string{
		{"model", model},
		{"prompt", prompt},
		{"n", strconv.FormatInt(imageCount(config), 10)},
		{"response_format", "b64_json"},
	}
	if size := imageSize(config.AspectRatio); size != "" {
		fields = append(fields, [2]string{"size", size})
	}
	if format := outputFormat(config.OutputFormat); format != "" {
		fields = append(fields, [2]string{"output_format", format})
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, fmt.Errorf("meta image edit: %w", err)
		}
	}
	for i, reference := range config.ReferenceImages {
		format := media.DetectFormat(reference)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(
			`form-data; name="image[%d]"; filename=%q`, i, editUploadName(i, format)))
		header.Set("Content-Type", format.MIMEType())
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, fmt.Errorf("meta image edit: %w", err)
		}
		if _, err := part.Write(reference); err != nil {
			return nil, fmt.Errorf("meta image edit: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("meta image edit: %w", err)
	}

	var response openai.ImagesResponse
	if err := p.client.Post(ctx, "images/edits", nil, &response,
		option.WithRequestBody(writer.FormDataContentType(), body.Bytes()),
	); err != nil {
		return nil, fmt.Errorf("meta image edit: %w", err)
	}
	return decodeImageResults(ctx, response.Data, model)
}

// editUploadName names one multipart part. Format.FileExtension already
// carries the leading dot, so composing one in here as well produced
// "image-0..png" on every upload.
func editUploadName(index int, format media.Format) string {
	return fmt.Sprintf("image-%d%s", index, format.FileExtension())
}

// decodeImageResults turns Meta's response entries into media results. Dive
// always asks for b64_json, but the endpoint can return a signed URL instead
// when response_format is overridden, so both are handled.
func decodeImageResults(
	ctx context.Context,
	data []openai.Image,
	model string,
) ([]*media.ImageResult, error) {
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
			imageData, err = downloadImage(ctx, item.URL)
			if err != nil {
				return nil, fmt.Errorf("downloading image from URL: %w", err)
			}
		default:
			continue
		}
		format := media.DetectFormat(imageData)
		var width, height int
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(imageData)); err == nil {
			width, height = cfg.Width, cfg.Height
		}
		results = append(results, &media.ImageResult{
			Data:     imageData,
			Model:    model,
			Format:   format,
			MimeType: format.MIMEType(),
			Width:    width,
			Height:   height,
			Metadata: map[string]any{"provider": "meta"},
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no images in response")
	}
	return results, nil
}

// downloadImage fetches an image returned as a signed URL rather than inline
// base64. The cap is a guard against an unbounded read, not a documented limit.
func downloadImage(ctx context.Context, url string) ([]byte, error) {
	const maxImageSize = 50 * 1024 * 1024 // 50 MB

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching image", response.StatusCode)
	}
	// Read one byte past the cap so an oversized image is an error rather than
	// silently truncated bytes that go on to be decoded and written to disk.
	data, err := io.ReadAll(io.LimitReader(response.Body, maxImageSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageSize {
		return nil, fmt.Errorf("image exceeds the %d byte read cap", maxImageSize)
	}
	return data, nil
}
