package main

import (
	"bytes"
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
	assert.True(t, selection.enabled(contextDemoWorkspace))
	assert.True(t, selection.enabled(contextDemoSources))
	assert.True(t, selection.enabled(contextDemoVerification))
	assert.False(t, selection.enabled(contextDemoRecovery))
	assert.False(t, selection.enabled(contextDemoPipeline))
	assert.False(t, selection.enabled(contextDemoGo))
	assert.False(t, selection.enabled(contextDemoQuality))
	assert.False(t, selection.enabled(contextDemoSecurity))

	all, err := parseContextDemoNames([]string{"all"})
	assert.NoError(t, err)
	assert.Equal(t, allContextDemos(), all)
	assert.Equal(t, "all 8 demos", all.displaySummary())

	_, err = parseContextDemoNames([]string{"telepathy"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "run 'dive context-demos' to list presets")
}

func TestContextDemoCatalogIsTheSingleDisplaySource(t *testing.T) {
	var output bytes.Buffer
	assert.NoError(t, writeContextDemoCatalog(&output))
	for _, demo := range contextDemoCatalog {
		assert.Contains(t, output.String(), demo.Name)
		assert.Contains(t, output.String(), demo.Description)
	}
	assert.Contains(t, output.String(), "/context")
	assert.Equal(t, []string{"pipeline", "quality", "security"}, contextDemoSelection(contextDemoPipeline|contextDemoQuality|contextDemoSecurity).names())
	assert.Equal(t, "pipeline, quality, security", contextDemoSelection(contextDemoPipeline|contextDemoQuality|contextDemoSecurity).displaySummary())
}

func TestPinnedContextDemoNoticesReportOnlyMeaningfulChanges(t *testing.T) {
	state := &contextDemoTurnState{}
	first, err := dive.NewContextReminder("workspace-pulse", "clean")
	assert.NoError(t, err)
	updated, err := dive.NewContextReminder("workspace-pulse", "dirty")
	assert.NoError(t, err)

	action, changed := state.recordPinnedReminder(first)
	assert.Equal(t, "set", action)
	assert.True(t, changed)
	_, changed = state.recordPinnedReminder(first)
	assert.False(t, changed)
	action, changed = state.recordPinnedReminder(updated)
	assert.Equal(t, "updated", action)
	assert.True(t, changed)
}

func TestWorkspaceSnapshotTracksGitState(t *testing.T) {
	workspace := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main", workspace)
	assert.NoError(t, cmd.Run())
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("hello\n"), 0o644))

	snapshot := workspaceSnapshot(context.Background(), workspace)
	assert.Contains(t, snapshot, "git branch: main")
	assert.Contains(t, snapshot, "1 changed path")
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
		"xcodebuild test -scheme Dive",
		"VERIFY_MODE=full go test ./...",
		"env VERIFY_MODE=full go test ./...",
		"command go test ./...",
		"A=1 B=2 /usr/local/bin/pytest -q",
		"make design-check",
	} {
		assert.True(t, isVerificationCommand(command), command)
	}
	for _, command := range []string{
		"echo go test ./...",
		`echo "later; go test ./..."`,
		"go build ./...",
		"make release",
		`bash -c "go test ./..."`,
		"go test ./... || true",
		"go test ./...; echo done",
		"go test $(go list ./...)",
		"xcodebuild -resultBundlePath test build",
	} {
		assert.False(t, isVerificationCommand(command), command)
	}
}

func TestApplyContextDemoAgentOptionsInstallsOnlyNeededState(t *testing.T) {
	var stateless dive.AgentOptions
	applyContextDemoAgentOptions(&stateless, t.TempDir(), contextDemoSelection(contextDemoWorkspace|contextDemoRecovery))
	assert.Len(t, stateless.Hooks.PreGeneration, 0)
	assert.Len(t, stateless.Hooks.PreIteration, 1)
	assert.Len(t, stateless.Hooks.PostToolUseFailure, 1)

	var stateful dive.AgentOptions
	applyContextDemoAgentOptions(&stateful, t.TempDir(), contextDemoSelection(contextDemoSources|contextDemoVerification))
	assert.Len(t, stateful.Hooks.PreGeneration, 1)
	assert.Len(t, stateful.Hooks.PreIteration, 2)
	assert.Len(t, stateful.Hooks.PostToolUse, 2)

	var all dive.AgentOptions
	applyContextDemoAgentOptions(&all, t.TempDir(), allContextDemos())
	assert.Len(t, all.Hooks.PreGeneration, 1)
	assert.Len(t, all.Hooks.PreIteration, 7)
	assert.Len(t, all.Hooks.PostToolUse, 4)
	assert.Len(t, all.Hooks.PostToolUseFailure, 3)
}

