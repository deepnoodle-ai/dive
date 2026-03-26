# PRD: Multi-Provider Media Generation

| Field | Content |
|-------|---------|
| Title | Multi-Provider Image & Video Generation |
| Author | Curtis / DeepNoodle |
| Status | Draft |
| Last Updated | 2026-03-25 |
| Stakeholders | Dive library users, CLI users, agent builders |

## Problem & Opportunity

### The Problem

Developers building applications that generate images or video face a fragmented landscape. Every provider -- OpenAI (gpt-image-1, Sora), Google (Imagen, Veo), Replicate, fal.ai, Stability AI, Black Forest Labs (FLUX) -- has its own SDK, authentication, parameter naming, aspect ratio conventions, polling patterns, and error shapes. Switching providers means rewriting integration code. Comparing outputs means maintaining parallel implementations.

This is the same problem OpenRouter and LiteLLM solved for LLMs -- but **nobody has solved it well for visual media**. The research is unambiguous:

> "None of these are OpenRouter equivalents for visual/audio media models because they are just single providers... this is one of the few AI wrapper/router ideas that could hold up longer term, many people would pay for this." -- r/LocalLLaMA

> "Juggling multiple AI image APIs is tedious. They each have their own SDKs, parameter names, and quirks." -- Hacker News

> "Each provider has different endpoints, formats, polling, etc (just Google has 2 different providers, crazy)." -- r/webdev

Dive already solves this problem for LLM text generation. The same provider-abstraction pattern should extend to image and video generation.

### Why Now

1. **Image generation just went mainstream in APIs.** OpenAI shipped gpt-image-1 (March 2025), Google shipped Imagen 4 and Veo 3 (2025-2026). These are production-ready APIs, not research previews.
2. **We have proven patterns internally.** We've already built multi-provider image and video generation against Google and OpenAI in other projects. The integration patterns are battle-tested.
3. **Dive's architecture is ready.** The provider registry, tool system, and content types already support image data. The gap is a clean generation interface.
4. **No credible Go solution exists.** The only Go attempt (Karma) got zero traction. Vercel AI SDK dominates JS but has no Go equivalent. This is Dive's lane.

### What Happens If We Do Nothing

Dive remains an LLM-only library while the market moves toward multimodal workflows. Developers who need image/video generation will reach for JS-based solutions (Vercel AI SDK) or build bespoke integrations, fragmenting their stack.

## Goals & Success Metrics

| Goal | Metric |
|------|--------|
| **Primary:** Developers can generate images across providers with one API | Generate identical-prompt images from 3+ providers with zero code changes beyond model name |
| **Secondary:** Video generation works with the same pattern | At least 2 video providers supported with unified interface |
| **Secondary:** The CLI is the fastest way to generate an image from the terminal | `dive image "a cat in space"` produces output in one command |
| **Guardrail:** No regression to LLM capabilities | All existing LLM tests pass, no breaking API changes |

## Target Users

### Primary: Go developers building AI-powered applications

Developers integrating image/video generation into products -- content platforms, design tools, game asset pipelines, marketing automation. They want to swap providers without rewriting code, compare outputs, and optimize for cost/quality.

### Secondary: CLI power users and AI agent builders

People who want to generate images from scripts, pipelines, or agent tool loops. They want `dive image "prompt" --model imagen-4` to just work, and they want agents to be able to generate images as a tool action.

## Jobs To Be Done

These are the real things people are trying to accomplish, grounded in the research.

### JTBD-1: "Generate an image from a prompt without reading provider docs"

The most basic job. A developer has a text prompt and wants pixels back. Today this requires choosing a provider, reading their SDK docs, handling their auth, learning their parameter names, and parsing their response format.

**What would blow them away:** Four lines of Go, same as Dive's LLM usage pattern. The provider is just a model name. Everything else is handled.

```go
result, _ := media.GenerateImage(ctx, "a cat in space",
    media.WithModel(google.Imagen4()),
)
result.WriteTo("cat.png")
```

### JTBD-2: "Compare the same prompt across providers to find the best output"

Developers routinely test prompts against multiple models to find the best quality/cost tradeoff. Today this means maintaining parallel integrations.

**What would blow them away:** Fan-out generation with one call. Get results from all providers, compare, pick the best.

```go
results, _ := media.GenerateImages(ctx, "a cat in space",
    media.WithModels(google.Imagen4(), openai.GPTImage1(), bfl.FluxPro()),
)
for _, r := range results {
    fmt.Printf("%s: %dx%d, %s\n", r.Model, r.Width, r.Height, r.Duration)
}
```

### JTBD-3: "Switch providers when costs get too high"

