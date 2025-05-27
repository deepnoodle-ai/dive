package llm

// NewMessage creates a new message with the given role and content blocks.
func NewMessage(role Role, content []Content) *Message {
	return &Message{Role: role, Content: content}
}

// NewUserMessage creates a new user message with the given content blocks.
func NewUserMessage(content ...Content) *Message {
	return &Message{Role: User, Content: content}
}

// NewUserTextMessage creates a new user message with a single text content
// block.
func NewUserTextMessage(text string) *Message {
	return &Message{
		Role:    User,
		Content: []Content{&TextContent{Text: text}},
	}
}

// NewAssistantMessage creates a new assistant message with the given content blocks.
func NewAssistantMessage(content ...Content) *Message {
	return &Message{Role: Assistant, Content: content}
}

// NewAssistantTextMessage creates a new assistant message with a single text
// content block.
func NewAssistantTextMessage(text string) *Message {
	return &Message{
		Role:    Assistant,
		Content: []Content{&TextContent{Text: text}},
	}
}

// NewToolResultMessage creates a new message with the user role and a list of
// tool outputs. Used to pass the results of tool calls back to an LLM.
func NewToolResultMessage(outputs ...*ToolResultContent) *Message {
	content := make([]Content, len(outputs))
	for i, output := range outputs {
		content[i] = &ToolResultContent{
			ToolUseID: output.ToolUseID,
			Content:   output.Content,
			IsError:   false,
		}
	}
	return &Message{Role: User, Content: content}
}

// NewUserFileMessage creates a new user message with a file content block
// using base64-encoded file data.
func NewUserFileMessage(filename, fileData string) *Message {
	return &Message{
		Role: User,
		Content: []Content{&FileContent{
			Filename: filename,
			FileData: fileData,
		}},
	}
}

// NewUserFileIDMessage creates a new user message with a file content block
// using an OpenAI file ID.
func NewUserFileIDMessage(fileID string) *Message {
	return &Message{
		Role: User,
		Content: []Content{&FileContent{
			FileID: fileID,
		}},
	}
}

// NewUserDocumentMessage creates a new user message with a document content block
// using base64-encoded document data. This is the preferred method for Anthropic PDF support.
func NewUserDocumentMessage(title, mediaType, base64Data string) *Message {
	return &Message{
		Role: User,
		Content: []Content{&DocumentContent{
			Title: title,
			Source: &ContentSource{
				Type:      ContentSourceTypeBase64,
				MediaType: mediaType,
				Data:      base64Data,
			},
		}},
	}
}

// NewUserDocumentURLMessage creates a new user message with a document content block
// using a URL reference to a PDF.
func NewUserDocumentURLMessage(title, url string) *Message {
	return &Message{
		Role: User,
		Content: []Content{&DocumentContent{
			Title: title,
			Source: &ContentSource{
				Type: ContentSourceTypeURL,
				URL:  url,
			},
		}},
	}
}

// NewUserDocumentFileIDMessage creates a new user message with a document content block
// using an Anthropic Files API file ID.
func NewUserDocumentFileIDMessage(title, fileID string) *Message {
	return &Message{
		Role: User,
		Content: []Content{&DocumentContent{
			Title: title,
			Source: &ContentSource{
				Type: "file",
				URL:  fileID, // Anthropic stores file ID in URL field
			},
		}},
	}
}
