# Implementation Plan: Multi-Provider Media Generation

**PRD:** [prd-01-media-generation.md](../prds/prd-01-media-generation.md)
**Last Updated:** 2026-03-25

## Overview

This plan implements the `media` package for Dive, adding unified image and video generation across providers. The work is organized into 6 phases, each producing a working, testable increment. Phases 1-4 are the core library. Phase 5 adds CLI commands. Phase 6 adds agent toolkit integration.

## Guiding Principles

- **Follow existing Dive patterns exactly.** The media registry mirrors `providers/registry.go`. Options use `func(*Config)` closures. Provider constructors use variadic options. Tests use `wonton/assert`.
- **Adapt internal code, don't copy verbatim.** We have battle-tested Google and OpenAI implementations. Extract the provider-specific SDK logic but reshape it behind Dive's unified interfaces.
- **Keep the `media` package independent of `llm`.** Image generation is not LLM inference. The `media` package defines its own interfaces, result types, and registry. Provider packages (e.g., `providers/google`) may register with both registries.
- **Test at every boundary.** Unit tests for types and utilities. Integration tests (skipped without API keys) for provider implementations. Table-driven tests for format detection, aspect ratio mapping, and option application.

---

## Phase 1: Core Types and Utilities

**Goal:** Define the `media` package's foundational types, options, and utility functions. No provider code yet -- just the shapes everything else depends on.

### Files to Create

**`media/aspect_ratio.go`**
```go
type AspectRatio string

const (
    AspectAuto AspectRatio = ""
    Aspect1x1  AspectRatio = "1:1"
    Aspect16x9 AspectRatio = "16:9"
    Aspect9x16 AspectRatio = "9:16"
    Aspect4x3  AspectRatio = "4:3"
    Aspect3x4  AspectRatio = "3:4"
)

// StandardImageDimensions returns default pixel dimensions for an aspect ratio.
func StandardImageDimensions(ar AspectRatio) (width, height int)

// StandardVideoDimensions returns default pixel dimensions for video.
func StandardVideoDimensions(ar AspectRatio) (width, height int)
```

Adapt dimension tables from internal reference code. Include the extended ratios (4:1, 1:4, 8:1, 1:8) but mark them as less common.

**`media/format.go`**
```go
type Format string

const (
    FormatPNG  Format = "png"
    FormatJPEG Format = "jpeg"
    FormatWebP Format = "webp"
)

// DetectFormat inspects magic bytes and returns the image format.
func DetectFormat(data []byte) Format

// MIMEType returns the MIME type string for a format.
func (f Format) MIMEType() string

// FileExtension returns the file extension (with dot) for a format.
func (f Format) FileExtension() string

// ValidateFormat returns an error if the format is not recognized.
func ValidateFormat(f Format) error

// ConvertImage re-encodes image data to the target format.
// Supports PNG and JPEG targets. WebP target returns an error
// (no Go stdlib encoder). If source and target match, returns data unchanged.
func ConvertImage(data []byte, target Format) ([]byte, error)
```

Adapt magic byte detection and conversion from internal reference. Use `image/png`, `image/jpeg`, `golang.org/x/image/webp` (decode only).

**`media/result.go`**
```go
type ImageResult struct {
    Data     []byte
    Model    string
    Format   Format
    MimeType string
    Width    int
    Height   int
    Metadata map[string]any
    Err      error  // non-nil if this result represents a provider failure (fan-out)
}

// WriteTo writes the image data to the given file path.
func (r *ImageResult) WriteTo(path string) error

type VideoResult struct {
    Data        []byte
    Model       string
    Format      string // "mp4", "webm"
    MimeType    string
    Width       int
    Height      int
    Duration    time.Duration
    AspectRatio AspectRatio
    Metadata    map[string]any
}

// WriteTo writes the video data to the given file path.
func (r *VideoResult) WriteTo(path string) error
```

`WriteTo` uses `os.WriteFile` with 0644 permissions. Auto-detect extension from format if path has none.

