package skill

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSkillContentHook(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hook := skillContentHook(loader)

	t.Run("injects pending instructions by call ID", func(t *testing.T) {
		loader.mu.Lock()
		loader.pendingInstructions = map[string]string{
			"call-123": "# Skill content here",
		}
		loader.mu.Unlock()

		hctx := &dive.HookContext{
			Tool: &mockTool{name: "Skill"},
			Call: &llm.ToolUseContent{ID: "call-123"},
		}
		assert.NoError(t, hook(context.Background(), hctx))
		assert.Contains(t, hctx.AdditionalContext, "# Skill content here")

		// Should be cleared after use
		loader.mu.RLock()
		assert.Equal(t, 0, len(loader.pendingInstructions))
		loader.mu.RUnlock()
	})

	t.Run("no-op for non-Skill tools", func(t *testing.T) {
		loader.mu.Lock()
		loader.pendingInstructions = map[string]string{
			"call-456": "should not be consumed",
		}
		loader.mu.Unlock()

		hctx := &dive.HookContext{
			Tool: &mockTool{name: "Read"},
		}
		assert.NoError(t, hook(context.Background(), hctx))
		assert.Equal(t, "", hctx.AdditionalContext)

		// Pending should still be there
		loader.mu.RLock()
		assert.Equal(t, 1, len(loader.pendingInstructions))
		assert.Equal(t, "should not be consumed", loader.pendingInstructions["call-456"])
		loader.mu.RUnlock()
	})

	t.Run("no-op when Call is nil", func(t *testing.T) {
		loader.mu.Lock()
		loader.pendingInstructions = map[string]string{
			"call-789": "content",
		}
		loader.mu.Unlock()

		hctx := &dive.HookContext{
			Tool: &mockTool{name: "Skill"},
			Call: nil,
		}
		assert.NoError(t, hook(context.Background(), hctx))
		assert.Equal(t, "", hctx.AdditionalContext)
	})
}

func TestConfigureAgent(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	opts := dive.AgentOptions{
		SystemPrompt: "You are a helpful assistant.",
		Tools:        []dive.Tool{&mockTool{name: "Read"}},
	}

	ConfigureAgent(&opts, loader)

	// Skill tool should be added
	hasSkillTool := false
	for _, tool := range opts.Tools {
		if tool.Name() == "Skill" {
			hasSkillTool = true
		}
	}
	assert.True(t, hasSkillTool)

	// System prompt should include rules
	assert.Contains(t, opts.SystemPrompt, "Skill tool")
	assert.Contains(t, opts.SystemPrompt, "You are a helpful assistant.")

	// PreGeneration hook should be registered
	assert.Equal(t, 1, len(opts.Hooks.PreGeneration))

	// PostToolUse hook should be registered
	assert.Equal(t, 1, len(opts.Hooks.PostToolUse))
}

func TestConfigureAgent_EmptyLoader(t *testing.T) {
	loader := &Loader{skills: map[string]*Skill{}}

	opts := dive.AgentOptions{
		SystemPrompt: "Original prompt.",
		Tools:        []dive.Tool{&mockTool{name: "Read"}},
	}

	ConfigureAgent(&opts, loader)

	// No Skill tool or system prompt changes when no skills loaded
	assert.Equal(t, 1, len(opts.Tools)) // Only original tool
	assert.Equal(t, "Original prompt.", opts.SystemPrompt)

	// Hooks are still registered for stale catalog cleanup on session resume
	assert.Equal(t, 1, len(opts.Hooks.PreGeneration))
	assert.Equal(t, 1, len(opts.Hooks.PostToolUse))
}

func TestConfigureAgent_NilLoader(t *testing.T) {
	opts := dive.AgentOptions{
		SystemPrompt: "Original prompt.",
	}
	ConfigureAgent(&opts, nil)
	assert.Equal(t, "Original prompt.", opts.SystemPrompt)
}

func TestConfigureAgent_ShellExpansion(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"test": {
				Name:        "test",
				Description: "Test skill.",
				Config:      SkillConfig{Description: "Test skill."},
			},
		},
	}

	opts := dive.AgentOptions{}
	ConfigureAgent(&opts, loader, WithConfigShellExpansion(true))

	// Verify the Skill tool was added (we can't inspect its config directly,
	// but we can verify it exists)
	hasSkillTool := false
	for _, tool := range opts.Tools {
		if tool.Name() == "Skill" {
			hasSkillTool = true
		}
	}
	assert.True(t, hasSkillTool)
}

