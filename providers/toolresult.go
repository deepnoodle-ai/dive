package providers

import (
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// EmptyToolResultText stands in for a tool result that carries no renderable
// text (a tool that returned no output, or only empty text blocks). Providers
// substitute it rather than emitting an empty content block or an empty
// content array, both of which are rejected or ambiguous on some APIs, and
// neither of which tells the model the call actually produced nothing.
const EmptyToolResultText = "(no output)"

// IsEmptyToolResultContent reports whether tool result content carries
// nothing: nil (reachable when a caller supplies a nil ToolResult on resume)
// or an empty list of blocks, in either the typed in-memory shape or the
// generic shape it takes after a JSON round-trip. An empty string is not
// treated as empty, since a caller chose it explicitly.
func IsEmptyToolResultContent(content any) bool {
	switch v := content.(type) {
	case nil:
		return true
	case []*dive.ToolResultContent:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return false
	}
}

// ToolResultBlocks extracts typed tool result content blocks from a
// tool_result content block, handling both the in-memory shape
// ([]*dive.ToolResultContent) and the generic shape the same data takes after
// a JSON round-trip (session persistence, Message.Copy). Returns nil when the
// content is not block-shaped, in which case callers should fall back to
// their string/JSON rendering.
func ToolResultBlocks(c *llm.ToolResultContent) []*dive.ToolResultContent {
	if blocks, ok := c.Content.([]*dive.ToolResultContent); ok {
		if len(blocks) == 0 {
			return nil
		}
		return blocks
	}
	var blocks []*dive.ToolResultContent
	if err := c.DecodeContent(&blocks); err != nil || len(blocks) == 0 {
		return nil
	}
	// Guard against arbitrary JSON arrays decoding into zero-valued blocks:
	// every element must carry a known content block type. An absent type is
	// accepted when the element still carries a block payload, since providers
	// render an untyped block as text; that keeps a hand-built untyped block
	// rendering the same before and after a JSON round-trip.
	for _, b := range blocks {
		if b == nil {
			return nil
		}
		switch b.Type {
		case dive.ToolResultContentTypeText,
			dive.ToolResultContentTypeImage,
			dive.ToolResultContentTypeAudio:
		case "":
			if b.Text == "" && b.Data == "" {
				return nil
			}
		default:
			return nil
		}
	}
	return blocks
}

// ToolResultImageMediaType returns the media type a tool-result image block is
// sent with: its MimeType, or the type detected from its data when the tool
// left MimeType empty. It returns "" when the block carries no data or its
// type cannot be determined; providers then send a placeholder, since there is
// nothing the model could be shown.
func ToolResultImageMediaType(b *dive.ToolResultContent) string {
	if b == nil || b.Data == "" {
		return ""
	}
	if b.MimeType != "" {
		return b.MimeType
	}
	detected, err := llm.DetectImageType(b.Data)
	if err != nil {
		return ""
	}
	return string(detected)
}

// LiftToolResultImages returns messages with each tool-result image moved out
// of its tool result and into ordinary image content at the end of the same
// message, for APIs that cannot carry an image inside a tool result (Chat
// Completions tool messages are text-only; Gemini 2.5 rejects media in a
// function response). Every encoder already emits a message's tool results
// before its other content, so on the wire the images land immediately after
// the run of tool results, in a user turn:
//
//	assistant(tool_calls) → tool(text + pointer) … → user[label, image, …]
//
// The tool result keeps its text and gains a line saying where its image went,
// and each image is preceded by a label naming the tool call it came from, so
// the model can connect the two. An image with no data or an undetectable type
// stays in place, and the encoder renders its placeholder.
//
// Only the request is rewritten: messages are never modified in place, so the
// caller's history keeps each image in its tool result, where a provider that
// takes tool-result images natively finds it. When nothing needs to move,
// messages is returned as is.
func LiftToolResultImages(messages []*llm.Message) []*llm.Message {
	return liftToolResultImages(messages, false)
}

// LiftErrorToolResultImages is LiftToolResultImages for error results only,
// for APIs that take images in a tool result unless it is marked as an error:
// Anthropic answers 400 "all content must be type `text` if `is_error` is
// true". The error result keeps its text and its is_error flag.
func LiftErrorToolResultImages(messages []*llm.Message) []*llm.Message {
	return liftToolResultImages(messages, true)
}

func liftToolResultImages(messages []*llm.Message, errorsOnly bool) []*llm.Message {
	var out []*llm.Message
	for i, m := range messages {
		lifted, ok := liftMessageImages(m, errorsOnly)
		if !ok {
			if out != nil {
				out = append(out, m)
			}
			continue
		}
		if out == nil {
			out = append(make([]*llm.Message, 0, len(messages)), messages[:i]...)
		}
		out = append(out, lifted)
	}
	if out == nil {
		return messages
	}
	return out
}

// liftMessageImages returns a copy of m with its tool-result images lifted,
// or false when m has none to lift.
func liftMessageImages(m *llm.Message, errorsOnly bool) (*llm.Message, bool) {
	if m == nil {
		return nil, false
	}
	var content, images []llm.Content
	for _, c := range m.Content {
		result, ok := c.(*llm.ToolResultContent)
		if !ok || (errorsOnly && !result.IsError) {
			content = append(content, c)
			continue
		}
		var kept []*dive.ToolResultContent
		var lifted []*llm.ImageContent
		for _, b := range ToolResultBlocks(result) {
			mediaType := ""
			if b.Type == dive.ToolResultContentTypeImage {
				mediaType = ToolResultImageMediaType(b)
			}
			if mediaType == "" {
				kept = append(kept, b)
				continue
			}
			lifted = append(lifted, &llm.ImageContent{Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: mediaType,
				Data:      b.Data,
			}})
		}
		if len(lifted) == 0 {
			content = append(content, c)
			continue
		}
		pointer := "(The image this call returned follows the tool results.)"
		if len(lifted) > 1 {
			pointer = fmt.Sprintf("(The %d images this call returned follow the tool results.)", len(lifted))
		}
		kept = append(kept, &dive.ToolResultContent{Type: dive.ToolResultContentTypeText, Text: pointer})
		// Built field by field rather than copied: a round-tripped result
		// holds its original JSON, which DecodeContent would prefer over the
		// new Content.
		content = append(content, &llm.ToolResultContent{
			ToolUseID:    result.ToolUseID,
			ToolsetName:  result.ToolsetName,
			Content:      kept,
			IsError:      result.IsError,
			CacheControl: result.CacheControl,
		})
		for n, image := range lifted {
			label := fmt.Sprintf("The image from tool call %s:", result.ToolUseID)
			if len(lifted) > 1 {
				label = fmt.Sprintf("Image %d of %d from tool call %s:", n+1, len(lifted), result.ToolUseID)
			}
			images = append(images, &llm.TextContent{Text: label}, image)
		}
	}
	if len(images) == 0 {
		return nil, false
	}
	return &llm.Message{
		ID:      m.ID,
		Role:    m.Role,
		Content: append(content, images...),
		Effort:  m.Effort,
	}, true
}
