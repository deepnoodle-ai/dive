package permission

import (
	"context"
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
}
