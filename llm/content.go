package llm

import (
	"encoding/json"
	"fmt"
)

// ContentType indicates the type of a content block in a message
type ContentType string

const (
	ContentTypeText                ContentType = "text"
	ContentTypeImage               ContentType = "image"
	ContentTypeDocument            ContentType = "document"
	ContentTypeToolUse             ContentType = "tool_use"
	ContentTypeToolResult          ContentType = "tool_result"
	ContentTypeThinking            ContentType = "thinking"
	ContentTypeRedactedThinking    ContentType = "redacted_thinking"
	ContentTypeServerToolUse       ContentType = "server_tool_use"
	ContentTypeWebSearchToolResult ContentType = "web_search_tool_result"
)

// ContentSourceType indicates the location of the media content.
type ContentSourceType string

const (
	ContentSourceTypeBase64 ContentSourceType = "base64"
	ContentSourceTypeURL    ContentSourceType = "url"
)

func (c ContentSourceType) String() string {
	return string(c)
}

// CacheControl is used to control caching of content blocks.
type CacheControl struct {
	Type CacheControlType `json:"type"`
}

// ToolCall is a call made by an LLM
// type ToolCall struct {
// 	ID    string `json:"id"`
// 	Name  string `json:"name"`
// 	Input string `json:"input"`
// }

// ContentChunk is used to pass pre-chunked document content to the LLM. These
// should only be used within a DocumentContent block.
type ContentChunk struct {
	Type string `json:"type"` // always "text"
	Text string `json:"text"`
}

// ContentSource conveys information about media content in a message.
type ContentSource struct {
	// Type is the type of the content source ("base64", "url", or "content")
	Type ContentSourceType `json:"type"`

	// MediaType is the media type of the content. E.g. "image/jpeg"
	MediaType string `json:"media_type,omitempty"`

	// Data is base64 encoded data
	Data string `json:"data,omitempty"`

	// URL is the URL of the content
	URL string `json:"url,omitempty"`

	// Chunks of content. Only use if chunking on the client side, for use
	// within a DocumentContent block.
	Content []*ContentChunk `json:"content,omitempty"`
}

// Content is a single block of content in a message. A message may contain
// multiple content blocks of varying types.
type Content interface {
	Type() ContentType
}

//// TextContent ///////////////////////////////////////////////////////////////

/* Examples:
{
  "type": "text",
  "text": "What color is the grass and sky?"
}

{
  "text": "Claude Shannon was born on April 30, 1916, in Petoskey, Michigan",
  "type": "text",
  "citations": [
    {
      "type": "web_search_result_location",
      "url": "https://en.wikipedia.org/wiki/Claude_Shannon",
      "title": "Claude Shannon - Wikipedia",
      "encrypted_index": "Eo8BCioIAhgBIiQyYjQ0OWJmZi1lNm..",
      "cited_text": "Claude Elwood Shannon (April 30, 1916 – February 24, ..."
    }
  ]
}
*/

type TextContent struct {
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
	Citations    []Citation    `json:"citations,omitempty"`
}

func (c *TextContent) Type() ContentType {
	return ContentTypeText
}

func (c *TextContent) MarshalJSON() ([]byte, error) {
	type Alias TextContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeText,
		Alias: (*Alias)(c),
	})
}

//// ImageContent //////////////////////////////////////////////////////////////

/* Examples:
{
  "type": "image",
  "source": {
    "type": "base64",
    "media_type": "image/jpeg",
    "data": "$BASE64_IMAGE_DATA"
  }
}

{
  "type": "image",
  "source": {
    "type": "url",
    "url": "https://upload.wikimedia.org/foo.jpg"
  }
}
*/

type ImageContent struct {
	Source       *ContentSource `json:"source"`
	CacheControl *CacheControl  `json:"cache_control,omitempty"`
}

func (c *ImageContent) Type() ContentType {
	return ContentTypeImage
}

func (c *ImageContent) MarshalJSON() ([]byte, error) {
	type Alias ImageContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeImage,
		Alias: (*Alias)(c),
	})
}

//// DocumentContent ///////////////////////////////////////////////////////////

