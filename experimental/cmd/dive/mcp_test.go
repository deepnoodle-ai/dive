package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/mcp"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/permission"
	"github.com/deepnoodle-ai/wonton/assert"
)

func writeTestJSON(t *testing.T, path, content string) {
	t.Helper()
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func mcpEntryByName(servers []mcpServerEntry, name string) *mcpServerEntry {
	for i := range servers {
		if servers[i].Config.Name == name {
			return &servers[i]
		}
	}
	return nil
}

func TestLoadMCPServerConfigsPrecedence(t *testing.T) {
	home := useTestUserSettingsRoot(t)
	workspace := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".dive", "settings.json"), `{
		"model": "x",
		"enabledMcpjsonServers": ["shared", "project-only"],
		"mcpServers": {
			"user-only": {"command": "user-cmd"},
			"shared": {"command": "user-shared"}
		}
	}`)
	writeTestJSON(t, filepath.Join(workspace, ".mcp.json"), `{"mcpServers": {
		"shared": {"command": "project-shared"},
		"project-only": {"type": "http", "url": "https://example.com/mcp"},
		"flagged": {"command": "project-flagged"}
	}}`)
	first := filepath.Join(t.TempDir(), "first.json")
	second := filepath.Join(t.TempDir(), "second.json")
	writeTestJSON(t, first, `{"mcpServers": {"flagged": {"command": "first"}, "from-flag": {"command": "flag-cmd"}}}`)
	writeTestJSON(t, second, `{"mcpServers": {"flagged": {"command": "second"}}}`)

	result := loadMCPServerConfigs(workspace, []string{first, second})
	assert.Len(t, result.Warnings, 0)
	assert.Len(t, result.Servers, 5)

	// Sorted by name.
	var names []string
	for _, s := range result.Servers {
		names = append(names, s.Config.Name)
	}
	assert.Equal(t, []string{"flagged", "from-flag", "project-only", "shared", "user-only"}, names)

	assert.Equal(t, "user-cmd", mcpEntryByName(result.Servers, "user-only").Config.Command)
	assert.Equal(t, mcpSourceUser, mcpEntryByName(result.Servers, "user-only").Source)
	// Approved project server beats the user's definition.
	assert.Equal(t, "project-shared", mcpEntryByName(result.Servers, "shared").Config.Command)
	assert.Equal(t, mcpSourceProject, mcpEntryByName(result.Servers, "shared").Source)
	assert.Equal(t, "http", mcpEntryByName(result.Servers, "project-only").Config.Type)
	// --mcp-config wins, later files over earlier ones, and approves project
	// servers by redefining them.
	assert.Equal(t, "second", mcpEntryByName(result.Servers, "flagged").Config.Command)
	assert.Equal(t, second, mcpEntryByName(result.Servers, "flagged").Source)
	// "flagged" was not approved in .mcp.json, but the flag supplied it, so
	// it is only listed as unapproved for the project file.
	assert.Equal(t, []string{"flagged"}, result.Unapproved)
}

func TestLoadMCPServerConfigsProjectApproval(t *testing.T) {
	t.Run("unapproved servers do not start", func(t *testing.T) {
		useTestUserSettingsRoot(t)
		workspace := t.TempDir()
		writeTestJSON(t, filepath.Join(workspace, ".mcp.json"), `{"mcpServers": {"b": {"command": "b"}, "a": {"command": "a"}}}`)
		result := loadMCPServerConfigs(workspace, nil)
		assert.Len(t, result.Servers, 0)
		assert.Equal(t, []string{"a", "b"}, result.Unapproved)
	})

	t.Run("enable all, minus disabled", func(t *testing.T) {
		home := useTestUserSettingsRoot(t)
		workspace := t.TempDir()
		writeTestJSON(t, filepath.Join(home, ".dive", "settings.json"),
			`{"enableAllProjectMcpServers": true, "disabledMcpjsonServers": ["b"]}`)
		writeTestJSON(t, filepath.Join(workspace, ".mcp.json"), `{"mcpServers": {"b": {"command": "b"}, "a": {"command": "a"}}}`)
		result := loadMCPServerConfigs(workspace, nil)
		assert.Len(t, result.Servers, 1)
		assert.Equal(t, "a", result.Servers[0].Config.Name)
		assert.Equal(t, []string{"b"}, result.Unapproved)
	})

	t.Run("project dive settings cannot approve", func(t *testing.T) {
		useTestUserSettingsRoot(t)
		workspace := t.TempDir()
		writeTestJSON(t, filepath.Join(workspace, ".dive", "settings.json"), `{"enableAllProjectMcpServers": true}`)
		writeTestJSON(t, filepath.Join(workspace, ".mcp.json"), `{"mcpServers": {"a": {"command": "a"}}}`)
		result := loadMCPServerConfigs(workspace, nil)
		assert.Len(t, result.Servers, 0)
		assert.Equal(t, []string{"a"}, result.Unapproved)
	})
}

