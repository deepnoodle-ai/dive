package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestChatSessionPersistence(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "test-chat-session.json")

	// Create a file repository
	repo := agent.NewFileThreadRepository(sessionFile)
	err := repo.Load(ctx)
	require.NoError(t, err)

	// Create a test thread that simulates a chat conversation
	thread := &dive.Thread{
		ID:     "cli-chat", // This is the thread ID used in the chat command
		UserID: "test-user",
		Messages: []*llm.Message{
			{
				ID:   "msg1",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Hello, I'm testing the chat history feature"},
				},
			},
			{
				ID:   "msg2",
				Role: llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "Hello! I can see that you're testing the chat history feature. How can I help you?"},
				},
			},
			{
				ID:   "msg3",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Can you remember what we talked about?"},
				},
			},
			{
				ID:   "msg4",
				Role: llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "Yes! You mentioned that you're testing the chat history feature. This conversation should persist across sessions when using the --session flag."},
				},
			},
		},
	}

	// Save the thread
	err = repo.PutThread(ctx, thread)
	require.NoError(t, err)

	// Verify the file was created and contains the expected data
	_, err = os.Stat(sessionFile)
	require.NoError(t, err)

	// Load the session in a new repository to simulate restarting the chat
	newRepo := agent.NewFileThreadRepository(sessionFile)
	err = newRepo.Load(ctx)
	require.NoError(t, err)

	// Retrieve the thread
	loadedThread, err := newRepo.GetThread(ctx, "cli-chat")
	require.NoError(t, err)
	require.Equal(t, thread.ID, loadedThread.ID)
	require.Equal(t, thread.UserID, loadedThread.UserID)
	require.Len(t, loadedThread.Messages, 4)

	// Verify message content
	require.Equal(t, "Hello, I'm testing the chat history feature", loadedThread.Messages[0].Text())
	require.Equal(t, "Hello! I can see that you're testing the chat history feature. How can I help you?", loadedThread.Messages[1].Text())
	require.Equal(t, "Can you remember what we talked about?", loadedThread.Messages[2].Text())
	require.Equal(t, "Yes! You mentioned that you're testing the chat history feature. This conversation should persist across sessions when using the --session flag.", loadedThread.Messages[3].Text())

	// Verify roles
	require.Equal(t, llm.User, loadedThread.Messages[0].Role)
	require.Equal(t, llm.Assistant, loadedThread.Messages[1].Role)
	require.Equal(t, llm.User, loadedThread.Messages[2].Role)
	require.Equal(t, llm.Assistant, loadedThread.Messages[3].Role)
}

func TestChatSessionWithComplexContent(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "complex-session.json")

	repo := agent.NewFileThreadRepository(sessionFile)
	err := repo.Load(ctx)
	require.NoError(t, err)

	// Create a thread with complex content types
	thread := &dive.Thread{
		ID:     "complex-chat",
		UserID: "test-user",
		Messages: []*llm.Message{
			{
				ID:   "msg1",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Here's a question with multiple content types"},
				},
			},
			{
				ID:   "msg2",
				Role: llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "I'll use a tool to help answer that"},
					&llm.ToolUseContent{
						ID:    "tool1",
						Name:  "web_search",
						Input: []byte(`{"query": "test query"}`),
					},
				},
			},
			{
				ID:   "msg3",
				Role: llm.User,
				Content: []llm.Content{
					&llm.ToolResultContent{
						ToolUseID: "tool1",
						Content: []llm.Content{
							&llm.TextContent{Text: "Search results here"},
						},
					},
				},
			},
		},
	}

	// Save and reload to test complex content serialization
	err = repo.PutThread(ctx, thread)
	require.NoError(t, err)

	newRepo := agent.NewFileThreadRepository(sessionFile)
	err = newRepo.Load(ctx)
	require.NoError(t, err)

	loadedThread, err := newRepo.GetThread(ctx, "complex-chat")
	require.NoError(t, err)
	require.Len(t, loadedThread.Messages, 3)

	// Verify the complex content was preserved
	msg2 := loadedThread.Messages[1]
	require.Len(t, msg2.Content, 2)
	
	// Check text content
	textContent, ok := msg2.Content[0].(*llm.TextContent)
	require.True(t, ok)
	require.Equal(t, "I'll use a tool to help answer that", textContent.Text)

	// Check tool use content
	toolContent, ok := msg2.Content[1].(*llm.ToolUseContent)
	require.True(t, ok)
	require.Equal(t, "tool1", toolContent.ID)
	require.Equal(t, "web_search", toolContent.Name)
}