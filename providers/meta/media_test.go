package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

// The CLI never names a provider for `dive image`: it hands the model id to the
// media package and lets the registry route. This is that lookup, so a matcher
// that stops matching breaks `dive image -m muse-image-1.0` with nothing else
// failing first.
func TestMediaRegistryRoutesMuseImage(t *testing.T) {
	for _, model := range []string{ModelMuseImage, "muse-image-1.0", "MUSE-IMAGE-1.0"} {
		provider, err := media.DefaultRegistry().ResolveImage(model)
		assert.NoError(t, err)
		_, ok := provider.(*MediaProvider)
		assert.True(t, ok, "model %q routed to %T, not the Meta provider", model, provider)
	}
}

// media.EditImage type-asserts the resolved provider up to ImageEditor and
// fails at runtime when the assertion misses, so pin it at compile time here.
func TestMediaProviderEdits(t *testing.T) {
	var editor media.ImageEditor = NewMediaProvider()
	assert.NotNil(t, editor)
}

// A truncated image is worse than a failed one: the bytes decode far enough to
// be written to disk, so the caller gets a corrupt file and no error.
func TestDownloadImageRejectsOversized(t *testing.T) {
	const cap = 50 * 1024 * 1024

	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"at the cap", cap, false},
		{"one byte over", cap + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write(make([]byte, tt.size))
			}))
			defer server.Close()

			data, err := downloadImage(context.Background(), server.URL)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, len(data), tt.size)
		})
	}
}
