package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// mockTool implements dive.Tool for testing.
type mockTool struct {
	name        string
	annotations *dive.ToolAnnotations
}

func (m *mockTool) Name() string                       { return m.name }
func (m *mockTool) Description() string                { return "Test tool" }
func (m *mockTool) Schema() *dive.Schema               { return nil }
func (m *mockTool) Annotations() *dive.ToolAnnotations { return m.annotations }
func (m *mockTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, nil
}

// testDialog implements dive.Dialog for testing.
type testDialog struct {
	showFunc func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error)
}

func (d *testDialog) Show(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	return d.showFunc(ctx, in)
}

func TestManager(t *testing.T) {
	t.Run("bypass mode allows all", func(t *testing.T) {
		config := &Config{Mode: ModeBypassPermissions}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("plan mode allows read-only tools", func(t *testing.T) {
		config := &Config{Mode: ModePlan}
		manager := NewManager(config, nil)

		readTool := &mockTool{
			name:        "Read",
			annotations: &dive.ToolAnnotations{ReadOnlyHint: true},
		}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file": "test.txt"}`)}

		err := manager.EvaluateToolUse(context.Background(), readTool, call)
		assert.NoError(t, err)
	})

	t.Run("plan mode denies non-read-only tools", func(t *testing.T) {
		config := &Config{Mode: ModePlan}
		manager := NewManager(config, nil)

		writeTool := &mockTool{
			name:        "Write",
			annotations: &dive.ToolAnnotations{ReadOnlyHint: false},
		}
		call := &llm.ToolUseContent{Name: "Write", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), writeTool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plan mode")
	})

	t.Run("accept edits mode allows edit operations", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		editTool := &mockTool{
			name:        "Edit",
			annotations: &dive.ToolAnnotations{EditHint: true},
		}
		call := &llm.ToolUseContent{Name: "Edit", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), editTool, call)
		assert.NoError(t, err)
	})

	t.Run("dontAsk mode denies non-allowed tools", func(t *testing.T) {
		config := &Config{Mode: ModeDontAsk}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dontAsk")
	})

	t.Run("dontAsk mode allows explicitly allowed tools", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowRule("Read"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("deny rules take precedence", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Bash"),
				DenyRule("Bash", "Bash is not allowed"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "Bash is not allowed", err.Error())
	})

	t.Run("allow rules match tool name", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Read"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("ask rules call dialog", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute this command?"),
			},
		}

		var confirmedMsg string
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			confirmedMsg = in.Message
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, "Execute this command?", confirmedMsg)
	})

	t.Run("ask rules deny when dialog denies", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: false}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("specifier pattern matching", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowSpecifierRule("Bash", "go build*"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		matchCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "go build ./..."}`)}
		noMatchCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, matchCall)
		assert.NoError(t, err)

		// Should fall through to default (confirm). No dialog = auto-allow.
		err = manager.EvaluateToolUse(context.Background(), tool, noMatchCall)
		assert.NoError(t, err)
	})

	t.Run("specifier deny rule blocks matching commands", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Bash", "rm -rf*", "Dangerous command"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "Dangerous command", err.Error())
	})

	t.Run("glob tool pattern matching", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("mcp__*"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "mcp__ide__getDiagnostics"}
		call := &llm.ToolUseContent{Name: "mcp__ide__getDiagnostics", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("wildcard tool pattern", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("*"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "AnyTool"}
		call := &llm.ToolUseContent{Name: "AnyTool", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("session allowlist", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		dialog := &dive.DenyAllDialog{}
		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// Before adding to allowlist - should deny (dialog says no)
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)

		// Add bash category to session allowlist
		manager.AllowForSession("bash")

		// After adding to allowlist - should allow
		err = manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)

		// Check IsSessionAllowed
		assert.True(t, manager.IsSessionAllowed("bash"))
		assert.False(t, manager.IsSessionAllowed("edit"))

		// Clear allowlist
		manager.ClearSessionAllowlist()
		assert.False(t, manager.IsSessionAllowed("bash"))
	})

	t.Run("mode can be changed dynamically", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, nil)

		assert.Equal(t, ModeDefault, manager.Mode())

		manager.SetMode(ModeBypassPermissions)
		assert.Equal(t, ModeBypassPermissions, manager.Mode())
	})

	t.Run("nil dialog auto-allows", func(t *testing.T) {
		manager := NewManager(nil, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// Default with nil dialog should auto-allow
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("nil config defaults to ModeDefault", func(t *testing.T) {
		manager := NewManager(nil, nil)
		assert.Equal(t, ModeDefault, manager.Mode())
	})
}

func TestRuleHelpers(t *testing.T) {
	t.Run("DenyRule", func(t *testing.T) {
		rule := DenyRule("Bash", "Not allowed")
		assert.Equal(t, RuleDeny, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "Not allowed", rule.Message)
	})

	t.Run("AllowRule", func(t *testing.T) {
		rule := AllowRule("Read")
		assert.Equal(t, RuleAllow, rule.Type)
		assert.Equal(t, "Read", rule.Tool)
	})

	t.Run("AskRule", func(t *testing.T) {
		rule := AskRule("Bash", "Execute?")
		assert.Equal(t, RuleAsk, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "Execute?", rule.Message)
	})

	t.Run("DenySpecifierRule", func(t *testing.T) {
		rule := DenySpecifierRule("Bash", "rm -rf*", "Dangerous command")
		assert.Equal(t, RuleDeny, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "rm -rf*", rule.Specifier)
		assert.Equal(t, "Dangerous command", rule.Message)
	})

	t.Run("AllowSpecifierRule", func(t *testing.T) {
		rule := AllowSpecifierRule("Bash", "go build*")
		assert.Equal(t, RuleAllow, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "go build*", rule.Specifier)
	})

	t.Run("AskSpecifierRule", func(t *testing.T) {
		rule := AskSpecifierRule("Bash", "git push*", "Push changes?")
		assert.Equal(t, RuleAsk, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "git push*", rule.Specifier)
		assert.Equal(t, "Push changes?", rule.Message)
	})

	t.Run("Rule.String", func(t *testing.T) {
		assert.Equal(t, "allow:Read", AllowRule("Read").String())
		assert.Equal(t, "deny:Bash(rm -rf*)", DenySpecifierRule("Bash", "rm -rf*", "").String())
		assert.Equal(t, "ask:Bash", AskRule("Bash", "").String())
	})
}

func TestGetToolCategory(t *testing.T) {
	t.Run("bash patterns", func(t *testing.T) {
		for _, name := range []string{"Bash", "Command", "Shell", "Exec", "RunCommand"} {
			cat := GetToolCategory(name)
			assert.Equal(t, "bash", cat.Key)
		}
	})

	t.Run("edit patterns", func(t *testing.T) {
		for _, name := range []string{"Edit", "Write", "Create", "Mkdir", "Touch"} {
			cat := GetToolCategory(name)
			assert.Equal(t, "edit", cat.Key)
		}
	})

	t.Run("read pattern", func(t *testing.T) {
		cat := GetToolCategory("Read")
		assert.Equal(t, "read", cat.Key)
	})

	t.Run("search patterns", func(t *testing.T) {
		for _, name := range []string{"Glob", "Grep", "Search"} {
			cat := GetToolCategory(name)
			assert.Equal(t, "search", cat.Key)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		cat := GetToolCategory("CustomTool")
		assert.Equal(t, "CustomTool", cat.Key)
		assert.Contains(t, cat.Label, "CustomTool")
	})
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		// Exact match
		{"Bash", "Bash", true},
		{"Bash", "Read", false},

		// Wildcard
		{"*", "anything", true},
		{"mcp__*", "mcp__ide__getDiagnostics", true},
		{"mcp__*", "Read", false},

		// Double star
		{"**", "a/b/c", true},

		// Alternatives
		{"{Bash,Read}", "Bash", true},
		{"{Bash,Read}", "Read", true},
		{"{Bash,Read}", "Write", false},

		// Specifier patterns
		{"go test*", "go test ./...", true},
		{"go test*", "go build", false},
		{"rm -rf*", "rm -rf /", true},
		{"rm -rf*", "rm foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			got := MatchGlob(tt.pattern, tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchDomain(t *testing.T) {
	tests := []struct {
		url    string
		domain string
		want   bool
	}{
		// Exact matches
		{"https://example.com/path", "example.com", true},
		{"http://example.com", "example.com", true},

		// Subdomain matches
		{"https://sub.example.com/path", "example.com", true},
		{"https://deep.sub.example.com", "example.com", true},

		// Non-matches
		{"https://notexample.com", "example.com", false},
		{"https://example.com.evil.com", "example.com", false},

		// Different domains
		{"https://other.com", "example.com", false},

		// With ports
		{"https://example.com:8080/path", "example.com", true},
		{"https://sub.example.com:443", "example.com", true},

		// IPv6 addresses
		{"https://[::1]:8080/path", "::1", true},
		{"https://[::1]/path", "::1", true},
		{"https://[::1]:8080/path", "example.com", false},

		// Bare host (no scheme)
		{"example.com/path", "example.com", true},
		{"sub.example.com", "example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.url+"_"+tt.domain, func(t *testing.T) {
			got := MatchDomain(tt.url, tt.domain)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/path/to/file", "/path/to/file", true},
		{"/path/to/*", "/path/to/file", true},
		{"/path/to/*", "/path/to/file.go", true},
		{"/path/**", "/path/to/file", true},
		{"/path/**", "/path/to/deep/nested/file", true},
		{"/path/to/*", "/other/path", false},
		{"*.go", "file.go", true},
		{"*.go", "file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := MatchPath(tt.pattern, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRule(t *testing.T) {
	t.Run("simple tool pattern", func(t *testing.T) {
		rule, err := ParseRule(RuleAllow, "Read")
		assert.NoError(t, err)
		assert.Equal(t, RuleAllow, rule.Type)
		assert.Equal(t, "Read", rule.Tool)
		assert.Equal(t, "", rule.Specifier)
	})

	t.Run("tool with specifier", func(t *testing.T) {
		rule, err := ParseRule(RuleAllow, "Bash(go test *)")
		assert.NoError(t, err)
		assert.Equal(t, RuleAllow, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "go test *", rule.Specifier)
	})

	t.Run("deny with specifier", func(t *testing.T) {
		rule, err := ParseRule(RuleDeny, "Bash(rm -rf*)")
		assert.NoError(t, err)
		assert.Equal(t, RuleDeny, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "rm -rf*", rule.Specifier)
	})

	t.Run("glob tool pattern", func(t *testing.T) {
		rule, err := ParseRule(RuleAllow, "mcp__*")
		assert.NoError(t, err)
		assert.Equal(t, "mcp__*", rule.Tool)
		assert.Equal(t, "", rule.Specifier)
	})

	t.Run("empty spec returns error", func(t *testing.T) {
		_, err := ParseRule(RuleAllow, "")
		assert.Error(t, err)
	})

	t.Run("empty specifier returns error", func(t *testing.T) {
		_, err := ParseRule(RuleAllow, "Bash()")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty specifier")
	})

	t.Run("whitespace-only tool pattern treated as simple pattern", func(t *testing.T) {
		// "  (x)" after trim is "(x)" — idx==0 so not parameterized
		rule, err := ParseRule(RuleAllow, "  (x)")
		assert.NoError(t, err)
		assert.Equal(t, "(x)", rule.Tool)
		assert.Equal(t, "", rule.Specifier)
	})

	t.Run("whitespace in tool and specifier is trimmed", func(t *testing.T) {
		rule, err := ParseRule(RuleAllow, " Bash ( go test * ) ")
		assert.NoError(t, err)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "go test *", rule.Specifier)
	})

	t.Run("roundtrip with String", func(t *testing.T) {
		rule, err := ParseRule(RuleAllow, "Bash(go test *)")
		assert.NoError(t, err)
		assert.Equal(t, "allow:Bash(go test *)", rule.String())
	})
}

func TestHook(t *testing.T) {
	t.Run("creates hook from config", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Read"),
			},
		}

		hook := Hook(config, nil)
		assert.NotNil(t, hook)

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}
		hookCtx := &dive.HookContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
	})

	t.Run("ask rule triggers confirmation", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		confirmed := false
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			confirmed = true
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		hook := Hook(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.HookContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.True(t, confirmed)
	})

	t.Run("denied confirmation returns error", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: false}, nil
		}}

		hook := Hook(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.HookContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})
}

func TestConfirmDialogInput(t *testing.T) {
	t.Run("dialog receives correct fields", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		var received *dive.DialogInput
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			received = in
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.NotNil(t, received)
		assert.True(t, received.Confirm)
		assert.Equal(t, "Bash", received.Title)
		assert.Equal(t, tool, received.Tool)
		assert.Equal(t, call, received.Call)
	})

	t.Run("dialog error propagates", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return nil, fmt.Errorf("dialog failed")
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dialog failed")
	})

	t.Run("dialog canceled treated as denial", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Canceled: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("ask rule message forwarded to dialog", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Are you sure about this?"),
			},
		}

		var received *dive.DialogInput
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			received = in
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.True(t, received.Confirm)
		assert.Equal(t, "Are you sure about this?", received.Message)
		assert.Equal(t, "Bash", received.Title)
	})

	t.Run("default fallthrough confirms via dialog", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		called := false
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			called = true
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("default fallthrough denied via dialog", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, &dive.DenyAllDialog{})

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("AutoApproveDialog allows via manager", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, &dive.AutoApproveDialog{})

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})
}

func TestConfirmAllowSession(t *testing.T) {
	t.Run("AllowSession adds category to session allowlist", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: true, AllowSession: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}

		// First call triggers dialog, which returns AllowSession
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)

		// Category should now be in session allowlist
		assert.True(t, manager.IsSessionAllowed("bash"))

		// Second call should skip dialog entirely (session allowed)
		dialogCalled := false
		dialog.showFunc = func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			dialogCalled = true
			return &dive.DialogOutput{Confirmed: true}, nil
		}

		err = manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.False(t, dialogCalled)
	})

	t.Run("AllowSession uses correct category for edit tools", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: true, AllowSession: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Write"}
		call := &llm.ToolUseContent{Name: "Write", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)

		// Write is in the "edit" category
		assert.True(t, manager.IsSessionAllowed("edit"))
		assert.False(t, manager.IsSessionAllowed("bash"))
	})
}

func TestConfirmFeedback(t *testing.T) {
	t.Run("Feedback returns UserFeedback error", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Feedback: "try using cat instead"}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)

		// Should be a UserFeedback error
		feedback, ok := dive.IsUserFeedback(err)
		assert.True(t, ok)
		assert.Equal(t, "try using cat instead", feedback)
	})

	t.Run("empty Feedback and not confirmed is regular denial", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: false}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")

		// Should NOT be a UserFeedback error
		_, ok := dive.IsUserFeedback(err)
		assert.False(t, ok)
	})
}

func TestAuditHook(t *testing.T) {
	t.Run("logs tool calls", func(t *testing.T) {
		var loggedName string
		var loggedInput []byte

		hook := AuditHook(func(name string, input []byte) {
			loggedName = name
			loggedInput = input
		})

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file": "test.txt"}`)}
		hookCtx := &dive.HookContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, "Read", loggedName)
		assert.Equal(t, `{"file": "test.txt"}`, string(loggedInput))
	})

	t.Run("handles nil tool", func(t *testing.T) {
		var loggedName string

		hook := AuditHook(func(name string, input []byte) {
			loggedName = name
		})

		hookCtx := &dive.HookContext{Tool: nil, Call: nil}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, "unknown", loggedName)
	})
}

