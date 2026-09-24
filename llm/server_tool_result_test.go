package llm

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

const webFetchResultBlock = `{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_2","content":{"type":"web_fetch_result","url":"https://example.com","content":{"type":"document","source":{"type":"text","media_type":"text/plain","data":"Example Domain"},"title":"Example","citations":{"enabled":true}},"retrieved_at":"2026-09-24T10:30:00Z"}}`

func TestUnmarshalContentServerToolResult(t *testing.T) {
	content, err := UnmarshalContent([]byte(webFetchResultBlock))
	assert.NoError(t, err)
	result, ok := content.(*ServerToolResultContent)
	assert.True(t, ok)
	assert.Equal(t, ContentTypeWebFetchToolResult, result.Type())
	assert.Equal(t, "srvtoolu_2", result.ToolUseID)
	assert.Contains(t, string(result.Content), `"retrieved_at":"2026-09-24T10:30:00Z"`)

	// The block is written back exactly as decoded.
	encoded, err := json.Marshal(result)
	assert.NoError(t, err)
	assert.Equal(t, webFetchResultBlock, string(encoded))

	search, err := UnmarshalContent([]byte(`{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_3","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"get_weather"}]}}`))
	assert.NoError(t, err)
	assert.Equal(t, ContentTypeToolSearchToolResult, search.Type())

	// A block with no tool_use_id, or whose type is not a tool result, is
	// still unsupported.
	_, err = UnmarshalContent([]byte(`{"type":"future_tool_result"}`))
	assert.Error(t, err)
	_, err = UnmarshalContent([]byte(`{"type":"future_block","tool_use_id":"x"}`))
	assert.Error(t, err)
}

func TestServerToolResultContentBuiltByHand(t *testing.T) {
	encoded, err := json.Marshal(&ServerToolResultContent{
		BlockType: ContentTypeWebFetchToolResult,
		ToolUseID: "srvtoolu_2",
		Content:   json.RawMessage(`{"type":"web_fetch_tool_error","error_code":"url_not_accessible"}`),
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_2","content":{"type":"web_fetch_tool_error","error_code":"url_not_accessible"}}`, string(encoded))
}

// A stored session keeps the web fetch result next to its call.
func TestMessageRoundTripKeepsServerToolResult(t *testing.T) {
	var message Message
	assert.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":[`+
		`{"type":"server_tool_use","id":"srvtoolu_2","name":"web_fetch","input":{"url":"https://example.com"}},`+
		webFetchResultBlock+`,{"type":"text","text":"Done."}]}`), &message))
	assert.Len(t, message.Content, 3)

	encoded, err := json.Marshal(&message)
	assert.NoError(t, err)
	var decoded Message
	assert.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Len(t, decoded.Content, 3)
	result, ok := decoded.Content[1].(*ServerToolResultContent)
	assert.True(t, ok)
	assert.Equal(t, "srvtoolu_2", result.ToolUseID)
}

func accumulate(t *testing.T, events []string) *Response {
	t.Helper()
	acc := NewResponseAccumulator()
	for _, raw := range events {
		var event Event
		assert.NoError(t, json.Unmarshal([]byte(raw), &event))
		assert.NoError(t, acc.AddEvent(&event))
	}
	return acc.Response()
}

// A streamed web fetch keeps its call and its result, so the next request
// holds both.
func TestResponseAccumulatorKeepsWebFetchResult(t *testing.T) {
	response := accumulate(t, []string{
		`{"type":"message_start","message":{"id":"msg_1","role":"assistant","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_2","name":"web_fetch","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"url\":\"https://example.com\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":` + webFetchResultBlock + `}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":"Done."}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_stop"}`,
	})
	assert.Len(t, response.Content, 3)
	use, ok := response.Content[0].(*ServerToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "https://example.com", use.Input["url"])
	result, ok := response.Content[1].(*ServerToolResultContent)
	assert.True(t, ok)
	assert.Equal(t, "srvtoolu_2", result.ToolUseID)
}

// When a server tool result cannot be decoded and is skipped, its call is
// dropped too: a call without its result is rejected on the next request.
func TestResponseAccumulatorDropsCallWhoseResultIsSkipped(t *testing.T) {
	response := accumulate(t, []string{
		`{"type":"message_start","message":{"id":"msg_1","role":"assistant","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_9","name":"future_tool","input":{}}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"future_output","tool_use_id":"srvtoolu_9"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{}}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_start","index":3,"content_block":{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[]}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"content_block_start","index":4,"content_block":{"type":"text","text":"Done."}}`,
		`{"type":"content_block_stop","index":4}`,
		`{"type":"message_stop"}`,
	})
	assert.Len(t, response.Content, 3)
	use, ok := response.Content[0].(*ServerToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "srvtoolu_1", use.ID)
	_, ok = response.Content[1].(*WebSearchToolResultContent)
	assert.True(t, ok)
}

// A paused turn (pause_turn) ends with a server tool call that has no result
// yet. It is kept, since sending it back is how the turn resumes.
func TestResponseAccumulatorKeepsPausedServerToolCall(t *testing.T) {
	response := accumulate(t, []string{
		`{"type":"message_start","message":{"id":"msg_1","role":"assistant","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{}}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"pause_turn"}}`,
		`{"type":"message_stop"}`,
	})
	assert.Len(t, response.Content, 1)
	_, ok := response.Content[0].(*ServerToolUseContent)
	assert.True(t, ok)
}

// A non-streaming response skips a block type Dive cannot decode, as a
// streamed one does, and drops the server tool call it answers.
func TestResponseUnmarshalSkipsUnsupportedBlocks(t *testing.T) {
	var response Response
	assert.NoError(t, json.Unmarshal([]byte(`{"id":"msg_1","role":"assistant","content":[`+
		`{"type":"server_tool_use","id":"srvtoolu_9","name":"future_tool","input":{}},`+
		`{"type":"future_output","tool_use_id":"srvtoolu_9"},`+
		`{"type":"server_tool_use","id":"srvtoolu_2","name":"web_fetch","input":{"url":"https://example.com"}},`+
		webFetchResultBlock+`,`+
		`{"type":"future_block"},`+
		`{"type":"text","text":"Done."}]}`), &response))
	assert.Len(t, response.Content, 3)
	use, ok := response.Content[0].(*ServerToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "srvtoolu_2", use.ID)
	_, ok = response.Content[1].(*ServerToolResultContent)
	assert.True(t, ok)
	_, ok = response.Content[2].(*TextContent)
	assert.True(t, ok)
}

// An event holding a server tool block survives an encode/decode round trip,
// for example when a stream is relayed or recorded.
func TestEventContentBlockRoundTrip(t *testing.T) {
	raw := `{"type":"content_block_start","index":1,"content_block":` + webFetchResultBlock + `}`
	var event Event
	assert.NoError(t, json.Unmarshal([]byte(raw), &event))
	encoded, err := json.Marshal(&event)
	assert.NoError(t, err)
	assert.Contains(t, string(encoded), webFetchResultBlock)

	var decoded Event
	assert.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, string(event.ContentBlock.raw), string(decoded.ContentBlock.raw))

	// A block without kept JSON encodes its fields as before.
	encoded, err = json.Marshal(&EventContentBlock{Type: ContentTypeText, Text: "hi"})
	assert.NoError(t, err)
	assert.Equal(t, `{"type":"text","text":"hi"}`, string(encoded))
}
