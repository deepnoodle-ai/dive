package google

import (
	"strconv"
	"strings"
)

const (
	// Gemini 3.6 models (stable)
	ModelGemini36Flash = "gemini-3.6-flash"

	// Gemini 3.5 models (stable)
	ModelGemini35Flash     = "gemini-3.5-flash"
	ModelGemini35FlashLite = "gemini-3.5-flash-lite"

	// Gemini 3.1 models
	ModelGemini31ProPreview            = "gemini-3.1-pro-preview"
	ModelGemini31ProPreviewCustomTools = "gemini-3.1-pro-preview-customtools" // optimized for custom tools + bash
	ModelGemini31FlashLite             = "gemini-3.1-flash-lite"              // stable
	ModelGemini31FlashLitePreview      = "gemini-3.1-flash-lite-preview"
	ModelGemini31FlashLivePreview      = "gemini-3.1-flash-live-preview" // Live API (audio-to-audio)
	ModelGemini31FlashTTSPreview       = "gemini-3.1-flash-tts-preview"
	ModelGemini31FlashLiteImage        = "gemini-3.1-flash-lite-image" // Nano Banana family (stable)
	ModelGemini31FlashImage            = "gemini-3.1-flash-image"      // Nano Banana 2 (stable)
	ModelGemini31FlashImagePrev        = "gemini-3.1-flash-image-preview"

	// Gemini 3 models
	ModelGemini3ProImage        = "gemini-3-pro-image" // Nano Banana Pro (stable)
	ModelGemini3ProImagePreview = "gemini-3-pro-image-preview"
	ModelGemini3FlashPreview    = "gemini-3-flash-preview" // preview alias for Gemini 3.5 Flash

	// Deprecated: use ModelGemini3ProImage.
	ModelGemini31ProImage = ModelGemini3ProImage

	// Deprecated: Model was shut down March 9, 2026. Use ModelGemini31ProPreview instead.
	ModelGemini3ProPreview = "gemini-3-pro-preview"

	// Gemini 2.5 models
	ModelGemini25Pro        = "gemini-2.5-pro"
	ModelGemini25ProLong    = "gemini-2.5-pro-long"
	ModelGemini25Flash      = "gemini-2.5-flash"
	ModelGemini25FlashLite  = "gemini-2.5-flash-lite"
	ModelGemini25FlashImage = "gemini-2.5-flash-image"
	ModelGemini25FlashTTS   = "gemini-2.5-flash-preview-tts"
	ModelGemini25ProTTS     = "gemini-2.5-pro-preview-tts"

	// Gemini 2.0 models (deprecated, shutdown March 31, 2026)
	ModelGemini20Flash = "gemini-2.0-flash"

	// Gemini 1.5 models (deprecated)
	ModelGemini15Pro   = "gemini-1.5-pro"
	ModelGemini15Flash = "gemini-1.5-flash"
)

// shouldOmitTemperature reports whether a model belongs to the Gemini request
// generation that deprecated temperature. The cutover starts with Gemini 3.5
// Flash-Lite and all Gemini 3.6+ models.
func shouldOmitTemperature(model string) bool {
	model = strings.TrimPrefix(model, "models/")
	if model == ModelGemini35FlashLite || strings.HasPrefix(model, ModelGemini35FlashLite+"-") {
		return true
	}

	version, ok := strings.CutPrefix(model, "gemini-")
	if !ok {
		return false
	}
	version, _, _ = strings.Cut(version, "-")
	majorText, minorText, hasMinor := strings.Cut(version, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil {
		return false
	}
	minor := 0
	if hasMinor {
		minor, err = strconv.Atoi(minorText)
		if err != nil {
			return false
		}
	}
	return major > 3 || (major == 3 && minor >= 6)
}
