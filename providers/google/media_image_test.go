package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// TestGenerateImage_ReportsWhyNoImage checks that a Gemini response that
// declines to draw names the finish reason, rather than only saying no image
// came back. genai drops finishMessage from Gemini API responses, so only the
// reason reaches Dive there.
func TestGenerateImage_ReportsWhyNoImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"IMAGE_RECITATION",` +
			`"finishMessage":"Unable to show the generated image."}]}`))
	}))
	t.Cleanup(server.Close)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	assert.NoError(t, err)
	p := &MediaProvider{client: client}

	_, err = p.GenerateImage(context.Background(), "a red circle", &media.Config{
		Model: ModelGeminiNanoBanana21,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IMAGE_RECITATION")
}

// TestGenerateImage_ImagenIsShutDown checks that an Imagen model fails with a
// pointer to a working model, without a request to Google.
func TestGenerateImage_ImagenIsShutDown(t *testing.T) {
	p := NewMediaProvider(WithMediaAPIKey("test-key"))
	_, err := p.GenerateImage(context.Background(), "a teapot", &media.Config{
		Model: "imagen-4.0-generate-001",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shut down")
	assert.Contains(t, err.Error(), ModelGeminiNanoBanana21)
}
