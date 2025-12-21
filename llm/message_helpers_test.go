package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestNewUserDocumentMessage(t *testing.T) {
	mediaType := "application/pdf"
	base64Data := "JVBERi0xLjQK..."

	message := NewUserMessage(NewDocumentContent(EncodedData(mediaType, base64Data)))

	assert.Equal(t, User, message.Role)
	assert.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	assert.True(t, ok)
	assert.NotNil(t, docContent.Source)
	assert.Equal(t, ContentSourceTypeBase64, docContent.Source.Type)
	assert.Equal(t, mediaType, docContent.Source.MediaType)
	assert.Equal(t, base64Data, docContent.Source.Data)
}

func TestNewUserDocumentURLMessage(t *testing.T) {
	url := "https://example.com/document.pdf"

	message := NewUserMessage(NewDocumentContent(ContentURL(url)))

	assert.Equal(t, User, message.Role)
	assert.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	assert.True(t, ok)
	assert.NotNil(t, docContent.Source)
	assert.Equal(t, ContentSourceTypeURL, docContent.Source.Type)
	assert.Equal(t, url, docContent.Source.URL)
}

func TestNewUserDocumentFileIDMessage(t *testing.T) {
	fileID := "file-xyz789"
	message := NewUserMessage(NewDocumentContent(FileID(fileID)))

	assert.Equal(t, User, message.Role)
	assert.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	assert.True(t, ok)
	assert.NotNil(t, docContent.Source)
	assert.Equal(t, ContentSourceTypeFile, docContent.Source.Type)
	assert.Equal(t, fileID, docContent.Source.FileID)
}