func TestLoadMCPServerConfigsWarnings(t *testing.T) {
	home := useTestUserSettingsRoot(t)
	workspace := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".dive", "settings.json"), `{"mcpServers": {"ok": {"command": "x"}, "bad": {"type": "ftp"}}}`)
	writeTestJSON(t, filepath.Join(workspace, ".mcp.json"), `{not json`)

	result := loadMCPServerConfigs(workspace, []string{filepath.Join(workspace, "missing.json")})
	assert.Len(t, result.Servers, 1)
	assert.Equal(t, "ok", result.Servers[0].Config.Name)
	assert.Len(t, result.Warnings, 3)
	joined := strings.Join(result.Warnings, "\n")
	assert.Contains(t, joined, `"bad"`)
	assert.Contains(t, joined, ".mcp.json")
	assert.Contains(t, joined, "missing.json")
}

func TestMCPConnectTimeout(t *testing.T) {
	t.Setenv("MCP_TIMEOUT", "")
	assert.Equal(t, defaultMCPConnectTimeout, mcpConnectTimeout())
	t.Setenv("MCP_TIMEOUT", "1500")
	assert.Equal(t, 1500*time.Millisecond, mcpConnectTimeout())
	t.Setenv("MCP_TIMEOUT", "nope")
	assert.Equal(t, defaultMCPConnectTimeout, mcpConnectTimeout())
}

var (
	testMCPServerOnce sync.Once
	testMCPServerPath string
	testMCPServerErr  error
)

// buildTestMCPServer compiles testdata/mcpserver once per test binary.
func buildTestMCPServer(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	testMCPServerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dive-mcpserver-")
		if err != nil {
			testMCPServerErr = err
			return
		}
		testMCPServerPath = filepath.Join(dir, "mcpserver")
		out, err := exec.Command("go", "build", "-o", testMCPServerPath, "./testdata/mcpserver").CombinedOutput()
		if err != nil {
			testMCPServerErr = &buildError{err: err, out: string(out)}
		}
	})
	if testMCPServerErr != nil {
		t.Fatalf("build test MCP server: %v", testMCPServerErr)
	}
	return testMCPServerPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + ": " + e.out }

func findTool(tools []dive.Tool, name string) dive.Tool {
	for _, tool := range tools {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}

func TestConnectMCPServersStdio(t *testing.T) {
	serverPath := buildTestMCPServer(t)
	cfg := mcpConfigResult{Servers: []mcpServerEntry{
		{Config: &mcp.ServerConfig{Name: "demo", Type: "stdio", Command: serverPath}, Source: "test"},
		{Config: &mcp.ServerConfig{Name: "broken", Type: "stdio", Command: filepath.Join(t.TempDir(), "does-not-exist")}, Source: "test"},
	}}

	rt := connectMCPServers(context.Background(), cfg, 10*time.Second)
	defer rt.Close()

	servers, toolCount := rt.connectedCount()
	assert.Equal(t, 1, servers)
	assert.Equal(t, 2, toolCount)
	assert.Equal(t, 1, rt.failedCount())
	assert.Contains(t, rt.summary(), "1 server connected (2 tools)")
	assert.Contains(t, rt.summary(), "1 failed")
	assert.Contains(t, strings.Join(rt.problems(), "\n"), `"broken" failed to start`)

	echo := findTool(rt.Tools(), "mcp__demo__echo")
	assert.NotNil(t, echo)
	result, err := echo.Call(context.Background(), json.RawMessage(`{"message":"hi"}`))
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, dive.ToolResultContentTypeText, result.Content[0].Type)
	assert.Equal(t, "echo: hi", result.Content[0].Text)

	img := findTool(rt.Tools(), "mcp__demo__test_image")
	assert.NotNil(t, img)
	result, err = img.Call(context.Background(), json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, dive.ToolResultContentTypeImage, result.Content[0].Type)
	assert.Equal(t, "image/png", result.Content[0].MimeType)
	data, err := base64.StdEncoding.DecodeString(result.Content[0].Data)
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "\x89PNG"), "decoded data is a PNG")

	report := strings.Join(rt.report(), "\n")
	assert.Contains(t, report, "✓ demo (stdio, from test): 2 tools")
	assert.Contains(t, report, "mcp__demo__test_image")
	assert.Contains(t, report, "✗ broken")
}

func TestConnectMCPServersTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for a stuck server")
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	cfg := mcpConfigResult{Servers: []mcpServerEntry{
		{Config: &mcp.ServerConfig{Name: "stuck", Type: "stdio", Command: sleep, Args: []string{"30"}}},
	}}
	start := time.Now()
	rt := connectMCPServers(context.Background(), cfg, 200*time.Millisecond)
	defer rt.Close()
	assert.Equal(t, 1, rt.failedCount())
	assert.Contains(t, rt.servers[0].Err.Error(), "timed out")
	assert.Len(t, rt.Tools(), 0)
	// The stuck child is stopped rather than awaited for its full sleep.
	assert.True(t, time.Since(start) < 15*time.Second)
}

func TestConnectMCPServersDuplicateNames(t *testing.T) {
	serverPath := buildTestMCPServer(t)
	// "a.b" and "a_b" sanitize to the same qualified prefix.
	cfg := mcpConfigResult{Servers: []mcpServerEntry{
		{Config: &mcp.ServerConfig{Name: "a.b", Type: "stdio", Command: serverPath}},
		{Config: &mcp.ServerConfig{Name: "a_b", Type: "stdio", Command: serverPath}},
	}}
	rt := connectMCPServers(context.Background(), cfg, 10*time.Second)
	defer rt.Close()
	assert.Len(t, rt.Tools(), 2)
	assert.Len(t, rt.servers[1].Tools, 0)
	assert.Len(t, rt.servers[1].Notes, 2)
	assert.Contains(t, rt.servers[1].Notes[0], `already used by server "a.b"`)
}

// recordingDialog denies every confirmation and remembers what it was asked.
type recordingDialog struct{ asked []string }

func (d *recordingDialog) Show(_ context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	if in.Tool != nil {
		d.asked = append(d.asked, in.Tool.Name())
	}
	return &dive.DialogOutput{Confirmed: false}, nil
}

func TestMCPToolsRequirePermission(t *testing.T) {
	serverPath := buildTestMCPServer(t)
	rt := connectMCPServers(context.Background(), mcpConfigResult{Servers: []mcpServerEntry{
		{Config: &mcp.ServerConfig{Name: "demo", Type: "stdio", Command: serverPath}},
	}}, 10*time.Second)
	defer rt.Close()

	// test_image claims readOnlyHint, but the CLI builds its default allow
	// rules from built-in tools only, so the MCP tool still prompts.
	img := findTool(rt.Tools(), "mcp__demo__test_image")
	assert.NotNil(t, img)
	assert.True(t, img.Annotations().ReadOnlyHint)

	dialog := &recordingDialog{}
	manager := permission.NewManager(&permission.Config{
		Mode:  permission.ModeDefault,
		Rules: defaultPermissionRules(nil),
	}, dialog)
	call := &llm.ToolUseContent{Name: img.Name(), Input: []byte(`{}`)}
	assert.Error(t, manager.EvaluateToolUse(context.Background(), img, call))
	assert.Equal(t, []string{"mcp__demo__test_image"}, dialog.asked)

	// A Claude Code-style server wildcard allows it.
	allowed := permission.NewManager(&permission.Config{
		Mode:  permission.ModeDefault,
		Rules: permission.Rules{permission.AllowRule("mcp__demo__*")},
	}, dialog)
	assert.NoError(t, allowed.EvaluateToolUse(context.Background(), img, call))
}

