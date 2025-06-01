package llm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// ContentType indicates the type of a content block in a message
type ContentType string

const (
	ContentTypeText                    ContentType = "text"
	ContentTypeImage                   ContentType = "image"
	ContentTypeDocument                ContentType = "document"
	ContentTypeFile                    ContentType = "file"
	ContentTypeToolUse                 ContentType = "tool_use"
	ContentTypeToolResult              ContentType = "tool_result"
	ContentTypeThinking                ContentType = "thinking"
	ContentTypeRedactedThinking        ContentType = "redacted_thinking"
	ContentTypeServerToolUse           ContentType = "server_tool_use"
	ContentTypeWebSearchToolResult     ContentType = "web_search_tool_result"
	ContentTypeMCPToolUse              ContentType = "mcp_tool_use"
	ContentTypeMCPToolResult           ContentType = "mcp_tool_result"
	ContentTypeCodeExecutionToolResult ContentType = "code_execution_tool_result"
	ContentTypeRefusal                 ContentType = "refusal"
)

// ContentSourceType indicates the location of the media content.
type ContentSourceType string

const (
	ContentSourceTypeBase64 ContentSourceType = "base64"
	ContentSourceTypeURL    ContentSourceType = "url"
	ContentSourceTypeText   ContentSourceType = "text"
	ContentSourceTypeFile   ContentSourceType = "file"
)

func (c ContentSourceType) String() string {
	return string(c)
}

// CacheControl is used to control caching of content blocks.
type CacheControl struct {
	Type CacheControlType `json:"type"`
}

// ContentChunk is used within a Content block to pass chunks of content.
type ContentChunk struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ContentSource conveys information about media content in a message.
type ContentSource struct {
	// Type is the type of the content source ("base64", "url", "text", or "file")
	Type ContentSourceType `json:"type"`

	// MediaType is the media type of the content. E.g. "image/jpeg", "application/pdf"
	MediaType string `json:"media_type,omitempty"`

	// Data is base64 encoded data (used with ContentSourceTypeBase64)
	Data string `json:"data,omitempty"`

	// URL is the URL of the content (used with ContentSourceTypeURL)
	URL string `json:"url,omitempty"`

	// FileID is the file ID from the Files API (used with ContentSourceTypeFile)
	FileID string `json:"file_id,omitempty"`

	// Chunks of content. Only use if chunking on the client side, for use
	// within a DocumentContent block.
	Content []*ContentChunk `json:"content,omitempty"`

	// GenerationID is an ID associated with the generation of this content,
	// if any. This may be set on content returned from image generation, for
	// example. Used for OpenAI image generation results.
	GenerationID string `json:"generation_id,omitempty"`

	// GenerationStatus is the status of the generation of this content,
	// if any. This may be set on content returned from image generation, for
	// example. Used for OpenAI image generation results.
	GenerationStatus string `json:"generation_status,omitempty"`
}

// DecodedData returns the decoded data if this content carries base64 encoded data.
func (c *ContentSource) DecodedData() ([]byte, error) {
	if c.Type == ContentSourceTypeBase64 {
		return base64.StdEncoding.DecodeString(c.Data)
	}
	return nil, fmt.Errorf("cannot decode data content source type: %s", c.Type)
}

// Content is a single block of content in a message. A message may contain
// multiple content blocks of varying types.
type Content interface {
	Type() ContentType
}

// CacheControlSetter is an interface that allows setting the cache control for
// a content block.
type CacheControlSetter interface {
	SetCacheControl(cacheControl *CacheControl)
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

{
  "type": "text",
  "text": "the grass is green",
  "citations": [{
    "type": "char_location",
    "cited_text": "The grass is green.",
    "document_index": 0,
    "document_title": "Example Document",
    "start_char_index": 0,
    "end_char_index": 20
  }]
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
	// Create a struct with Citations as json.RawMessage to handle custom marshaling
	type TextWithRawCitations struct {
		Text         string          `json:"text"`
		CacheControl *CacheControl   `json:"cache_control,omitempty"`
		Citations    json.RawMessage `json:"citations,omitempty"`
	}

	// Start with base fields
	twc := TextWithRawCitations{
		Text:         c.Text,
		CacheControl: c.CacheControl,
	}

	// Marshal Citations if present
	if len(c.Citations) > 0 {
		citationsJSON, err := json.Marshal(c.Citations)
		if err != nil {
			return nil, err
		}
		twc.Citations = citationsJSON
	}

	// Marshal with type field
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		TextWithRawCitations
	}{
		Type:                 ContentTypeText,
		TextWithRawCitations: twc,
	})
}

func (c *TextContent) SetCacheControl(cacheControl *CacheControl) {
	c.CacheControl = cacheControl
}

//// RefusalContent ////////////////////////////////////////////////////////////

type RefusalContent struct {
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

func (c *RefusalContent) Type() ContentType {
	return ContentTypeRefusal
}

func (c *RefusalContent) MarshalJSON() ([]byte, error) {
	type Alias RefusalContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeRefusal,
		Alias: (*Alias)(c),
	})
}

func (c *RefusalContent) SetCacheControl(cacheControl *CacheControl) {
	c.CacheControl = cacheControl
}

//// ImageContent //////////////////////////////////////////////////////////////

// https://docs.anthropic.com/en/docs/build-with-claude/vision

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

