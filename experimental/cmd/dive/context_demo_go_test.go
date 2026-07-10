package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGoDevelopmentSnapshotExplainsModuleCoverage(t *testing.T) {
	workspace := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/DO_NOT_FOLLOW_THIS\n\ngo 1.24.1\n"), 0o644))
	for _, relative := range []string{"providers/example/go.mod", "experimental/cmd/tool/go.mod"} {
		path := filepath.Join(workspace, relative)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.NoError(t, os.WriteFile(path, []byte("module example.com/nested\n\ngo 1.23\n"), 0o644))
	}

	snapshot, ok := goDevelopmentSnapshot(workspace)
	assert.True(t, ok)
	assert.Contains(t, snapshot, "Go module (go.mod)")
	assert.Contains(t, snapshot, "declared Go version: 1.24.1")
	assert.Contains(t, snapshot, "nested Go module manifests observed: 2")
	assert.Contains(t, snapshot, "go test ./... and go vet ./...")
	assert.Contains(t, snapshot, "go test -race ./...")
	assert.Contains(t, snapshot, "not evidence that any check ran or passed")
	assert.NotContains(t, snapshot, "DO_NOT_FOLLOW_THIS")
}

func TestGoDevelopmentSnapshotStaysSilentOutsideGoProjects(t *testing.T) {
	snapshot, ok := goDevelopmentSnapshot(t.TempDir())
	assert.False(t, ok)
	assert.Equal(t, "", snapshot)
}

func TestSafeGoVersion(t *testing.T) {
	for _, version := range []string{"1.24", "1.24.1"} {
		assert.True(t, safeGoVersion(version), version)
	}
	for _, version := range []string{"", "1", "1.", ".24", "1.24rc1", "1.2.3.4"} {
		assert.False(t, safeGoVersion(version), version)
	}
}

func TestPipelineIncludesGoDevelopmentContext(t *testing.T) {
	workspace := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644))
	model := &contextDemoScriptedModel{responses: []*llm.Response{{
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: "ready"}},
		StopReason: "stop",
	}}}
	var notices []contextDemoNotice
	options := dive.AgentOptions{Model: model}
	applyContextDemoAgentOptions(&options, workspace, contextDemoSelection(contextDemoPipeline), func(notice contextDemoNotice) {
		notices = append(notices, notice)
	})
	agent, err := dive.NewAgent(options)
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("inspect the Go module"))
	assert.NoError(t, err)

	reminder, ok := dive.FindLatestReminder(model.calls[0], "delivery-pipeline")
	assert.True(t, ok)
	assert.Contains(t, reminder.Content, "Go module/workspace: build, test, vet")
	assert.Contains(t, reminder.Content, "declared Go version: 1.24")
	assert.Len(t, notices, 1)
	assert.Equal(t, "delivery-pipeline", notices[0].Reminder.Name)
	assert.Equal(t, contextDemoModelOnly, notices[0].Delivery)
}
