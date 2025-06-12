package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/require"
)

// MockLLM is a simple mock for testing
type MockLLM struct {
	responses []string
	callCount int
}

func (m *MockLLM) Name() string {
	return "mock"
}

func (m *MockLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	// Capture the messages for verification in tests
	m.callCount++

	return &llm.Response{
		Usage: llm.Usage{},
	}, nil
}

func TestAgentContextMessages(t *testing.T) {
	// Create test files
	tmpDir := t.TempDir()
	contextFile := filepath.Join(tmpDir, "context.txt")
	contextContent := "This is important context information."
	err := os.WriteFile(contextFile, []byte(contextContent), 0644)
	require.NoError(t, err)

	// Test context message building directly
	contextEntries := []Content{
		{Path: contextFile},
		{Text: "Additional inline context"},
	}

	messages, err := buildContextContent(context.Background(), nil, "", contextEntries)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	// First message should be the file content
	firstContent := messages[0]
	textContent, ok := firstContent.(*llm.TextContent)
	require.True(t, ok)
	require.Equal(t, contextContent, textContent.Text)

	// Second message should be the inline content
	secondContent := messages[1]
	inlineContent, ok := secondContent.(*llm.TextContent)
	require.True(t, ok)
	require.Equal(t, "Additional inline context", inlineContent.Text)

	// Test agent creation with context messages
	mockLLM := &MockLLM{responses: []string{"Agent response with context"}}

	agentWithContext, err := agent.New(agent.Options{
		Name:    "TestAgent",
		Model:   mockLLM,
		Context: messages,
	})
	require.NoError(t, err)
	require.NotNil(t, agentWithContext)

	// Verify agent has context messages
	contextMsgs := agentWithContext.Context()
	require.Len(t, contextMsgs, 2)
	require.Equal(t, messages, contextMsgs)
}

func TestWorkflowStepWithContext(t *testing.T) {
	// Create test files
	tmpDir := t.TempDir()
	stepContextFile := filepath.Join(tmpDir, "step_context.md")
	stepContextContent := "# Step Context\nThis is step-specific context."
	err := os.WriteFile(stepContextFile, []byte(stepContextContent), 0644)
	require.NoError(t, err)

	// Create mock agent
	mockLLM := &MockLLM{responses: []string{"Step response with context"}}
	mockAgent, err := agent.New(agent.Options{
		Name:  "MockAgent",
		Model: mockLLM,
	})
	require.NoError(t, err)

	// Build context messages for step
	contextEntries := []Content{
		{Path: stepContextFile},
		{Text: "Step inline context"},
	}

	contextMessages, err := buildContextContent(context.Background(), nil, "", contextEntries)
	require.NoError(t, err)
	require.Len(t, contextMessages, 2)

	// Build workflow with step context using buildWorkflow
	workflowDef := Workflow{
		Name: "TestWorkflow",
		Steps: []Step{
			{
				Name:    "TestStep",
				Type:    "prompt",
				Prompt:  "Process the context and respond",
				Content: contextEntries,
			},
		},
	}

	builtWorkflow, err := buildWorkflow(context.Background(), nil, workflowDef, []dive.Agent{mockAgent}, "")
	require.NoError(t, err)
	require.NotNil(t, builtWorkflow)
	require.Len(t, builtWorkflow.Steps(), 1)

	// Verify step has context messages
	step := builtWorkflow.Steps()[0]
	stepContent := step.Content()
	require.Len(t, stepContent, 2)

	// First message should be the file content
	firstMsg := stepContent[0]
	textContent, ok := firstMsg.(*llm.TextContent)
	require.True(t, ok)
	require.Equal(t, stepContextContent, textContent.Text)

	// Second message should be the inline content
	secondMsg := stepContent[1]
	inlineContent, ok := secondMsg.(*llm.TextContent)
	require.True(t, ok)
	require.Equal(t, "Step inline context", inlineContent.Text)
}

func TestMixedContextTypes(t *testing.T) {
	// Create different types of test files
	tmpDir := t.TempDir()

	textFile := filepath.Join(tmpDir, "document.txt")
	err := os.WriteFile(textFile, []byte("Text document content"), 0644)
	require.NoError(t, err)

	markdownFile := filepath.Join(tmpDir, "readme.md")
	err = os.WriteFile(markdownFile, []byte("# Markdown\nThis is **markdown** content"), 0644)
	require.NoError(t, err)

	imageFile := filepath.Join(tmpDir, "image.png")
	err = os.WriteFile(imageFile, []byte("fake-png-data"), 0644)
	require.NoError(t, err)

	unknownFile := filepath.Join(tmpDir, "data.bin")
	err = os.WriteFile(unknownFile, []byte("binary data"), 0644)
	require.NoError(t, err)

	entries := []Content{
		{Path: textFile},
		{Path: markdownFile},
		{Path: imageFile},
		{Path: unknownFile},
		{URL: "https://example.com/remote.pdf"},
		{URL: "https://example.com/image.jpg"},
		{Text: "Text text context"},
	}

	content, err := buildContextContent(context.Background(), nil, "", entries)
	require.NoError(t, err)
	require.Len(t, content, 7)

	// Verify content types
	contentTypes := make([]string, len(content))
	for i, c := range content {
		switch c.(type) {
		case *llm.TextContent:
			contentTypes[i] = "text"
		case *llm.ImageContent:
			contentTypes[i] = "image"
		case *llm.DocumentContent:
			contentTypes[i] = "document"
		default:
			contentTypes[i] = "unknown"
		}
	}

	expected := []string{"text", "text", "image", "document", "document", "image", "text"}
	require.Equal(t, expected, contentTypes)
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		entries   []Content
		expectErr bool
	}{
		{
			name: "empty context entry",
			entries: []Content{
				{}, // All fields empty
			},
			expectErr: true,
		},
		{
			name: "multiple fields set",
			entries: []Content{
				{Text: "text", Path: "/some/path"}, // Both Text and Path set
			},
			expectErr: true,
		},
		{
			name: "all fields set",
			entries: []Content{
				{
					Text: "text",
					Path: "/some/path",
					URL:  "https://example.com",
				},
			},
			expectErr: true,
		},
		{
			name: "valid context - non-existent file",
			entries: []Content{
				{Path: "/definitely/non/existent/file.txt"},
			},
			expectErr: false, // Non-existent files are handled gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildContextContent(context.Background(), nil, "", tt.entries)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