**`media/options.go`**
```go
type Config struct {
    Model           string
    Models          []string // for fan-out
    AspectRatio     AspectRatio
    OutputFormat    Format
    Count           int
    ReferenceImages [][]byte
    Duration        time.Duration
    Timeout         time.Duration
}

type Option func(*Config)

func WithModel(model string) Option
func WithModels(models ...string) Option
func WithAspectRatio(ar AspectRatio) Option
func WithOutputFormat(f Format) Option
func WithCount(n int) Option
func WithReferenceImage(data []byte) Option
func WithDuration(d time.Duration) Option
func WithTimeout(d time.Duration) Option

// Apply applies all options to the config and sets defaults.
func (c *Config) Apply(opts ...Option)
```

Defaults: Count=1, Timeout=5min (image) / 15min (video), AspectRatio=AspectAuto.

### Tests

**`media/format_test.go`** — Table-driven tests for:
- `DetectFormat` with PNG, JPEG, WebP, and unknown magic bytes
- `Format.MIMEType()` and `Format.FileExtension()`
- `ConvertImage` between PNG and JPEG (use a small test image)
- `ValidateFormat` with valid and invalid inputs

**`media/aspect_ratio_test.go`** — Table-driven tests for:
- `StandardImageDimensions` for each defined ratio
- `StandardVideoDimensions` for each defined ratio
- Unknown ratio returns sensible default (1024x1024 for images, 1920x1080 for video)

**`media/options_test.go`** — Tests for:
- Each `With*` function sets the correct field
- `Apply` with multiple options
- Default values when no options given

**`media/result_test.go`** — Tests for:
- `WriteTo` creates a file with correct contents (use `t.TempDir()`)
- `WriteTo` auto-appends extension if path lacks one

---

## Phase 2: Provider Interface and Registry

**Goal:** Define the provider contracts and the registry that routes model names to providers. Mirrors `providers/registry.go`.

### Files to Create

**`media/provider.go`**
```go
// ImageProvider generates images from text prompts.
type ImageProvider interface {
    GenerateImage(ctx context.Context, prompt string, config *Config) ([]*ImageResult, error)
}

// ImageEditor edits images using a text prompt and reference image.
// Providers that support editing implement this in addition to ImageProvider.
type ImageEditor interface {
    EditImage(ctx context.Context, prompt string, config *Config) ([]*ImageResult, error)
}

// VideoProvider generates videos from text prompts.
type VideoProvider interface {
    GenerateVideo(ctx context.Context, prompt string, config *Config) (*VideoResult, error)
}
```

Note: `Config` carries `ReferenceImages` for editing. `ImageEditor` reads them from there.

**`media/registry.go`**
```go
type ImageProviderFactory func(model string) ImageProvider
type VideoProviderFactory func(model string) VideoProvider
type ModelMatcher func(model string) bool

type ImageProviderEntry struct {
    Name    string
    Match   ModelMatcher
    Factory ImageProviderFactory
}

type VideoProviderEntry struct {
    Name    string
    Match   ModelMatcher
    Factory VideoProviderFactory
}

type Registry struct {
    imageProviders []ImageProviderEntry
    videoProviders []VideoProviderEntry
    mu             sync.RWMutex
}

func (r *Registry) RegisterImage(entry ImageProviderEntry)
func (r *Registry) RegisterVideo(entry VideoProviderEntry)
func (r *Registry) ResolveImage(model string) (ImageProvider, error)
func (r *Registry) ResolveVideo(model string) (VideoProvider, error)

// DefaultRegistry is the global registry. Provider init() functions register here.
var DefaultRegistry = &Registry{}

// Package-level registration helpers
func RegisterImage(entry ImageProviderEntry)
func RegisterVideo(entry VideoProviderEntry)
```

Reuse matcher helpers from `providers` package: `PrefixMatcher`, `PrefixesMatcher`.

**`media/media.go`** — Top-level API functions
```go
// GenerateImage generates a single image (or Count images) using one provider.
func GenerateImage(ctx context.Context, prompt string, opts ...Option) (*ImageResult, error)

// GenerateImages generates images across multiple models concurrently (fan-out).
// Requires WithModels(). Returns one result per model. Individual failures
// are captured in ImageResult.Err rather than failing the entire call.
func GenerateImages(ctx context.Context, prompt string, opts ...Option) ([]*ImageResult, error)

// EditImage edits a reference image using a text prompt.
// Requires WithReferenceImage(). Returns ErrEditNotSupported if the
// resolved provider does not implement ImageEditor.
func EditImage(ctx context.Context, prompt string, opts ...Option) (*ImageResult, error)

// GenerateVideo generates a video from a text prompt. Blocks until complete
// or context is cancelled.
func GenerateVideo(ctx context.Context, prompt string, opts ...Option) (*VideoResult, error)
```

