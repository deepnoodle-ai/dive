package openaicompletions

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

func screenshotHistory() []*llm.Message {
	return []*llm.Message{
		llm.NewUserTextMessage("Take a screenshot."),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "screenshot", Input: []byte(`{}`)},
		}},
		{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "call_1", Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "the screen"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1nMQ==", MimeType: "image/png"},
			}},
		}},
	}
}

// Without lifting, a tool-result image stays a placeholder in its tool
// message and no user image message follows.
func TestConvertMessagesWithoutLiftingImages(t *testing.T) {
	converted, err := convertMessagesWith(screenshotHistory(), "", false)
	assert.NoError(t, err)
	assert.Len(t, converted, 3)
	assert.Equal(t, "tool", converted[2].Role)
	assert.Equal(t, "the screen\n[image content omitted]", converted[2].Content)
}

func TestWithoutToolResultImages(t *testing.T) {
	for _, tc := range []struct {
		name       string
		opts       []Option
		wantImage  bool
		wantMarker bool
	}{
		{name: "default sends the image", wantImage: true},
		{name: "option omits the image", opts: []Option{WithoutToolResultImages()}, wantMarker: true},
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
				WithModel(ModelGPT55),
				WithMaxRetries(0),
			}, tc.opts...)
			_, err := New(opts...).Generate(context.Background(), llm.WithMessages(screenshotHistory()...))
			assert.NoError(t, err)
			assert.Equal(t, tc.wantImage, strings.Contains(body, "data:image/png;base64,aW1nMQ=="))
			assert.Equal(t, tc.wantMarker, strings.Contains(body, "[image content omitted]"))
		})
	}
}