func TestDefaultSpecifierFields(t *testing.T) {
	t.Run("Bash command field", func(t *testing.T) {
		fn := DefaultSpecifierFields["Bash"]
		assert.Equal(t, "go test ./...", fn([]byte(`{"command": "go test ./..."}`)))
	})

	t.Run("Bash cmd field", func(t *testing.T) {
		fn := DefaultSpecifierFields["Bash"]
		assert.Equal(t, "ls -la", fn([]byte(`{"cmd": "ls -la"}`)))
	})

	t.Run("Read file_path field", func(t *testing.T) {
		fn := DefaultSpecifierFields["Read"]
		assert.Equal(t, "/path/to/file", fn([]byte(`{"file_path": "/path/to/file"}`)))
	})

	t.Run("WebFetch url field", func(t *testing.T) {
		fn := DefaultSpecifierFields["WebFetch"]
		assert.Equal(t, "https://example.com", fn([]byte(`{"url": "https://example.com"}`)))
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		fn := DefaultSpecifierFields["Bash"]
		assert.Equal(t, "", fn([]byte(`{}`)))
	})

	t.Run("Write file_path field", func(t *testing.T) {
		fn := DefaultSpecifierFields["Write"]
		assert.Equal(t, "/tmp/out.txt", fn([]byte(`{"file_path": "/tmp/out.txt"}`)))
	})

	t.Run("Edit filePath field", func(t *testing.T) {
		fn := DefaultSpecifierFields["Edit"]
		assert.Equal(t, "main.go", fn([]byte(`{"filePath": "main.go"}`)))
	})

	t.Run("invalid JSON returns empty", func(t *testing.T) {
		fn := DefaultSpecifierFields["Bash"]
		assert.Equal(t, "", fn([]byte(`not json`)))
	})
}