func TestContextDemoStateIsBounded(t *testing.T) {
	state := &contextDemoTurnState{}
	for i := contextDemoItemLimit + 2; i >= 0; i-- {
		state.addSource(fmt.Sprintf("file: source-%02d.go", i))
		state.addBatchChange(fmt.Sprintf("change-%02d.go", i))
	}

	ledger := state.sourceSnapshot()
	assert.Len(t, ledger.sources, contextDemoItemLimit)
	assert.Equal(t, 3, ledger.omitted)
	assert.Equal(t, "file: source-00.go", ledger.sources[0])
	assert.Equal(t, "file: source-11.go", ledger.sources[contextDemoItemLimit-1])

	debt := state.applyVerificationBatch()
	assert.Len(t, debt.unverified, contextDemoItemLimit)
	assert.Equal(t, 3, debt.unverifiedOmitted)
	assert.True(t, debt.emitDebt)
	assert.Equal(t, "change-00.go", debt.unverified[0])
	assert.Equal(t, "change-11.go", debt.unverified[contextDemoItemLimit-1])

	state.addBatchCheck("go test ./...")
	state.addBatchCheck("make lint")
	checkpoint := state.applyVerificationBatch()
	assert.Len(t, checkpoint.checkedPaths, contextDemoItemLimit)
	assert.Equal(t, 3, checkpoint.checkedOmitted)
	assert.Len(t, checkpoint.unverified, 0)
	assert.Equal(t, "go test ./...", checkpoint.checkCommand)
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
				&llm.ToolUseContent{ID: "write-1", Name: "Write", Input: []byte(`{"file_path":"auth/policy.go"}`)},
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
		{
			Role:       llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "fresh turn"}},
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
	selection, err := parseContextDemoNames([]string{"all"})
	assert.NoError(t, err)
	var notices []contextDemoNotice
	applyContextDemoAgentOptions(&agentOpts, t.TempDir(), selection, func(notice contextDemoNotice) {
		notices = append(notices, notice)
	})
	agent, err := dive.NewAgent(agentOpts)
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), dive.WithInput("inspect and update the project"))
	assert.NoError(t, err)
	assert.Len(t, model.calls, 3)

	workspace, ok := dive.FindLatestReminder(model.calls[0], "workspace-pulse")
	assert.True(t, ok)
	assert.Contains(t, workspace.Content, "Live workspace snapshot")
	pipeline, ok := dive.FindLatestReminder(model.calls[0], "delivery-pipeline")
	assert.True(t, ok)
	assert.Contains(t, pipeline.Content, "Detected repository delivery surfaces")

	ledger, ok := dive.FindLatestReminder(model.calls[1], "evidence-ledger")
	assert.True(t, ok)
	assert.Contains(t, ledger.Content, "file: README.md")
	debt, ok := dive.FindLatestReminder(model.calls[1], "verification-debt")
	assert.True(t, ok)
	assert.Contains(t, debt.Content, "auth/policy.go")
	assert.Equal(t, dive.ReminderTierOperator, debt.Tier)
	security, ok := dive.FindLatestReminder(model.calls[1], "security-review")
	assert.True(t, ok)
	assert.Contains(t, security.Content, "identity and access: 1 file change")
	assert.NotContains(t, security.Content, "auth/policy.go")
	assert.Equal(t, dive.ReminderTierOperator, security.Tier)

	checkpoint, ok := dive.FindLatestReminder(model.calls[2], "verification-checkpoint")
	assert.True(t, ok)
	assert.Contains(t, checkpoint.Content, "go test ./...")
	recovery, ok := dive.FindLatestReminder(model.calls[2], "recovery-coach")
	assert.True(t, ok)
	assert.Contains(t, recovery.Content, "Broken")
	assert.Contains(t, recovery.Content, "missing.txt")
	quality, ok := dive.FindLatestReminder(model.calls[2], "quality-gates")
	assert.True(t, ok)
	assert.Contains(t, quality.Content, "test: passed (go test)")
	assert.Equal(t, dive.ReminderTierContextual, quality.Tier)

	noticeByName := make(map[string]contextDemoNotice)
	for _, notice := range notices {
		noticeByName[notice.Reminder.Name] = notice
	}
	for _, name := range []string{
		"workspace-pulse", "delivery-pipeline", "evidence-ledger", "verification-debt",
		"security-review", "recovery-coach", "verification-checkpoint", "quality-gates",
	} {
		_, ok := noticeByName[name]
		assert.True(t, ok, name)
	}
	assert.Equal(t, contextDemoPinned, noticeByName["workspace-pulse"].Delivery)
	assert.Equal(t, "set", noticeByName["workspace-pulse"].Action)
	assert.Equal(t, contextDemoModelOnly, noticeByName["verification-debt"].Delivery)
	assert.Equal(t, "queued", noticeByName["verification-debt"].Action)

	_, err = agent.CreateResponse(context.Background(), dive.WithInput("start a new turn"))
	assert.NoError(t, err)
	assert.Len(t, model.calls, 4)
	_, ok = dive.FindLatestReminder(model.calls[3], "evidence-ledger")
	assert.False(t, ok, "turn-local evidence must not leak into a later CreateResponse call")
	_, ok = dive.FindLatestReminder(model.calls[3], "quality-gates")
	assert.False(t, ok, "turn-local quality results must not leak into a later CreateResponse call")
	_, ok = dive.FindLatestReminder(model.calls[3], "security-review")
	assert.False(t, ok, "batch-local security triggers must not leak into a later CreateResponse call")
}
