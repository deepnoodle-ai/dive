package dive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestFileSessionRepository_NewFileSessionRepository(t *testing.T) {
	t.Run("creates directory if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		sessionsDir := filepath.Join(tmpDir, "sessions")

		repo, err := NewFileSessionRepository(sessionsDir)
		assert.NoError(t, err)
		assert.NotNil(t, repo)

		// Check directory was created
		info, err := os.Stat(sessionsDir)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("works with existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		sessionsDir := filepath.Join(tmpDir, "sessions")
		assert.NoError(t, os.Mkdir(sessionsDir, 0755))

		repo, err := NewFileSessionRepository(sessionsDir)
		assert.NoError(t, err)
		assert.NotNil(t, repo)
	})

	t.Run("expands tilde in path", func(t *testing.T) {
		// Skip if running as root or if HOME isn't set
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot determine home directory")
		}

		// Use a unique temp directory under home with consistent timestamp
		timestamp := time.Now().Format("20060102150405")
		uniqueDir := filepath.Join(home, ".dive-test-"+timestamp)
		defer os.RemoveAll(uniqueDir)

		repo, err := NewFileSessionRepository("~/.dive-test-" + timestamp)
		assert.NoError(t, err)
		assert.NotNil(t, repo)
	})
}

func TestFileSessionRepository_PutAndGet(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	t.Run("put and get session", func(t *testing.T) {
		session := &Session{
			ID:        "session-123",
			UserID:    "user-1",
			AgentID:   "agent-1",
			AgentName: "Test Agent",
			Title:     "Test Conversation",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages: []*llm.Message{
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
				{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hi there!"}}},
			},
			Metadata: map[string]interface{}{
				"workspace": "/path/to/project",
			},
		}

		err := repo.PutSession(ctx, session)
		assert.NoError(t, err)

		// Verify file was created
		_, err = os.Stat(filepath.Join(tmpDir, "session-123.json"))
		assert.NoError(t, err)

		// Get the session back
		retrieved, err := repo.GetSession(ctx, "session-123")
		assert.NoError(t, err)
		assert.Equal(t, session.ID, retrieved.ID)
		assert.Equal(t, session.UserID, retrieved.UserID)
		assert.Equal(t, session.Title, retrieved.Title)
		assert.Len(t, retrieved.Messages, 2)
		assert.Equal(t, "/path/to/project", retrieved.Metadata["workspace"])
	})

	t.Run("update existing session", func(t *testing.T) {
		session := &Session{
			ID:        "session-update",
			Title:     "Original Title",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := repo.PutSession(ctx, session)
		assert.NoError(t, err)

		// Update the session
		session.Title = "Updated Title"
		session.UpdatedAt = time.Now()
		err = repo.PutSession(ctx, session)
		assert.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetSession(ctx, "session-update")
		assert.NoError(t, err)
		assert.Equal(t, "Updated Title", retrieved.Title)
	})
}

func TestFileSessionRepository_GetSession_NotFound(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	_, err = repo.GetSession(ctx, "nonexistent")
	assert.Equal(t, ErrSessionNotFound, err)
}

func TestFileSessionRepository_DeleteSession(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	t.Run("delete existing session", func(t *testing.T) {
		session := &Session{
			ID:        "session-delete",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := repo.PutSession(ctx, session)
		assert.NoError(t, err)

		err = repo.DeleteSession(ctx, "session-delete")
		assert.NoError(t, err)

		// Verify deletion
		_, err = repo.GetSession(ctx, "session-delete")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("delete nonexistent session is idempotent", func(t *testing.T) {
		err := repo.DeleteSession(ctx, "nonexistent")
		assert.NoError(t, err)
	})
}

func TestFileSessionRepository_ListSessions(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	// Create multiple sessions with different timestamps
	now := time.Now()
	sessions := []*Session{
		{ID: "session-1", Title: "First", CreatedAt: now, UpdatedAt: now.Add(-3 * time.Hour)},
		{ID: "session-2", Title: "Second", CreatedAt: now, UpdatedAt: now.Add(-1 * time.Hour)},
		{ID: "session-3", Title: "Third", CreatedAt: now, UpdatedAt: now.Add(-2 * time.Hour)},
	}
	for _, session := range sessions {
		assert.NoError(t, repo.PutSession(ctx, session))
	}

	t.Run("list all sessions sorted by UpdatedAt desc", func(t *testing.T) {
		result, err := repo.ListSessions(ctx, nil)
		assert.NoError(t, err)
		assert.Len(t, result.Items, 3)

		// Should be sorted most recent first
		assert.Equal(t, "session-2", result.Items[0].ID) // -1 hour
		assert.Equal(t, "session-3", result.Items[1].ID) // -2 hours
		assert.Equal(t, "session-1", result.Items[2].ID) // -3 hours
	})

	t.Run("list with limit", func(t *testing.T) {
		result, err := repo.ListSessions(ctx, &ListSessionsInput{Limit: 2})
		assert.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, "session-2", result.Items[0].ID)
		assert.Equal(t, "session-3", result.Items[1].ID)
	})

	t.Run("list with offset", func(t *testing.T) {
		result, err := repo.ListSessions(ctx, &ListSessionsInput{Offset: 1})
		assert.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, "session-3", result.Items[0].ID)
		assert.Equal(t, "session-1", result.Items[1].ID)
	})

	t.Run("list with offset and limit", func(t *testing.T) {
		result, err := repo.ListSessions(ctx, &ListSessionsInput{Offset: 1, Limit: 1})
		assert.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "session-3", result.Items[0].ID)
	})

	t.Run("list with offset beyond count returns empty", func(t *testing.T) {
		result, err := repo.ListSessions(ctx, &ListSessionsInput{Offset: 10})
		assert.NoError(t, err)
		assert.Len(t, result.Items, 0)
	})
}

func TestFileSessionRepository_ListSessions_EmptyDirectory(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	result, err := repo.ListSessions(ctx, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Items, 0)
}

func TestFileSessionRepository_ForkSession(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	t.Run("fork creates new session with copied messages", func(t *testing.T) {
		original := &Session{
			ID:        "session-original",
			UserID:    "user-1",
			AgentID:   "agent-1",
			AgentName: "Test Agent",
			Title:     "Original Session",
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
			Messages: []*llm.Message{
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
				{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "World"}}},
			},
			Metadata: map[string]interface{}{
				"workspace": "/original/path",
			},
		}
		assert.NoError(t, repo.PutSession(ctx, original))

		forked, err := repo.ForkSession(ctx, "session-original")
		assert.NoError(t, err)
		assert.NotNil(t, forked)

		// Check forked session has new ID
		assert.NotEqual(t, original.ID, forked.ID)
		assert.True(t, len(forked.ID) > 0)

		// Check metadata is copied
		assert.Equal(t, original.UserID, forked.UserID)
		assert.Equal(t, original.AgentID, forked.AgentID)
		assert.Equal(t, original.Title, forked.Title)
		assert.Equal(t, "/original/path", forked.Metadata["workspace"])

		// Check messages are copied
		assert.Len(t, forked.Messages, 2)

		// Check timestamps are new
		assert.True(t, forked.CreatedAt.After(original.CreatedAt))

		// Verify forked session is persisted
		retrieved, err := repo.GetSession(ctx, forked.ID)
		assert.NoError(t, err)
		assert.Equal(t, forked.ID, retrieved.ID)

		// Verify original is unchanged
		originalRetrieved, err := repo.GetSession(ctx, "session-original")
		assert.NoError(t, err)
		assert.Equal(t, "session-original", originalRetrieved.ID)
	})

	t.Run("fork nonexistent session returns error", func(t *testing.T) {
		_, err := repo.ForkSession(ctx, "nonexistent")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("forked messages are independent", func(t *testing.T) {
		original := &Session{
			ID:        "session-independent",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages: []*llm.Message{
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Original message"}}},
			},
		}
		assert.NoError(t, repo.PutSession(ctx, original))

		forked, err := repo.ForkSession(ctx, "session-independent")
		assert.NoError(t, err)

		// Modify forked message
		forked.Messages[0].Content = []llm.Content{&llm.TextContent{Text: "Modified message"}}
		assert.NoError(t, repo.PutSession(ctx, forked))

		// Original should be unchanged
		originalRetrieved, err := repo.GetSession(ctx, "session-independent")
		assert.NoError(t, err)
		originalText := originalRetrieved.Messages[0].Content[0].(*llm.TextContent).Text
		assert.Equal(t, "Original message", originalText)
	})
}

func TestFileSessionRepository_IgnoresNonJSONFiles(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo, err := NewFileSessionRepository(tmpDir)
	assert.NoError(t, err)

	// Create a valid session
	session := &Session{
		ID:        "valid-session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	assert.NoError(t, repo.PutSession(ctx, session))

	// Create non-JSON files that should be ignored
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("test"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("test"), 0644))
	assert.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755))

	// List should only return the valid session
	result, err := repo.ListSessions(ctx, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "valid-session", result.Items[0].ID)
}
