# Media Generation Library

This package provides a unified interface for image and video generation across multiple AI providers, including OpenAI and Google GenAI.

## Overview

The media package abstracts image and video generation capabilities behind common interfaces, allowing for easy switching between providers and future extensibility.

## Key Components

### Interfaces

- **`ImageGenerator`**: Interface for image generation and editing
- **`VideoGenerator`**: Interface for video generation
- **`MediaGenerator`**: Combined interface for both image and video generation
- **`VideoOperationChecker`**: Interface for checking long-running video operation status

### Core Types

- **`ImageGenerationRequest`**: Request parameters for image generation
- **`ImageGenerationResponse`**: Response containing generated images
- **`ImageEditRequest`**: Request parameters for image editing
- **`ImageEditResponse`**: Response containing edited images
- **`VideoGenerationRequest`**: Request parameters for video generation
- **`VideoGenerationResponse`**: Response containing generated videos or operation status
- **`GeneratedImage`**: Represents a single generated image
- **`GeneratedVideo`**: Represents a single generated video
- **`OperationStatus`**: Status of long-running operations
- **`Usage`**: Usage information (tokens, credits, cost)

## Providers

### OpenAI Provider

Located in `providers/openai/`, supports:
- Image generation using DALL-E 2, DALL-E 3, and gpt-image-1 models
- Image editing using DALL-E 2
- Various quality settings and output formats
- Provider-specific parameters for moderation, output format, and compression

**Supported Models:**
- `dall-e-2`: Basic image generation and editing
- `dall-e-3`: Advanced image generation with higher quality
- `gpt-image-1`: Latest model with flexible sizing and quality options

### Google GenAI Provider

Located in `providers/google/`, supports:
- Image generation using Imagen models
- Video generation using Veo models
- Async operation handling for video generation
- Safety attributes and RAI (Responsible AI) information

**Supported Models:**
- `imagen-3.0-generate-001`: Image generation
- `imagen-3.0-generate-002`: Advanced image generation
- `veo-2.0-generate-001`: Video generation

### Provider Registry

The `providers` package includes a registry system for managing and accessing providers:
- **`Registry`**: Manages registered providers
- **`DefaultRegistry()`**: Creates a registry with available providers
- **`CreateProvider()`**: Factory function for creating specific providers
- **`GetAvailableProviders()`**: Lists providers that can be initialized

## Usage Examples

### Basic Image Generation

```go
import (
    "context"
    "github.com/deepnoodle-ai/dive/media"
    "github.com/deepnoodle-ai/dive/media/providers"
)

// Create provider registry
registry, err := providers.DefaultRegistry()
if err != nil {
    panic(err)
}

// Get image generator
generator, err := registry.GetImageGenerator("openai")
if err != nil {
    panic(err)
}

// Create request
req := &media.ImageGenerationRequest{
    Prompt:  "A beautiful sunset over mountains",
    Model:   "gpt-image-1",
    Size:    "1024x1024",
    Quality: "high",
    Count:   1,
}

// Generate image
ctx := context.Background()
response, err := generator.GenerateImage(ctx, req)
if err != nil {
    panic(err)
}

// Access generated images
for _, image := range response.Images {
    // image.B64JSON contains base64-encoded image data
    // image.URL contains image URL (if available)
    // image.RevisedPrompt contains any prompt revisions
}
```

### Video Generation

```go
// Get video generator
generator, err := registry.GetVideoGenerator("google")
if err != nil {
    panic(err)
}

// Create request
req := &media.VideoGenerationRequest{
    Prompt: "A cat walking in a garden",
    Model:  "veo-2.0-generate-001",
}

// Generate video (async operation)
response, err := generator.GenerateVideo(ctx, req)
if err != nil {
    panic(err)
}

// Check operation status
if checker, ok := generator.(media.VideoOperationChecker); ok {
    status, err := checker.CheckVideoOperation(ctx, response.OperationID)
    if err != nil {
        panic(err)
    }
    
    if status.Status == "completed" {
        // Video is ready
        if result, ok := status.Result.(*media.VideoGenerationResponse); ok {
            for _, video := range result.Videos {
                // video.URL contains video URL
                // video.Duration contains video duration
                // video.Format contains video format
            }
        }
    }
}
```

### Provider-Specific Parameters

```go
// OpenAI-specific parameters
req := &media.ImageGenerationRequest{
    Prompt:  "A futuristic city",
    Model:   "gpt-image-1",
    ProviderSpecific: map[string]interface{}{
        "moderation":         "low",
        "output_format":      "webp",
        "output_compression": 80,
    },
}

// Google-specific parameters
req := &media.ImageGenerationRequest{
    Prompt: "A serene landscape",
    Model:  "imagen-3.0-generate-002",
    ProviderSpecific: map[string]interface{}{
        "output_mime_type":           "image/png",
        "include_rai_reason":         true,
        "include_safety_attributes":  false,
    },
}
```

## CLI Integration

The media package is integrated into Dive's CLI:

### Image Commands

```bash
# Generate image with OpenAI
dive image generate --prompt "A beautiful sunset" --provider openai --model gpt-image-1

# Generate image with Google
dive image generate --prompt "A beautiful sunset" --provider google --model imagen-3.0-generate-002

# Edit image
dive image edit --input image.png --prompt "Make the sky blue" --provider openai
```

### Video Commands

```bash
# Generate video
dive video generate --prompt "A cat walking" --provider google --model veo-2.0-generate-001

# Check video status
dive video status --operation-id op-12345 --provider google

# Wait for completion
dive video generate --prompt "A cat walking" --wait
```

## Toolkit Integration

The existing `toolkit.ImageGenerationTool` has been updated to use the media package, providing backward compatibility while leveraging the new provider system.

## Testing

Comprehensive tests are provided for:
- Core media types and interfaces
- Provider implementations
- Registry functionality
- CLI commands
- Toolkit integration

## Environment Variables

- **`OPENAI_API_KEY`**: Required for OpenAI provider
- **Google credentials**: Required for Google GenAI provider (follows Google's authentication patterns)

## Future Extensions

The architecture supports easy addition of new providers by:
1. Implementing the `ImageGenerator` and/or `VideoGenerator` interfaces
2. Adding the provider to the registry
3. Updating CLI and toolkit integrations as needed

Potential future providers:
- Anthropic (when they add image/video generation)
- Stability AI
- Midjourney API
- Adobe Firefly
- And more...
