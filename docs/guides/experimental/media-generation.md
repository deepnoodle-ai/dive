# Media Generation

> **Experimental**: The media generation system is functional but its API may
> change in future releases.

Dive provides cross-provider image and video generation through the `media`
package and corresponding toolkit tools. Supported providers include OpenAI
(gpt-image, Sora), Google (Imagen, Gemini, Veo), and Grok (grok-imagine).

## Setup

Import the `media` package and at least one provider. Providers self-register
via `init()` when imported:

```go
import (
    "github.com/deepnoodle-ai/dive/media"

    // Register providers — import any combination
    _ "github.com/deepnoodle-ai/dive/providers/openai"
    _ "github.com/deepnoodle-ai/dive/providers/google"
    _ "github.com/deepnoodle-ai/dive/providers/grok"
)
```

Each provider reads its API key from the environment:

| Provider | Environment Variable |
|----------|---------------------|
| OpenAI | `OPENAI_API_KEY` |
| Google | `GOOGLE_API_KEY` or `GEMINI_API_KEY` |
| Grok | `XAI_API_KEY` or `GROK_API_KEY` |

## Image Generation

### Single Image

```go
result, err := media.GenerateImage(ctx, "a cat astronaut on the moon",
    media.WithModel("gpt-image-1"),
    media.WithAspectRatio(media.Aspect16x9),
    media.WithOutputFormat(media.FormatPNG),
)
if err != nil {
    log.Fatal(err)
}

// WriteTo returns the actual path written (may differ if file already exists)
path, err := result.WriteTo("cat-astronaut.png")
fmt.Printf("Saved: %s (%dx%d)\n", path, result.Width, result.Height)
```

### Batch Generation

Generate multiple images from a single prompt:

```go
results, err := media.GenerateImageBatch(ctx, "sunset over mountains",
    media.WithModel("imagen-4.0-generate-001"),
    media.WithCount(4),
)
for i, img := range results {
    img.WriteTo(fmt.Sprintf("sunset-%d.png", i+1))
}
```

### Fan-Out Across Models

Generate the same prompt with multiple models concurrently:

```go
results, err := media.GenerateImages(ctx, "a red panda",
    media.WithModels("gpt-image-1", "imagen-4.0-generate-001", "grok-imagine-image"),
)
for _, img := range results {
    if img.Err != nil {
        fmt.Printf("%s failed: %v\n", img.Model, img.Err)
        continue
    }
    img.WriteTo(fmt.Sprintf("panda-%s.png", img.Model))
}
```

Individual provider failures are captured in `ImageResult.Err` rather than
failing the entire call.

### Image Editing

Providers that support editing (OpenAI, Google Gemini) can modify existing
images:

```go
refImage, _ := os.ReadFile("photo.png")

result, err := media.EditImage(ctx, "make the sky purple",
    media.WithModel("gpt-image-1"),
    media.WithReferenceImage(refImage),
)
```

Returns `media.ErrEditNotSupported` if the provider does not implement editing.

## Video Generation

```go
result, err := media.GenerateVideo(ctx, "a leaf falling from a tree",
    media.WithModel("veo-3.1-generate-preview"),
    media.WithAspectRatio(media.Aspect16x9),
    media.WithDuration(8 * time.Second),
    media.WithTimeout(15 * time.Minute),
)
if err != nil {
    log.Fatal(err)
}

path, err := result.WriteTo("leaf.mp4")
fmt.Printf("Saved: %s (%dx%d %s, %s)\n",
    path, result.Width, result.Height, result.Format, result.Duration)
```

Video generation is synchronous from the caller's perspective — the call blocks
until the provider completes or the context is cancelled.

## Supported Models

### Image Models

| Provider | Models |
|----------|--------|
| OpenAI | `gpt-image-1`, `gpt-image-1.5`, `gpt-image-1-mini` |
| Google | `imagen-4.0-generate-001`, `imagen-4.0-ultra-generate-001`, `imagen-4.0-fast-generate-001`, `gemini-2.5-flash-image`, `gemini-3.1-flash-image-preview` |
| Grok | `grok-imagine-image`, `grok-imagine-image-pro` |

