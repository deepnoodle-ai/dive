package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/deepnoodle-ai/dive/llm"
)

// Dropping a file onto a terminal makes it insert that file's path as if it had
// been pasted. The input watches for those insertions, resolves them to local
// files, and replaces each one with a placeholder such as "[Image #1]". The file
// itself is read when the message is sent, at which point images, PDFs, and
// videos become native content blocks and text files are inlined into the
// prompt.
//
// Unlike an @reference, a dropped file may live anywhere on disk: dragging it
// onto the input is the user pointing at that exact file, so the workspace
// boundary that guards @references doesn't apply.

const (
	// maxAttachmentBytes caps a dropped file that is sent as a native content
	// block. Providers reject requests well below this, but the limit keeps a
	// stray multi-gigabyte drop from being base64-encoded into memory.
	maxAttachmentBytes = 32 << 20

	// maxInlineTextBytes caps a dropped file with no native media type, which
	// is inlined into the prompt as text.
	maxInlineTextBytes = 256 << 10
)

// mediaKind describes how an attachment is sent to the model.
type mediaKind string

const (
	mediaKindImage    mediaKind = "image"
	mediaKindDocument mediaKind = "document"
	mediaKindVideo    mediaKind = "video"
	mediaKindFile     mediaKind = "file"
)

// label returns the form used in input placeholders, e.g. "[Image #1]".
func (k mediaKind) label() string {
	switch k {
	case mediaKindImage:
		return "Image"
	case mediaKindDocument:
		return "Document"
	case mediaKindVideo:
		return "Video"
	default:
		return "File"
	}
}

// attachment is a file the user dropped onto the input. It is referenced from
// the input text by Placeholder until the message is sent; the contents are not
// read until then, so a large drop never blocks the event loop.
type attachment struct {
	Placeholder string
	Path        string
	Name        string
	Kind        mediaKind
	Size        int64
}

// mediaKindForExt reports how a file extension is sent to the model, without
// reading the file. Returns false for extensions with no native handling.
func mediaKindForExt(ext string) (mediaKind, bool) {
	ext = strings.ToLower(ext)
	if mediaType, ok := fileMediaTypes[ext]; ok {
		switch {
		case strings.HasPrefix(mediaType, "image/"):
			return mediaKindImage, true
		case strings.HasPrefix(mediaType, "video/"):
			return mediaKindVideo, true
		default:
			return mediaKindDocument, true
		}
	}
	if textDocExtensions[ext] {
		return mediaKindDocument, true
	}
	return "", false
}

// mediaContentFor builds the native content block for a file whose extension has
// known media handling, reporting false for anything that should be sent as
// plain text instead. Shared by @references and dropped attachments.
func mediaContentFor(filename string, data []byte) (llm.Content, mediaKind, bool) {
	ext := strings.ToLower(filepath.Ext(filename))

	if mediaType, ok := fileMediaTypes[ext]; ok {
		source := &llm.ContentSource{
			Type:      llm.ContentSourceTypeBase64,
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		}
		if strings.HasPrefix(mediaType, "image/") {
			return &llm.ImageContent{Source: source}, mediaKindImage, true
		}
		kind := mediaKindDocument
		if strings.HasPrefix(mediaType, "video/") {
			kind = mediaKindVideo
		}
		return &llm.DocumentContent{Source: source, Title: filename}, kind, true
	}

	if textDocExtensions[ext] {
		return &llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeText,
				MediaType: "text/plain",
				Data:      string(data),
			},
			Title: filename,
		}, mediaKindDocument, true
	}

	return nil, "", false
}

// pathToken is a whitespace-delimited token from a run of inserted text, along
// with its byte range within that run.
type pathToken struct {
	start int
	end   int
	value string
}

// scanPathTokens splits text the way a shell would: on unquoted whitespace,
// honoring single quotes, double quotes, and backslash escapes. Terminals use
// exactly these forms when they insert the path of a dropped file, so a path
// containing spaces survives the round trip.
func scanPathTokens(text string) []pathToken {
	var (
		tokens []pathToken
		value  strings.Builder
		start  = -1
		quote  = byte(0)
	)
	flush := func(end int) {
		if start >= 0 {
			tokens = append(tokens, pathToken{start: start, end: end, value: value.String()})
			value.Reset()
			start = -1
		}
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote == 0 && (c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			flush(i)
		case quote == 0 && (c == '\'' || c == '"'):
			if start < 0 {
				start = i
			}
			quote = c
		case quote != 0 && c == quote:
			quote = 0
		// A backslash escapes only the characters a terminal actually escapes
		// when it inserts a path, so a Windows separator stays literal.
		case c == '\\' && quote != '\'' && i+1 < len(text) && isEscapable(text[i+1]):
			if start < 0 {
				start = i
			}
			i++
			value.WriteByte(text[i])
		default:
			if start < 0 {
				start = i
			}
			value.WriteByte(c)
		}
	}
	flush(len(text))
	return tokens
}

// isEscapable reports whether a backslash before c should be read as an escape.
func isEscapable(c byte) bool {
	switch c {
	case ' ', '\t', '\'', '"', '\\', '(', ')', '[', ']', '&', ';', '$', '`', '!', '*', '?', '#':
		return true
	}
	return false
}

