package meta

import (
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