func TestParseRuleWithSpecifier(t *testing.T) {
	rule := ParseRuleWithSpecifier(RuleAllow, "Bash", "go test*")
	assert.Equal(t, RuleAllow, rule.Type)
	assert.Equal(t, "Bash", rule.Tool)
	assert.Equal(t, "go test*", rule.Specifier)

	rule = ParseRuleWithSpecifier(RuleDeny, "Read", "/etc/**")
	assert.Equal(t, RuleDeny, rule.Type)
	assert.Equal(t, "Read", rule.Tool)
	assert.Equal(t, "/etc/**", rule.Specifier)
}

func TestInputMatchRule(t *testing.T) {
	t.Run("InputMatch allows when matcher returns true", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				{
					Type: RuleAllow,
					Tool: "Bash",
					InputMatch: func(input any) bool {
						m, ok := input.(map[string]any)
						if !ok {
							return false
						}
						cmd, _ := m["command"].(string)
						return cmd == "ls"
					},
				},
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("InputMatch denies when matcher returns false", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				{
					Type: RuleDeny,
					Tool: "Bash",
					InputMatch: func(input any) bool {
						m, ok := input.(map[string]any)
						if !ok {
							return false
						}
						cmd, _ := m["command"].(string)
						return cmd == "rm -rf /"
					},
					Message: "dangerous",
				},
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}

		// Matching input - should deny
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "dangerous", err.Error())

		// Non-matching input - should fall through to default (nil dialog = auto-allow)
		call = &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}
		err = manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("InputMatch with invalid JSON does not match", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				{
					Type: RuleAllow,
					Tool: "Bash",
					InputMatch: func(input any) bool {
						return true // would match if JSON parses
					},
				},
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`not json`)}
		// InputMatch path fails to unmarshal, so rule does not match.
		// Falls through to default (nil dialog = auto-allow).
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})
}

