package permission

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Regression tests for the 2026-06-09 API review, §1 (permission).

// §1.1: deny rules must be absolute — a session grant must not bypass them.
func TestDenyBeatsSessionAllowlist(t *testing.T) {
	t.Run("legacy category grant does not bypass deny", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Bash", "*rm -rf*", "Recursive deletion blocked"),
			},
		}
		manager := NewManager(config, nil)
		manager.AllowForSession("bash")

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /tmp/dive-test-no-such-dir"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "Recursive deletion blocked", err.Error())

		// The grant still covers non-denied commands
		ok := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}
		assert.NoError(t, manager.EvaluateToolUse(context.Background(), tool, ok))
	})

	t.Run("scoped grant does not bypass deny", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Bash", "*rm -rf*", "blocked"),
			},
		}
		manager := NewManager(config, nil)
		manager.AllowToolForSession("Bash", "")

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /tmp/dive-test-no-such-dir"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
	})

	t.Run("dialog session approval does not disarm deny rules", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Bash", "*rm -rf*", "blocked"),
			},
		}
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: true, AllowSession: true}, nil
		}}
		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		benign := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}
		assert.NoError(t, manager.EvaluateToolUse(context.Background(), tool, benign))

		evil := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /tmp/dive-test-no-such-dir"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, evil)
		assert.Error(t, err)
		assert.Equal(t, "blocked", err.Error())
	})
}

// §1.3: specifier-bearing deny rules fail closed when no specifier can be
// extracted from the tool input.
func TestDenyFailsClosed(t *testing.T) {
	t.Run("unknown tool shape matches deny", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("CustomExec", "*rm*", "blocked"),
			},
		}
		manager := NewManager(config, nil)

		// CustomExec has no specifier extractor, so the specifier is "".
		tool := &mockTool{name: "CustomExec"}
		call := &llm.ToolUseContent{Name: "CustomExec", Input: []byte(`{"program": "rm -rf /tmp/dive-test-no-such-dir"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "blocked", err.Error())
	})

	t.Run("missing field matches deny", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Bash", "rm *", "blocked"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"unexpected": "rm -rf /tmp/dive-test-no-such-dir"}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
	})

	t.Run("allow rules still fail open on missing specifier", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("Bash", "go test*"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"unexpected": "go test"}`)}
		// No specifier extracted: allow rule does not match, dontAsk denies.
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
	})
}

// §1.4: command-aware matching for Bash specifiers.
func TestCommandAwareMatching(t *testing.T) {
	t.Run("allow rule does not authorize chained commands", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("Bash", "go test *"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()
		tool := &mockTool{name: "Bash"}

		allowed := []string{
			"go test ./...",
			"go test -v ./permission/",
		}
		for _, cmd := range allowed {
			call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "` + cmd + `"}`)}
			assert.NoError(t, manager.EvaluateToolUse(ctx, tool, call), "expected allow: %s", cmd)
		}

		denied := []string{
			"go test ./...; rm -rf /tmp/dive-test-no-such-dir",
			"go test ./... && rm -rf /tmp/dive-test-no-such-dir",
			"go test ./... || rm -rf /tmp/dive-test-no-such-dir",
			"go test ./... | cat",
			"go test ./... & rm -rf /tmp/dive-test-no-such-dir",
			"go test $(rm -rf /tmp/dive-test-no-such-dir)",
			"go test `rm -rf /tmp/dive-test-no-such-dir`",
			"rm -rf /tmp/dive-test-no-such-dir # go test",
		}
		for _, cmd := range denied {
			call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "` + cmd + `"}`)}
			assert.Error(t, manager.EvaluateToolUse(ctx, tool, call), "expected deny: %s", cmd)
		}

		// Newline-chained command (JSON-escaped newline)
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "go test ./...\nrm -rf /tmp/dive-test-no-such-dir"}`)}
		assert.Error(t, manager.EvaluateToolUse(ctx, tool, call))
	})

	t.Run("deny rule is not escaped by newlines or chaining", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Bash", "*rm*", "blocked"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()
		tool := &mockTool{name: "Bash"}

		for _, cmd := range []string{
			"rm -rf /tmp/dive-test-no-such-dir",
			"ls\nrm -rf /tmp/dive-test-no-such-dir",
			"ls; rm -rf /tmp/dive-test-no-such-dir",
			"ls && rm -rf /tmp/dive-test-no-such-dir",
		} {
			call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": ` + jsonQuote(cmd) + `}`)}
			err := manager.EvaluateToolUse(ctx, tool, call)
			assert.Error(t, err, "expected deny: %q", cmd)
		}
	})

	t.Run("quoted operators do not split", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("Bash", "echo *"),
			},
		}
		manager := NewManager(config, nil)
		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "echo 'a; b'"}`)}
		assert.NoError(t, manager.EvaluateToolUse(context.Background(), tool, call))
	})
}

// §1.5: path specifiers are cleaned and segment-aware.
func TestPathSpecifierMatching(t *testing.T) {
	t.Run("traversal does not escape an allow rule", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("Read", "/safe/dir/*"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()
		tool := &mockTool{name: "Read"}

		ok := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path": "/safe/dir/file.txt"}`)}
		assert.NoError(t, manager.EvaluateToolUse(ctx, tool, ok))

		traversal := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path": "/safe/dir/../../etc/shadow"}`)}
		assert.Error(t, manager.EvaluateToolUse(ctx, tool, traversal))

		// * does not cross directory boundaries; ** is required for that
		nested := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path": "/safe/dir/sub/file.txt"}`)}
		assert.Error(t, manager.EvaluateToolUse(ctx, tool, nested))
	})

	t.Run("double star allows nested paths", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("Read", "/safe/dir/**"),
			},
		}
		manager := NewManager(config, nil)
		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path": "/safe/dir/sub/deep/file.txt"}`)}
		assert.NoError(t, manager.EvaluateToolUse(context.Background(), tool, call))
	})

	t.Run("deny rule catches relative traversal to an absolute target", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("Read", "/etc/**", "no reading /etc"),
			},
		}
		manager := NewManager(config, nil)
		tool := &mockTool{name: "Read"}

		// Enough ../ to reach the root from any cwd, then into /etc.
		rel := strings.Repeat("../", 64) + "etc/shadow"
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path": ` + jsonQuote(rel) + `}`)}
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "no reading /etc", err.Error())
	})
}