/* Examples:
{
  "type": "document",
  "source": {
    "type": "text",
    "media_type": "text/plain",
    "data": "The grass is green. The sky is blue."
  },
  "title": "My Document",
  "context": "This is a trustworthy document.",
  "citations": {"enabled": true}
}

{
  "type": "document",
  "source": {
    "type": "content",
    "content": [
      {"type": "text", "text": "First chunk"},
      {"type": "text", "text": "Second chunk"}
    ]
  },
  "title": "Document Title",
  "context": "Context about the document that will not be cited from",
  "citations": {"enabled": true}
}

{
  "type": "document",
  "source": {
    "type": "url",
    "url": "https://site.com/foo.pdf"
  }
}

{
  "type": "document",
  "source": {
    "type": "base64",
    "media_type": "application/pdf",
    "data": "$PDF_BASE64"
  }
}
*/

type DocumentContent struct {
	Source       *ContentSource    `json:"source"`
	Title        string            `json:"title,omitempty"`
	Context      string            `json:"context,omitempty"`
	Citations    *CitationSettings `json:"citations,omitempty"`
	CacheControl *CacheControl     `json:"cache_control,omitempty"`
}

func (c *DocumentContent) Type() ContentType {
	return ContentTypeDocument
}

func (c *DocumentContent) MarshalJSON() ([]byte, error) {
	type Alias DocumentContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeDocument,
		Alias: (*Alias)(c),
	})
}

//// ToolUseContent ////////////////////////////////////////////////////////////

/* Examples:
{
  "type": "tool_use",
  "id": "toolu_01A09q90qw90lq917835lq9",
  "name": "get_weather",
  "input": {"location": "San Francisco, CA", "unit": "celsius"}
}
*/

type ToolUseContent struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (c *ToolUseContent) Type() ContentType {
	return ContentTypeToolUse
}

func (c *ToolUseContent) MarshalJSON() ([]byte, error) {
	type Alias ToolUseContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeToolUse,
		Alias: (*Alias)(c),
	})
}

//// ToolResultContent /////////////////////////////////////////////////////////

/* Examples:
{
  "type": "tool_result",
  "tool_use_id": "toolu_01A09q90qw90lq917835lq9",
  "content": "15 degrees"
}

{
  "type": "tool_result",
  "tool_use_id": "toolu_01A09q90qw90lq917835lq9",
  "content": [
    {"type": "text", "text": "15 degrees"},
    {"type": "image", "source": {"type":"base64", "media_type":"image/jpeg", "data":"/9j/4AAQSkZJRg..."}}
  ]
}

{
  "type": "tool_result",
  "tool_use_id": "toolu_01A09q90qw90lq917835lq9",
  "content": "Error: Missing required 'location' parameter",
  "is_error": true
}
*/

type ToolResultContent struct {
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (c *ToolResultContent) Type() ContentType {
	return ContentTypeToolResult
}

func (c *ToolResultContent) MarshalJSON() ([]byte, error) {
	type Alias ToolResultContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeToolResult,
		Alias: (*Alias)(c),
	})
}

//// ServerToolUseContent //////////////////////////////////////////////////////

/* Examples:
{
  "type": "server_tool_use",
  "id": "srvtoolu_01WYG3ziw53XMcoyKL4XcZmE",
  "name": "web_search",
  "input": {
    "query": "claude shannon birth date"
  }
}
*/

type ServerToolUseContent struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (c *ServerToolUseContent) Type() ContentType {
	return ContentTypeServerToolUse
}

func (c *ServerToolUseContent) MarshalJSON() ([]byte, error) {
	type Alias ServerToolUseContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeServerToolUse,
		Alias: (*Alias)(c),
	})
}

//// WebSearchToolResultContent ////////////////////////////////////////////////

/* Examples:
{
  "type": "web_search_tool_result",
  "tool_use_id": "srvtoolu_01WYG3ziw53XMcoyKL4XcZmE",
  "content": [
    {
      "type": "web_search_result",
      "url": "https://en.wikipedia.org/wiki/Claude_Shannon",
      "title": "Claude Shannon - Wikipedia",
      "encrypted_content": "EqgfCioIARgBIiQ3YTAwMjY1Mi1mZjM5LTQ1NGUtODgxNC1kNjNjNTk1ZWI3Y...",
      "page_age": "April 30, 2025"
    }
  ]
}

{
  "type": "web_search_tool_result",
  "tool_use_id": "servertoolu_a93jad",
  "content": {
    "type": "web_search_tool_result_error",
    "error_code": "max_uses_exceeded"
  }
}
*/

