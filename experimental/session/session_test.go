package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMemoryRepository(t *testing.T) {
	t.Run("put and get session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		session := &Session{
			ID:        "test-session",
			UserID:    "user-123",
			CreatedAt: time.Now(),
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
			},
		}

		err := repo.PutSession(ctx, session)
		assert.NoError(t, err)

		retrieved, err := repo.GetSession(ctx, "test-session")
		assert.NoError(t, err)
		assert.Equal(t, "test-session", retrieved.ID)
		assert.Equal(t, "user-123", retrieved.UserID)
		assert.Equal(t, 1, len(retrieved.Messages))
	})

	t.Run("get non-existent session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		_, err := repo.GetSession(ctx, "non-existent")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("delete session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		session := &Session{ID: "to-delete"}
		err := repo.PutSession(ctx, session)
		assert.NoError(t, err)

		err = repo.DeleteSession(ctx, "to-delete")
		assert.NoError(t, err)

		_, err = repo.GetSession(ctx, "to-delete")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("delete non-existent session is idempotent", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		err := repo.DeleteSession(ctx, "non-existent")
		assert.NoError(t, err)
	})

	t.Run("list sessions with pagination", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		for i := 0; i < 5; i++ {
			repo.PutSession(ctx, &Session{ID: "session-" + string(rune('A'+i))})
		}

		// List all
		result, err := repo.ListSessions(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 5, len(result.Items))

		// List with limit
		result, err = repo.ListSessions(ctx, &ListSessionsInput{Limit: 2})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result.Items))

		// List with offset
		result, err = repo.ListSessions(ctx, &ListSessionsInput{Offset: 3})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result.Items))

		// List with offset beyond range
		result, err = repo.ListSessions(ctx, &ListSessionsInput{Offset: 10})
		assert.NoError(t, err)
		assert.Equal(t, 0, len(result.Items))
	})

	t.Run("fork session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		original := &Session{
			ID:        "original",
			UserID:    "user-123",
			Title:     "Original Title",
			Messages:  []*llm.Message{llm.NewUserTextMessage("Hello")},
			Metadata:  map[string]interface{}{"key": "value"},
			CreatedAt: time.Now(),
		}
		repo.PutSession(ctx, original)

		forked, err := repo.ForkSession(ctx, "original")
		assert.NoError(t, err)
		assert.NotEqual(t, "original", forked.ID)
		assert.Equal(t, "user-123", forked.UserID)
		assert.Equal(t, "Original Title", forked.Title)
		assert.Equal(t, 1, len(forked.Messages))
		assert.Equal(t, "Hello", forked.Messages[0].Text())
		assert.Equal(t, "value", forked.Metadata["key"])

		// Verify forked session is stored
		stored, err := repo.GetSession(ctx, forked.ID)
		assert.NoError(t, err)
		assert.Equal(t, forked.ID, stored.ID)
	})

	t.Run("fork non-existent session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		_, err := repo.ForkSession(ctx, "non-existent")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("with sessions initializer", func(t *testing.T) {
		sessions := []*Session{
			{ID: "s1"},
			{ID: "s2"},
		}
		repo := NewMemoryRepository().WithSessions(sessions)
		ctx := context.Background()

		s1, err := repo.GetSession(ctx, "s1")
		assert.NoError(t, err)
		assert.Equal(t, "s1", s1.ID)

		s2, err := repo.GetSession(ctx, "s2")
		assert.NoError(t, err)
		assert.Equal(t, "s2", s2.ID)
	})
}

