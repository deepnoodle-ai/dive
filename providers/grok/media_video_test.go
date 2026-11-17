package grok

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// newFakeVideoProvider returns a provider whose client talks to handler.
func newFakeVideoProvider(t *testing.T, handler http.HandlerFunc) (*MediaProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := openai.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL+"/v1"))
	return &MediaProvider{client: &client, videoPollInterval: time.Millisecond}, server
}

func TestGrokGenerateVideo_NativeAPI(t *testing.T) {
	var polls atomic.Int32
	var body map[string]any
	var serverURL string
	p, server := newFakeVideoProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/videos/generations":
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			_, _ = w.Write([]byte(`{"request_id":"req-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/req-1":
			if polls.Add(1) < 2 {
				_, _ = w.Write([]byte(`{"status":"pending","progress":18}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"done","model":"grok-imagine-video",` +
				`"video":{"url":"` + serverURL + `/file.mp4","duration":6},` +
				`"usage":{"cost_in_usd_ticks":2500000000}}`))
		case r.URL.Path == "/file.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("\x00\x00\x00\x18ftypmp42"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	serverURL = server.URL

	result, err := p.GenerateVideo(context.Background(), "a wave", &media.Config{
		Duration:    6 * time.Second,
		AspectRatio: media.Aspect9x16,
	})
	assert.NoError(t, err)
	assert.Equal(t, ModelImagineVideo, body["model"])
	assert.Equal(t, float64(6), body["duration"])
	assert.Equal(t, "9:16", body["aspect_ratio"])
	assert.Equal(t, "grok-imagine-video", result.Model)
	assert.Equal(t, 6*time.Second, result.Duration)
	assert.Equal(t, media.Aspect9x16, result.AspectRatio)
	assert.Equal(t, "req-1", result.Metadata["request_id"])
	assert.Equal(t, 0.25, result.Metadata["cost_usd"])
	assert.True(t, len(result.Data) > 0)
}

func TestGrokGenerateVideo_Failed(t *testing.T) {
	p, _ := newFakeVideoProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"request_id":"req-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"failed","error":{"message":"content moderated"}}`))
	})
	_, err := p.GenerateVideo(context.Background(), "a wave", &media.Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed: content moderated")
}
