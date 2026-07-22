package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

// These tests cover terminal drag and drop: a dropped file arrives as a pasted
// run of text holding its path, which the input turns into an attachment.

// simulateInput mimics an edit from the input widget, which writes the new
// value into the bound field before firing the change hook.
func simulateInput(a *App, value string) {
	a.inputText = value
	a.handleInputChange(value)
}

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	assert.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

// pngBytes is a 1x1 PNG. Only the header matters here; nothing decodes it.
var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func TestScanPathTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"plain", "/tmp/a.png", []string{"/tmp/a.png"}},
		{"trailing space", "/tmp/a.png ", []string{"/tmp/a.png"}},
		{"multiple", "/tmp/a.png /tmp/b.pdf", []string{"/tmp/a.png", "/tmp/b.pdf"}},
		{"newline separated", "/tmp/a.png\n/tmp/b.pdf", []string{"/tmp/a.png", "/tmp/b.pdf"}},
		// macOS Terminal and iTerm2 backslash-escape spaces on drop.
		{"escaped spaces", `/tmp/my\ shot.png`, []string{"/tmp/my shot.png"}},
		{"single quoted", `'/tmp/my shot.png'`, []string{"/tmp/my shot.png"}},
		{"double quoted", `"/tmp/my shot.png"`, []string{"/tmp/my shot.png"}},
		{"sentence", "look at /tmp/a.png please", []string{"look", "at", "/tmp/a.png", "please"}},
		{"empty", "   ", nil},
		// A backslash is a separator, not an escape, unless what follows is
		// something a terminal would actually escape.
		{"windows separator", `C:\Users\me\a.png`, []string{`C:\Users\me\a.png`}},
		{"escaped backslash", `/tmp/a\\b.png`, []string{`/tmp/a\b.png`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := scanPathTokens(tt.input)
			got := make([]string, 0, len(tokens))
			for _, token := range tokens {
				got = append(got, token.value)
				// Ranges must index back into the original text.
				assert.True(t, token.start >= 0 && token.end <= len(tt.input) && token.start < token.end)
			}
			if len(tt.want) == 0 {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveDroppedPath(t *testing.T) {
	home, err := os.UserHomeDir()
	assert.NoError(t, err)

	tests := []struct {
		name  string
		token string
		want  string
		ok    bool
	}{
		{"absolute", "/tmp/a.png", "/tmp/a.png", true},
		{"cleaned", "/tmp//sub/../a.png", "/tmp/a.png", true},
		{"file url", "file:///tmp/a.png", "/tmp/a.png", true},
		{"file url percent encoded", "file:///tmp/my%20shot.png", "/tmp/my shot.png", true},
		{"file url localhost", "file://localhost/tmp/a.png", "/tmp/a.png", true},
		{"home relative", "~/a.png", filepath.Join(home, "a.png"), true},
		// A bare word in a pasted sentence must never look like a path.
		{"relative rejected", "a.png", "", false},
		{"word rejected", "please", "", false},
		{"remote url rejected", "file://example.com/a.png", "", false},
		{"empty rejected", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveDroppedPath(tt.token)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestInsertedRange(t *testing.T) {
	// Typing one character at a time is never treated as an insertion.
	_, _, ok := insertedRange("hello", "hello!")
	assert.False(t, ok, "a single typed character is not a paste")

	// Deletions and no-ops are ignored.
	_, _, ok = insertedRange("hello", "hell")
	assert.False(t, ok)
	_, _, ok = insertedRange("hello", "hello")
	assert.False(t, ok)

	// A paste into an empty input covers the whole value.
	start, end, ok := insertedRange("", "/tmp/a.png")
	assert.True(t, ok)
	assert.Equal(t, "/tmp/a.png", "/tmp/a.png"[start:end])

	// A paste at the end of a draft isolates just the new run.
	next := "look at /tmp/a.png"
	start, end, ok = insertedRange("look at ", next)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/a.png", next[start:end])

	// A paste in the middle isolates just the new run.
	next = "look at /tmp/a.png now"
	start, end, ok = insertedRange("look at  now", next)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/a.png", next[start:end])

	// Multi-byte text on either side must not split a rune.
	next = "héllo /tmp/a.png wörld"
	start, end, ok = insertedRange("héllo  wörld", next)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/a.png", next[start:end])
}

func TestHandleInputChange_CapturesDroppedImage(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "shot.png", pngBytes)

	// A drop after some typed text: the terminal pastes the path.
	a.setInputText("what is this ")
	simulateInput(a, "what is this "+path)

	assert.Equal(t, "what is this [Image #1] ", a.inputText)
	assert.Len(t, a.attachments, 1)
	assert.Equal(t, path, a.attachments[0].Path)
	assert.Equal(t, "shot.png", a.attachments[0].Name)
	assert.Equal(t, mediaKindImage, a.attachments[0].Kind)
	assert.Equal(t, int64(len(pngBytes)), a.attachments[0].Size)
}

func TestHandleInputChange_CapturesEscapedAndQuotedPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my shot.png")
	assert.NoError(t, os.WriteFile(path, pngBytes, 0o644))

	for _, dropped := range []string{
		strings.ReplaceAll(path, " ", `\ `),              // macOS Terminal / iTerm2
		`'` + path + `'`,                                 // shells that quote instead
		"file://" + strings.ReplaceAll(path, " ", "%20"), // file URL
	} {
		a := newTestApp()
		simulateInput(a, dropped)
		assert.Equal(t, "[Image #1] ", a.inputText, dropped)
		assert.Len(t, a.attachments, 1, dropped)
		assert.Equal(t, path, a.attachments[0].Path, dropped)
	}
}

func TestHandleInputChange_MultipleFilesNumberedInOrder(t *testing.T) {
	a := newTestApp()
	dir := t.TempDir()
	image := filepath.Join(dir, "shot.png")
	doc := filepath.Join(dir, "paper.pdf")
	assert.NoError(t, os.WriteFile(image, pngBytes, 0o644))
	assert.NoError(t, os.WriteFile(doc, []byte("%PDF-1.4"), 0o644))

	simulateInput(a, image+" "+doc+" ")

	assert.Equal(t, "[Image #1] [Document #2] ", a.inputText)
	assert.Len(t, a.attachments, 2)
	assert.Equal(t, mediaKindImage, a.attachments[0].Kind)
	assert.Equal(t, mediaKindDocument, a.attachments[1].Kind)
}

func TestHandleInputChange_NormalizesTerminalWhitespace(t *testing.T) {
	path := writeTempFile(t, "shot.png", pngBytes)

	// Terminals pad a drop with their own whitespace. Whatever they add, the
	// placeholder ends up inline with the text and separated by single spaces
	// rather than stranded on a line of its own.
	for _, dropped := range []string{
		"\n" + path,        // a leading newline
		path + " ",         // a trailing space
		"\n" + path + "\n", // both
		"  " + path + "  ",
	} {
		a := newTestApp()
		a.setInputText("what is this?")
		simulateInput(a, "what is this?"+dropped)
		assert.Equal(t, "what is this? [Image #1] ", a.inputText, dropped)
	}

	// A newline the user typed themselves is not part of the drop, so it stays.
	a := newTestApp()
	a.setInputText("what is this?\n")
	simulateInput(a, "what is this?\n"+path)
	assert.Equal(t, "what is this?\n[Image #1] ", a.inputText)
}

func TestHandleInputChange_TypingIsNotADrop(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "shot.png", pngBytes)

	// Typing the same path character by character leaves it as text: only a
	// contiguous multi-character insertion is treated as a paste.
	for i := 1; i <= len(path); i++ {
		simulateInput(a, path[:i])
	}
	assert.Equal(t, path, a.inputText)
	assert.Empty(t, a.attachments)
}

func TestHandleInputChange_PastedProseIsNotADrop(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "shot.png", pngBytes)

	// A pasted log or stack trace mentioning a real path must not become an
	// attachment: only a run made up entirely of paths counts as a drop.
	pasted := "panic: boom\n\tat " + path + ":42"
	simulateInput(a, pasted)

	assert.Equal(t, pasted, a.inputText)
	assert.Empty(t, a.attachments)
}

func TestHandleInputChange_UnsupportedBinaryIsNotAttached(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "archive.bin", []byte{0x00, 0x01, 0x02, 0x00})

	simulateInput(a, path)

	assert.Equal(t, path, a.inputText, "the path stays in the message as text")
	assert.Empty(t, a.attachments)
}

func TestHandleInputChange_TextFileIsAttached(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "notes.md", []byte("# Notes\n"))

	simulateInput(a, path)

	assert.Equal(t, "[File #1] ", a.inputText)
	assert.Len(t, a.attachments, 1)
	assert.Equal(t, mediaKindFile, a.attachments[0].Kind)
}