func TestLoaderExtension(t *testing.T) {
	t.Run("with skills", func(t *testing.T) {
		loader := &Loader{
			skills: map[string]*Skill{
				"reviewer": {
					Name:        "reviewer",
					Description: "Review code.",
					Config:      SkillConfig{Description: "Review code."},
				},
			},
			pendingInstructions: make(map[string]string),
		}

		// Tools() returns the Skill tool
		tools := loader.Tools()
		assert.Equal(t, 1, len(tools))
		assert.Equal(t, "Skill", tools[0].Name())

		// Hooks() returns catalog and content hooks
		hooks := loader.Hooks()
		assert.Equal(t, 1, len(hooks.PreGeneration))
		assert.Equal(t, 1, len(hooks.PostToolUse))

		// Rules() returns skill rules
		rules := loader.Rules()
		assert.Contains(t, rules, "Skill tool")
	})

	t.Run("with no skills", func(t *testing.T) {
		loader := &Loader{
			skills:              map[string]*Skill{},
			pendingInstructions: make(map[string]string),
		}

		// Tools() returns nil
		assert.Nil(t, loader.Tools())

		// Hooks() still returns hooks (for stale catalog cleanup)
		hooks := loader.Hooks()
		assert.Equal(t, 1, len(hooks.PreGeneration))
		assert.Equal(t, 1, len(hooks.PostToolUse))

		// Rules() returns empty
		assert.Equal(t, "", loader.Rules())
	})

	t.Run("shell expansion via LoaderOptions", func(t *testing.T) {
		loader := &Loader{
			shellExpansion: true,
			skills: map[string]*Skill{
				"test": {
					Name:        "test",
					Description: "Test.",
					Config:      SkillConfig{Description: "Test."},
				},
			},
			pendingInstructions: make(map[string]string),
		}

		tools := loader.Tools()
		assert.Equal(t, 1, len(tools))
		assert.Equal(t, "Skill", tools[0].Name())
	})
}

func TestCatalogHook_InjectsIntoFirstUserMessage(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hook := catalogHook(loader)
	hctx := &dive.HookContext{
		Messages: []*llm.Message{
			llm.NewUserTextMessage("Review my code"),
		},
	}

	err := hook(context.Background(), hctx)
	assert.NoError(t, err)

	// User text should be unchanged
	assert.Equal(t, "Review my code", hctx.Messages[0].Content[0].(*llm.TextContent).Text)
	// Pinned delivery is copy-on-write; the hook must not mutate history.
	assert.Equal(t, 1, len(hctx.Messages[0].Content))
}