A game developer is spending $0.002/image on Stability AI and needs to cut costs. A content creator burned $2,400 in three weeks on video generation. They need to switch providers, but every switch means rewriting code.

**What would blow them away:** Change one model name, everything else stays the same. No code changes, no parameter translation, no new SDK to learn.

### JTBD-4: "Generate an image from the terminal in one command"

A designer wants to quickly test a prompt. A scripter wants to batch-generate assets. A developer wants to eyeball output quality before writing integration code.

**What would blow them away:** A CLI that generates and opens the image in one shot, with inline terminal preview for supported terminals.

```bash
dive image "a medieval town at sunset" --model imagen-4 --aspect 16:9
dive image "a sword icon" --model gpt-image-1 --format webp --out sword.webp
dive video "a cat walking through snow" --model veo-3 --duration 8s
```

### JTBD-5: "Use image generation as a tool inside an AI agent"

An agent is building a website, writing a blog post, or creating a game. It needs to generate images as part of its workflow -- not as the primary task, but as one step among many.

**What would blow them away:** Image generation is a Dive tool, just like Bash or ReadFile. The agent generates images naturally as part of its reasoning loop.

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Tools: []dive.Tool{
        toolkit.ImageGenerationTool(google.Imagen4()),
    },
})
```

### JTBD-6: "Edit or refine an existing image with a prompt"

A developer has a reference image and wants to modify it -- change the background, adjust colors, add elements. Today this is only available through specific provider APIs with incompatible interfaces.

**What would blow them away:** Same unified interface, just add a reference image.

```go
result, _ := media.EditImage(ctx, "make the background a sunset",
    media.WithModel(openai.GPTImage1()),
    media.WithReferenceImage(existingImage),
)
```

### JTBD-7: "Generate video from a prompt or an image"

Video generation is newer and more expensive, but the same fragmentation problems exist. Veo and Sora have completely different APIs, polling patterns, and parameter conventions.

**What would blow them away:** Same pattern as image generation. Async by nature (video takes longer), but the API handles polling transparently.

```go
result, _ := media.GenerateVideo(ctx, "a timelapse of a flower blooming",
    media.WithModel(google.Veo3()),
    media.WithDuration(8 * time.Second),
    media.WithAspectRatio(media.Aspect16x9),
)
result.WriteTo("flower.mp4")
```

## User Stories

### US-001: Generate an Image

**Description:** As a developer, I want to generate an image from a text prompt using any supported provider so that I don't need to learn provider-specific APIs.

**Acceptance Criteria:**
- [ ] `media.GenerateImage(ctx, prompt, opts...)` returns an `*ImageResult`
- [ ] Result contains image bytes, dimensions, format, model used, and generation metadata
- [ ] Works with Google (Imagen, Gemini), OpenAI (gpt-image-1) at minimum
- [ ] Provider is selected by model name; no provider-specific code needed by the caller
- [ ] Aspect ratio is normalized across providers (e.g., `media.Aspect16x9`)
- [ ] Output format is requestable (PNG, JPEG, WebP) and normalized across providers

### US-002: Generate a Video

**Description:** As a developer, I want to generate a video from a text prompt using any supported provider.

**Acceptance Criteria:**
- [ ] `media.GenerateVideo(ctx, prompt, opts...)` returns a `*VideoResult`
- [ ] Result contains video bytes, dimensions, duration, format, and model used
- [ ] Works with Google (Veo) and OpenAI (Sora) at minimum
- [ ] Duration is specified in `time.Duration`, normalized across providers
- [ ] Polling is handled internally; the call blocks until complete or context is cancelled
- [ ] Timeout is configurable (default 10 minutes for video)

### US-003: Edit an Image

**Description:** As a developer, I want to edit an existing image using a text prompt.

**Acceptance Criteria:**
- [ ] `media.EditImage(ctx, prompt, opts...)` accepts reference image bytes via option
- [ ] Works with providers that support editing (OpenAI initially)
- [ ] Returns `*ImageResult` with the same shape as generation
- [ ] Returns a clear error for providers that don't support editing

### US-004: Fan-Out Generation

**Description:** As a developer, I want to send the same prompt to multiple models and collect all results.

**Acceptance Criteria:**
- [ ] `media.GenerateImages(ctx, prompt, opts...)` with `WithModels(...)` fans out concurrently
- [ ] Returns `[]*ImageResult` with one result per model (or an error per model)
- [ ] Individual provider failures don't fail the entire call
- [ ] Results include which model produced each image

### US-005: CLI Image Generation

**Description:** As a CLI user, I want to generate an image with a single command.

**Acceptance Criteria:**
- [ ] `dive image "prompt"` generates an image and saves it to the current directory
- [ ] `--model` flag selects the model (defaults to a sensible default)
- [ ] `--aspect` flag sets aspect ratio (e.g., `16:9`, `1:1`)
- [ ] `--format` flag sets output format (png, jpeg, webp)
- [ ] `--out` flag specifies output path
- [ ] `--open` flag opens the result in the default viewer
- [ ] Output filename is auto-generated from prompt slug if `--out` is not set

### US-006: CLI Video Generation

**Description:** As a CLI user, I want to generate a video with a single command.

**Acceptance Criteria:**
- [ ] `dive video "prompt"` generates a video and saves it
- [ ] `--duration` flag sets duration (e.g., `4s`, `8s`, `12s`)
- [ ] Progress indication while waiting for generation
- [ ] Same `--model`, `--aspect`, `--out`, `--open` flags as image

### US-007: Agent Image Tool

**Description:** As an agent builder, I want to give my agent the ability to generate images as a tool.

**Acceptance Criteria:**
- [ ] `toolkit.ImageGenerationTool(model)` returns a `dive.Tool`
- [ ] Tool accepts prompt, aspect ratio, and output path as parameters
- [ ] Tool writes the generated image to disk and returns the file path
- [ ] Works within the standard Dive agent loop with permission hooks
- [ ] Tool description is clear enough for an LLM to use it effectively

### US-008: Provider Registration

**Description:** As a library author, I want to add new image/video providers using the same registry pattern as LLM providers.

**Acceptance Criteria:**
- [ ] `media.Registry` follows the same pattern as `providers.Registry`
- [ ] Providers self-register via `init()`
- [ ] Model name matching routes to the correct provider
- [ ] New providers can be added without modifying core code

## Functional Requirements

- **FR-1:** The `media` package provides `GenerateImage`, `EditImage`, `GenerateVideo`, and `GenerateImages` top-level functions.
- **FR-2:** All functions accept a context, a prompt string, and variadic options.
- **FR-3:** Options include: `WithModel`, `WithModels`, `WithAspectRatio`, `WithOutputFormat`, `WithCount`, `WithReferenceImage`, `WithDuration`, `WithTimeout`.
- **FR-4:** `ImageResult` contains: `Data []byte`, `Model string`, `Format string`, `MimeType string`, `Width int`, `Height int`, `Metadata map[string]any`. It has a `WriteTo(path) error` convenience method.
- **FR-5:** `VideoResult` contains: `Data []byte`, `Model string`, `Format string`, `MimeType string`, `Width int`, `Height int`, `Duration time.Duration`, `AspectRatio string`, `Metadata map[string]any`. It has a `WriteTo(path) error` convenience method.
- **FR-6:** Aspect ratios are defined as typed constants (`Aspect1x1`, `Aspect16x9`, `Aspect9x16`, `Aspect4x3`, `Aspect3x4`) and translated to provider-specific formats internally.
- **FR-7:** Output format normalization: if a provider returns a different format than requested, the library converts it (PNG and JPEG targets; WebP encode not supported in Go stdlib, return as-is if provider supports it natively).
- **FR-8:** Image format detection from magic bytes when provider metadata is missing.
- **FR-9:** Provider interface: `ImageProvider` with `GenerateImage(ctx, prompt, opts) ([]*ImageResult, error)` and optionally `EditImage`. `VideoProvider` with `GenerateVideo(ctx, prompt, opts) (*VideoResult, error)`.
- **FR-10:** The media provider registry is separate from the LLM provider registry. A single provider package (e.g., `providers/google`) may register both LLM and media providers.
- **FR-11:** Video generation handles async polling internally. The caller sees a blocking call. Context cancellation aborts the poll.
- **FR-12:** Error types distinguish between provider errors, timeout errors, and invalid parameter errors.
- **FR-13:** The CLI `dive image` and `dive video` subcommands are added to the experimental CLI.
- **FR-14:** `toolkit.ImageGenerationTool` and `toolkit.VideoGenerationTool` are added to the toolkit package.

## Non-Goals (Out of Scope)

| Non-Goal | Rationale |
|----------|-----------|
| **Cost tracking / routing optimization** | Valuable but separate concern. Build the generation layer first; cost intelligence can layer on top. |
| **LoRA / custom model support** | Provider-specific and complex. Defer until base generation is solid. Can be added via provider-specific options later. |
| **Image-to-image (ControlNet, IP-Adapter)** | Advanced workflows beyond basic edit. Defer to a future iteration. |
| **Streaming partial image results** | No providers support this meaningfully yet for images. Video streaming may come later. |
| **Hosted proxy / API gateway** | This is a Go library, not a SaaS. Network APIs are out of scope. |
| **Audio generation** | Different enough modality to deserve its own PRD. |
| **Batch/queue job management** | Job queuing belongs at the application layer. Dive stays synchronous (or context-aware blocking for video). |

**Future considerations worth designing for:**
- Provider-specific option pass-through (e.g., `WithProviderOption("seed", 42)`) for power users
- Callback/progress reporting for video generation polling
- Image evaluation/comparison (e.g., using Gemini to score or compare generated images)

## Dependencies & Risks

| Risk / Dependency | Impact | Mitigation |
|-------------------|--------|------------|
| Provider API instability | APIs are young and change frequently (Imagen went through 3 versions in a year) | Isolate provider specifics behind interfaces; encode/decode pattern from existing LLM providers |
| Async video polling complexity | Veo uses long-polling (up to 10 min), Sora uses `NewAndPoll` -- different patterns | Abstract behind blocking call with context; each provider implements its own poll loop |
| WebP encoding not in Go stdlib | Can't convert to WebP output format | Document limitation; return WebP as-is when provider generates it natively |
| Large binary data in memory | Images are megabytes, videos can be hundreds of megabytes | `WriteTo` method streams to disk; consider `io.Reader` interface for large video results |
| Rate limiting varies wildly by provider | Google Imagen has strict quotas; OpenAI is more generous | Surface rate limit errors clearly; don't retry automatically (let caller decide) |

## Assumptions & Constraints

**Assumptions:**
- Developers have their own API keys for each provider they want to use
- Provider SDKs (google/genai, openai-go) are available as Go modules
- Image generation latency (2-30 seconds) is acceptable as a blocking call
- Video generation latency (1-10 minutes) requires context-aware blocking, not a job queue

**Constraints:**
- Must not break existing Dive APIs (additive only)
- Must follow Dive's existing patterns: provider registry, tool interface, option functions
- Go stdlib only for image format handling (no CGo image libraries)
- The `media` package is the new top-level package; providers extend existing provider packages

## Technical Considerations

### Package Layout

```
media/                          # New top-level package
  media.go                      # GenerateImage, EditImage, GenerateVideo, GenerateImages
  options.go                    # WithModel, WithAspectRatio, etc.
  result.go                     # ImageResult, VideoResult
  provider.go                   # ImageProvider, VideoProvider interfaces
  registry.go                   # Media provider registry
  aspect_ratio.go               # AspectRatio type and constants
  format.go                     # Format detection, conversion utilities