type WebSearchResult struct {
	Type             string `json:"type"`
	URL              string `json:"url"`
	Title            string `json:"title"`
	EncryptedContent string `json:"encrypted_content"`
	PageAge          string `json:"page_age"`
}

type WebSearchToolResultContent struct {
	ToolUseID string             `json:"tool_use_id"`
	Content   []*WebSearchResult `json:"content"`
	ErrorCode string             `json:"error_code,omitempty"`
}

func (c *WebSearchToolResultContent) Type() ContentType {
	return ContentTypeWebSearchToolResult
}

func (c *WebSearchToolResultContent) MarshalJSON() ([]byte, error) {
	type Alias WebSearchToolResultContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeWebSearchToolResult,
		Alias: (*Alias)(c),
	})
}

//// ThinkingContent ///////////////////////////////////////////////////////////

/* Examples:
{
  "type": "thinking",
  "thinking": "Let me analyze this step by step...",
  "signature": "WaUjzkypQ2mUEVM36O2TxuC06KN8xyfbFG/UvLEczmEsUjavL...."
}
*/

// ThinkingContent is a content block that contains the LLM's internal thought
// process. The provider may use the signature to verify that the content was
// generated by the LLM.
//
// Per Anthropic's documentation:
// It is only strictly necessary to send back thinking blocks when using tool
// use with extended thinking. Otherwise you can omit thinking blocks from
// previous turns, or let the API strip them for you if you pass them back.
type ThinkingContent struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

func (c *ThinkingContent) Type() ContentType {
	return ContentTypeThinking
}

func (c *ThinkingContent) MarshalJSON() ([]byte, error) {
	type Alias ThinkingContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeThinking,
		Alias: (*Alias)(c),
	})
}

//// RedactedThinkingContent ///////////////////////////////////////////////////

/* Examples:
{
  "type": "redacted_thinking",
  "data": "EmwKAhgBEgy3va3pzix/LafPsn4aDFIT2Xlxh0L5L8rLVyIwxtE3rAFBa8cr3qpP..."
}
*/

// RedactedThinkingContent is a content block that contains encrypted thinking,
// due to being flagged by the provider's safety systems. These are decrypted
// when passed back to the LLM, so that it can continue the thought process.
type RedactedThinkingContent struct {
	Data string `json:"data"`
}

func (c *RedactedThinkingContent) Type() ContentType {
	return ContentTypeRedactedThinking
}

func (c *RedactedThinkingContent) MarshalJSON() ([]byte, error) {
	type Alias RedactedThinkingContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeRedactedThinking,
		Alias: (*Alias)(c),
	})
}

type contentTypeIndicator struct {
	Type ContentType `json:"type"`
}

//// Unmarshalling /////////////////////////////////////////////////////////////

// UnmarshalContent unmarshals the JSON of one content block into the
// appropriate concrete Content type.
func UnmarshalContent(data []byte) (Content, error) {
	// Extract the type field
	var ct contentTypeIndicator
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, err
	}
	// Create and unmarshal the appropriate concrete type
	var content Content
	switch ct.Type {
	case ContentTypeText:
		content = &TextContent{}
	case ContentTypeImage:
		content = &ImageContent{}
	case ContentTypeDocument:
		content = &DocumentContent{}
	case ContentTypeToolUse:
		content = &ToolUseContent{}
	case ContentTypeToolResult:
		content = &ToolResultContent{}
	case ContentTypeThinking:
		content = &ThinkingContent{}
	case ContentTypeRedactedThinking:
		content = &RedactedThinkingContent{}
	case ContentTypeServerToolUse:
		content = &ServerToolUseContent{}
	case ContentTypeWebSearchToolResult:
		content = &WebSearchToolResultContent{}
	default:
		return nil, fmt.Errorf("unsupported content type: %s", ct.Type)
	}
	// Unmarshal into the concrete type
	if err := json.Unmarshal(data, content); err != nil {
		return nil, err
	}
	return content, nil
}
