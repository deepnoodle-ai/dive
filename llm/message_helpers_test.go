package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUserDocumentMessage(t *testing.T) {
	mediaType := "application/pdf"
	base64Data := "JVBERi0xLjQK..."

	message := NewUserMessage(NewDocumentContent(EncodedData(mediaType, base64Data)))

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	require.True(t, ok)
	require.NotNil(t, docContent.Source)
	require.Equal(t, ContentSourceTypeBase64, docContent.Source.Type)
	require.Equal(t, mediaType, docContent.Source.MediaType)
	require.Equal(t, base64Data, docContent.Source.Data)
}

func TestNewUserDocumentURLMessage(t *testing.T) {
	url := "https://example.com/document.pdf"

	message := NewUserMessage(NewDocumentContent(ContentURL(url)))

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	require.True(t, ok)
	require.NotNil(t, docContent.Source)
	require.Equal(t, ContentSourceTypeURL, docContent.Source.Type)
	require.Equal(t, url, docContent.Source.URL)
}

func TestNewUserDocumentFileIDMessage(t *testing.T) {
	fileID := "file-xyz789"
	message := NewUserMessage(NewDocumentContent(FileID(fileID)))

	require.Equal(t, User, message.Role)
	require.Len(t, message.Content, 1)

	docContent, ok := message.Content[0].(*DocumentContent)
	require.True(t, ok)
	require.NotNil(t, docContent.Source)
	require.Equal(t, ContentSourceTypeFile, docContent.Source.Type)
	require.Equal(t, fileID, docContent.Source.FileID)
}
