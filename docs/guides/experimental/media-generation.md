# Media Generation

> **Experimental**: The media generation system is functional but its API may
> change in future releases.

Dive provides cross-provider image, video, and speech media through the `media`
package. Image and video are also available as toolkit tools. Supported
providers include OpenAI (gpt-image, Sora, GPT-4o mini TTS/transcribe), Google
(Imagen, Gemini, Veo), and Grok (grok-imagine).

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
    media.WithModel("gpt-image-2"),
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
    media.WithModels("gpt-image-2", "imagen-4.0-generate-001", "grok-imagine-image"),
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

Providers that support editing (OpenAI, Google Gemini) can modify existing images:

```go
refImage, _ := os.ReadFile("photo.png")

result, err := media.EditImage(ctx, "make the sky purple",
    media.WithModel("gpt-image-2"),
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

## Text-to-Speech

Generate spoken audio from text with OpenAI TTS or Gemini TTS models:

```go
result, err := media.TextToSpeech(ctx, "Welcome to Dive.",
    media.WithModel("gpt-4o-mini-tts"),
    media.WithVoice("alloy"),
    media.WithAudioFormat(media.AudioFormatMP3),
    media.WithVoiceInstructions("Speak warmly and clearly."),
)
if err != nil {
    log.Fatal(err)
}

path, err := result.WriteTo("welcome")
fmt.Printf("Saved: %s (%s, %s)\n", path, result.Format, result.MimeType)
```

Gemini TTS returns PCM audio from the API; Dive wraps it as WAV by default so
`WriteTo("welcome")` produces a playable `.wav` file:

```go
result, err := media.TextToSpeech(ctx, "Say cheerfully: Have a wonderful day!",
    media.WithModel("gemini-3.1-flash-tts-preview"),
    media.WithVoice("Kore"),
    media.WithAudioFormat(media.AudioFormatWAV),
)
```

## Transcription

Transcribe audio bytes with OpenAI speech-to-text models or Gemini audio
understanding models:

```go
audio, _ := os.ReadFile("meeting.wav")

result, err := media.Transcribe(ctx, audio,
    media.WithModel("gpt-4o-mini-transcribe"),
    media.WithAudioMIMEType("audio/wav"),
    media.WithLanguage("en"),
    media.WithTranscriptionPrompt("The audio is about the Dive Go library."),
)
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Text)
```

For Gemini, use a general audio-capable Gemini model and an optional prompt:

```go
result, err := media.Transcribe(ctx, audio,
    media.WithModel("gemini-3.5-flash"),
    media.WithAudioMIMEType("audio/wav"),
    media.WithTranscriptionPrompt("Generate a concise transcript of the speech."),
)
```

## Supported Models

### Image Models

| Provider | Models |
|----------|--------|
| OpenAI | `gpt-image-2`, `gpt-image-1.5`, `gpt-image-1`, `gpt-image-1-mini` |
| Google | `imagen-4.0-generate-001`, `imagen-4.0-ultra-generate-001`, `imagen-4.0-fast-generate-001`, `gemini-3.1-flash-lite-image`, `gemini-3.1-flash-image`, `gemini-3-pro-image`, `gemini-2.5-flash-image` |
| Grok | `grok-imagine-image`, `grok-imagine-image-pro` |

### Video Models

| Provider | Models |
|----------|--------|
| Google | `veo-3.1-generate-preview`, `veo-3-generate-preview` |
| OpenAI | `sora-2`, `sora-2-pro` |
| Grok | `grok-imagine-video` |

### Text-to-Speech Models

| Provider | Models |
|----------|--------|
| OpenAI | `gpt-4o-mini-tts`, `tts-1`, `tts-1-hd` |
| Google | `gemini-3.1-flash-tts-preview`, `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts` |

### Transcription Models

| Provider | Models |
|----------|--------|
| OpenAI | `gpt-4o-mini-transcribe`, `gpt-4o-transcribe`, `gpt-4o-transcribe-diarize`, `whisper-1` |
| Google | Audio-capable Gemini models such as `gemini-3.5-flash` |

## Options Reference

| Option | Description | Default |
|--------|-------------|---------|
| `WithModel(m)` | Model name for generation | (required) |
| `WithModels(m...)` | Multiple models for fan-out | — |
| `WithAspectRatio(ar)` | `1:1`, `16:9`, `9:16` | `1:1` (image), `16:9` (video) |
| `WithOutputFormat(f)` | `FormatPNG`, `FormatJPEG`, `FormatWebP` | Provider default |
| `WithCount(n)` | Number of images to generate | 1 |
| `WithReferenceImage(data)` | Reference image bytes for editing | — |
| `WithDuration(d)` | Video duration | Provider default |
| `WithAudioFormat(f)` | `AudioFormatMP3`, `AudioFormatWAV`, `AudioFormatPCM`, etc. | Provider default |
| `WithAudioMIMEType(mime)` | Input audio MIME hint for transcription | Auto-detected |
| `WithVoice(v)` | Text-to-speech voice | Provider default |
| `WithVoiceInstructions(s)` | Text-to-speech style instructions | — |
| `WithSpeechSpeed(n)` | Speech speed when supported | Provider default |
| `WithLanguage(code)` | Transcription or speech language hint | Provider default |
| `WithTranscriptionPrompt(p)` | Transcription context prompt | Provider default |
| `WithTimeout(d)` | Max generation wait time | 5min (image), 15min (video) |

## File Output

`ImageResult.WriteTo`, `VideoResult.WriteTo`, and `AudioResult.WriteTo` handle file writing with
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
        toolkit.NewImageGenerationTool("gpt-image-2",
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
      --aspect   Aspect ratio: 1:1, 16:9, 9:16
      --format   Output format: png, jpeg, webp
  -n, --count    Number of images (default: 1)
  -r, --ref      Reference image file path (repeatable)
  -e, --edit     Edit reference images instead of generating
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

# Edit an existing image
dive image "make the sky purple" --edit --ref photo.png

# Generate with reference images (style/composition guidance)
dive image "a painting in this style" --ref reference.jpg
```

### Video Generation

```text
dive video "prompt" [flags]

Flags:
  -m, --model      Model name (default: auto-detect)
      --aspect     Aspect ratio: 16:9, 9:16, 1:1
  -d, --duration   Video duration, e.g. 8s, 16s, 20s (default: 8s)
  -o, --out        Output file path (default: auto from prompt)
      --open       Open result in default viewer
```

Examples:

```bash
# Basic generation
dive video "ocean waves at sunset"

# Specific model and duration
dive video "timelapse of clouds" -m sora-2 -d 20s

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
