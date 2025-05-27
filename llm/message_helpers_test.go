package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUserFileMessage(t *testing.T) {
	filename := "test.pdf"
	fileData := "data:application/pdf;base64,JVBERi0xLjQK..."

	message := NewUserFileMessage(filename, fileData)

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	fileContent, ok := message.Content[0].(*FileContent)
	require.True(t, ok)
	require.Equal(t, filename, fileContent.Filename)
	require.Equal(t, fileData, fileContent.FileData)
	require.Empty(t, fileContent.FileID)
}

func TestNewUserFileIDMessage(t *testing.T) {
	fileID := "file-abc123"

	message := NewUserFileIDMessage(fileID)

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	fileContent, ok := message.Content[0].(*FileContent)
	require.True(t, ok)
	require.Equal(t, fileID, fileContent.FileID)
	require.Empty(t, fileContent.Filename)
	require.Empty(t, fileContent.FileData)
}

func TestNewUserDocumentMessage(t *testing.T) {
	title := "Test Document"
	mediaType := "application/pdf"
	base64Data := "JVBERi0xLjQK..."

	message := NewUserDocumentMessage(title, mediaType, base64Data)

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	require.True(t, ok)
	require.Equal(t, title, docContent.Title)
	require.NotNil(t, docContent.Source)
	require.Equal(t, ContentSourceTypeBase64, docContent.Source.Type)
	require.Equal(t, mediaType, docContent.Source.MediaType)
	require.Equal(t, base64Data, docContent.Source.Data)
}

func TestNewUserDocumentURLMessage(t *testing.T) {
	title := "Remote Document"
	url := "https://example.com/document.pdf"

	message := NewUserDocumentURLMessage(title, url)

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	require.True(t, ok)
	require.Equal(t, title, docContent.Title)
	require.NotNil(t, docContent.Source)
	require.Equal(t, ContentSourceTypeURL, docContent.Source.Type)
	require.Equal(t, url, docContent.Source.URL)
}

func TestNewUserDocumentFileIDMessage(t *testing.T) {
	title := "File API Document"
	fileID := "file-xyz789"

	message := NewUserDocumentFileIDMessage(title, fileID)

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	require.True(t, ok)
	require.Equal(t, title, docContent.Title)
	require.NotNil(t, docContent.Source)
	require.Equal(t, ContentSourceType("file"), docContent.Source.Type)
	require.Equal(t, fileID, docContent.Source.URL)
}
