package google

const (
	// Gemini 3.5 models (stable)
	ModelGemini35Flash = "gemini-3.5-flash"

	// Gemini 3.1 models
	ModelGemini31ProPreview            = "gemini-3.1-pro-preview"
	ModelGemini31ProPreviewCustomTools = "gemini-3.1-pro-preview-customtools" // optimized for custom tools + bash
	ModelGemini31FlashLite             = "gemini-3.1-flash-lite"              // stable
	ModelGemini31FlashLitePreview      = "gemini-3.1-flash-lite-preview"
	ModelGemini31FlashLivePreview      = "gemini-3.1-flash-live-preview" // Live API (audio-to-audio)
	ModelGemini31ProImage              = "gemini-3.1-pro-image"          // Nano Banana Pro (stable)
	ModelGemini31FlashImage            = "gemini-3.1-flash-image"        // Nano Banana 2 (stable)
	ModelGemini31FlashImagePrev        = "gemini-3.1-flash-image-preview"

	// Gemini 3 models (preview)
	ModelGemini3ProImagePreview = "gemini-3-pro-image-preview"
	ModelGemini3FlashPreview    = "gemini-3-flash-preview" // preview alias for Gemini 3.5 Flash

	// Deprecated: Model was shut down March 9, 2026. Use ModelGemini31ProPreview instead.
	ModelGemini3ProPreview = "gemini-3-pro-preview"

	// Gemini 2.5 models
	ModelGemini25Pro        = "gemini-2.5-pro"
	ModelGemini25ProLong    = "gemini-2.5-pro-long"
	ModelGemini25Flash      = "gemini-2.5-flash"
	ModelGemini25FlashLite  = "gemini-2.5-flash-lite"
	ModelGemini25FlashImage = "gemini-2.5-flash-image"

	// Gemini 2.0 models (deprecated, shutdown March 31, 2026)
	ModelGemini20Flash = "gemini-2.0-flash"

	// Gemini 1.5 models (deprecated)
	ModelGemini15Pro   = "gemini-1.5-pro"
	ModelGemini15Flash = "gemini-1.5-flash"
)
