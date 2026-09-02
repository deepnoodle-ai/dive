package openai

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

// unmarshalWebSearchCall builds the SDK value from wire JSON, since the action
// union is populated by the SDK's own decoder rather than by field assignment.
func unmarshalWebSearchCall(t *testing.T, raw string) responses.ResponseFunctionWebSearch {
	t.Helper()
	var call responses.ResponseFunctionWebSearch
	assert.NoError(t, json.Unmarshal([]byte(raw), &call))
	return call
}

// The sources arrive only when the request asked for
// web_search_call.action.sources, which costs tokens; decoding used to keep
// nothing but the call ID, so that spend bought the caller nothing readable.
func TestDecodeWebSearchCallKeepsAction(t *testing.T) {
	call := unmarshalWebSearchCall(t, `{
		"id": "ws_1",
		"type": "web_search_call",
		"status": "completed",
		"action": {
			"type": "search",
			"query": "Meta Connect 2025 announcements",
			"sources": [
				{"type": "url", "url": "https://example.com/a"},
				{"type": "url", "url": "https://example.com/b"}
			]
		}
	}`)

	content, err := decodeWebSearchCallContent(call)
	assert.NoError(t, err)
	assert.Equal(t, len(content), 1)

	use, ok := content[0].(*llm.ServerToolUseContent)
	assert.True(t, ok, "decoded to %T", content[0])
	assert.Equal(t, use.ID, "ws_1")
	assert.Equal(t, use.Name, "web_search_call")
	assert.Equal(t, use.Input["type"], "search")
	assert.Equal(t, use.Input["query"], "Meta Connect 2025 announcements")

	sources, ok := use.Input["sources"].([]any)
	assert.True(t, ok, "sources decoded to %T", use.Input["sources"])
	assert.Equal(t, len(sources), 2)
	assert.Equal(t, sources[0], map[string]any{"type": "url", "url": "https://example.com/a"})
}

// The hits behind a search arrive in a top-level results array that the SDK
// struct has no field for, so they are read back out of the raw payload.
func TestDecodeWebSearchCallKeepsResults(t *testing.T) {
	call := unmarshalWebSearchCall(t, `{
		"id": "ws_5",
		"type": "web_search_call",
		"status": "completed",
		"action": {"type": "search", "query": "Meta Connect 2025 announced"},
		"results": [
			{"type": "text_result", "title": "Everything announced", "url": "https://example.com/a", "snippet": "Zuckerberg unveiled..."}
		]
	}`)

	content, err := decodeWebSearchCallContent(call)
	assert.NoError(t, err)
	use := content[0].(*llm.ServerToolUseContent)

	results, ok := use.Input["results"].([]any)
	assert.True(t, ok, "results decoded to %T", use.Input["results"])
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].(map[string]any)["title"], "Everything announced")
	assert.Equal(t, results[0].(map[string]any)["snippet"], "Zuckerberg unveiled...")
}

// An open_page call reports an empty results list. An empty "results" key would
// read as "the include was on and found nothing", which is not what happened.
func TestDecodeWebSearchCallEmptyResults(t *testing.T) {
	call := unmarshalWebSearchCall(t, `{"id":"ws_6","type":"web_search_call","status":"completed",
		"action":{"type":"open_page","url":"https://example.com/a"},"results":[]}`)
	content, err := decodeWebSearchCallContent(call)
	assert.NoError(t, err)
	use := content[0].(*llm.ServerToolUseContent)
	_, hasResults := use.Input["results"]
	assert.False(t, hasResults)
}

// open_page and find_in_page are the other two actions the model can take, and
// neither carries a query.
func TestDecodeWebSearchCallOtherActions(t *testing.T) {
	open := unmarshalWebSearchCall(t, `{"id":"ws_2","type":"web_search_call","status":"completed",
		"action":{"type":"open_page","url":"https://example.com/a"}}`)
	content, err := decodeWebSearchCallContent(open)
	assert.NoError(t, err)
	use := content[0].(*llm.ServerToolUseContent)
	assert.Equal(t, use.Input["type"], "open_page")
	assert.Equal(t, use.Input["url"], "https://example.com/a")
	_, hasQuery := use.Input["query"]
	assert.False(t, hasQuery)

	find := unmarshalWebSearchCall(t, `{"id":"ws_3","type":"web_search_call","status":"completed",
		"action":{"type":"find_in_page","url":"https://example.com/a","pattern":"Connect"}}`)
	content, err = decodeWebSearchCallContent(find)
	assert.NoError(t, err)
	use = content[0].(*llm.ServerToolUseContent)
	assert.Equal(t, use.Input["pattern"], "Connect")
}

// A provider that omits the action has to read exactly as it did before.
func TestDecodeWebSearchCallWithoutAction(t *testing.T) {
	call := unmarshalWebSearchCall(t, `{"id":"ws_4","type":"web_search_call","status":"completed"}`)
	content, err := decodeWebSearchCallContent(call)
	assert.NoError(t, err)
	use := content[0].(*llm.ServerToolUseContent)
	assert.Equal(t, use.ID, "ws_4")
	assert.Nil(t, use.Input)
}

// Replaying a search into a later turn used to send an empty query, which said
// a search had happened without saying what it asked.
func TestEncodeWebSearchCallReplaysQuery(t *testing.T) {
	item, err := encodeAssistantServerToolUseContent(&llm.ServerToolUseContent{
		ID:    "ws_1",
		Name:  "web_search_call",
		Input: map[string]any{"type": "search", "query": "Meta Connect 2025 announcements"},
	})
	assert.NoError(t, err)

	encoded, err := json.Marshal(item)
	assert.NoError(t, err)
	assert.Contains(t, string(encoded), "Meta Connect 2025 announcements")
}

// Input is nil for a call decoded before this field was kept, and for any
// provider that omits the action.
func TestEncodeWebSearchCallWithoutQuery(t *testing.T) {
	_, err := encodeAssistantServerToolUseContent(&llm.ServerToolUseContent{
		ID:   "ws_1",
		Name: "web_search_call",
	})
	assert.NoError(t, err)
}
