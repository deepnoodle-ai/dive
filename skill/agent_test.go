package skill

import (
	"context"
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

	t.Run("injects pending instructions", func(t *testing.T) {
		loader.mu.Lock()
		loader.pendingInstructions = "# Skill content here"
		loader.mu.Unlock()

		hctx := &dive.HookContext{
			Tool: &mockTool{name: "Skill"},
		}
		assert.NoError(t, hook(context.Background(), hctx))
		assert.Contains(t, hctx.AdditionalContext, "# Skill content here")

		// Should be cleared after use
		loader.mu.RLock()
		assert.Equal(t, "", loader.pendingInstructions)
		loader.mu.RUnlock()
	})

	t.Run("no-op for non-Skill tools", func(t *testing.T) {
		loader.mu.Lock()
		loader.pendingInstructions = "should not be consumed"
		loader.mu.Unlock()

		hctx := &dive.HookContext{
			Tool: &mockTool{name: "Read"},
		}
		assert.NoError(t, hook(context.Background(), hctx))
		assert.Equal(t, "", hctx.AdditionalContext)

		// Pending should still be there
		loader.mu.RLock()
		assert.Equal(t, "should not be consumed", loader.pendingInstructions)
		loader.mu.RUnlock()
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

	// Toolset should be added
	assert.Equal(t, 1, len(opts.Toolsets))
	assert.Equal(t, "skill-filter", opts.Toolsets[0].Name())

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

	// Should be a no-op
	assert.Equal(t, 1, len(opts.Tools)) // Only original tool
	assert.Equal(t, 0, len(opts.Toolsets))
	assert.Equal(t, "Original prompt.", opts.SystemPrompt)
	assert.Equal(t, 0, len(opts.Hooks.PreGeneration))
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

	text := hctx.Messages[0].Content[0].(*llm.TextContent).Text
	assert.Contains(t, text, `<system-reminder name="skills">`)
	assert.Contains(t, text, "<skills>")
	assert.Contains(t, text, "reviewer: Review code.")
	assert.Contains(t, text, "</system-reminder>")
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

	// First generation — injects into first user message
	firstMsg := llm.NewUserTextMessage("First")
	hctx := &dive.HookContext{
		Messages: []*llm.Message{firstMsg},
	}
	assert.NoError(t, hook(context.Background(), hctx))
	assert.Contains(t, firstMsg.Content[0].(*llm.TextContent).Text, "<skills>")

	// Second generation — same messages plus a new turn
	// The catalog should already be in firstMsg, hook should be a no-op
	hctx2 := &dive.HookContext{
		Messages: []*llm.Message{
			firstMsg, // already has catalog baked in
			{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Done"}}},
			llm.NewUserTextMessage("Second turn"),
		},
	}
	assert.NoError(t, hook(context.Background(), hctx2))

	// First message should still have exactly one catalog block
	text := firstMsg.Content[0].(*llm.TextContent).Text
	count := 0
	for i := 0; i < len(text); i++ {
		if i+len(`<system-reminder name="skills">`) <= len(text) &&
			text[i:i+len(`<system-reminder name="skills">`)] == `<system-reminder name="skills">` {
			count++
		}
	}
	assert.Equal(t, 1, count)

	// Second user message should NOT have a catalog
	secondText := hctx2.Messages[2].Content[0].(*llm.TextContent).Text
	assert.Equal(t, "Second turn", secondText)
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
	assert.Contains(t, firstMsg.Content[0].(*llm.TextContent).Text, "reviewer")

	// Add a skill — catalog changes
	loader.mu.Lock()
	loader.skills["deploy"] = &Skill{
		Name:        "deploy",
		Description: "Deploy.",
		Config:      SkillConfig{Description: "Deploy."},
	}
	loader.mu.Unlock()

	// Next generation — should replace the block in first message
	hctx2 := &dive.HookContext{Messages: []*llm.Message{firstMsg}}
	assert.NoError(t, hook(context.Background(), hctx2))

	text := firstMsg.Content[0].(*llm.TextContent).Text
	assert.Contains(t, text, "deploy: Deploy.")
	assert.Contains(t, text, "reviewer: Review code.")
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

	// Should detect existing block and not duplicate
	text := existingMsg.Content[0].(*llm.TextContent).Text
	count := 0
	needle := `<system-reminder name="skills">`
	for i := 0; i <= len(text)-len(needle); i++ {
		if text[i:i+len(needle)] == needle {
			count++
		}
	}
	assert.Equal(t, 1, count)
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

	// SetSystemReminder creates a user message if none exists
	err := hook(context.Background(), hctx)
	assert.NoError(t, err)
	assert.True(t, dive.HasSystemReminder(hctx.Messages, "skills"))
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
