package providers

import (
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