func TestIsEditOperation(t *testing.T) {
	t.Run("acceptEdits allows tools with EditHint annotation", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "CustomEditor", annotations: &dive.ToolAnnotations{EditHint: true}}
		call := &llm.ToolUseContent{Name: "CustomEditor", Input: []byte(`{}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("acceptEdits allows tools with edit names", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		for _, name := range []string{"Edit", "Write", "Create", "Mkdir", "Touch", "FileWrite", "CreateFile"} {
			tool := &mockTool{name: name}
			call := &llm.ToolUseContent{Name: name, Input: []byte(`{}`)}
			err := manager.EvaluateToolUse(context.Background(), tool, call)
			assert.NoError(t, err, "expected %s to be allowed in acceptEdits mode", name)
		}
	})

	t.Run("acceptEdits falls through for non-edit tools", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		// Non-edit tool with no dialog = auto-allow via confirm
		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err) // nil dialog = auto-allow
	})

	t.Run("acceptEdits with nil tool", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		// nil tool: isEditOperation returns false, falls through to confirm (nil dialog = auto-allow)
		err := manager.EvaluateToolUse(context.Background(), nil, call)
		assert.NoError(t, err)
	})
}

func TestCustomSpecifierFields(t *testing.T) {
	t.Run("custom specifier field takes precedence", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowSpecifierRule("Bash", "safe*"),
			},
			SpecifierFields: map[string]SpecifierFieldFunc{
				"Bash": func(input json.RawMessage) string {
					return jsonStringField(input, "script")
				},
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}

		// Uses custom "script" field, not default "command"
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"script": "safe-run", "command": "dangerous"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)

		// Custom field doesn't match, falls through
		call = &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"script": "danger", "command": "safe-run"}`)}
		err = manager.EvaluateToolUse(context.Background(), tool, call)
		// Falls through to default (nil dialog = auto-allow)
		assert.NoError(t, err)
	})
}