func TestPruneAttachments_DropsDeletedPlaceholders(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "shot.png", pngBytes)

	simulateInput(a, path)
	assert.Len(t, a.attachments, 1)

	// Deleting the placeholder from the draft releases the attachment, and the
	// numbering restarts so the next drop is "#1" again.
	simulateInput(a, "")
	assert.Empty(t, a.attachments)
	assert.Equal(t, 0, a.attachSeq)
}

func TestTakeAttachments(t *testing.T) {
	a := newTestApp()
	dir := t.TempDir()
	kept := filepath.Join(dir, "kept.png")
	gone := filepath.Join(dir, "gone.png")
	assert.NoError(t, os.WriteFile(kept, pngBytes, 0o644))
	assert.NoError(t, os.WriteFile(gone, pngBytes, 0o644))

	simulateInput(a, kept+" "+gone)
	assert.Len(t, a.attachments, 2)

	// Only the attachments the submitted text still refers to are sent.
	taken := a.takeAttachments("[Image #1]")
	assert.Len(t, taken, 1)
	assert.Equal(t, kept, taken[0].Path)
	assert.Empty(t, a.attachments)
	assert.Equal(t, 0, a.attachSeq)
}

func TestSubmitInput_CapturesADropDeliveredWithEnter(t *testing.T) {
	a := newTestApp()
	path := writeTempFile(t, "shot.png", pngBytes)

	// The input hands submitInput its own value. If the drop and Enter arrive
	// in one batch the change hook hasn't run yet, so submit must fold it in
	// rather than sending a raw path with no attachment. processing keeps the
	// agent out of it; the draft is what's under test.
	a.processing = true
	a.submitInput(path)

	assert.Equal(t, "[Image #1] ", a.inputText)
	assert.Len(t, a.attachments, 1)
	assert.Equal(t, path, a.attachments[0].Path)
}

