package dive

import (
	"bytes"
	"encoding/json"

	"github.com/deepnoodle-ai/dive/llm"
)

// canonicalizeToolInputs rewrites each tool call's JSON input in a canonical
// form: object keys sorted, no insignificant whitespace. It runs before the
// assistant message is recorded, so the request that next carries the call,
// and every later replay of it, send identical bytes.
//
// Providers cache prompts by exact prefix, and key order is part of the
// prefix. A model writes keys in its own order, but a transcript stored as
// structured JSON (a Postgres jsonb column, a Go map) comes back with its keys
// sorted. Without this, the first replay of every turn diverges at that turn's
// first tool call with more than one key, and everything after it is
// rewritten in the provider's cache instead of read.
//
// Values are preserved exactly: numbers keep their literal text, and an input
// that is not valid JSON is left untouched.
func canonicalizeToolInputs(contents []llm.Content) {
	for _, content := range contents {
		switch c := content.(type) {
		case *llm.ToolUseContent:
			c.Input = canonicalJSON(c.Input)
		case *llm.MCPToolUseContent:
			c.Input = canonicalJSON(c.Input)
		}
	}
}

// canonicalJSON returns raw re-encoded with sorted object keys and compact
// spacing, or raw itself when it is empty or not valid JSON.
func canonicalJSON(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.More() {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}