Implementation:
- Resolve provider from registry using `config.Model`
- Apply context timeout from `config.Timeout`
- For `GenerateImages`: launch goroutines per model, collect results, return all
- For `EditImage`: type-assert provider to `ImageEditor`, return `ErrEditNotSupported` if not
- For `GenerateVideo`: resolve from video registry, call with context

**`media/errors.go`**
```go
var (
    ErrNoModel          = errors.New("media: no model specified")
    ErrProviderNotFound = errors.New("media: no provider found for model")
    ErrEditNotSupported = errors.New("media: provider does not support image editing")
    ErrTimeout          = errors.New("media: generation timed out")
    ErrNoResult         = errors.New("media: provider returned no results")
)
```

### Tests

**`media/registry_test.go`** — Tests for:
- Register and resolve image provider by model prefix
- Register and resolve video provider
- Unknown model returns `ErrProviderNotFound`
- Multiple providers, first match wins

**`media/media_test.go`** — Tests using a mock provider:
- `GenerateImage` with mock returns expected result
- `GenerateImages` fan-out with 2 mock providers returns 2 results
- `GenerateImages` with one failing provider returns partial results (Err on failed one)
- `EditImage` returns `ErrEditNotSupported` for non-editor provider
- `GenerateImage` with no model returns `ErrNoModel`
- Context cancellation propagates

Create a `media/testing.go` (or `media/mock_test.go`) with:
```go
type mockImageProvider struct {
    result []*ImageResult
    err    error
}

func (m *mockImageProvider) GenerateImage(ctx context.Context, prompt string, config *Config) ([]*ImageResult, error) {
    return m.result, m.err
}
```

---

## Phase 3: Google Provider

**Goal:** Implement image generation (Imagen + Gemini) and video generation (Veo) for Google.

### Files to Create

**`providers/google/media.go`**
```go
type MediaProvider struct {
    apiKey         string
    vertexAI       bool
    location       string
    imagenLocation string
    client         *genai.Client
    imagenClient   *genai.Client
    mu             sync.Mutex
}

// Implement media.ImageProvider
func (p *MediaProvider) GenerateImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error)

// Implement media.VideoProvider
func (p *MediaProvider) GenerateVideo(ctx context.Context, prompt string, config *media.Config) (*media.VideoResult, error)
```

Implementation details:
- **Client initialization:** Lazy via `sync.Mutex`, same pattern as existing Google LLM provider. Reads `GEMINI_API_KEY` / `GOOGLE_API_KEY`. Supports Vertex AI backend via option.
- **Model branching:** Detect `gemini-*` prefix → `generateImageWithGemini()`. Otherwise → `generateImageWithImagen()`.
- **Gemini image generation:** Call `client.Models.GenerateContent` with `ResponseModalities: []string{"IMAGE"}` and `ImageConfig`. Extract `InlineData` from response parts. Apply format conversion if requested format differs from provider output.
- **Imagen generation:** Call `client.Models.GenerateImages` with `GenerateImagesConfig`. Filter out safety-filtered results (`RAIFilteredReason != ""`). Apply safety filter level based on model prefix (`imagen-4` → `BlockLowAndAbove`, else `BlockOnlyHigh`).
- **Veo video generation:** Submit via `client.Models.GenerateVideos`. Poll with `client.Operations.GetVideosOperation` every 10 seconds, up to context deadline. Download video bytes via `client.Files.Download`. Return `VideoResult` with dimensions from `StandardVideoDimensions`.
- **Aspect ratio:** Pass through as string to Google SDK (it uses "1:1", "16:9" format natively).
- **Format handling:** Imagen returns the requested MIME type. Gemini returns PNG by default. Convert if `config.OutputFormat` differs.

**`providers/google/media_register.go`**
```go
func init() {
    media.RegisterImage(media.ImageProviderEntry{
        Name:  "google",
        Match: googleImageMatcher,
        Factory: func(model string) media.ImageProvider {
            return newMediaProvider()
        },
    })
    media.RegisterVideo(media.VideoProviderEntry{
        Name:  "google",
        Match: googleVideoMatcher,
        Factory: func(model string) media.VideoProvider {
            return newMediaProvider()
        },
    })
}
```

