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
	selection, err := parseContextDemoNames([]string{"workspace,pipeline", "verification", "workspace"})
	assert.NoError(t, err)
	assert.True(t, selection.enabled(contextDemoWorkspace))
	assert.True(t, selection.enabled(contextDemoPipeline))
	assert.True(t, selection.enabled(contextDemoVerification))
	assert.False(t, selection.enabled(contextDemoRecovery))
	assert.False(t, selection.enabled(contextDemoSecurity))

	all, err := parseContextDemoNames([]string{"all"})
	assert.NoError(t, err)
	assert.Equal(t, allContextDemos(), all)
	assert.Equal(t, "all 5 demos", all.displaySummary())

	_, err = parseContextDemoNames([]string{"telepathy"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "run 'dive context-demos' to list presets")

	for _, removed := range []string{"sources", "go", "quality"} {
		_, err = parseContextDemoNames([]string{removed})
		assert.Error(t, err, removed)
	}
}

func TestContextDemoCatalogIsTheSingleDisplaySource(t *testing.T) {
	var output bytes.Buffer
	assert.NoError(t, writeContextDemoCatalog(&output))
	for _, demo := range contextDemoCatalog {
		assert.Contains(t, output.String(), demo.Name)
		assert.Contains(t, output.String(), demo.Description)
	}
	assert.Contains(t, output.String(), "/context")
	assert.Equal(t, []string{"pipeline", "verification", "security"}, contextDemoSelection(contextDemoPipeline|contextDemoVerification|contextDemoSecurity).names())
	assert.Equal(t, "pipeline, verification, security", contextDemoSelection(contextDemoPipeline|contextDemoVerification|contextDemoSecurity).displaySummary())
}

func TestModelOnlyContextDemoNoticesReportOnlyMeaningfulChanges(t *testing.T) {
	state := &contextDemoTurnState{}
	first, err := dive.NewContextReminder("workspace-pulse", "clean")
	assert.NoError(t, err)
	updated, err := dive.NewContextReminder("workspace-pulse", "dirty")
	assert.NoError(t, err)

	action, changed := state.recordModelOnlyReminder(first)
	assert.Equal(t, "queued", action)
	assert.True(t, changed)
	_, changed = state.recordModelOnlyReminder(first)
	assert.False(t, changed)
	action, changed = state.recordModelOnlyReminder(updated)
	assert.Equal(t, "refreshed", action)
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

func TestApplyContextDemoAgentOptionsInstallsTurnState(t *testing.T) {
	var workspace dive.AgentOptions
	applyContextDemoAgentOptions(&workspace, t.TempDir(), contextDemoSelection(contextDemoWorkspace|contextDemoRecovery))
	assert.Len(t, workspace.Hooks.PreGeneration, 1)
	assert.Len(t, workspace.Hooks.PreIteration, 1)
	assert.Len(t, workspace.Hooks.PostToolUseFailure, 1)

	var stateful dive.AgentOptions
	applyContextDemoAgentOptions(&stateful, t.TempDir(), contextDemoSelection(contextDemoVerification))
	assert.Len(t, stateful.Hooks.PreGeneration, 1)
	assert.Len(t, stateful.Hooks.PreIteration, 2)
	assert.Len(t, stateful.Hooks.PostToolUse, 2)
	assert.Len(t, stateful.Hooks.PostToolUseFailure, 1)

	var all dive.AgentOptions
	applyContextDemoAgentOptions(&all, t.TempDir(), allContextDemos())
	assert.Len(t, all.Hooks.PreGeneration, 1)
	assert.Len(t, all.Hooks.PreIteration, 5)
	assert.Len(t, all.Hooks.PostToolUse, 3)
	assert.Len(t, all.Hooks.PostToolUseFailure, 3)
}

func TestContextDemoStateIsBounded(t *testing.T) {
	state := &contextDemoTurnState{}
	for i := contextDemoItemLimit + 2; i >= 0; i-- {
		state.addBatchChange(fmt.Sprintf("change-%02d.go", i))
	}

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
	quality, ok := dive.FindLatestReminder(model.calls[2], "verification-gates")
	assert.True(t, ok)
	assert.Contains(t, quality.Content, "test: passed (go test)")
	assert.Equal(t, dive.ReminderTierContextual, quality.Tier)

	noticeByName := make(map[string]contextDemoNotice)
	for _, notice := range notices {
		noticeByName[notice.Reminder.Name] = notice
	}
	for _, name := range []string{
		"workspace-pulse", "delivery-pipeline", "verification-debt", "security-review",
		"recovery-coach", "verification-checkpoint", "verification-gates",
	} {
		_, ok := noticeByName[name]
		assert.True(t, ok, name)
	}
	assert.Equal(t, contextDemoModelOnly, noticeByName["workspace-pulse"].Delivery)
	assert.Equal(t, "queued", noticeByName["workspace-pulse"].Action)
	assert.Equal(t, contextDemoModelOnly, noticeByName["verification-debt"].Delivery)
	assert.Equal(t, "queued", noticeByName["verification-debt"].Action)

	_, err = agent.CreateResponse(context.Background(), dive.WithInput("start a new turn"))
	assert.NoError(t, err)
	assert.Len(t, model.calls, 4)
	_, ok = dive.FindLatestReminder(model.calls[3], "verification-gates")
	assert.False(t, ok, "turn-local quality results must not leak into a later CreateResponse call")
	_, ok = dive.FindLatestReminder(model.calls[3], "security-review")
	assert.False(t, ok, "batch-local security triggers must not leak into a later CreateResponse call")
}
