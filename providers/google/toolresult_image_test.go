package google

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// screenshotTurn is a tool call and its result, a text block and a PNG.
func screenshotTurn(isError bool) []*llm.Message {
	return []*llm.Message{
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "screenshot", Input: []byte(`{}`)},
		}},
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "call_1",
			IsError:   isError,
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "shot taken"},
				{Type: dive.ToolResultContentTypeImage, Data: "iVBORw0KGgo="},
			},
		}),
	}
}

// roundTrip returns messages as session persistence would replay them, with
// tool result blocks decoded to generic []any.
func roundTrip(t *testing.T, messages []*llm.Message) []*llm.Message {
	body, err := json.Marshal(messages)
	assert.NoError(t, err)
	var replayed []*llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))
	return replayed
}

// Gemini 3 takes a tool-result image inside the function response: the text
// stays in Response and the image bytes go in Parts.
func TestToolResultImageSentInFunctionResponseParts(t *testing.T) {
	for _, isError := range []bool{false, true} {
		key := "output"
		if isError {
			key = "error"
		}
		history := screenshotTurn(isError)
		for name, messages := range map[string][]*llm.Message{
			"typed":         history,
			"round-tripped": roundTrip(t, history),
		} {
			t.Run(key+"/"+name, func(t *testing.T) {
				contents, err := messagesToContents(liftToolResultImages(ModelGemini38Flash, messages))
				assert.NoError(t, err)
				assert.Len(t, contents, 2)
				assert.Len(t, contents[1].Parts, 1)
				response := contents[1].Parts[0].FunctionResponse
				assert.Equal(t, map[string]any{key: "shot taken"}, response.Response)
				assert.Equal(t, []*genai.FunctionResponsePart{{InlineData: &genai.FunctionResponseBlob{
					MIMEType: "image/png",
					Data:     []byte("\x89PNG\r\n\x1a\n"),
				}}}, response.Parts)
			})
		}
	}
}

// Gemini 2.5 rejects images in a function response, and a model Dive has not
// verified may too, so the image moves to the user turn that follows the
// function responses, labelled with its call.
func TestToolResultImageLiftedForOlderModels(t *testing.T) {
	for _, model := range []string{ModelGemini25Flash, "gemini-tuned-unknown"} {
		history := screenshotTurn(false)
		for name, messages := range map[string][]*llm.Message{
			"typed":         history,
			"round-tripped": roundTrip(t, history),
		} {
			t.Run(model+"/"+name, func(t *testing.T) {
				contents, err := messagesToContents(liftToolResultImages(model, messages))
				assert.NoError(t, err)
				assert.Len(t, contents, 3)

				response := contents[1].Parts[0].FunctionResponse
				assert.Equal(t, map[string]any{
					"output": "shot taken\n\n(The image this call returned follows the tool results.)",
				}, response.Response)
				assert.Len(t, response.Parts, 0)

				assert.Equal(t, "user", contents[2].Role)
				assert.Len(t, contents[2].Parts, 2)
				assert.Equal(t, "The image from tool call call_1:", contents[2].Parts[0].Text)
				assert.Equal(t, &genai.Blob{MIMEType: "image/png", Data: []byte("\x89PNG\r\n\x1a\n")},
					contents[2].Parts[1].InlineData)
			})
		}
		// Only the request changes; the caller's history keeps the image.
		blocks := history[1].Content[0].(*llm.ToolResultContent).Content.([]*dive.ToolResultContent)
		assert.Len(t, blocks, 2)
	}
}

// Every catalogued 3.x text model was verified live to take images in a
// function response, and every 2.5 one to reject them.
func TestImagesInFunctionResponsesByGeneration(t *testing.T) {
	for _, model := range []string{
		ModelGemini38Flash, "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash",
		"gemini-3.5-flash-lite", "gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools",
		"gemini-3.1-flash-lite", "gemini-3.1-flash-lite-preview", "gemini-3-flash-preview",
	} {
		caps, ok := lookupCapabilities(model)
		assert.True(t, ok && caps.imagesInFunctionResponses, model)
	}
	for _, model := range []string{"gemini-2.5-pro", ModelGemini25Flash, "gemini-2.5-flash-lite"} {
		caps, _ := lookupCapabilities(model)
		assert.False(t, caps.imagesInFunctionResponses, model)
	}
}
