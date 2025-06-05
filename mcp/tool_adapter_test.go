package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/schema"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// boolPtr returns a pointer to the given bool value
func boolPtr(b bool) *bool {
	return &b
}

func TestNewToolAdapter(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "test-tool",
		Description: "A test tool",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "string",
					"description": "Input parameter",
				},
			},
		},
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")
	require.NotNil(t, adapter)
	require.Equal(t, client, adapter.mcpClient)
	require.Equal(t, mcpTool, adapter.toolInfo)
	require.Equal(t, "test-server", adapter.serverName)
}

func TestMCPToolAdapter_Name(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "my-tool",
		Description: "A test tool",
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")
	require.Equal(t, "my-tool", adapter.Name())
}

func TestMCPToolAdapter_Description(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		mcpTool  mcp.Tool
		expected string
	}{
		{
			name: "with description",
			mcpTool: mcp.Tool{
				Name:        "test-tool",
				Description: "A test tool",
			},
			expected: "A test tool",
		},
		{
			name: "without description",
			mcpTool: mcp.Tool{
				Name:        "test-tool",
				Description: "",
			},
			expected: "MCP tool test-tool from server test-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewToolAdapter(client, tt.mcpTool, "test-server")
			require.Equal(t, tt.expected, adapter.Description())
		})
	}
}

func TestMCPToolAdapter_Schema(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		mcpTool  mcp.Tool
		expected *schema.Schema
	}{
		{
			name: "empty schema",
			mcpTool: mcp.Tool{
				Name: "test-tool",
			},
			expected: &schema.Schema{
				Type:       "object",
				Properties: map[string]*schema.Property{},
			},
		},
		{
			name: "simple object schema",
			mcpTool: mcp.Tool{
				Name: "test-tool",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"input": map[string]interface{}{
							"type":        "string",
							"description": "Input parameter",
						},
					},
				},
			},
			expected: &schema.Schema{
				Type: "object",
				Properties: map[string]*schema.Property{
					"input": {
						Type:        "string",
						Description: "Input parameter",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewToolAdapter(client, tt.mcpTool, "test-server")
			schema := adapter.Schema()
			require.Equal(t, tt.expected, schema)
		})
	}
}

func TestMCPToolAdapter_Annotations(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "test-tool",
		Description: "A test tool",
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")
	annotations := adapter.Annotations()

	require.NotNil(t, annotations)
	require.Equal(t, "test-tool (MCP:test-server)", annotations.Title)
	require.False(t, annotations.ReadOnlyHint)
	require.False(t, annotations.DestructiveHint)
	require.False(t, annotations.IdempotentHint)
	require.True(t, annotations.OpenWorldHint)
}

func TestMCPToolAdapter_Call_NotConnected(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "test-tool",
		Description: "A test tool",
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")

	ctx := context.Background()
	input := map[string]interface{}{"param": "value"}

	result, err := adapter.Call(ctx, input)
	require.NoError(t, err) // Tool adapter handles errors by returning error result
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	require.Equal(t, dive.ToolResultContentTypeText, result.Content[0].Type)
	require.Contains(t, result.Content[0].Text, "MCP tool call failed")
}

func TestMCPToolAdapter_Call_InputFormats(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "test-tool",
		Description: "A test tool",
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")
	ctx := context.Background()

	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name:  "map input",
			input: map[string]interface{}{"param": "value"},
		},
		{
			name:  "json raw message",
			input: json.RawMessage(`{"param": "value"}`),
		},
		{
			name: "struct input",
			input: struct {
				Param string `json:"param"`
			}{
				Param: "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.Call(ctx, tt.input)
			require.NoError(t, err)
			require.NotNil(t, result)
			// All should fail because we're not connected, but they should handle input conversion
			require.True(t, result.IsError)
			require.Contains(t, result.Content[0].Text, "MCP tool call failed")
		})
	}
}

func TestMCPToolAdapter_Call_InvalidJSON(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "test-tool",
		Description: "A test tool",
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")
	ctx := context.Background()

	// Test with invalid JSON
	invalidJSON := json.RawMessage(`{"invalid": json}`)
	result, err := adapter.Call(ctx, invalidJSON)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "Failed to unmarshal input")
}