func TestExpandAttachments_Image(t *testing.T) {
	path := writeTempFile(t, "shot.png", pngBytes)
	attachments := []attachment{{
		Placeholder: "[Image #1]",
		Path:        path,
		Name:        "shot.png",
		Kind:        mediaKindImage,
	}}

	text, blocks, err := expandAttachments("what is this [Image #1]", attachments)
	assert.NoError(t, err)
	assert.Equal(t, "what is this [image: "+path+"]", text)
	assert.Len(t, blocks, 1)

	image, ok := blocks[0].(*llm.ImageContent)
	assert.True(t, ok, "expected an image content block")
	assert.Equal(t, "image/png", image.Source.MediaType)
	assert.Equal(t, llm.ContentSourceTypeBase64, image.Source.Type)
	assert.NotEmpty(t, image.Source.Data)
}

func TestExpandAttachments_TextFileIsInlined(t *testing.T) {
	path := writeTempFile(t, "notes.md", []byte("# Notes"))
	attachments := []attachment{{
		Placeholder: "[File #1]",
		Path:        path,
		Name:        "notes.md",
		Kind:        mediaKindFile,
	}}

	text, blocks, err := expandAttachments("summarize [File #1]", attachments)
	assert.NoError(t, err)
	assert.Empty(t, blocks, "text files are inlined rather than sent as blocks")
	assert.Contains(t, text, `<file path="`+path+`">`)
	assert.Contains(t, text, "# Notes")
}

func TestExpandAttachments_MissingFileReportsError(t *testing.T) {
	attachments := []attachment{{
		Placeholder: "[Image #1]",
		Path:        filepath.Join(t.TempDir(), "deleted.png"),
		Name:        "deleted.png",
		Kind:        mediaKindImage,
	}}

	// A file removed between the drop and the send leaves the path in the text
	// rather than a dangling placeholder.
	text, blocks, err := expandAttachments("look [Image #1]", attachments)
	assert.Error(t, err)
	assert.Empty(t, blocks)
	assert.Contains(t, text, "deleted.png")
	assert.NotContains(t, text, "[Image #1]")
}

func TestAttachmentsView(t *testing.T) {
	a := newTestApp()
	assert.Nil(t, a.attachmentsView(), "no view when nothing is attached")

	path := writeTempFile(t, "shot.png", pngBytes)
	simulateInput(a, path)

	// The name and size, but not the placeholder, which is already in the input.
	rendered := tui.Sprint(a.attachmentsView())
	assert.Contains(t, rendered, "shot.png")
	assert.Contains(t, rendered, formatBytes(len(pngBytes)))
	assert.NotContains(t, rendered, "[Image #1]")
}

func TestLooksLikeText(t *testing.T) {
	assert.True(t, looksLikeText(writeTempFile(t, "a.txt", []byte("hello"))))
	assert.True(t, looksLikeText(writeTempFile(t, "empty.txt", nil)))
	assert.True(t, looksLikeText(writeTempFile(t, "utf8.txt", []byte("héllo wörld"))))
	assert.False(t, looksLikeText(writeTempFile(t, "bin", []byte{0x7f, 0x45, 0x00, 0x01})))
	assert.False(t, looksLikeText(filepath.Join(t.TempDir(), "missing")))
}