// §1.6: domain matching is case/trailing-dot tolerant and wired into
// WebFetch rules.
func TestURLSpecifierMatching(t *testing.T) {
	t.Run("domain pattern is not fooled by lookalike hosts", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("WebFetch", "domain:example.com"),
			},
		}
		manager := NewManager(config, nil)
		ctx := context.Background()
		tool := &mockTool{name: "WebFetch"}

		for _, url := range []string{
			"https://example.com/page",
			"https://sub.example.com/page",
			"HTTPS://EXAMPLE.COM/page",
			"https://example.com./page",
		} {
			call := &llm.ToolUseContent{Name: "WebFetch", Input: []byte(`{"url": ` + jsonQuote(url) + `}`)}
			assert.NoError(t, manager.EvaluateToolUse(ctx, tool, call), "expected allow: %s", url)
		}

		for _, url := range []string{
			"https://example.com.attacker.net/",
			"https://notexample.com/",
		} {
			call := &llm.ToolUseContent{Name: "WebFetch", Input: []byte(`{"url": ` + jsonQuote(url) + `}`)}
			assert.Error(t, manager.EvaluateToolUse(ctx, tool, call), "expected deny: %s", url)
		}
	})

	t.Run("bare domain pattern is treated as a domain match", func(t *testing.T) {
		config := &Config{
			Mode: ModeDontAsk,
			Rules: Rules{
				AllowSpecifierRule("WebFetch", "example.com"),
			},
		}
		manager := NewManager(config, nil)
		tool := &mockTool{name: "WebFetch"}

		ok := &llm.ToolUseContent{Name: "WebFetch", Input: []byte(`{"url": "https://api.example.com/v1"}`)}
		assert.NoError(t, manager.EvaluateToolUse(context.Background(), tool, ok))

		bad := &llm.ToolUseContent{Name: "WebFetch", Input: []byte(`{"url": "https://example.com.evil.io/"}`)}
		assert.Error(t, manager.EvaluateToolUse(context.Background(), tool, bad))
	})

	t.Run("deny by domain is not escaped by case or trailing dot", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				DenySpecifierRule("WebFetch", "domain:internal.corp", "internal hosts blocked"),
			},
		}
		manager := NewManager(config, nil)
		tool := &mockTool{name: "WebFetch"}

		for _, url := range []string{
			"https://internal.corp/secrets",
			"HTTPS://INTERNAL.CORP/secrets",
			"https://internal.corp./secrets",
			"https://api.internal.corp/secrets",
		} {
			call := &llm.ToolUseContent{Name: "WebFetch", Input: []byte(`{"url": ` + jsonQuote(url) + `}`)}
			assert.Error(t, manager.EvaluateToolUse(context.Background(), tool, call), "expected deny: %s", url)
		}
	})
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		command  string
		segments []string
		hasSub   bool
	}{
		{"ls", []string{"ls"}, false},
		{"ls -la /tmp", []string{"ls -la /tmp"}, false},
		{"ls; rm -rf /tmp/dive-test-no-such-dir", []string{"ls", "rm -rf /tmp/dive-test-no-such-dir"}, false},
		{"ls && rm -rf /tmp/dive-test-no-such-dir", []string{"ls", "rm -rf /tmp/dive-test-no-such-dir"}, false},
		{"ls || rm -rf /tmp/dive-test-no-such-dir", []string{"ls", "rm -rf /tmp/dive-test-no-such-dir"}, false},
		{"ls | grep foo", []string{"ls", "grep foo"}, false},
		{"ls & rm -rf /tmp/dive-test-no-such-dir", []string{"ls", "rm -rf /tmp/dive-test-no-such-dir"}, false},
		{"ls\nrm -rf /tmp/dive-test-no-such-dir", []string{"ls", "rm -rf /tmp/dive-test-no-such-dir"}, false},
		{"echo 'a; b'", []string{"echo 'a; b'"}, false},
		{`echo "a && b"`, []string{`echo "a && b"`}, false},
		{`echo a\;b`, []string{`echo a\;b`}, false},
		{"echo $(whoami)", []string{"echo $(whoami)"}, true},
		{"echo `whoami`", []string{"echo `whoami`"}, true},
		{`echo "$(whoami)"`, []string{`echo "$(whoami)"`}, true},
		{"echo '$(whoami)'", []string{"echo '$(whoami)'"}, false},
		{"diff <(ls a) b", []string{"diff <(ls a) b"}, true},
		{"echo $HOME", []string{"echo $HOME"}, false},
		{"", nil, false},
		{"  ;  ; ", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			segments, hasSub := SplitCommand(tt.command)
			assert.Equal(t, tt.segments, segments)
			assert.Equal(t, tt.hasSub, hasSub)
		})
	}
}