func TestMCPToolAdapter_Call_EmptyInputs(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	mcpTool := mcp.Tool{
		Name:        "test-tool",
		Description: "A test tool",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
	}

	adapter := NewToolAdapter(client, mcpTool, "test-server")
	ctx := context.Background()

	// Test cases for empty inputs - they should all be handled properly
	// and not cause unmarshaling errors
	testCases := []struct {
		name  string
		input interface{}
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "empty json raw message",
			input: json.RawMessage(""),
		},
		{
			name:  "json empty string",
			input: json.RawMessage(`""`),
		},
		{
			name:  "empty object",
			input: map[string]interface{}{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := adapter.Call(ctx, tc.input)
			require.NoError(t, err)
			require.NotNil(t, result)
			// Should fail because client is not connected, but the error should be
			// about connection, not JSON unmarshaling
			require.True(t, result.IsError)
			require.Contains(t, result.Content[0].Text, "MCP tool call failed")
			// Should NOT contain unmarshaling error
			require.NotContains(t, result.Content[0].Text, "Failed to unmarshal")
		})
	}
}

func TestConvertMCPSchemaToDiv(t *testing.T) {
	tests := []struct {
		name      string
		mcpSchema map[string]interface{}
		expected  *schema.Property
	}{
		{
			name: "simple string property",
			mcpSchema: map[string]interface{}{
				"type":        "string",
				"description": "A string parameter",
			},
			expected: &schema.Property{
				Type:        "string",
				Description: "A string parameter",
			},
		},
		{
			name: "object with properties",
			mcpSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "The name",
					},
				},
				"required": []interface{}{"name"},
			},
			expected: &schema.Property{
				Type: "object",
				Properties: map[string]*schema.Property{
					"name": {
						Type:        "string",
						Description: "The name",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			name: "array with items",
			mcpSchema: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			expected: &schema.Property{
				Type: "array",
				Items: &schema.Property{
					Type: "string",
				},
			},
		},
		{
			name: "enum property",
			mcpSchema: map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"option1", "option2"},
			},
			expected: &schema.Property{
				Type: "string",
				Enum: []string{"option1", "option2"},
			},
		},
		{
			name: "with additional properties",
			mcpSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			expected: &schema.Property{
				Type:                 "object",
				AdditionalProperties: boolPtr(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMCPSchemaToDiv(tt.mcpSchema)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertMCPResultToDive(t *testing.T) {
	tests := []struct {
		name      string
		mcpResult *mcp.CallToolResult
		expected  *dive.ToolResult
		expectErr bool
	}{
		{
			name:      "nil result",
			mcpResult: nil,
			expected: &dive.ToolResult{
				Content: []*dive.ToolResultContent{
					{
						Type: dive.ToolResultContentTypeText,
						Text: "mcp tool returned nil result",
					},
				},
				IsError: true,
			},
		},
		{
			name: "text content",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Hello, world!",
					},
				},
				IsError: false,
			},
			expected: &dive.ToolResult{
				Content: []*dive.ToolResultContent{
					{
						Type: dive.ToolResultContentTypeText,
						Text: "Hello, world!",
					},
				},
				IsError: false,
			},
		},
		{
			name: "image content",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.ImageContent{
						Type:     "image",
						Data:     "base64imagedata",
						MIMEType: "image/png",
					},
				},
				IsError: false,
			},
			expected: &dive.ToolResult{
				Content: []*dive.ToolResultContent{
					{
						Type:     dive.ToolResultContentTypeImage,
						Data:     "base64imagedata",
						MimeType: "image/png",
					},
				},
				IsError: false,
			},
		},
		{
			name: "error result",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Error occurred",
					},
				},
				IsError: true,
			},
			expected: &dive.ToolResult{
				Content: []*dive.ToolResultContent{
					{
						Type: dive.ToolResultContentTypeText,
						Text: "Error occurred",
					},
				},
				IsError: true,
			},
		},
		{
			name: "multiple content items",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "First content",
					},
					mcp.TextContent{
						Type: "text",
						Text: "Second content",
					},
				},
				IsError: false,
			},
			expected: &dive.ToolResult{
				Content: []*dive.ToolResultContent{
					{
						Type: dive.ToolResultContentTypeText,
						Text: "First content",
					},
					{
						Type: dive.ToolResultContentTypeText,
						Text: "Second content",
					},
				},
				IsError: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertMCPResultToDive(tt.mcpResult)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertMCPResultToDive_ContentTypes(t *testing.T) {
	tests := []struct {
		name           string
		mcpResult      *mcp.CallToolResult
		expectedLength int
		expectedType   dive.ToolResultContentType
		expectedText   string
		expectedData   string
		expectedMime   string
		hasAnnotations bool
	}{
		{
			name: "TextContent",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Hello world",
						Annotated: mcp.Annotated{
							Annotations: &mcp.Annotations{
								Priority: 0.8,
								Audience: []mcp.Role{mcp.RoleUser},
							},
						},
					},
				},
			},
			expectedLength: 1,
			expectedType:   dive.ToolResultContentTypeText,
			expectedText:   "Hello world",
			// hasAnnotations: true,
		},
		{
			name: "ImageContent",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.ImageContent{
						Type:     "image",
						Data:     "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						MIMEType: "image/png",
					},
				},
			},
			expectedLength: 1,
			expectedType:   dive.ToolResultContentTypeImage,
			expectedData:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			expectedMime:   "image/png",
		},
		{
			name: "AudioContent",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.AudioContent{
						Type:     "audio",
						Data:     "UklGRiQAAABXQVZFZm10IBAAAAABAAEAgLsAAADuAgAEABAAZGF0YQAAAAAA",
						MIMEType: "audio/wav",
					},
				},
			},
			expectedLength: 1,
			expectedType:   dive.ToolResultContentTypeAudio,
			expectedData:   "UklGRiQAAABXQVZFZm10IBAAAAABAAEAgLsAAADuAgAEABAAZGF0YQAAAAAA",
			expectedMime:   "audio/wav",
		},
		{
			name: "EmbeddedResource_Text",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.EmbeddedResource{
						Type: "resource",
						Resource: mcp.TextResourceContents{
							URI:      "file:///example.txt",
							MIMEType: "text/plain",
							Text:     "File content here",
						},
					},
				},
			},
			expectedLength: 1,
			expectedType:   dive.ToolResultContentTypeText,
			expectedText:   "File content here",
			hasAnnotations: true,
		},
		{
			name: "EmbeddedResource_Blob",
			mcpResult: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.EmbeddedResource{
						Type: "resource",
						Resource: mcp.BlobResourceContents{
							URI:      "file:///example.bin",
							MIMEType: "application/octet-stream",
							Blob:     "SGVsbG8gd29ybGQ=",
						},
					},
				},
			},
			expectedLength: 1,
			expectedType:   dive.ToolResultContentTypeText,
			expectedText:   "Binary resource: file:///example.bin (application/octet-stream)",
			hasAnnotations: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertMCPResultToDive(tt.mcpResult)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.Content, tt.expectedLength)

			content := result.Content[0]
			require.Equal(t, tt.expectedType, content.Type)

			if tt.expectedText != "" {
				require.Equal(t, tt.expectedText, content.Text)
			}
			if tt.expectedData != "" {
				require.Equal(t, tt.expectedData, content.Data)
			}
			if tt.expectedMime != "" {
				require.Equal(t, tt.expectedMime, content.MimeType)
			}
		})
	}
}