### Video Models

| Provider | Models |
|----------|--------|
| Google | `veo-3.1-generate-preview`, `veo-3-generate-preview` |
| OpenAI | `sora-2`, `sora-2-pro` |
| Grok | `grok-imagine-video` |

## Options Reference

| Option | Description | Default |
|--------|-------------|---------|
| `WithModel(m)` | Model name for generation | (required) |
| `WithModels(m...)` | Multiple models for fan-out | — |
| `WithAspectRatio(ar)` | `1:1`, `16:9`, `9:16`, `4:3`, `3:4` | `1:1` (image), `16:9` (video) |
| `WithOutputFormat(f)` | `FormatPNG`, `FormatJPEG`, `FormatWebP` | Provider default |
| `WithCount(n)` | Number of images to generate | 1 |
| `WithReferenceImage(data)` | Reference image bytes for editing | — |
| `WithDuration(d)` | Video duration | Provider default |
| `WithTimeout(d)` | Max generation wait time | 5min (image), 15min (video) |

## File Output

Both `ImageResult.WriteTo` and `VideoResult.WriteTo` handle file writing with
two safety features:

- **Auto-extension**: If the path has no extension, the appropriate one is
  appended (`.png`, `.jpg`, `.mp4`, etc.)
- **No-overwrite**: If the destination exists, a numeric suffix is appended
  automatically (`photo1.png`, `photo2.png`, ...)

Both methods return `(string, error)` where the string is the actual path
written.

## Agent Tools

The `toolkit` package provides `ImageGeneration` and `VideoGeneration` tools
that agents can use to generate media during conversations.

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Creative Assistant",
    SystemPrompt: "You help users create images and videos.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewImageGenerationTool("gpt-image-1",
            toolkit.WithImageToolWorkDir("/tmp/output"),
        ),
        toolkit.NewVideoGenerationTool("veo-3.1-generate-preview",
            toolkit.WithVideoToolWorkDir("/tmp/output"),
        ),
    },
})
```

The tools automatically:
- Generate filenames from the prompt if no output path is specified
- Return the absolute file path to the agent
- Avoid overwriting existing files

## CLI Commands

The experimental CLI includes `image` and `video` subcommands. The model is
auto-detected from available API keys if not specified.

### Image Generation

```text
dive image "prompt" [flags]

Flags:
  -m, --model    Model name (default: auto-detect)
      --aspect   Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4
      --format   Output format: png, jpeg, webp
  -n, --count    Number of images (default: 1)
  -o, --out      Output file path (default: auto from prompt)
      --open     Open result in default viewer
```

Examples:

```bash
# Basic generation
dive image "a cat in a spacesuit"

# Specific model and format
dive image "mountain landscape" -m imagen-4.0-generate-001 --aspect 16:9 --format jpeg

# Multiple images
dive image "abstract art" -n 4

# Specify output path
dive image "logo design" -o logo.png
```

### Video Generation

```text
dive video "prompt" [flags]

Flags:
  -m, --model      Model name (default: auto-detect)
      --aspect     Aspect ratio: 16:9, 9:16, 1:1
  -d, --duration   Video duration: 4s, 8s, 12s (default: 8s)
  -o, --out        Output file path (default: auto from prompt)
      --open       Open result in default viewer
```

Examples:

```bash
# Basic generation
dive video "ocean waves at sunset"

# Specific model and duration
dive video "timelapse of clouds" -m sora-2 -d 16s

# Portrait video
dive video "person walking" --aspect 9:16
```

## Adding a Provider

Media providers are registered via the global registry in `init()`:

```go
package myprovider

import "github.com/deepnoodle-ai/dive/media"

func init() {
    media.RegisterImage(media.ImageProviderEntry{
        Name:  "myprovider",
        Match: media.PrefixMatcher("mymodel-"),
        Factory: func(model string) media.ImageProvider {
            return NewMediaProvider()
        },
    })
}
```

Implement `media.ImageProvider`, `media.ImageEditor`, or `media.VideoProvider`
as needed. The registry uses prefix matching to route model names to providers.