func TestMatchCommand(t *testing.T) {
	t.Run("allow", func(t *testing.T) {
		assert.True(t, MatchCommandAllow("go test *", "go test ./..."))
		assert.False(t, MatchCommandAllow("go test *", "go test ./...; rm -rf /tmp/dive-test-no-such-dir"))
		assert.False(t, MatchCommandAllow("go test *", "go test $(rm -rf /tmp/dive-test-no-such-dir)"))
		assert.False(t, MatchCommandAllow("go test *", ""))
		// Both segments must match the pattern
		assert.True(t, MatchCommandAllow("git *", "git fetch && git status"))
		assert.False(t, MatchCommandAllow("git *", "cd /tmp && git status"))
	})

	t.Run("deny", func(t *testing.T) {
		assert.True(t, MatchCommandDeny("rm *", "rm -rf /tmp/dive-test-no-such-dir"))
		assert.True(t, MatchCommandDeny("rm *", "ls; rm -rf /tmp/dive-test-no-such-dir"))
		assert.True(t, MatchCommandDeny("*rm*", "ls\nrm -rf /tmp/dive-test-no-such-dir"))
		assert.False(t, MatchCommandDeny("rm *", "ls -la"))
	})
}

func TestMatchURLSpecifier(t *testing.T) {
	// domain: prefix
	assert.True(t, MatchURLSpecifier("domain:example.com", "https://example.com/x"))
	assert.True(t, MatchURLSpecifier("domain:example.com", "https://a.example.com/x"))
	assert.False(t, MatchURLSpecifier("domain:example.com", "https://example.com.evil.net/x"))

	// bare domain
	assert.True(t, MatchURLSpecifier("example.com", "https://example.com/x"))
	assert.False(t, MatchURLSpecifier("example.com", "https://example.com.evil.net/x"))

	// glob fallback for patterns with wildcards or paths
	assert.True(t, MatchURLSpecifier("https://example.com/api/*", "https://example.com/api/users"))
	assert.False(t, MatchURLSpecifier("https://example.com/api/*", "https://example.com/admin"))
}

// jsonQuote returns s as a JSON string literal.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