func TestMCPSlashCommandAndIntro(t *testing.T) {
	app, fake := newFakeApp(t)
	assert.True(t, app.handleCommand("/mcp", nil))
	assert.Contains(t, fake.scrollback(120), "No MCP servers configured.")

	app, fake = newFakeApp(t)
	app.mcp = &mcpRuntime{
		servers: []mcpServerStatus{
			{Name: "demo", Type: "stdio", Source: ".mcp.json", Tools: []string{"mcp__demo__echo"}},
			{Name: "down", Type: "http", Source: "~/.dive/settings.json", Err: context.DeadlineExceeded},
		},
		unapproved: []string{"other"},
	}
	app.emit(app.appendIntro())
	assert.Contains(t, fake.scrollback(160), "mcp: 1 server connected (0 tools), 1 failed, 1 needs approval · /mcp for details")

	assert.True(t, app.handleCommand("/mcp", nil))
	out := fake.scrollback(200)
	assert.Contains(t, out, "✓ demo (stdio, from .mcp.json): 1 tool")
	assert.Contains(t, out, "mcp__demo__echo")
	assert.Contains(t, out, "✗ down (http, from ~/.dive/settings.json): failed")
	assert.Contains(t, out, `"enabledMcpjsonServers": ["other"]`)
}

// scriptedMCPModel asks for one tool call, then records what came back.
type scriptedMCPModel struct {
	tool     string
	calls    int
	received []*llm.Message
}

func (m *scriptedMCPModel) Name() string { return "scripted-mcp" }

func (m *scriptedMCPModel) Generate(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	m.calls++
	if m.calls == 1 {
		return &llm.Response{
			ID:         "resp_1",
			Model:      m.Name(),
			Role:       llm.Assistant,
			StopReason: "tool_use",
			Content: []llm.Content{&llm.ToolUseContent{
				ID: "call_1", Name: m.tool, Input: json.RawMessage(`{}`),
			}},
		}, nil
	}
	m.received = cfg.Messages
	return &llm.Response{
		ID:         "resp_2",
		Model:      m.Name(),
		Role:       llm.Assistant,
		StopReason: "stop",
		Content:    []llm.Content{&llm.TextContent{Text: "red and blue"}},
	}, nil
}

// TestMCPImageReachesModel runs an agent turn against the real server and
// checks the image block arrives in the tool result sent back to the model.
func TestMCPImageReachesModel(t *testing.T) {
	serverPath := buildTestMCPServer(t)
	rt := connectMCPServers(context.Background(), mcpConfigResult{Servers: []mcpServerEntry{
		{Config: &mcp.ServerConfig{Name: "demo", Type: "stdio", Command: serverPath}},
	}}, 10*time.Second)
	defer rt.Close()

	model := &scriptedMCPModel{tool: "mcp__demo__test_image"}
	agent, err := dive.NewAgent(dive.AgentOptions{Model: model, Tools: rt.Tools()})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("look"))
	assert.NoError(t, err)
	assert.Equal(t, 2, model.calls)

	var found *llm.ToolResultContent
	for _, msg := range model.received {
		for _, c := range msg.Content {
			if tr, ok := c.(*llm.ToolResultContent); ok && tr.ToolUseID == "call_1" {
				found = tr
			}
		}
	}
	assert.NotNil(t, found)
	assert.False(t, found.IsError, "%s", func() string { b, _ := json.Marshal(found.Content); return string(b) }())
	blocks, ok := found.Content.([]*dive.ToolResultContent)
	assert.True(t, ok, "tool result content is %T", found.Content)
	assert.Len(t, blocks, 1)
	assert.Equal(t, dive.ToolResultContentTypeImage, blocks[0].Type)
	assert.Equal(t, "image/png", blocks[0].MimeType)
}
