package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

func TestWorkspaceBoundaryExplainsNestedGitScope(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, exec.Command("git", "init", "-b", "main", root).Run())
	nested := filepath.Join(root, "experimental", "cmd", "dive")
	assert.NoError(t, os.MkdirAll(nested, 0o755))

	resolved, err := resolveWorkspaceDir("experimental/cmd/dive", root)
	assert.NoError(t, err)
	assert.Equal(t, nested, resolved)

	boundary, limited := detectWorkspaceBoundary(nested)
	assert.True(t, limited)
	assert.True(t, samePath(root, boundary.GitRoot))
	assert.Equal(t, filepath.Join("..", "..", ".."), boundary.Relative)
	assert.Contains(t, workspaceScopeSummary(nested), "directory only")

	prompt := defaultSystemPrompt(nested, "test-model")
	assert.Contains(t, prompt, "Workspace boundary: tools are limited to the working directory")
	assert.Contains(t, prompt, root)

	app := NewApp(&dive.Agent{}, nil, nested, "test-model", "", nil, "", nil, "")
	app.contextDemos = allContextDemos()
	var output bytes.Buffer
	tui.Fprint(&output, app.buildIntroView(), tui.WithWidth(120))
	assert.Contains(t, output.String(), "context: all 8 demos · /context to inspect")
	assert.Contains(t, output.String(), "scope: directory only · Git root:")
}

func TestWorkspaceBoundaryIsSilentAtGitRoot(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, exec.Command("git", "init", "-b", "main", root).Run())
	_, limited := detectWorkspaceBoundary(root)
	assert.False(t, limited)
	assert.Equal(t, "", workspaceScopeSummary(root))
}