func TestConvertMCPResultToDive_Annotations(t *testing.T) {
	mcpResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Test with annotations",
				// Annotated: mcp.Annotated{
				// 	Annotations: &mcp.Annotations{
				// 		Priority: 0.9,
				// 		Audience: []mcp.Role{mcp.RoleUser, mcp.RoleAssistant},
				// 	},
				// },
			},
		},
	}

	result, err := ConvertMCPResultToDive(mcpResult)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)

	// content := result.Content[0]
	// require.NotNil(t, content.Annotations)

	// // Check that MCP annotations are properly converted
	// require.Equal(t, 0.9, content.Annotations["mcp_priority"])
	// audience, ok := content.Annotations["mcp_audience"].([]string)
	// require.True(t, ok)
	// require.Contains(t, audience, "user")
	// require.Contains(t, audience, "assistant")
}

func TestConvertMCPResultToDive_ErrorResult(t *testing.T) {
	mcpResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "An error occurred",
			},
		},
		IsError: true,
	}

	result, err := ConvertMCPResultToDive(mcpResult)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Equal(t, "An error occurred", result.Content[0].Text)
}

func TestConvertMCPResultToDive_NilResult(t *testing.T) {
	result, err := ConvertMCPResultToDive(nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Equal(t, "mcp tool returned nil result", result.Content[0].Text)
}
