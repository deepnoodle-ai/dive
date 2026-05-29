package grok

const (
	// Grok 4.3 (latest, most intelligent and fastest, 1M context)
	ModelGrok43 = "grok-4.3"

	// Grok 4.20 models (1M context)
	ModelGrok420Reasoning    = "grok-4.20-0309-reasoning"
	ModelGrok420NonReasoning = "grok-4.20-0309-non-reasoning"
	ModelGrok420MultiAgent   = "grok-4.20-multi-agent-0309"

	// Grok coding model (256K context)
	ModelGrokBuild01 = "grok-build-0.1"

	// Grok 4.1 models (2M context)
	ModelGrok41FastReasoning    = "grok-4-1-fast-reasoning"
	ModelGrok41FastNonReasoning = "grok-4-1-fast-non-reasoning"

	// Grok 4 models (2M context)
	ModelGrok4FastReasoning    = "grok-4-fast-reasoning"
	ModelGrok4FastNonReasoning = "grok-4-fast-non-reasoning"
	ModelGrok40709             = "grok-4-0709"
	ModelGrok4                 = "grok-4"
	ModelGrok4Latest           = "grok-4-latest"

	// Grok 3 models (131K context)
	ModelGrok3       = "grok-3"
	ModelGrok3Latest = "grok-3-latest"
	ModelGrok3Mini   = "grok-3-mini"

	// Specialized models (256K context)
	ModelGrokCodeFast1 = "grok-code-fast-1"

	// Image generation models
	ModelImagineImagePro     = "grok-imagine-image-pro"
	ModelImagineImage        = "grok-imagine-image"
	ModelImagineImageQuality = "grok-imagine-image-quality"

	// Video generation model
	ModelImagineVideo = "grok-imagine-video"

	// Deprecated: No longer listed in xAI docs.
	ModelGrok2Vision1212 = "grok-2-vision-1212"
	// Deprecated: No longer listed in xAI docs.
	ModelGrok2Image1212 = "grok-2-image-1212"
)
