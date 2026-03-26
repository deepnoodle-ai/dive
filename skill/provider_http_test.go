package skill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestHTTPProvider_ManifestMode(t *testing.T) {
	// Create skill endpoint
	skillContent := `---
name: remote-skill
description: A remote skill.
---

Remote instructions.`

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		manifest := skillManifest{
			Skills: []skillManifestEntry{
				{Name: "remote-skill", URL: "/skills/remote-skill/SKILL.md"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	})
	mux.HandleFunc("/skills/remote-skill/SKILL.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte(skillContent))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewHTTPProvider(server.URL)
	skills, err := provider.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(skills))
	assert.Equal(t, "remote-skill", skills[0].Name)
	assert.Equal(t, "A remote skill.", skills[0].Description)
	assert.Equal(t, "", skills[0].FilePath)
	assert.Contains(t, skills[0].SourceURI, server.URL)
}

func TestHTTPProvider_DirectMode(t *testing.T) {
	skillContent := `---
name: direct-skill
description: A direct skill.
---

Direct instructions.`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte(skillContent))
	}))
	defer server.Close()

	provider := NewHTTPProvider(server.URL)
	skills, err := provider.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(skills))
	assert.Equal(t, "direct-skill", skills[0].Name)
	assert.Equal(t, server.URL, skills[0].SourceURI)
}

func TestHTTPProvider_ETag(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		w.Header().Set("ETag", `"abc123"`)
		w.Write([]byte(`---
name: cached
description: Cached skill.
---
Instructions.`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(server.URL)

	// First call - should get the skill
	skills, err := provider.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(skills))
	assert.Equal(t, "cached", skills[0].Name)

	// Second call - 304, should return cached skill (not empty)
	skills, err = provider.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(skills))
	assert.Equal(t, "cached", skills[0].Name)
	assert.Equal(t, 2, callCount)
}

func TestHTTPProvider_NetworkError(t *testing.T) {
	provider := NewHTTPProvider("http://localhost:1/nonexistent")
	skills, err := provider.Load(context.Background())
	assert.NoError(t, err) // Errors are logged, not returned
	assert.Equal(t, 0, len(skills))
}

func TestHTTPProvider_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewHTTPProvider(server.URL)
	skills, err := provider.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(skills))
}

func TestHTTPProvider_Name(t *testing.T) {
	provider := NewHTTPProvider("http://example.com")
	assert.Equal(t, "http", provider.Name())
}
