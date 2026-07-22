package providers

import (
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

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
	// every element must carry a known content block type.
	for _, b := range blocks {
		if b == nil {
			return nil
		}
		switch b.Type {
		case dive.ToolResultContentTypeText,
			dive.ToolResultContentTypeImage,
			dive.ToolResultContentTypeAudio:
		default:
			return nil
		}
	}
	return blocks
}