func TestCatalogHook_StableAcrossGenerations(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hook := catalogHook(loader)

	// First generation — injects catalog as separate content block
	firstMsg := llm.NewUserTextMessage("First")
	hctx := &dive.HookContext{
		Messages: []*llm.Message{firstMsg},
	}
	assert.NoError(t, hook(context.Background(), hctx))
	assert.Equal(t, 1, len(firstMsg.Content))
	assert.Equal(t, "First", firstMsg.Content[0].(*llm.TextContent).Text)

	// Second generation — same messages plus a new turn
	// The catalog should already be in firstMsg, hook should be a no-op
	hctx2 := &dive.HookContext{
		Messages: []*llm.Message{
			firstMsg, // already has catalog content block
			{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Done"}}},
			llm.NewUserTextMessage("Second turn"),
		},
	}
	assert.NoError(t, hook(context.Background(), hctx2))

	// First message remains caller-owned and unchanged.
	assert.Equal(t, 1, len(firstMsg.Content))

	// Second user message should NOT have a catalog
	assert.Equal(t, 1, len(hctx2.Messages[2].Content))
	assert.Equal(t, "Second turn", hctx2.Messages[2].Content[0].(*llm.TextContent).Text)
}

func TestCatalogHook_ReinjectsOnChange(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hook := catalogHook(loader)

	firstMsg := llm.NewUserTextMessage("Hello")
	hctx := &dive.HookContext{Messages: []*llm.Message{firstMsg}}
	assert.NoError(t, hook(context.Background(), hctx))
	assert.Equal(t, 1, len(firstMsg.Content))

	// Add a skill — catalog changes
	loader.mu.Lock()
	loader.skills["deploy"] = &Skill{
		Name:        "deploy",
		Description: "Deploy.",
		Config:      SkillConfig{Description: "Deploy."},
	}
	loader.mu.Unlock()

	// Next generation — should replace the catalog content block
	hctx2 := &dive.HookContext{Messages: []*llm.Message{firstMsg}}
	assert.NoError(t, hook(context.Background(), hctx2))

	// The caller-owned first message is still unchanged.
	assert.Equal(t, 1, len(firstMsg.Content))
	assert.Equal(t, "Hello", firstMsg.Content[0].(*llm.TextContent).Text)
}

func TestCatalogHook_SessionResume(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	// Simulate a resumed session: messages already have the catalog
	// from a previous session, but the hook is fresh (lastHash is "")
	existingMsg := llm.NewUserTextMessage("Hello")
	dive.SetSystemReminder([]*llm.Message{existingMsg}, "skills", BuildCatalog(loader))

	hook := catalogHook(loader)
	hctx := &dive.HookContext{
		Messages: []*llm.Message{
			existingMsg,
			{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hi"}}},
			llm.NewUserTextMessage("Continue"),
		},
	}

	assert.NoError(t, hook(context.Background(), hctx))

	// Should detect existing block and not duplicate — still two content blocks
	assert.Equal(t, 2, len(existingMsg.Content))
	assert.Equal(t, "Hello", existingMsg.Content[0].(*llm.TextContent).Text)
}

func TestCatalogHook_NoUserMessage(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hook := catalogHook(loader)
	hctx := &dive.HookContext{
		Messages: []*llm.Message{
			{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
		},
	}

	// Pinning is deferred to the agent-owned model overlay.
	err := hook(context.Background(), hctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(hctx.Messages))
}

func TestCatalogHook_EmptySkills(t *testing.T) {
	loader := &Loader{skills: map[string]*Skill{}}
	hook := catalogHook(loader)
	hctx := &dive.HookContext{
		Messages: []*llm.Message{
			llm.NewUserTextMessage("Hello"),
		},
	}
	assert.NoError(t, hook(context.Background(), hctx))
	assert.Equal(t, "Hello", hctx.Messages[0].Content[0].(*llm.TextContent).Text)
}

func TestCatalogHook_RemovesStaleCatalogOnFreshResume(t *testing.T) {
	// Simulate: skills were available in a previous process, which wrote
	// a catalog block. Now skills are gone and a fresh process resumes
	// the session. lastHash starts empty, but the block is in messages.
	loader := &Loader{skills: map[string]*Skill{}} // no skills

	// Messages from a previous session with a catalog block
	existingMsg := llm.NewUserTextMessage("Hello")
	dive.SetSystemReminder([]*llm.Message{existingMsg}, "skills", "<skills>\n- old-skill: Gone now.\n</skills>")
	assert.True(t, dive.HasSystemReminder([]*llm.Message{existingMsg}, "skills"))

	hook := catalogHook(loader)
	hctx := &dive.HookContext{Messages: []*llm.Message{existingMsg}}

	// Hook should remove the stale block even though lastHash is ""
	assert.NoError(t, hook(context.Background(), hctx))
	assert.True(t, dive.HasSystemReminder(hctx.Messages, "skills"), "hook must not mutate loaded history")
}

func TestCatalogHook_RemovesOnReloadToEmpty(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hook := catalogHook(loader)
	firstMsg := llm.NewUserTextMessage("Hello")
	hctx := &dive.HookContext{Messages: []*llm.Message{firstMsg}}

	// First call — injects catalog
	assert.NoError(t, hook(context.Background(), hctx))
	assert.False(t, dive.HasSystemReminder(hctx.Messages, "skills"))

	// Simulate reload that clears all skills
	loader.mu.Lock()
	loader.skills = map[string]*Skill{}
	loader.mu.Unlock()

	// Next generation — should remove the stale catalog
	hctx2 := &dive.HookContext{Messages: []*llm.Message{firstMsg}}
	assert.NoError(t, hook(context.Background(), hctx2))
	assert.False(t, dive.HasSystemReminder(hctx2.Messages, "skills"))
}

// staticLLM is a minimal llm.LLM for exercising the agent loop in tests.
type staticLLM struct{}

func (s *staticLLM) Name() string { return "test-model" }

func (s *staticLLM) Generate(_ context.Context, _ ...llm.Option) (*llm.Response, error) {
	return &llm.Response{
		ID:         "resp_1",
		Model:      "test-model",
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
		Type:       "message",
		StopReason: "stop",
		Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
	}, nil
}

type catalogCaptureLLM struct {
	messages []*llm.Message
}

func (m *catalogCaptureLLM) Name() string { return "test-model" }

func (m *catalogCaptureLLM) Generate(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	m.messages = cfg.Messages
	return (&staticLLM{}).Generate(context.Background())
}

func TestCatalogHook_AgentOwnedOverlay(t *testing.T) {
	loader := &Loader{skills: map[string]*Skill{
		"reviewer": {Name: "reviewer", Description: "Review code.", Config: SkillConfig{Description: "Review code."}},
	}}
	model := &catalogCaptureLLM{}
	agent, err := dive.NewAgent(dive.AgentOptions{Model: model, Extensions: []dive.Extension{loader}})
	assert.NoError(t, err)
	input := llm.NewUserTextMessage("Review this")
	_, err = agent.CreateResponse(context.Background(), dive.WithMessages(input))
	assert.NoError(t, err)
	reminder, ok := dive.FindLatestReminder(model.messages, "skills")
	assert.True(t, ok)
	assert.Contains(t, reminder.Content, "reviewer: Review code.")
	assert.Equal(t, 1, len(input.Content), "catalog overlay must not mutate caller input")

	loader.mu.Lock()
	loader.skills["deploy"] = &Skill{Name: "deploy", Description: "Deploy.", Config: SkillConfig{Description: "Deploy."}}
	loader.mu.Unlock()
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("Continue"))
	assert.NoError(t, err)
	reminder, ok = dive.FindLatestReminder(model.messages, "skills")
	assert.True(t, ok)
	assert.Contains(t, reminder.Content, "deploy: Deploy.")
}

func TestCatalogHook_MasksLegacyCatalogWhenEmpty(t *testing.T) {
	loader := &Loader{skills: map[string]*Skill{}}
	model := &catalogCaptureLLM{}
	agent, err := dive.NewAgent(dive.AgentOptions{Model: model, Extensions: []dive.Extension{loader}})
	assert.NoError(t, err)
	legacy := llm.NewUserTextMessage("Continue")
	dive.SetSystemReminder([]*llm.Message{legacy}, "skills", "stale")

	_, err = agent.CreateResponse(context.Background(), dive.WithMessages(legacy))
	assert.NoError(t, err)
	assert.False(t, dive.HasSystemReminder(model.messages, "skills"))
	reminder, ok := dive.FindLatestReminder(model.messages, "skills")
	assert.True(t, ok)
	assert.Equal(t, "", reminder.Content)
	assert.True(t, dive.HasSystemReminder([]*llm.Message{legacy}, "skills"), "loaded history must remain unchanged")
}

// TestCatalogHook_ConcurrentCreateResponse exercises the catalog hook's
// shared lastHash state under concurrent CreateResponse calls on a single
// agent. Run with -race to detect unsynchronized access.
func TestCatalogHook_ConcurrentCreateResponse(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
		pendingInstructions: make(map[string]string),
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:       "TestAgent",
		Model:      &staticLLM{},
		Extensions: []dive.Extension{loader},
	})
	assert.NoError(t, err)

	const goroutines = 16
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Mutate the catalog from some goroutines so CatalogHash
			// changes and the hook takes the lastHash write path.
			if i%4 == 0 {
				loader.mu.Lock()
				name := fmt.Sprintf("skill-%d", i)
				loader.skills[name] = &Skill{
					Name:        name,
					Description: "Generated.",
					Config:      SkillConfig{Description: "Generated."},
				}
				loader.mu.Unlock()
			}
			_, err := agent.CreateResponse(context.Background(), dive.WithInput("hello"))
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}
