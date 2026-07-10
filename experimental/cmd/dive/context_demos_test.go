package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestParseContextDemoNames(t *testing.T) {
	selection, err := parseContextDemoNames([]string{"workspace,sources", "verification", "workspace"})
	assert.NoError(t, err)
	assert.True(t, selection.workspace)
	assert.True(t, selection.sources)
	assert.True(t, selection.verification)
	assert.False(t, selection.recovery)

	all, err := parseContextDemoNames([]string{"all"})
	assert.NoError(t, err)
	assert.True(t, all.workspace)
	assert.True(t, all.sources)
	assert.True(t, all.verification)
	assert.True(t, all.recovery)

	_, err = parseContextDemoNames([]string{"telepathy"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "workspace, sources, verification, recovery")
}

func TestWorkspaceSnapshotTracksGitState(t *testing.T) {
	workspace := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main", workspace)
	assert.NoError(t, cmd.Run())
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("hello\n"), 0o644))

	snapshot := workspaceSnapshot(context.Background(), workspace)
	assert.Contains(t, snapshot, "git branch: main")
	assert.Contains(t, snapshot, "1 changed path(s)")
	assert.Contains(t, snapshot, "note.txt")
}

func TestToolSourceSummary(t *testing.T) {
	tests := []struct {
		name  string
		call  *llm.ToolUseContent
		match string
		ok    bool
	}{
		{name: "read", call: &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file_path":"README.md"}`)}, match: "file: README.md", ok: true},
		{name: "grep", call: &llm.ToolUseContent{Name: "Grep", Input: []byte(`{"pattern":"TODO","path":"docs"}`)}, match: `text search: "TODO" in docs`, ok: true},
		{name: "web", call: &llm.ToolUseContent{Name: "WebFetch", Input: []byte(`{"url":"https://example.com"}`)}, match: "web page: https://example.com", ok: true},
		{name: "mutation is not evidence", call: &llm.ToolUseContent{Name: "Write", Input: []byte(`{"file_path":"main.go"}`)}, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, ok := toolSourceSummary(tt.call)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.match, summary)
		})
	}
}

func TestVerificationCommandDetection(t *testing.T) {
	for _, command := range []string{
		"go test ./...",
		"npm run test:unit",
		"make lint && go vet ./...",
		"echo prepare; pytest -q",
		"xcodebuild -scheme Dive test",
	} {
		assert.True(t, isVerificationCommand(command), command)
	}
	for _, command := range []string{
		"echo go test ./...",
		`echo "later; go test ./..."`,
		"go build ./...",
		"make release",
	} {
		assert.False(t, isVerificationCommand(command), command)
	}
}

func TestVerificationBatchDoesNotTreatParallelCheckAsCoverage(t *testing.T) {
	state := &contextDemoTurnState{}
	state.addBatchChange("main.go")
	state.addBatchCheck("go test ./...")
	first := state.applyVerificationBatch()
	assert.Len(t, first.checkedPaths, 0)
	assert.Equal(t, []string{"main.go"}, first.unverified)
	assert.True(t, first.emitDebt)

	unchanged := state.applyVerificationBatch()
	assert.Equal(t, []string{"main.go"}, unchanged.unverified)
	assert.False(t, unchanged.emitDebt)

	state.addBatchCheck("go test ./...")
	second := state.applyVerificationBatch()
	assert.Equal(t, []string{"main.go"}, second.checkedPaths)
	assert.Len(t, second.unverified, 0)

	// A new edit to the same path in the checking batch creates fresh debt.
	state.addBatchChange("main.go")
	state.addBatchCheck("go test ./...")
	third := state.applyVerificationBatch()
	assert.Len(t, third.checkedPaths, 0)
	assert.Equal(t, []string{"main.go"}, third.unverified)
}

type contextDemoScriptedModel struct {
	responses []*llm.Response
	calls     [][]*llm.Message
}

func (m *contextDemoScriptedModel) Name() string { return "context-demo-test" }

func (m *contextDemoScriptedModel) Generate(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	m.calls = append(m.calls, append([]*llm.Message(nil), cfg.Messages...))
	index := len(m.calls) - 1
	if index >= len(m.responses) {
		return nil, fmt.Errorf("unexpected model call %d", index+1)
	}
	return m.responses[index], nil
}

type contextDemoFileInput struct {
	FilePath string `json:"file_path"`
}

type contextDemoCommandInput struct {
	Command string `json:"command"`
}

type contextDemoEmptyInput struct{}

func TestContextDemosEvolveAcrossToolIterations(t *testing.T) {
	model := &contextDemoScriptedModel{responses: []*llm.Response{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{ID: "read-1", Name: "Read", Input: []byte(`{"file_path":"README.md"}`)},
				&llm.ToolUseContent{ID: "write-1", Name: "Write", Input: []byte(`{"file_path":"main.go"}`)},
			},
			StopReason: "tool_use",
		},
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{ID: "check-1", Name: "Bash", Input: []byte(`{"command":"go test ./..."}`)},
				&llm.ToolUseContent{ID: "broken-1", Name: "Broken", Input: []byte(`{"path":"missing.txt"}`)},
			},
			StopReason: "tool_use",
		},
		{
			Role:       llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "done"}},
			StopReason: "stop",
		},
	}}

	fileTool := func(name string) dive.Tool {
		return dive.FuncTool(name, name+" a file", func(_ context.Context, _ *contextDemoFileInput) (*dive.ToolResult, error) {
			return dive.NewToolResultText("ok"), nil
		})
	}
	agentOpts := dive.AgentOptions{
		Model: model,
		Tools: []dive.Tool{
			fileTool("Read"),
			fileTool("Write"),
			dive.FuncTool("Bash", "Run a command", func(_ context.Context, _ *contextDemoCommandInput) (*dive.ToolResult, error) {
				return dive.NewToolResultText("tests passed"), nil
			}),
			dive.FuncTool("Broken", "Always fail", func(_ context.Context, _ *contextDemoEmptyInput) (*dive.ToolResult, error) {
				return dive.NewToolResultError("missing file"), nil
			}),
		},
	}
	assert.NoError(t, applyContextDemoAgentOptions(&agentOpts, t.TempDir(), []string{"all"}))
	agent, err := dive.NewAgent(agentOpts)
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), dive.WithInput("inspect and update the project"))
	assert.NoError(t, err)
	assert.Len(t, model.calls, 3)

	workspace, ok := dive.FindLatestReminder(model.calls[0], "workspace-pulse")
	assert.True(t, ok)
	assert.Contains(t, workspace.Content, "Live workspace snapshot")

	ledger, ok := dive.FindLatestReminder(model.calls[1], "evidence-ledger")
	assert.True(t, ok)
	assert.Contains(t, ledger.Content, "file: README.md")
	debt, ok := dive.FindLatestReminder(model.calls[1], "verification-debt")
	assert.True(t, ok)
	assert.Contains(t, debt.Content, "main.go")
	assert.Equal(t, dive.ReminderTierOperator, debt.Tier)

	checkpoint, ok := dive.FindLatestReminder(model.calls[2], "verification-checkpoint")
	assert.True(t, ok)
	assert.Contains(t, checkpoint.Content, "go test ./...")
	recovery, ok := dive.FindLatestReminder(model.calls[2], "recovery-coach")
	assert.True(t, ok)
	assert.Contains(t, recovery.Content, "Broken")
	assert.Contains(t, recovery.Content, "missing.txt")
}