Model matchers: prefix-based for `gemini-*-image*`, `imagen-*`, `veo-*`.

**`providers/google/media_options.go`** — Provider-specific options for `MediaProvider` (Vertex AI, location, Imagen location). These are constructor options, not `media.Option`.

### Known Models to Register

```
// Image models
gemini-3.1-flash-lite-image
gemini-3.1-flash-image
gemini-3-pro-image
gemini-2.5-flash-image
gemini-3.1-flash-image-preview
gemini-3-pro-image-preview
imagen-3.0-generate-001
imagen-3.0-fast-generate-001
imagen-4.0-generate-001
imagen-4.0-ultra-generate-001

// Video models
veo-3.1-generate-preview
```

### Tests

**`providers/google/media_test.go`**

Unit tests (no API key required):
- `TestGoogleMediaProvider_ModelBranching` — verify Gemini vs Imagen detection logic
- `TestGoogleImageMatcher` — verify model matcher hits and misses
- `TestGoogleVideoMatcher` — verify model matcher

Integration tests (require `GOOGLE_API_KEY` or `GEMINI_API_KEY`):
- `TestGoogleGenerateImage_Imagen` — generate with `imagen-4.0-generate-001`, verify result has data, dimensions, PNG format
- `TestGoogleGenerateImage_Gemini` — generate with `gemini-2.5-flash-image`, verify result
- `TestGoogleGenerateImage_AspectRatio` — generate 16:9, verify dimensions
- `TestGoogleGenerateImage_FormatConversion` — request JPEG from Gemini (returns PNG), verify conversion
- `TestGoogleGenerateVideo_Veo` — generate short video, verify result has data and MP4 format (long-running, use `testing.Short()` skip)

All integration tests follow the skip pattern:
```go
func requireGoogleAPIKey(t *testing.T) {
    if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
        t.Skip("GOOGLE_API_KEY or GEMINI_API_KEY not set")
    }
}
```

---

## Phase 4: OpenAI Provider

**Goal:** Implement image generation (gpt-image-1), image editing, and video generation (Sora) for OpenAI.

### Files to Create

**`providers/openai/media.go`**
```go
type MediaProvider struct {
    client openai.Client
}

// Implement media.ImageProvider
func (p *MediaProvider) GenerateImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error)

// Implement media.ImageEditor
func (p *MediaProvider) EditImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error)

// Implement media.VideoProvider
func (p *MediaProvider) GenerateVideo(ctx context.Context, prompt string, config *media.Config) (*media.VideoResult, error)
```

Implementation details:
- **Client init:** `openai.NewClient()` reads `OPENAI_API_KEY` from environment.
- **Image generation:** Call `client.Images.Generate` with `ImageGenerateParams{Model: ImageModelGPTImage1, Prompt, Size, Quality: High, N, OutputFormat}`. Decode base64 results. Detect format from data bytes.
- **Image editing:** Call `client.Images.Generate` with reference image bytes. OpenAI's edit endpoint accepts the reference in the request.
- **Aspect ratio to size mapping:** Convert `media.AspectRatio` to OpenAI size strings: `Aspect1x1` → `"1024x1024"`, `Aspect16x9` → `"1792x1024"`, `Aspect9x16` → `"1024x1792"`.
- **Video generation:** Call `client.Videos.NewAndPoll` with `VideoNewParams{Model: VideoModelSora2, Prompt, Size, Seconds}`. Map `config.Duration` to discrete seconds ("4", "8", "12"). Check status == `VideoStatusCompleted`. Download via `client.Videos.DownloadContent`. Read full body.
- **Duration mapping:** `<= 5s` → 4, `<= 10s` → 8, else → 12.

**`providers/openai/media_register.go`**
```go
func init() {
    media.RegisterImage(media.ImageProviderEntry{
        Name:  "openai",
        Match: openaiImageMatcher,
        Factory: func(model string) media.ImageProvider {
            return newMediaProvider()
        },
    })
    media.RegisterVideo(media.VideoProviderEntry{
        Name:  "openai",
        Match: openaiVideoMatcher,
        Factory: func(model string) media.VideoProvider {
            return newMediaProvider()
        },
    })
}
```

