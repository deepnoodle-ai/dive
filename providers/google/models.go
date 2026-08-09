package google

import (
	"strconv"
	"strings"
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
