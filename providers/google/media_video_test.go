package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// pngHeader is enough for media.DetectMIMEFromBytes to report image/png.
var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")

// fakeVeo is an httptest server that answers Veo requests. It records the
// paths and the start request body. The start request returns pending when
// it is set, and final otherwise. Each poll returns final.
type fakeVeo struct {
	pending string
	final   string

	mu        sync.Mutex
	paths     []string
	startBody map[string]any
}

func (f *fakeVeo) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.paths = append(f.paths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, ":predictLongRunning"):
			assert.NoError(t, json.Unmarshal(body, &f.startBody))
			w.Header().Set("Content-Type", "application/json")
			if f.pending != "" {
				_, _ = io.WriteString(w, f.pending)
				return
			}
			_, _ = io.WriteString(w, f.final)
		case strings.HasSuffix(r.URL.Path, ":download"):
			_, _ = io.WriteString(w, "downloaded-video")
		default:
			// An operation poll: fetchPredictOperation on Vertex, a GET on
			// the Gemini API.
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, f.final)
		}
	}
}

func newFakeVeoProvider(t *testing.T, backend genai.Backend, fake *fakeVeo) *MediaProvider {
	t.Helper()
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     backend,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	assert.NoError(t, err)
	return &MediaProvider{
		client:            client,
		vertexAI:          backend == genai.BackendVertexAI,
		videoPollInterval: time.Millisecond,
	}
}

func TestGenerateVideo_VertexSendsDurationImageAndAudio(t *testing.T) {
	videoBytes := []byte("vertex-video")
	const opName = "projects/p/locations/us-central1/publishers/google/models/veo-3.1-fast-generate-001/operations/op-1"
	fake := &fakeVeo{
		pending: `{"name":"` + opName + `"}`,
		final: `{"name":"` + opName + `","done":true,"response":{"videos":[{"bytesBase64Encoded":"` +
			base64.StdEncoding.EncodeToString(videoBytes) + `","mimeType":"video/mp4"}]}}`,
	}
	p := newFakeVeoProvider(t, genai.BackendVertexAI, fake)

	result, err := p.GenerateVideo(context.Background(), "clouds drift", &media.Config{
		Model:           "veo-3.1-fast-generate-001",
		AspectRatio:     media.Aspect16x9,
		Duration:        4 * time.Second,
		ReferenceImages: [][]byte{pngHeader},
	})
	assert.NoError(t, err)
	assert.Equal(t, videoBytes, result.Data)
	assert.Equal(t, "mp4", result.Format)
	assert.Equal(t, 4*time.Second, result.Duration)

	instance := fake.startBody["instances"].([]any)[0].(map[string]any)
	assert.Equal(t, "clouds drift", instance["prompt"])
	image := instance["image"].(map[string]any)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngHeader), image["bytesBase64Encoded"])
	assert.Equal(t, "image/png", image["mimeType"])

	params := fake.startBody["parameters"].(map[string]any)
	assert.Equal(t, float64(4), params["durationSeconds"])
	assert.Equal(t, true, params["generateAudio"])
	assert.Equal(t, "16:9", params["aspectRatio"])

	// The provider polled once and never called Files.Download.
	assert.Equal(t, 2, len(fake.paths))
	assert.Contains(t, fake.paths[1], ":fetchPredictOperation")
}

func TestGenerateVideo_GeminiDownloadsFile(t *testing.T) {
	fake := &fakeVeo{
		final: `{"name":"operations/op-1","done":true,"response":{"generateVideoResponse":{"generatedSamples":[` +
			`{"video":{"uri":"https://generativelanguage.googleapis.com/v1beta/files/abc:download?alt=media"}}]}}}`,
	}
	p := newFakeVeoProvider(t, genai.BackendGeminiAPI, fake)

	result, err := p.GenerateVideo(context.Background(), "a leaf falls", &media.Config{
		Model:    "veo-3.1-generate-preview",
		Duration: 6 * time.Second,
	})
	assert.NoError(t, err)
	assert.Equal(t, []byte("downloaded-video"), result.Data)
	assert.Equal(t, "mp4", result.Format)

	params := fake.startBody["parameters"].(map[string]any)
	assert.Equal(t, float64(6), params["durationSeconds"])
	_, hasAudio := params["generateAudio"]
	assert.False(t, hasAudio)
	assert.Contains(t, fake.paths[len(fake.paths)-1], "files/abc:download")
}

func TestGenerateVideo_SafetyFilterReasons(t *testing.T) {
	fake := &fakeVeo{
		final: `{"name":"operations/op-1","done":true,"response":{"raiMediaFilteredCount":1,` +
			`"raiMediaFilteredReasons":["The prompt may violate our policies."]}}`,
	}
	p := newFakeVeoProvider(t, genai.BackendVertexAI, fake)

	_, err := p.GenerateVideo(context.Background(), "blocked", &media.Config{Model: "veo-3.1-fast-generate-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "safety filters")
	assert.Contains(t, err.Error(), "The prompt may violate our policies.")
}

func TestGenerateVideo_SafetyFilterCountOnly(t *testing.T) {
	fake := &fakeVeo{
		final: `{"name":"operations/op-1","done":true,"response":{"raiMediaFilteredCount":1}}`,
	}
	p := newFakeVeoProvider(t, genai.BackendVertexAI, fake)

	_, err := p.GenerateVideo(context.Background(), "blocked", &media.Config{Model: "veo-3.1-fast-generate-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "safety filters")
}

// TestGoogleGenerateVideo_Vertex_Integration makes a paid Veo call on Vertex
// AI. It needs RUN_INTEGRATION_TESTS, GOOGLE_CLOUD_PROJECT, and application
// default credentials. GOOGLE_CLOUD_LOCATION defaults to us-central1.
func TestGoogleGenerateVideo_Vertex_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping video generation test in short mode")
	}
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("RUN_INTEGRATION_TESTS not set")
	}
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT not set")
	}
	location := os.Getenv("GOOGLE_CLOUD_LOCATION")
	if location == "" {
		location = "us-central1"
	}

	// A plain blue gradient is the first frame.
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 90, B: uint8(120 + y/3), A: 255})
		}
	}
	var start bytes.Buffer
	assert.NoError(t, png.Encode(&start, img))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	began := time.Now()
	p := NewMediaProvider(WithMediaVertexAI(location))
	result, err := p.GenerateVideo(ctx, "Slow clouds drift across a blue evening sky.", &media.Config{
		Model:           "veo-3.1-fast-generate-001",
		AspectRatio:     media.Aspect16x9,
		Duration:        4 * time.Second,
		ReferenceImages: [][]byte{start.Bytes()},
	})
	assert.NoError(t, err)
	assert.True(t, len(result.Data) > 0)
	assert.Equal(t, "mp4", result.Format)
	t.Logf("video: %d bytes, %s, %s, took %s",
		len(result.Data), result.MimeType, result.AspectRatio, time.Since(began).Round(time.Second))
	if out := os.Getenv("VEO_OUTPUT"); out != "" {
		assert.NoError(t, os.WriteFile(out, result.Data, 0o644))
	}
}

func TestGenerateVideo_OperationError(t *testing.T) {
	fake := &fakeVeo{
		final: `{"name":"operations/op-1","done":true,"error":{"code":3,"message":"duration not supported"}}`,
	}
	p := newFakeVeoProvider(t, genai.BackendVertexAI, fake)

	_, err := p.GenerateVideo(context.Background(), "x", &media.Config{Model: "veo-3.1-fast-generate-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duration not supported (code 3)")
}