// resolveDroppedPath turns an inserted token into an absolute filesystem path,
// reporting false when it doesn't look like one. Terminals emit either a plain
// (possibly escaped) path or a file:// URL when a file is dropped; both are
// accepted, as is a leading ~. Relative tokens are rejected so an ordinary word
// in a pasted sentence is never mistaken for an attachment.
func resolveDroppedPath(token string) (string, bool) {
	path := token

	if strings.HasPrefix(strings.ToLower(path), "file://") {
		u, err := url.Parse(path)
		if err != nil || (u.Host != "" && u.Host != "localhost") {
			return "", false
		}
		decoded, err := url.PathUnescape(u.Path)
		if err != nil {
			return "", false
		}
		path = decoded
	}

	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		switch {
		case path == "~":
			path = home
		case path[1] == '/' || path[1] == '\\':
			path = filepath.Join(home, path[2:])
		}
	}

	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	return filepath.Clean(path), true
}

// isFileDrop reports whether every token in a run of inserted text resolves to
// something that exists on disk, which is exactly what a terminal inserts for a
// file drop. Requiring the whole run keeps a pasted log or stack trace that
// merely mentions absolute paths from being turned into attachments.
func isFileDrop(tokens []pathToken) ([]string, bool) {
	if len(tokens) == 0 {
		return nil, false
	}
	paths := make([]string, 0, len(tokens))
	for _, token := range tokens {
		path, ok := resolveDroppedPath(token.value)
		if !ok {
			return nil, false
		}
		if _, err := os.Lstat(path); err != nil {
			return nil, false
		}
		paths = append(paths, path)
	}
	return paths, true
}

// classifyAttachment decides how a dropped file will be sent, returning an error
// phrase describing why it can't be attached at all.
func classifyAttachment(path string) (mediaKind, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, errors.New("it cannot be read")
	}
	if info.IsDir() {
		return "", 0, errors.New("it is a directory")
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("it is not a regular file")
	}

	if kind, ok := mediaKindForExt(filepath.Ext(path)); ok {
		if info.Size() > maxAttachmentBytes {
			return "", 0, fmt.Errorf("it is larger than %s", formatBytes(maxAttachmentBytes))
		}
		return kind, info.Size(), nil
	}

	// No native media type. Attach it only if it reads as text, in which case
	// it is inlined into the prompt exactly like an @reference.
	if !looksLikeText(path) {
		return "", 0, errors.New("its file type is not supported")
	}
	if info.Size() > maxInlineTextBytes {
		return "", 0, fmt.Errorf("it is larger than %s", formatBytes(maxInlineTextBytes))
	}
	return mediaKindFile, info.Size(), nil
}

// looksLikeText reports whether the head of a file decodes as UTF-8 with no NUL
// bytes, the same cheap test editors use to avoid opening binaries.
func looksLikeText(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if n == 0 {
		// An empty file counts as text; anything else means it can't be read.
		return errors.Is(err, io.EOF)
	}
	buf = buf[:n]
	if bytes.IndexByte(buf, 0) >= 0 {
		return false
	}
	// The read may have stopped mid-rune, so allow trimming a truncated tail.
	for i := 0; i < 4; i++ {
		if utf8.Valid(buf) {
			return true
		}
		if len(buf) == 0 {
			return false
		}
		buf = buf[:len(buf)-1]
	}
	return false
}

// insertedRange locates the single contiguous run of text present in next but
// not in prev, returning its byte range in next. It reports false unless more
// than one character was inserted, which is what separates a paste (or a
// terminal file drop) from ordinary typing.
func insertedRange(prev, next string) (int, int, bool) {
	if len(next) <= len(prev) {
		return 0, 0, false
	}
	prevRunes := []rune(prev)
	nextRunes := []rune(next)
	if len(nextRunes)-len(prevRunes) < 2 {
		return 0, 0, false
	}

	head := 0
	for head < len(prevRunes) && prevRunes[head] == nextRunes[head] {
		head++
	}
	tail := 0
	for tail < len(prevRunes)-head && prevRunes[len(prevRunes)-1-tail] == nextRunes[len(nextRunes)-1-tail] {
		tail++
	}

	start := len(string(nextRunes[:head]))
	end := len(next) - len(string(nextRunes[len(nextRunes)-tail:]))
	return start, end, true
}

// expandAttachments reads each attachment and returns the message text with
// text-only files inlined, plus the native content blocks for images, documents,
// and videos. Placeholders for native blocks become a short "[image: path]"
// marker so the model can tell where each attachment belongs. Runs off the event
// loop, so it must not touch App state.
func expandAttachments(text string, attachments []attachment) (string, []llm.Content, error) {
	var (
		blocks   []llm.Content
		firstErr error
	)
	for _, att := range attachments {
		data, err := os.ReadFile(att.Path)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("cannot read %s: %w", att.Name, err)
			}
			text = strings.ReplaceAll(text, att.Placeholder, att.Path)
			continue
		}
		if content, kind, ok := mediaContentFor(att.Name, data); ok {
			blocks = append(blocks, content)
			text = strings.ReplaceAll(text, att.Placeholder, fmt.Sprintf("[%s: %s]", kind, att.Path))
			continue
		}
		text = strings.ReplaceAll(text, att.Placeholder,
			fmt.Sprintf("\n<file path=%q>\n%s\n</file>\n", att.Path, string(data)))
	}
	return text, blocks, firstErr
}