providers/
  google/
    media.go                    # Google ImageProvider + VideoProvider (Imagen, Gemini, Veo)
  openai/
    media.go                    # OpenAI ImageProvider + VideoProvider (gpt-image-1, Sora)

toolkit/
  image_generation.go           # ImageGenerationTool
  video_generation.go           # VideoGenerationTool
```

### Proven Internal Patterns to Adapt

We have battle-tested code from internal projects covering:
- Image format detection from magic bytes
- Aspect ratio to dimension mapping
- Google GenAI SDK integration (Gemini vs Imagen model detection, Vertex AI support)
- OpenAI image generation and editing
- Video generation with async polling (Veo long-poll, Sora `NewAndPoll`)
- Format conversion (PNG/JPEG re-encoding)

This code should be adapted to Dive's unified provider interface pattern, not carried over as provider-specific interfaces (`GoogleProvider`, `OpenAIProvider`). Dive needs a single `ImageProvider` interface that all providers implement.

### API Design Principle

Follow Dive's existing pattern: the simplest call should be one line, with progressive disclosure through options.

```go
// Simplest possible call
result, err := media.GenerateImage(ctx, "a cat")

// With options
result, err := media.GenerateImage(ctx, "a cat",
    media.WithModel(google.Imagen4()),
    media.WithAspectRatio(media.Aspect16x9),
    media.WithOutputFormat(media.FormatPNG),
)
```

## Open Questions

1. **Default model:** What should `media.GenerateImage` use if no model is specified? Follow the LLM pattern (provider-specific defaults) or pick an opinionated default?
2. **Result type vs Content type:** Should `ImageResult` be a new type, or should it reuse/extend `llm.ImageContent`? The latter keeps things unified but may conflate input images (for LLM vision) with generated images.
3. **Provider package co-location:** Should media providers live alongside LLM providers (e.g., `providers/google/media.go`) or in a separate tree (e.g., `media/providers/google/`)? Co-location feels more natural but may bloat provider packages.
4. **Video result streaming:** For very large videos, should `VideoResult` support an `io.Reader` interface instead of holding all bytes in memory?
5. **Progress callbacks for video:** Should video generation support a progress callback option for long-running generations, or is context cancellation sufficient?
