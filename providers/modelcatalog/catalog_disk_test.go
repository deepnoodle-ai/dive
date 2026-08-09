package modelcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// Every provider embeds its catalog through MustParse at package init, so a
// catalog this package rejects panics at process start for anything importing
// that provider. scripts/generate_provider_catalogs.py applies a narrower set of
// rules, and providers/{openai,google,grok} are separate modules that `go test
// ./...` from the root does not reach, so validate the checked-in documents
// directly against the authoritative schema here.
func TestCheckedInCatalogsParse(t *testing.T) {
	entries, err := os.ReadDir("..")
	assert.NoError(t, err)

	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		provider := entry.Name()
		path := filepath.Join("..", provider, "catalog.json")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		assert.NoError(t, err)
		found++

		t.Run(provider, func(t *testing.T) {
			catalog, err := Parse(provider, data)
			assert.NoError(t, err)
			_, ok := catalog.DefaultModel()
			assert.True(t, ok, "catalog must declare a default model")
		})
	}
	assert.True(t, found > 0, "expected to find provider catalogs on disk")
}
