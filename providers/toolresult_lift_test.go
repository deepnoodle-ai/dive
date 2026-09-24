package providers

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// pngHeader is base64 for the PNG signature: enough for DetectImageType.
const pngHeader = "iVBORw0KGgo="

func TestToolResultImageMediaType(t *testing.T) {
	assert.Equal(t, "image/jpeg", ToolResultImageMediaType(&dive.ToolResultContent{
		Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/jpeg",
	}))
	assert.Equal(t, "image/png", ToolResultImageMediaType(&dive.ToolResultContent{
		Type: dive.ToolResultContentTypeImage, Data: pngHeader,
	}))
	assert.Equal(t, "", ToolResultImageMediaType(&dive.ToolResultContent{
		Type: dive.ToolResultContentTypeImage, Data: "aW1n",
	}))
	assert.Equal(t, "", ToolResultImageMediaType(&dive.ToolResultContent{
		Type: dive.ToolResultContentTypeImage, MimeType: "image/png",
	}))
	assert.Equal(t, "", ToolResultImageMediaType(nil))
}

func TestLiftToolResultImages(t *testing.T) {
	history := []*llm.Message{
		llm.NewUserTextMessage("look"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "screenshot", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_2", Name: "compare", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_3", Name: "echo", Input: []byte(`{}`)},
		}},
		{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "call_1", Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "shot taken"},
				{Type: dive.ToolResultContentTypeImage, Data: pngHeader},
			}},
			&llm.ToolResultContent{ToolUseID: "call_2", IsError: true, Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeImage, Data: "YQ==", MimeType: "image/jpeg"},
				{Type: dive.ToolResultContentTypeImage, Data: "Yg==", MimeType: "image/webp"},
			}},
			&llm.ToolResultContent{ToolUseID: "call_3", Content: "plain"},
			&llm.TextContent{Text: "reminder"},
		}},
	}
	// Session replay hands the encoder JSON-decoded content ([]any), which
	// must lift the same way.
	body, err := json.Marshal(history)
	assert.NoError(t, err)
	var replayed []*llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))

	for name, in := range map[string][]*llm.Message{"typed": history, "round-tripped": replayed} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(in)
			assert.NoError(t, err)

			out := LiftToolResultImages(in)
			assert.Len(t, out, 3)
			assert.True(t, out[0] == in[0], "messages without images pass through")
			assert.True(t, out[1] == in[1], "messages without images pass through")

			content := out[2].Content
			assert.Len(t, content, 10)
			first := content[0].(*llm.ToolResultContent)
			assert.Equal(t, "call_1", first.ToolUseID)
			assert.Equal(t, []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "shot taken"},
				{Type: dive.ToolResultContentTypeText, Text: "(The image this call returned follows the tool results.)"},
			}, ToolResultBlocks(first))
			second := content[1].(*llm.ToolResultContent)
			assert.True(t, second.IsError)
			assert.Equal(t, []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "(The 2 images this call returned follow the tool results.)"},
			}, ToolResultBlocks(second))
			assert.True(t, content[2] == in[2].Content[2], "a result without images is kept as is")
			assert.Equal(t, "reminder", content[3].(*llm.TextContent).Text)

			// The images follow everything else, each labelled with its call.
			assert.Equal(t, "The image from tool call call_1:", content[4].(*llm.TextContent).Text)
			assert.Equal(t, &llm.ContentSource{Type: llm.ContentSourceTypeBase64, MediaType: "image/png", Data: pngHeader},
				content[5].(*llm.ImageContent).Source)
			assert.Equal(t, "Image 1 of 2 from tool call call_2:", content[6].(*llm.TextContent).Text)
			assert.Equal(t, "image/jpeg", content[7].(*llm.ImageContent).Source.MediaType)
			assert.Equal(t, "Image 2 of 2 from tool call call_2:", content[8].(*llm.TextContent).Text)
			assert.Equal(t, "image/webp", content[9].(*llm.ImageContent).Source.MediaType)

			after, err := json.Marshal(in)
			assert.NoError(t, err)
			assert.Equal(t, string(before), string(after), "the caller's messages must not change")
		})
	}
}

// LiftErrorToolResultImages moves only the images of error results.
func TestLiftErrorToolResultImages(t *testing.T) {
	image := &dive.ToolResultContent{Type: dive.ToolResultContentTypeImage, Data: "YQ==", MimeType: "image/png"}
	in := []*llm.Message{{Role: llm.User, Content: []llm.Content{
		&llm.ToolResultContent{ToolUseID: "call_1", Content: []*dive.ToolResultContent{image}},
		&llm.ToolResultContent{ToolUseID: "call_2", IsError: true, Content: []*dive.ToolResultContent{image}},
	}}}
	out := LiftErrorToolResultImages(in)
	content := out[0].Content
	assert.Len(t, content, 4)
	assert.True(t, content[0] == in[0].Content[0], "a successful result keeps its image")
	failed := content[1].(*llm.ToolResultContent)
	assert.True(t, failed.IsError)
	assert.Equal(t, []*dive.ToolResultContent{
		{Type: dive.ToolResultContentTypeText, Text: "(The image this call returned follows the tool results.)"},
	}, ToolResultBlocks(failed))
	assert.Equal(t, "The image from tool call call_2:", content[2].(*llm.TextContent).Text)
	assert.Equal(t, "YQ==", content[3].(*llm.ImageContent).Source.Data)
}

// An image that cannot be shown stays in its tool result for the encoder's
// placeholder, and a request with nothing to lift is returned as it came.
func TestLiftToolResultImagesLeavesUnshowableImages(t *testing.T) {
	in := []*llm.Message{llm.NewToolResultMessage(&llm.ToolResultContent{
		ToolUseID: "call_1",
		Content: []*dive.ToolResultContent{
			{Type: dive.ToolResultContentTypeImage, Data: "aW1n"}, // no MIME type, undetectable
			{Type: dive.ToolResultContentTypeImage, MimeType: "image/png"},
		},
	})}
	out := LiftToolResultImages(in)
	assert.True(t, &out[0] == &in[0], "nothing to lift returns the same slice")

	plain := []*llm.Message{llm.NewUserTextMessage("hi")}
	assert.True(t, &LiftToolResultImages(plain)[0] == &plain[0])
}