func TestFileRepository(t *testing.T) {
	t.Run("put and get session", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewFileRepository(dir)
		assert.NoError(t, err)

		ctx := context.Background()

		session := &Session{
			ID:        "test-session",
			UserID:    "user-123",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
			},
		}

		err = repo.PutSession(ctx, session)
		assert.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(filepath.Join(dir, "test-session.json"))
		assert.NoError(t, err)

		retrieved, err := repo.GetSession(ctx, "test-session")
		assert.NoError(t, err)
		assert.Equal(t, "test-session", retrieved.ID)
		assert.Equal(t, "user-123", retrieved.UserID)
	})

	t.Run("get non-existent session", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewFileRepository(dir)
		assert.NoError(t, err)

		ctx := context.Background()

		_, err = repo.GetSession(ctx, "non-existent")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("delete session", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewFileRepository(dir)
		assert.NoError(t, err)

		ctx := context.Background()

		session := &Session{ID: "to-delete"}
		repo.PutSession(ctx, session)

		err = repo.DeleteSession(ctx, "to-delete")
		assert.NoError(t, err)

		_, err = os.Stat(filepath.Join(dir, "to-delete.json"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("list sessions sorted by UpdatedAt", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewFileRepository(dir)
		assert.NoError(t, err)

		ctx := context.Background()

		// Create sessions with different UpdatedAt times
		for i := 0; i < 3; i++ {
			repo.PutSession(ctx, &Session{
				ID:        "session-" + string(rune('A'+i)),
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Hour),
			})
		}

		result, err := repo.ListSessions(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result.Items))

		// Should be sorted by UpdatedAt descending
		assert.Equal(t, "session-C", result.Items[0].ID)
		assert.Equal(t, "session-B", result.Items[1].ID)
		assert.Equal(t, "session-A", result.Items[2].ID)
	})

	t.Run("fork session", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewFileRepository(dir)
		assert.NoError(t, err)

		ctx := context.Background()

		original := &Session{
			ID:        "original",
			UserID:    "user-123",
			Messages:  []*llm.Message{llm.NewUserTextMessage("Hello")},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		repo.PutSession(ctx, original)

		forked, err := repo.ForkSession(ctx, "original")
		assert.NoError(t, err)
		assert.NotEqual(t, "original", forked.ID)
		assert.Equal(t, "user-123", forked.UserID)
		assert.Equal(t, 1, len(forked.Messages))

		// Verify forked session file exists
		_, err = os.Stat(filepath.Join(dir, forked.ID+".json"))
		assert.NoError(t, err)
	})
}

func TestSessionHooks(t *testing.T) {
	t.Run("loader prepends session messages", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		// Create existing session with history
		existing := &Session{
			ID: "test-session",
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Previous message 1"),
				llm.NewAssistantTextMessage("Previous response 1"),
			},
		}
		repo.PutSession(ctx, existing)

		loader := Loader(repo)

		state := dive.NewGenerationState()
		state.SessionID = "test-session"
		state.Messages = []*llm.Message{llm.NewUserTextMessage("New message")}

		err := loader(ctx, state)
		assert.NoError(t, err)

		assert.Equal(t, 3, len(state.Messages))
		assert.Equal(t, "Previous message 1", state.Messages[0].Text())
		assert.Equal(t, "Previous response 1", state.Messages[1].Text())
		assert.Equal(t, "New message", state.Messages[2].Text())

		// Verify session is stored in Values
		storedSession, ok := state.Values["session"].(*Session)
		assert.True(t, ok)
		assert.Equal(t, "test-session", storedSession.ID)
	})

	t.Run("loader handles missing session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		loader := Loader(repo)

		state := dive.NewGenerationState()
		state.SessionID = "non-existent"
		state.Messages = []*llm.Message{llm.NewUserTextMessage("New message")}

		err := loader(ctx, state)
		assert.NoError(t, err)

		// Messages should be unchanged
		assert.Equal(t, 1, len(state.Messages))
	})

	t.Run("loader handles empty session ID", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		loader := Loader(repo)

		state := dive.NewGenerationState()
		state.SessionID = ""
		state.Messages = []*llm.Message{llm.NewUserTextMessage("New message")}

		err := loader(ctx, state)
		assert.NoError(t, err)

		assert.Equal(t, 1, len(state.Messages))
	})

	t.Run("saver creates new session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		saver := Saver(repo)

		state := dive.NewGenerationState()
		state.SessionID = "new-session"
		state.UserID = "user-123"
		state.Messages = []*llm.Message{llm.NewUserTextMessage("Hello")}
		state.OutputMessages = []*llm.Message{llm.NewAssistantTextMessage("Hi there")}

		err := saver(ctx, state)
		assert.NoError(t, err)

		// Verify session was saved
		saved, err := repo.GetSession(ctx, "new-session")
		assert.NoError(t, err)
		assert.Equal(t, "new-session", saved.ID)
		assert.Equal(t, "user-123", saved.UserID)
		assert.Equal(t, 2, len(saved.Messages))
	})

	t.Run("saver updates existing session", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		// Pre-load session via loader
		existing := &Session{
			ID:        "existing-session",
			CreatedAt: time.Now().Add(-time.Hour),
			Messages:  []*llm.Message{llm.NewUserTextMessage("Old message")},
		}
		repo.PutSession(ctx, existing)

		// Simulate loader storing session
		state := dive.NewGenerationState()
		state.SessionID = "existing-session"
		state.Values["session"] = existing
		state.Messages = existing.Messages
		state.OutputMessages = []*llm.Message{llm.NewAssistantTextMessage("New response")}

		saver := Saver(repo)
		err := saver(ctx, state)
		assert.NoError(t, err)

		saved, err := repo.GetSession(ctx, "existing-session")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(saved.Messages))
		// Should preserve original CreatedAt
		assert.True(t, saved.CreatedAt.Before(saved.UpdatedAt))
	})

	t.Run("saver handles empty session ID", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		saver := Saver(repo)

		state := dive.NewGenerationState()
		state.SessionID = ""
		state.Messages = []*llm.Message{llm.NewUserTextMessage("Hello")}

		err := saver(ctx, state)
		assert.NoError(t, err)

		// Nothing should be saved
		result, _ := repo.ListSessions(ctx, nil)
		assert.Equal(t, 0, len(result.Items))
	})

	t.Run("hooks function returns both hooks", func(t *testing.T) {
		repo := NewMemoryRepository()

		preHook, postHook := Hooks(repo)
		assert.NotNil(t, preHook)
		assert.NotNil(t, postHook)
	})
}