Model matchers: `gpt-image-*` for images, `sora*` for video.

### Tests

**`providers/openai/media_test.go`**

Unit tests:
- `TestOpenAIAspectRatioToSize` — verify mapping for all aspect ratios
- `TestOpenAIDurationMapping` — verify 3s→4, 5s→4, 8s→8, 12s→12
- `TestOpenAIImageMatcher` / `TestOpenAIVideoMatcher`

Integration tests (require `OPENAI_API_KEY`):
- `TestOpenAIGenerateImage` — generate with `gpt-image-1`, verify result
- `TestOpenAIEditImage` — edit with reference image, verify result
- `TestOpenAIGenerateVideo` — generate short video (long-running, `testing.Short()` skip)

---

## Phase 5: CLI Commands

**Goal:** Add `dive image` and `dive video` subcommands to the experimental CLI.

### Files to Modify/Create

**`experimental/cmd/dive/cmd_image.go`** (new)

```
dive image "prompt" [flags]

Flags:
  --model   string   Model to use (default: imagen-4.0-generate-001)
  --aspect  string   Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4
  --format  string   Output format: png, jpeg, webp (default: png)
  --out     string   Output file path (default: auto-generated from prompt)
  --count   int      Number of images to generate (default: 1)
  --open    bool     Open result in default viewer
```

Implementation:
- Parse flags, build `media.Option` slice
- Call `media.GenerateImage(ctx, prompt, opts...)`
- Auto-generate filename: slugify first 50 chars of prompt + timestamp + extension
- Write to disk via `result.WriteTo(path)`
- If `--open`, use `open` (macOS) / `xdg-open` (Linux) to launch viewer
- Print path and metadata to stdout

**`experimental/cmd/dive/cmd_video.go`** (new)

```
dive video "prompt" [flags]

Flags:
  --model     string     Model to use (default: veo-3.1-generate-preview)
  --aspect    string     Aspect ratio: 16:9, 9:16, 1:1
  --duration  duration   Video duration: 4s, 8s, 12s (default: 8s)
  --out       string     Output file path
  --open      bool       Open result in default viewer
```

Implementation:
- Same pattern as `cmd_image.go`
- Add a simple progress indicator (spinner or dots) while waiting for video generation
- Print elapsed time on completion

**`experimental/cmd/dive/main.go`** — Register the new subcommands.

### Provider Import Side Effects

The CLI's `main.go` must import the provider registration packages for side effects:
```go
import (
    _ "github.com/deepnoodle-ai/dive/providers/google"
    _ "github.com/deepnoodle-ai/dive/providers/openai"
)
```

This triggers `init()` and registers both LLM and media providers.

### Tests

- `TestImageCommandFlagParsing` — verify flags are parsed correctly
- `TestVideoCommandFlagParsing` — verify flags
- `TestSlugifyPrompt` — verify filename generation from prompt text
- Manual smoke tests with real API keys (documented in test file as comments)

---

## Phase 6: Agent Toolkit Integration

**Goal:** Add image and video generation as Dive tools so agents can generate media.

### Files to Create

**`toolkit/image_generation.go`**
```go
type imageGenerationTool struct {
    model    string
    provider media.ImageProvider
    workDir  string
}

type ImageGenerationInput struct {
    Prompt      string `json:"prompt" jsonschema:"description=Text description of the image to generate"`
    AspectRatio string `json:"aspect_ratio,omitempty" jsonschema:"description=Aspect ratio (1:1, 16:9, 9:16, 4:3, 3:4),enum=1:1,enum=16:9,enum=9:16,enum=4:3,enum=3:4"`
    OutputPath  string `json:"output_path,omitempty" jsonschema:"description=File path to save the image. Auto-generated if omitted."`
    Format      string `json:"format,omitempty" jsonschema:"description=Output format (png, jpeg, webp),enum=png,enum=jpeg,enum=webp"`
}

func ImageGenerationTool(model string, opts ...ImageGenerationToolOption) *dive.TypedToolAdapter[*ImageGenerationInput]
```

