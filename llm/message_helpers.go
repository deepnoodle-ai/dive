package llm

import (
	"encoding/base64"
	"encoding/json"
)

// NewMessage creates a message with the given role and content blocks.
func NewMessage(role Role, content []Content) *Message {
	return &Message{Role: role, Content: content}
}

// NewUserMessage creates a user message with the given content blocks.
func NewUserMessage(content ...Content) *Message {
	return &Message{Role: User, Content: content}
}

// NewUserTextMessage creates a user message with a single text content block.
func NewUserTextMessage(text string) *Message {
	return &Message{
		Role:    User,
		Content: []Content{&TextContent{Text: text}},
	}
}

// NewSystemMessage creates a system message with a single text content block.
func NewSystemMessage(text string) *Message {
	return &Message{
		Role:    System,
		Content: []Content{&TextContent{Text: text}},
	}
}

// NewTextContent creates a text content block with the given text.
func NewTextContent(text string) *TextContent {
	return &TextContent{Text: text}
}

// NewToolUseContent creates a tool use content block.
func NewToolUseContent(id, name string, input json.RawMessage) *ToolUseContent {
	return &ToolUseContent{
		ID:    id,
		Name:  name,
		Input: input,
	}
}

// NewToolResultContent creates a tool result content block.
func NewToolResultContent(toolUseID string, content any, isError bool) *ToolResultContent {
	return &ToolResultContent{
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}
}

// NewAssistantMessage creates an assistant message with the given content.
func NewAssistantMessage(content ...Content) *Message {
	return &Message{Role: Assistant, Content: content}
}

// NewAssistantTextMessage creates an assistant message with a single text
// content block.
func NewAssistantTextMessage(text string) *Message {
	return &Message{
		Role:    Assistant,
		Content: []Content{&TextContent{Text: text}},
	}
}

// NewToolResultMessage creates a message with the user role and a list of
// tool outputs. Used to pass the results of tool calls back to an LLM.
func NewToolResultMessage(outputs ...*ToolResultContent) *Message {
	content := make([]Content, len(outputs))
	for i, output := range outputs {
		content[i] = &ToolResultContent{
			ToolUseID: output.ToolUseID,
			Content:   output.Content,
			IsError:   output.IsError,
		}
	}
	return &Message{Role: User, Content: content}
}

// NewImageContent creates an image content block with the given content source.
func NewImageContent(source *ContentSource) *ImageContent {
	return &ImageContent{Source: source}
}

// NewDocumentContent creates a document content block with the given
// content source.
func NewDocumentContent(source *ContentSource) *DocumentContent {
	return &DocumentContent{Source: source}
}

// EncodedData creates a content source with the given media type and
// base64-encoded data.
func EncodedData(mediaType, base64Data string) *ContentSource {
	return &ContentSource{
		Type:      ContentSourceTypeBase64,
		MediaType: mediaType,
		Data:      base64Data,
	}
}

// RawData creates a content source with the given media type and raw data.
// Automatically base64 encodes the provided data.
func RawData(mediaType string, data []byte) *ContentSource {
	base64Data := base64.StdEncoding.EncodeToString(data)
	return &ContentSource{
		Type:      ContentSourceTypeBase64,
		MediaType: mediaType,
		Data:      base64Data,
	}
}

// ContentURL creates a content source with the given URL.
func ContentURL(url string) *ContentSource {
	return &ContentSource{
		Type: ContentSourceTypeURL,
		URL:  url,
	}
}

// FileID creates a content source with the given file ID.
func FileID(id string) *ContentSource {
	return &ContentSource{
		Type:   ContentSourceTypeFile,
		FileID: id,
	}
}
