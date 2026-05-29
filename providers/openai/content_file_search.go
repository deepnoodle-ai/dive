package openai

import (
	"encoding/json"

	"github.com/deepnoodle-ai/dive/llm"
)

const ContentTypeFileSearchCall llm.ContentType = "file_search_call"

// FileSearchCallResult mirrors openai.ResponseFileSearchToolCallResult. It
// represents a single document chunk returned by a file/collections search.
type FileSearchCallResult struct {
	// FileID is the unique identifier of the matched file.
	FileID string `json:"file_id,omitempty"`

	// Filename is the name of the matched file.
	Filename string `json:"filename,omitempty"`

	// Score is the relevance score of the result (0 to 1).
	Score float64 `json:"score,omitempty"`

	// Text is the chunk of text retrieved from the file.
	Text string `json:"text,omitempty"`
}

// FileSearchCallContent mirrors openai.ResponseFileSearchToolCall. It is emitted
// when the model invokes a file_search (OpenAI) / collections_search (xAI/Grok)
// server-side tool. Result text is only populated when the request opts in via
// the "file_search_call.results" include parameter.
type FileSearchCallContent struct {
	// ID is the unique identifier for this tool call.
	ID string `json:"id"`

	// Queries are the search queries the model issued.
	Queries []string `json:"queries,omitempty"`

	// Status is the execution status, e.g. "in_progress", "searching",
	// "completed", "incomplete", or "failed".
	Status string `json:"status"`

	// Results contains the matched document chunks, when requested via include.
	Results []FileSearchCallResult `json:"results,omitempty"`
}

func (c *FileSearchCallContent) Type() llm.ContentType {
	return ContentTypeFileSearchCall
}

// MarshalJSON ensures the content includes a top-level "type" field.
func (c *FileSearchCallContent) MarshalJSON() ([]byte, error) {
	type Alias FileSearchCallContent
	return json.Marshal(struct {
		Type llm.ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeFileSearchCall,
		Alias: (*Alias)(c),
	})
}
