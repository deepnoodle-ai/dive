package memory_test

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/memory"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMemoryService(t *testing.T) {
	ctx := context.Background()

	t.Run("save and search", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Save(ctx, memory.Entry{Content: "The user prefers dark mode in all applications"})
		assert.NoError(t, err)
		err = svc.Save(ctx, memory.Entry{Content: "The user speaks French fluently"})
		assert.NoError(t, err)

		results, err := svc.Search(ctx, "dark mode UI preferences", 5)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, "The user prefers dark mode in all applications", results[0].Content)
	})

	t.Run("ID is auto-assigned when empty", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Save(ctx, memory.Entry{Content: "test"})
		assert.NoError(t, err)

		results, err := svc.Search(ctx, "test", 5)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.NotEqual(t, "", results[0].ID)
	})

	t.Run("delete removes entry", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Save(ctx, memory.Entry{ID: "e1", Content: "something memorable"})
		assert.NoError(t, err)

		err = svc.Delete(ctx, "e1")
		assert.NoError(t, err)

		results, err := svc.Search(ctx, "memorable", 5)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(results))
	})

	t.Run("delete unknown ID is a no-op", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Delete(ctx, "nonexistent")
		assert.NoError(t, err)
	})

	t.Run("search respects limit", func(t *testing.T) {
		svc := memory.NewMemoryService()
		for i := range 10 {
			err := svc.Save(ctx, memory.Entry{Content: "user likes golang programming"})
			assert.NoError(t, err)
			_ = i
		}
		results, err := svc.Search(ctx, "golang programming", 3)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(results))
	})

	t.Run("search returns empty on no match", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Save(ctx, memory.Entry{Content: "user prefers coffee"})
		assert.NoError(t, err)

		results, err := svc.Search(ctx, "quantum physics", 5)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(results))
	})

	t.Run("tags improve search relevance", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Save(ctx, memory.Entry{
			Content: "User is an expert cook",
			Tags:    []string{"cooking", "food", "skills"},
		})
		assert.NoError(t, err)

		results, err := svc.Search(ctx, "cooking skills", 5)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
	})

	t.Run("update existing entry by ID", func(t *testing.T) {
		svc := memory.NewMemoryService()
		err := svc.Save(ctx, memory.Entry{ID: "e1", Content: "old content"})
		assert.NoError(t, err)
		err = svc.Save(ctx, memory.Entry{ID: "e1", Content: "updated content"})
		assert.NoError(t, err)

		results, err := svc.Search(ctx, "updated content", 5)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, "updated content", results[0].Content)
	})
}

func TestInjectMemoriesHook(t *testing.T) {
	ctx := context.Background()

	t.Run("injects relevant memories into system prompt", func(t *testing.T) {
		svc := memory.NewMemoryService()
		svc.Save(ctx, memory.Entry{ID: "m1", Content: "User prefers dark mode themes"})

		hook := memory.InjectMemoriesHook(svc, 5)
		hctx := dive.NewHookContext()
		hctx.SystemPrompt = "You are helpful."
		hctx.Messages = []*llm.Message{
			llm.NewUserTextMessage("What dark mode themes are available?"),
		}

		err := hook(ctx, hctx)
		assert.NoError(t, err)
		assert.True(t, len(hctx.SystemPrompt) > len("You are helpful."))
		assert.True(t, contains(hctx.SystemPrompt, "User prefers dark mode themes"))
		assert.True(t, contains(hctx.SystemPrompt, "<memory>"))
	})

	t.Run("no-op when no messages", func(t *testing.T) {
		svc := memory.NewMemoryService()
		hook := memory.InjectMemoriesHook(svc, 5)
		hctx := dive.NewHookContext()
		hctx.SystemPrompt = "original"
		hctx.Messages = []*llm.Message{}

		err := hook(ctx, hctx)
		assert.NoError(t, err)
		assert.Equal(t, "original", hctx.SystemPrompt)
	})

	t.Run("no-op when no relevant memories", func(t *testing.T) {
		svc := memory.NewMemoryService()
		svc.Save(ctx, memory.Entry{Content: "user likes jazz music"})

		hook := memory.InjectMemoriesHook(svc, 5)
		hctx := dive.NewHookContext()
		hctx.SystemPrompt = "original"
		hctx.Messages = []*llm.Message{
			llm.NewUserTextMessage("quantum physics question"),
		}

		err := hook(ctx, hctx)
		assert.NoError(t, err)
		assert.Equal(t, "original", hctx.SystemPrompt)
	})

	t.Run("works with empty initial system prompt", func(t *testing.T) {
		svc := memory.NewMemoryService()
		svc.Save(ctx, memory.Entry{ID: "m1", Content: "user is a developer"})

		hook := memory.InjectMemoriesHook(svc, 5)
		hctx := dive.NewHookContext()
		hctx.SystemPrompt = ""
		hctx.Messages = []*llm.Message{
			llm.NewUserTextMessage("developer tools"),
		}

		err := hook(ctx, hctx)
		assert.NoError(t, err)
		assert.True(t, contains(hctx.SystemPrompt, "<memory>"))
	})
}

func TestMemoryTools(t *testing.T) {
	tools := memory.MemoryTools(memory.NewMemoryService())
	assert.Equal(t, 2, len(tools))

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	assert.Contains(t, names, "memory_save")
	assert.Contains(t, names, "memory_search")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := range s {
		if i+len(sub) <= len(s) && s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
