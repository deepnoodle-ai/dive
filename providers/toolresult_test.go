package providers

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestToolResultBlocksTyped(t *testing.T) {
	blocks := ToolResultBlocks(&llm.ToolResultContent{
		ToolUseID: "call_1",
		Content: []*dive.ToolResultContent{
			{Type: dive.ToolResultContentTypeText, Text: "hello"},
			{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
		},
	})
	assert.Len(t, blocks, 2)
	assert.Equal(t, "hello", blocks[0].Text)
	assert.Equal(t, dive.ToolResultContentTypeImage, blocks[1].Type)
}

func TestToolResultBlocksJSONRoundTrip(t *testing.T) {
	original := &llm.ToolResultContent{
		ToolUseID: "call_1",
		Content: []*dive.ToolResultContent{
			{Type: dive.ToolResultContentTypeText, Text: "hello"},
		},
	}
	body, err := json.Marshal(original)
	assert.NoError(t, err)
	var replayed llm.ToolResultContent
	assert.NoError(t, json.Unmarshal(body, &replayed))

	blocks := ToolResultBlocks(&replayed)
	assert.Len(t, blocks, 1)
	assert.Equal(t, "hello", blocks[0].Text)
}

// TestToolResultBlocksUntypedText verifies a block with no explicit type but
// real text survives the round-trip guard, matching how the typed in-memory
// path treats it.
func TestToolResultBlocksUntypedText(t *testing.T) {
	blocks := ToolResultBlocks(&llm.ToolResultContent{
		Content: []any{map[string]any{"text": "hello"}},
	})
	assert.Len(t, blocks, 1)
	assert.Equal(t, "hello", blocks[0].Text)
}

func TestToolResultBlocksNonBlockContent(t *testing.T) {
	assert.Nil(t, ToolResultBlocks(&llm.ToolResultContent{Content: "plain string"}))
	assert.Nil(t, ToolResultBlocks(&llm.ToolResultContent{Content: nil}))
	assert.Nil(t, ToolResultBlocks(&llm.ToolResultContent{Content: []*dive.ToolResultContent{}}))
	assert.Nil(t, ToolResultBlocks(&llm.ToolResultContent{Content: []any{map[string]any{"foo": "bar"}}}))
	assert.Nil(t, ToolResultBlocks(&llm.ToolResultContent{Content: []any{1, 2, 3}}))
}