func TestMatchGlobExtended(t *testing.T) {
	t.Run("question mark wildcard", func(t *testing.T) {
		assert.True(t, MatchGlob("Bas?", "Bash"))
		assert.True(t, MatchGlob("Bas?", "Bass"))
		assert.False(t, MatchGlob("Bas?", "Ba"))
		assert.False(t, MatchGlob("Bas?", "Basher"))
	})

	t.Run("unclosed brace treated as literal", func(t *testing.T) {
		assert.True(t, MatchGlob("{unclosed", "{unclosed"))
		assert.False(t, MatchGlob("{unclosed", "unclosed"))
	})

	t.Run("special regex chars are escaped", func(t *testing.T) {
		assert.True(t, MatchGlob("file.go", "file.go"))
		assert.False(t, MatchGlob("file.go", "filexgo"))
	})
}

func TestMatchPathExtended(t *testing.T) {
	t.Run("question mark in path mode", func(t *testing.T) {
		assert.True(t, MatchPath("/path/to/fil?", "/path/to/file"))
		assert.False(t, MatchPath("/path/to/fil?", "/path/to/fi"))
		// ? should not cross directory boundary in path mode
		assert.False(t, MatchPath("/path/to?file", "/path/to/file"))
	})

	t.Run("star does not cross directories in path mode", func(t *testing.T) {
		assert.True(t, MatchPath("/path/*/file", "/path/to/file"))
		assert.False(t, MatchPath("/path/*/file", "/path/to/sub/file"))
	})
}