func TestLoaderWithOptions(t *testing.T) {
	t.Run("calls OnSessionLoaded callback", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		existing := &Session{
			ID:       "test-session",
			Messages: []*llm.Message{llm.NewUserTextMessage("Hello")},
		}
		repo.PutSession(ctx, existing)

		var loadedSession *Session
		loader := LoaderWithOptions{
			Repository: repo,
			OnSessionLoaded: func(ctx context.Context, s *Session) {
				loadedSession = s
			},
		}.Build()

		state := dive.NewGenerationState()
		state.SessionID = "test-session"
		state.Messages = []*llm.Message{}

		err := loader(ctx, state)
		assert.NoError(t, err)
		assert.NotNil(t, loadedSession)
		assert.Equal(t, "test-session", loadedSession.ID)
	})
}

func TestSaverWithOptions(t *testing.T) {
	t.Run("sets AgentID and AgentName on new sessions", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		saver := SaverWithOptions{
			Repository: repo,
			AgentID:    "agent-123",
			AgentName:  "TestAgent",
		}.Build()

		state := dive.NewGenerationState()
		state.SessionID = "new-session"
		state.Messages = []*llm.Message{llm.NewUserTextMessage("Hello")}
		state.OutputMessages = []*llm.Message{}

		err := saver(ctx, state)
		assert.NoError(t, err)

		saved, _ := repo.GetSession(ctx, "new-session")
		assert.Equal(t, "agent-123", saved.AgentID)
		assert.Equal(t, "TestAgent", saved.AgentName)
	})

	t.Run("calls OnSessionSaved callback", func(t *testing.T) {
		repo := NewMemoryRepository()
		ctx := context.Background()

		var savedSession *Session
		saver := SaverWithOptions{
			Repository: repo,
			OnSessionSaved: func(ctx context.Context, s *Session) {
				savedSession = s
			},
		}.Build()

		state := dive.NewGenerationState()
		state.SessionID = "test-session"
		state.Messages = []*llm.Message{llm.NewUserTextMessage("Hello")}
		state.OutputMessages = []*llm.Message{}

		err := saver(ctx, state)
		assert.NoError(t, err)
		assert.NotNil(t, savedSession)
		assert.Equal(t, "test-session", savedSession.ID)
	})
}