{
  "type": "image",
  "source": {
    "type": "file",
    "file_id": "file_abc123"
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

func (c *ImageContent) SetCacheControl(cacheControl *CacheControl) {
	c.CacheControl = cacheControl
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

{
  "type": "document",
  "source": {
    "type": "file",
    "file_id": "file_abc123"
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

func (c *DocumentContent) SetCacheControl(cacheControl *CacheControl) {
	c.CacheControl = cacheControl
}

//// FileContent ///////////////////////////////////////////////////////////////

/* Examples:
{
  "type": "file",
  "filename": "draconomicon.pdf",
  "file_data": "data:application/pdf;base64,JVBERi0xLjQKMSAwIG9iago8PAovVHlwZSAvQ2F0YWxvZwovUGFnZXMgMiAwIFIKPj4KZW5kb2JqCg==",
  "cache_control": {"type": "ephemeral"}
}

{
  "type": "file",
  "file_id": "file-abc123",
  "cache_control": {"type": "ephemeral"}
}
*/

type FileContent struct {
	// Filename is the name of the file (required when using file_data)
	Filename string `json:"filename,omitempty"`

	// FileData is the base64-encoded file data with data URI format
	// e.g., "data:application/pdf;base64,JVBERi0xLjQK..."
	FileData string `json:"file_data,omitempty"`

	// FileID is the OpenAI file ID (alternative to file_data)
	FileID string `json:"file_id,omitempty"`

	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

func (c *FileContent) Type() ContentType {
	return ContentTypeFile
}

func (c *FileContent) MarshalJSON() ([]byte, error) {
	type Alias FileContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeFile,
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
	ToolUseID    string        `json:"tool_use_id"`
	Content      any           `json:"content"`
	IsError      bool          `json:"is_error,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
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

func (c *ToolResultContent) SetCacheControl(cacheControl *CacheControl) {
	c.CacheControl = cacheControl
}

//// ServerToolUseContent //////////////////////////////////////////////////////

// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool#response

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

// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool#response

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

// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking

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

//// MCPToolUse ////////////////////////////////////////////////////////////////

// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#mcp-tool-use-block

/* Examples:
{
  "type": "mcp_tool_use",
  "id": "mcptoolu_014Q35RayjACSWkSj4X2yov1",
  "name": "echo",
  "server_name": "example-mcp",
  "input": { "param1": "value1", "param2": "value2" }
}
*/

type MCPToolUseContent struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	ServerName string          `json:"server_name"`
	Input      json.RawMessage `json:"input"`
}

func (c *MCPToolUseContent) Type() ContentType {
	return ContentTypeMCPToolUse
}

func (c *MCPToolUseContent) MarshalJSON() ([]byte, error) {
	type Alias MCPToolUseContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeMCPToolUse,
		Alias: (*Alias)(c),
	})
}

//// MCPToolResult //////////////////////////////////////////////////////////////

// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#mcp-tool-result-block

/* Examples:
{
  "type": "mcp_tool_result",
  "tool_use_id": "mcptoolu_014Q35RayjACSWkSj4X2yov1",
  "is_error": false,
  "content": [
    {
      "type": "text",
      "text": "Hello"
    }
  ]
}
*/

type MCPToolResultContent struct {
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error,omitempty"`
	Content   []*ContentChunk `json:"content,omitempty"`
}

func (c *MCPToolResultContent) Type() ContentType {
	return ContentTypeMCPToolResult
}

func (c *MCPToolResultContent) MarshalJSON() ([]byte, error) {
	type Alias MCPToolResultContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeMCPToolResult,
		Alias: (*Alias)(c),
	})
}

//// CodeExecutionToolResult ///////////////////////////////////////////////////

// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool

/* Examples:
   {
     "type": "code_execution_tool_result",
     "tool_use_id": "srvtoolu_01A2B3C4D5E6F7G8H9I0J1K2",
     "content": {
       "type": "code_execution_result",
       "stdout": "Mean: 5.5\nStandard deviation: 2.8722813232690143\n",
       "stderr": "",
       "return_code": 0
     }
   }
*/

type CodeExecutionResult struct {
	Type       string `json:"type"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"return_code"`
}

type CodeExecutionToolResultContent struct {
	ToolUseID string              `json:"tool_use_id"`
	Content   CodeExecutionResult `json:"content"`
}

func (c *CodeExecutionToolResultContent) Type() ContentType {
	return ContentTypeCodeExecutionToolResult
}

func (c *CodeExecutionToolResultContent) MarshalJSON() ([]byte, error) {
	type Alias CodeExecutionToolResultContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeCodeExecutionToolResult,
		Alias: (*Alias)(c),
	})
}

//// Unmarshalling /////////////////////////////////////////////////////////////

type contentTypeIndicator struct {
	Type ContentType `json:"type"`
}

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
		text := &TextContent{}
		type textContent struct {
			Text         string          `json:"text"`
			CacheControl *CacheControl   `json:"cache_control"`
			Citations    json.RawMessage `json:"citations"`
		}
		var tc textContent
		if err := json.Unmarshal(data, &tc); err != nil {
			return nil, err
		}
		text.Text = tc.Text
		if tc.CacheControl != nil {
			text.CacheControl = tc.CacheControl
		}
		if tc.Citations != nil {
			citations, err := unmarshalCitations(tc.Citations)
			if err != nil {
				return nil, err
			}
			text.Citations = citations
		}
		return text, nil
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
	case ContentTypeMCPToolUse:
		content = &MCPToolUseContent{}
	case ContentTypeMCPToolResult:
		content = &MCPToolResultContent{}
	case ContentTypeCodeExecutionToolResult:
		content = &CodeExecutionToolResultContent{}
	default:
		return nil, fmt.Errorf("unsupported content type: %s", ct.Type)
	}
	// Unmarshal into the concrete type
	if err := json.Unmarshal(data, content); err != nil {
		return nil, err
	}
	return content, nil
}
