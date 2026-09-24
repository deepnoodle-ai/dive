package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// WithoutToolResultImages passes through to the Chat Completions encoder.
func TestWithoutToolResultImages(t *testing.T) {
	history := []*llm.Message{
		llm.NewUserTextMessage("Take a screenshot."),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "screenshot", Input: []byte(`{}`)},
		}},
		{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "call_1", Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeImage, Data: "aW1nMQ==", MimeType: "image/png"},
			}},
		}},
	}
	for _, tc := range []struct {
		name      string
		opts      []Option
		wantImage bool
	}{
		{name: "default sends the image", wantImage: true},
		{name: "option omits the image", opts: []Option{WithoutToolResultImages()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				body = string(raw)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"c","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			}))
			t.Cleanup(server.Close)
			opts := append([]Option{
				WithAPIKey("test-key"),
				WithEndpoint(server.URL),
				WithModel(DefaultModel),
				WithMaxRetries(0),
			}, tc.opts...)
			_, err := New(opts...).Generate(context.Background(), llm.WithMessages(history...))
			assert.NoError(t, err)
			assert.Equal(t, tc.wantImage, strings.Contains(body, "data:image/png;base64,aW1nMQ=="))
			assert.Equal(t, !tc.wantImage, strings.Contains(body, "[image content omitted]"))
		})
	}
}