func TestEvaluateNilInputs(t *testing.T) {
	t.Run("nil tool and nil call", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, nil)
		// nil tool/call: rules return noDecision, evaluateMode uses default,
		// nil dialog = auto-allow
		err := manager.EvaluateToolUse(context.Background(), nil, nil)
		assert.NoError(t, err)
	})

	t.Run("nil tool with bypass mode", func(t *testing.T) {
		config := &Config{Mode: ModeBypassPermissions}
		manager := NewManager(config, nil)
		err := manager.EvaluateToolUse(context.Background(), nil, nil)
		assert.NoError(t, err)
	})
}

func TestPlanModeWithNilAnnotations(t *testing.T) {
	config := &Config{Mode: ModePlan}
	manager := NewManager(config, nil)

	// Tool with nil annotations should be denied
	tool := &mockTool{name: "CustomTool", annotations: nil}
	call := &llm.ToolUseContent{Name: "CustomTool", Input: []byte(`{}`)}
	err := manager.EvaluateToolUse(context.Background(), tool, call)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plan mode")
}

// Integration-style tests that exercise the full permission flow.
func TestIntegrationFlow(t *testing.T) {
	t.Run("realistic agent config: allow reads, ask bash, deny rm", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Read"),
				AllowRule("Glob"),
				AllowRule("Grep"),
				DenySpecifierRule("Bash", "rm *", "rm commands are not allowed"),
				AskRule("Bash", "Execute shell command?"),
				AllowRule("Edit"),
			},
		}

		var dialogCalls int
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			dialogCalls++
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)
		ctx := context.Background()

		// Read should be allowed without dialog
		err := manager.EvaluateToolUse(ctx,
			&mockTool{name: "Read", annotations: &dive.ToolAnnotations{ReadOnlyHint: true}},
			&llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path": "main.go"}`)})
		assert.NoError(t, err)
		assert.Equal(t, 0, dialogCalls)

		// Glob should be allowed without dialog
		err = manager.EvaluateToolUse(ctx,
			&mockTool{name: "Glob", annotations: &dive.ToolAnnotations{ReadOnlyHint: true}},
			&llm.ToolUseContent{Name: "Glob", Input: []byte(`{"pattern": "*.go"}`)})
		assert.NoError(t, err)
		assert.Equal(t, 0, dialogCalls)

		// rm command should be denied by specifier rule
		err = manager.EvaluateToolUse(ctx,
			&mockTool{name: "Bash"},
			&llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /"}`)})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rm commands are not allowed")
		assert.Equal(t, 0, dialogCalls) // deny rules don't trigger dialog

		// Safe bash should trigger ask dialog
		err = manager.EvaluateToolUse(ctx,
			&mockTool{name: "Bash"},
			&llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "go test ./..."}`)})
		assert.NoError(t, err)
		assert.Equal(t, 1, dialogCalls)

		// Edit should be allowed without dialog
		err = manager.EvaluateToolUse(ctx,
			&mockTool{name: "Edit"},
			&llm.ToolUseContent{Name: "Edit", Input: []byte(`{"file_path": "main.go"}`)})
		assert.NoError(t, err)
		assert.Equal(t, 1, dialogCalls)
	})

	t.Run("mode transition: default -> plan -> bypass -> dontAsk", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, nil)
		ctx := context.Background()

		bashTool := &mockTool{name: "Bash"}
		readTool := &mockTool{name: "Read", annotations: &dive.ToolAnnotations{ReadOnlyHint: true}}
		bashCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		readCall := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}

		// Default mode: both allowed (nil dialog = auto-allow)
		assert.NoError(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))
		assert.NoError(t, manager.EvaluateToolUse(ctx, readTool, readCall))

		// Plan mode: only read-only allowed
		manager.SetMode(ModePlan)
		assert.Error(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))
		assert.NoError(t, manager.EvaluateToolUse(ctx, readTool, readCall))

		// Bypass mode: everything allowed
		manager.SetMode(ModeBypassPermissions)
		assert.NoError(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))
		assert.NoError(t, manager.EvaluateToolUse(ctx, readTool, readCall))

		// DontAsk mode: everything denied without explicit rules
		manager.SetMode(ModeDontAsk)
		assert.Error(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))
		assert.Error(t, manager.EvaluateToolUse(ctx, readTool, readCall))
	})

	t.Run("session allowlist overrides mode", func(t *testing.T) {
		config := &Config{Mode: ModeDontAsk}
		manager := NewManager(config, nil)
		ctx := context.Background()

		bashTool := &mockTool{name: "Bash"}
		bashCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// DontAsk denies everything
		assert.Error(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))

		// Session allowlist overrides
		manager.AllowForSession("bash")
		assert.NoError(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))

		// Clear and verify it's denied again
		manager.ClearSessionAllowlist()
		assert.Error(t, manager.EvaluateToolUse(ctx, bashTool, bashCall))
	})

	t.Run("rules override mode", func(t *testing.T) {
		config := &Config{
			Mode: ModeBypassPermissions,
			Rules: Rules{
				DenyRule("Bash", "bash is forbidden"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()

		bashTool := &mockTool{name: "Bash"}
		bashCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// Deny rule takes precedence even in bypass mode
		err := manager.EvaluateToolUse(ctx, bashTool, bashCall)
		assert.Error(t, err)
		assert.Equal(t, "bash is forbidden", err.Error())
	})

	t.Run("MCP tool glob patterns with specifiers", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("mcp__*"),
				DenySpecifierRule("Bash", "curl *evil*", "blocked URL"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()

		// MCP tools should all be allowed
		for _, name := range []string{"mcp__ide__getDiagnostics", "mcp__git__status", "mcp__custom__tool"} {
			tool := &mockTool{name: name}
			call := &llm.ToolUseContent{Name: name, Input: []byte(`{}`)}
			assert.NoError(t, manager.EvaluateToolUse(ctx, tool, call))
		}

		// Non-MCP tool should not match MCP rule
		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}
		// Falls through to default (nil dialog = auto-allow)
		assert.NoError(t, manager.EvaluateToolUse(ctx, tool, call))
	})

	t.Run("brace alternatives in tool patterns", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("{Read,Glob,Grep}"),
			},
		}
		manager := NewManager(config, &dive.DenyAllDialog{})
		ctx := context.Background()

		// Matching tools should be allowed
		for _, name := range []string{"Read", "Glob", "Grep"} {
			tool := &mockTool{name: name}
			call := &llm.ToolUseContent{Name: name, Input: []byte(`{}`)}
			assert.NoError(t, manager.EvaluateToolUse(ctx, tool, call))
		}

		// Non-matching tool should be denied (DenyAllDialog)
		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		assert.Error(t, manager.EvaluateToolUse(ctx, tool, call))
	})

	t.Run("hook integration with agent HookContext", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Read"),
				DenyRule("Bash", "not allowed"),
			},
		}
		hook := Hook(config, nil)

		// Simulate agent calling PreToolUse hook
		readCtx := &dive.HookContext{
			Tool: &mockTool{name: "Read"},
			Call: &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)},
		}
		assert.NoError(t, hook(context.Background(), readCtx))

		bashCtx := &dive.HookContext{
			Tool: &mockTool{name: "Bash"},
			Call: &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)},
		}
		err := hook(context.Background(), bashCtx)
		assert.Error(t, err)
		assert.Equal(t, "not allowed", err.Error())
	})

	t.Run("audit hook captures all tool calls", func(t *testing.T) {
		var calls []string
		auditHook := AuditHook(func(name string, input []byte) {
			calls = append(calls, name)
		})

		// Simulate a sequence of tool calls
		tools := []string{"Read", "Bash", "Edit", "Glob"}
		for _, name := range tools {
			hookCtx := &dive.HookContext{
				Tool: &mockTool{name: name},
				Call: &llm.ToolUseContent{Name: name, Input: []byte(`{}`)},
			}
			err := auditHook(context.Background(), hookCtx)
			assert.NoError(t, err) // audit hook never blocks
		}
		assert.Equal(t, tools, calls)
	})

	t.Run("HookFromManager preserves manager state", func(t *testing.T) {
		config := &Config{Mode: ModeDontAsk}
		manager := NewManager(config, nil)
		hook := HookFromManager(manager)
		ctx := context.Background()

		bashCtx := &dive.HookContext{
			Tool: &mockTool{name: "Bash"},
			Call: &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)},
		}

		// Initially denied
		assert.Error(t, hook(ctx, bashCtx))

		// Allow via session allowlist on the same manager
		manager.AllowForSession("bash")
		assert.NoError(t, hook(ctx, bashCtx))

		// Change mode on the same manager
		manager.SetMode(ModeBypassPermissions)
		manager.ClearSessionAllowlist()
		assert.NoError(t, hook(ctx, bashCtx))
	})

	t.Run("specifier with no matching default field", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowSpecifierRule("CustomTool", "safe*"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()

		tool := &mockTool{name: "CustomTool"}
		// CustomTool has no default specifier field, so specifier is "",
		// and the specifier rule doesn't match. Falls through to default.
		call := &llm.ToolUseContent{Name: "CustomTool", Input: []byte(`{"value": "safe-thing"}`)}
		err := manager.EvaluateToolUse(ctx, tool, call)
		assert.NoError(t, err) // nil dialog = auto-allow
	})

	t.Run("audit hook with nil logger is safe", func(t *testing.T) {
		hook := AuditHook(nil)
		hookCtx := &dive.HookContext{
			Tool: &mockTool{name: "Read"},
			Call: &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)},
		}
		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
	})
}
