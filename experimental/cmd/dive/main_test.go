package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGetDefaultModel(t *testing.T) {
	// All provider env vars that getDefaultModel checks.
	allKeys := []string{
		"ANTHROPIC_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"OPENAI_API_KEY",
		"XAI_API_KEY",
		"GROK_API_KEY",
		"MISTRAL_API_KEY",
	}

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no api keys defaults to claude",
			envVars:  map[string]string{},
			expected: "claude-haiku-4-5",
		},
		{
			name: "anthropic key present",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "test",
			},
			expected: "claude-haiku-4-5",
		},
		{
			name: "google key present",
			envVars: map[string]string{
				"GOOGLE_API_KEY": "test",
			},
			expected: "gemini-3-flash-preview",
		},
		{
			name: "openai key present",
			envVars: map[string]string{
				"OPENAI_API_KEY": "test",
			},
			expected: "gpt-5.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all provider env vars so only the test's vars are set.
			for _, key := range allKeys {
				t.Setenv(key, "")
			}
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}
			model := getDefaultModel()
			assert.Equal(t, tt.expected, model)
		})
	}
}

func TestCreateTools(t *testing.T) {
	workspaceDir := t.TempDir()
	tools := createTools(workspaceDir, nil)

	// Verify we have some basic tools
	assert.True(t, len(tools) > 0, "should create at least some tools")

	// Verify we have the essential file tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name()] = true
	}

	assert.True(t, toolNames["Read"], "should have Read tool")
	assert.True(t, toolNames["Write"], "should have Write tool")
	assert.True(t, toolNames["Edit"], "should have Edit tool")
	assert.True(t, toolNames["Bash"], "should have Bash tool")
	assert.True(t, toolNames["Glob"], "should have Glob tool")
	assert.True(t, toolNames["Grep"], "should have Grep tool")
}

func TestDefaultPermissionRules_AllowsReadOnlyToolsByDefault(t *testing.T) {
	workspaceDir := t.TempDir()
	tools := createTools(workspaceDir, nil)
	rules := defaultPermissionRules(tools)

	allowed := map[string]bool{}
	for _, rule := range rules {
		if string(rule.Type) == "allow" {
			allowed[rule.Tool] = true
		}
	}

	// Read-only tools should be auto-allowed.
	assert.True(t, allowed["Read"], "Read should be auto-allowed")
	assert.True(t, allowed["Glob"], "Glob should be auto-allowed")
	assert.True(t, allowed["Grep"], "Grep should be auto-allowed")
	assert.True(t, allowed["ListDirectory"], "ListDirectory should be auto-allowed")
	assert.True(t, allowed["AskUserQuestion"], "AskUserQuestion should be auto-allowed")

	// Mutating tools should still require normal permission flow.
	assert.False(t, allowed["Write"], "Write should not be auto-allowed")
	assert.False(t, allowed["Edit"], "Edit should not be auto-allowed")
	assert.False(t, allowed["Bash"], "Bash should not be auto-allowed")
}

func TestLoadStartupInstructionAttachment_PrefersAgents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents"), 0o644); err != nil {
		t.Fatal(err)
	}

	attachment, err := loadStartupInstructionAttachment(dir)
	assert.Nil(t, err)
	assert.Contains(t, attachment, `<file path="AGENTS.md">`)
	assert.Contains(t, attachment, "agents")
}

func TestLoadStartupInstructionAttachment_FallsBackToClaude(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude"), 0o644); err != nil {
		t.Fatal(err)
	}

	attachment, err := loadStartupInstructionAttachment(dir)
	assert.Nil(t, err)
	assert.Contains(t, attachment, `<file path="CLAUDE.md">`)
	assert.Contains(t, attachment, "claude")
}