Tool behavior:
- `Name()`: `"ImageGeneration"`
- `Description()`: `"Generate an image from a text prompt. Saves the image to disk and returns the file path."`
- `Annotations()`: `ReadOnlyHint: false, DestructiveHint: false, OpenWorldHint: true`
- `Call()`: Resolve provider from registry, call `GenerateImage`, write to disk, return `NewToolResultText(path)` with metadata about dimensions and format
- Auto-generate output path in `workDir` if not specified (slugified prompt + extension)

**`toolkit/video_generation.go`**
```go
type videoGenerationTool struct {
    model    string
    provider media.VideoProvider
    workDir  string
}

type VideoGenerationInput struct {
    Prompt      string `json:"prompt" jsonschema:"description=Text description of the video to generate"`
    Duration    string `json:"duration,omitempty" jsonschema:"description=Video duration (4s, 8s, 12s),enum=4s,enum=8s,enum=12s"`
    AspectRatio string `json:"aspect_ratio,omitempty" jsonschema:"description=Aspect ratio (16:9, 9:16, 1:1),enum=16:9,enum=9:16,enum=1:1"`
    OutputPath  string `json:"output_path,omitempty" jsonschema:"description=File path to save the video"`
}

func VideoGenerationTool(model string, opts ...VideoGenerationToolOption) *dive.TypedToolAdapter[*VideoGenerationInput]
```

Same pattern. Parse duration string to `time.Duration`.

### Tests

**`toolkit/image_generation_test.go`**
- `TestImageGenerationTool_Name` — verify name
- `TestImageGenerationTool_Description` — verify non-empty
- `TestImageGenerationTool_Annotations` — verify hints
- `TestImageGenerationTool_Schema` — verify JSON schema contains prompt field
- Integration test: `TestImageGenerationTool_Call` — requires API key, generates real image, verifies file exists on disk

**`toolkit/video_generation_test.go`**
- Same pattern. Video integration test uses `testing.Short()` skip.

---

## File Summary

| Phase | New Files | Modified Files |
|-------|-----------|----------------|
| 1 | `media/aspect_ratio.go`, `media/format.go`, `media/result.go`, `media/options.go` + tests | — |
| 2 | `media/provider.go`, `media/registry.go`, `media/media.go`, `media/errors.go` + tests | — |
| 3 | `providers/google/media.go`, `providers/google/media_register.go`, `providers/google/media_options.go` + tests | — |
| 4 | `providers/openai/media.go`, `providers/openai/media_register.go` + tests | — |
| 5 | `experimental/cmd/dive/cmd_image.go`, `experimental/cmd/dive/cmd_video.go` | `experimental/cmd/dive/main.go` |
| 6 | `toolkit/image_generation.go`, `toolkit/video_generation.go` + tests | — |

## Testing Strategy

### Unit Tests (no API keys, run in CI)
- All type methods, format detection, aspect ratio mapping, option application
- Registry resolution with mock providers
- Fan-out concurrency with mock providers (verify partial failure handling)
- CLI flag parsing and filename generation
- Tool schema, name, annotations

### Integration Tests (require API keys, skipped in CI by default)
- Each provider × each operation (image gen, image edit, video gen)
- End-to-end: `media.GenerateImage` with real providers
- Video tests gated behind `testing.Short()` due to long polling times
- CLI smoke tests documented as manual steps

### Test Conventions
- `wonton/assert` for all assertions
- `t.TempDir()` for file output tests
- `requireGoogleAPIKey(t)` / `requireOpenAIAPIKey(t)` skip helpers
- Table-driven tests for mapping/conversion functions

## Open Questions (from PRD) — Proposed Answers

1. **Default model:** Require `WithModel()`. Return `ErrNoModel` if omitted. No opinionated default -- media models change fast and cost real money. The CLI can have a default.
2. **Result type vs Content type:** New `ImageResult` type. `llm.ImageContent` is for LLM conversation context (vision input). `ImageResult` is for generated output. Different concerns, different shapes.
3. **Provider package co-location:** Co-locate. `providers/google/media.go` lives next to `providers/google/google.go`. Both register in their respective `init()`. This keeps related provider code together.
4. **Video result streaming:** Start with `[]byte`. Add `io.Reader` variant later if memory becomes an issue. Most videos are < 50MB.
5. **Progress callbacks:** Defer. Context cancellation is sufficient for v1. Progress callbacks can be added as an option later without breaking changes.
