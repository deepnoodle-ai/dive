package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestDiskThreadRepository(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory for testing
	tmpDir := t.TempDir()

	repo := NewDiskThreadRepository(tmpDir)

	// Create a test thread
	thread := &dive.Thread{
		ID:     "test-thread-1",
		UserID: "test-user",
		Messages: []*llm.Message{
			{
				ID:   "msg1",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Hello, world!"},
				},
			},
			{
				ID:   "msg2",
				Role: llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "Hello! How can I help you today?"},
				},
			},
		},
	}

	// Put thread should save to file
	err := repo.PutThread(ctx, thread)
	require.NoError(t, err)

	// Thread file should exist now
	threadFile := filepath.Join(tmpDir, "thread-test-thread-1.json")
	_, err = os.Stat(threadFile)
	require.NoError(t, err)

	// Get thread should work
	retrievedThread, err := repo.GetThread(ctx, "test-thread-1")
	require.NoError(t, err)
	require.Equal(t, thread.ID, retrievedThread.ID)
	require.Equal(t, thread.UserID, retrievedThread.UserID)
	require.Len(t, retrievedThread.Messages, 2)
	require.Equal(t, "Hello, world!", retrievedThread.Messages[0].Text())
	require.Equal(t, "Hello! How can I help you today?", retrievedThread.Messages[1].Text())

	// Test persistence by creating new repository
	newRepo := NewDiskThreadRepository(tmpDir)

	persistedThread, err := newRepo.GetThread(ctx, "test-thread-1")
	require.NoError(t, err)
	require.Equal(t, thread.ID, persistedThread.ID)
	require.Equal(t, thread.UserID, persistedThread.UserID)
	require.Len(t, persistedThread.Messages, 2)

	// Test ListThreads
	output, err := newRepo.ListThreads(ctx, nil)
	require.NoError(t, err)
	require.Len(t, output.Items, 1)
	require.Equal(t, "test-thread-1", output.Items[0].ID)

	// Test DeleteThread
	err = newRepo.DeleteThread(ctx, "test-thread-1")
	require.NoError(t, err)

	// Thread should no longer exist
	_, err = newRepo.GetThread(ctx, "test-thread-1")
	require.Equal(t, dive.ErrThreadNotFound, err)

	// Test getting non-existent thread
	_, err = repo.GetThread(ctx, "non-existent")
	require.Equal(t, dive.ErrThreadNotFound, err)
}

func TestDiskThreadRepository_MultipleThreads(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	repo := NewDiskThreadRepository(tmpDir)

	// Add multiple threads
	for i := 0; i < 3; i++ {
		thread := &dive.Thread{
			ID:     fmt.Sprintf("thread-%d", i),
			UserID: "test-user",
			Messages: []*llm.Message{
				{
					ID:   fmt.Sprintf("msg-%d", i),
					Role: llm.User,
					Content: []llm.Content{
						&llm.TextContent{Text: fmt.Sprintf("Message %d", i)},
					},
				},
			},
		}
		err := repo.PutThread(ctx, thread)
		require.NoError(t, err)
	}

	// List all threads
	output, err := repo.ListThreads(ctx, nil)
	require.NoError(t, err)
	require.Len(t, output.Items, 3)

	// Test persistence
	newRepo := NewDiskThreadRepository(tmpDir)

	output, err = newRepo.ListThreads(ctx, nil)
	require.NoError(t, err)
	require.Len(t, output.Items, 3)
}

func TestDiskThreadRepository_UpdateExistingThread(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	repo := NewDiskThreadRepository(tmpDir)

	// Create initial thread
	thread := &dive.Thread{
		ID:     "update-test",
		UserID: "test-user",
		Messages: []*llm.Message{
			{
				ID:   "msg1",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "First message"},
				},
			},
		},
	}

	err := repo.PutThread(ctx, thread)
	require.NoError(t, err)

	// Update thread with more messages
	thread.Messages = append(thread.Messages, &llm.Message{
		ID:   "msg2",
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{Text: "Response to first message"},
		},
	})

	err = repo.PutThread(ctx, thread)
	require.NoError(t, err)

	// Verify update persisted
	retrievedThread, err := repo.GetThread(ctx, "update-test")
	require.NoError(t, err)
	require.Len(t, retrievedThread.Messages, 2)
	require.Equal(t, "First message", retrievedThread.Messages[0].Text())
	require.Equal(t, "Response to first message", retrievedThread.Messages[1].Text())
}

func TestDiskThreadRepository_InvalidFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create invalid JSON file with thread naming pattern
	invalidFile := filepath.Join(tmpDir, "thread-invalid.json")
	err := os.WriteFile(invalidFile, []byte("invalid json"), 0644)
	require.NoError(t, err)

	repo := NewDiskThreadRepository(tmpDir)

	// Should have no threads since the invalid file was skipped
	output, err := repo.ListThreads(ctx, nil)
	require.NoError(t, err)
	require.Len(t, output.Items, 0)
}
