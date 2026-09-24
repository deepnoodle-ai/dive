// Package toolimagetest checks, against a live model, that an image a tool
// returns reaches the model. Each provider's opt-in integration test uses it.
//
// The check has to be one a blind model fails. A model shown "[image content
// omitted]" guesses, and asked about a square it says "red" often enough to
// pass a fixed picture, so the colour and the number of squares are picked at
// random for each scene.
package toolimagetest

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

// ToolCallID is the id of the screenshot call in Messages. It is nine
// alphanumerics, the only shape Mistral accepts.
const ToolCallID = "call00001"

var colours = []struct {
	name string
	rgba color.RGBA
}{
	{"green", color.RGBA{20, 170, 40, 255}},
	{"blue", color.RGBA{30, 60, 220, 255}},
	{"yellow", color.RGBA{240, 210, 20, 255}},
	{"purple", color.RGBA{130, 40, 170, 255}},
	{"orange", color.RGBA{250, 130, 10, 255}},
}

var numbers = []string{"zero", "one", "two", "three", "four", "five"}

// Scene is a screenshot of Count squares of one Colour on white.
type Scene struct {
	Colour string
	Count  int
	// PNG is the screenshot, base64-encoded.
	PNG string
}

// NewScene draws a scene with a random colour and 2 to 5 squares.
func NewScene(t testing.TB) Scene {
	t.Helper()
	pick := colours[rand.IntN(len(colours))]
	count := 2 + rand.IntN(4)
	img := image.NewRGBA(image.Rect(0, 0, 400, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x/80 < count && x%80 >= 15 && x%80 < 65 && y >= 55 && y < 105 {
				c = pick.rgba
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return Scene{Colour: pick.name, Count: count, PNG: base64.StdEncoding.EncodeToString(buf.Bytes())}
}

// Tool declares the screenshot tool the conversation calls.
func (s Scene) Tool() llm.Tool {
	return llm.NewToolDefinition().
		WithName("screenshot").
		WithDescription("Take a screenshot of the screen and see it.").
		WithSchema(&schema.Schema{Type: schema.Object})
}

// Messages is a conversation in which the model has called the screenshot
// tool and the tool has answered with the scene, ready for the model to say
// what it sees. With isError the result is marked as a failed call that still
// carries the screenshot, as a computer-use action that went wrong would.
func (s Scene) Messages(isError bool) []*llm.Message {
	text := `{"action":"screenshot","width":400,"height":160}`
	if isError {
		text = "The click failed: no element at that point. Screenshot of the screen as it is now:"
	}
	return []*llm.Message{
		llm.NewUserTextMessage("Take a screenshot, then tell me how many squares it shows and what colour they are, as a number and a colour word, nothing else."),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: ToolCallID, Name: "screenshot", Input: []byte(`{}`)},
		}},
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: ToolCallID,
			IsError:   isError,
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: text},
				{Type: dive.ToolResultContentTypeImage, Data: s.PNG, MimeType: "image/png"},
			},
		}),
	}
}

// Check fails the test unless reply names the scene's colour and count.
func (s Scene) Check(t testing.TB, reply string) {
	t.Helper()
	t.Logf("want %d %s, got %q", s.Count, s.Colour, reply)
	lower := strings.ToLower(reply)
	if !strings.Contains(lower, s.Colour) ||
		!(strings.Contains(lower, strconv.Itoa(s.Count)) || strings.Contains(lower, numbers[s.Count])) {
		t.Fatalf("the model did not see the screenshot of %d %s squares: %q", s.Count, s.Colour, reply)
	}
}

// Reply is every non-empty text block of a response, joined. Gemini can end
// its message with an empty text block that only carries a thought signature.
func Reply(response *llm.Response) string {
	var parts []string
	for _, c := range response.Content {
		if text, ok := c.(*llm.TextContent); ok && text.Text != "" {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, " ")
}
