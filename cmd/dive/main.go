package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/tui"
)

func main() {
	app := cli.New("dive").
		Description("Interactive AI assistant for coding tasks").
		Version("0.1.0")

	app.Command("").
		Description("Start interactive chat").
		Flags(
			cli.String("model", "m").
				Default("claude-sonnet-4-20250514").
				Env("DIVE_MODEL").
				Help("Model to use"),
			cli.String("workspace", "w").
				Default("").
				Help("Workspace directory (defaults to current directory)"),
		).
		Run(runInteractive)

	if err := app.Execute(); err != nil {
		if cli.IsHelpRequested(err) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(cli.GetExitCode(err))
	}
}

// TUIInteractor implements UserInteractor to work with the TUI
type TUIInteractor struct {
	mu  sync.RWMutex
	app *App
}

func NewTUIInteractor() *TUIInteractor {
	return &TUIInteractor{}
}

func (t *TUIInteractor) SetApp(app *App) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.app = app
}

func (t *TUIInteractor) Confirm(ctx context.Context, req *dive.ConfirmRequest) (bool, error) {
	t.mu.RLock()
	app := t.app
	t.mu.RUnlock()

	if app == nil {
		// No app set, auto-approve
		return true, nil
	}

	// Get summary from the request
	summary := req.Message
	if summary == "" && req.Title != "" {
		summary = req.Title
	}
	if summary == "" && req.Tool != nil {
		summary = fmt.Sprintf("Execute %s", req.Tool.Name())
	}

	// Get input from call if available
	var input []byte
	if req.Call != nil {
		input = req.Call.Input
	}

	toolName := "unknown"
	if req.Tool != nil {
		toolName = req.Tool.Name()
	}

	return app.ConfirmTool(ctx, toolName, summary, input)
}

func (t *TUIInteractor) Select(ctx context.Context, req *dive.SelectRequest) (*dive.SelectResponse, error) {
	// For now, return default or first option
	for _, opt := range req.Options {
		if opt.Default {
			return &dive.SelectResponse{Value: opt.Value}, nil
		}
	}
	if len(req.Options) > 0 {
		return &dive.SelectResponse{Value: req.Options[0].Value}, nil
	}
	return &dive.SelectResponse{Canceled: true}, nil
}

func (t *TUIInteractor) MultiSelect(ctx context.Context, req *dive.MultiSelectRequest) (*dive.MultiSelectResponse, error) {
	// Return default options
	var values []string
	for _, opt := range req.Options {
		if opt.Default {
			values = append(values, opt.Value)
		}
	}
	return &dive.MultiSelectResponse{Values: values}, nil
}

func (t *TUIInteractor) Input(ctx context.Context, req *dive.InputRequest) (*dive.InputResponse, error) {
	return &dive.InputResponse{Value: req.Default}, nil
}

var _ dive.UserInteractor = (*TUIInteractor)(nil)

func runInteractive(ctx *cli.Context) error {
	modelName := ctx.String("model")
	workspaceDir := ctx.String("workspace")

	// Default workspace to current directory
	if workspaceDir == "" {
		var err error
		workspaceDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Resolve to absolute path
	workspaceDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	// Create LLM provider
	model := anthropic.New(anthropic.WithModel(modelName))

	// Create standard tools with workspace validation
	tools := createTools(workspaceDir)

	// Create interactor that we'll wire up to the TUI later
	interactor := NewTUIInteractor()

	// Create permission config that requires approval for write operations
	permissionConfig := createPermissionConfig()

	// Create the agent
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Dive",
		Instructions: systemInstructions(workspaceDir),
		Model:        model,
		Tools:        tools,
		Permission:   permissionConfig,
		Interactor:   interactor,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Create the TUI app
	tuiApp := NewApp(agent, workspaceDir)

	// Wire up the interactor to the app
	interactor.SetApp(tuiApp)

	return tui.Run(tuiApp,
		tui.WithAlternateScreen(true),
		tui.WithHideCursor(true),
		tui.WithMouseTracking(true),
		tui.WithBracketedPaste(true),
	)
}

func createTools(workspaceDir string) []dive.Tool {
	return []dive.Tool{
		// Read-only tools
		toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewGlobTool(toolkit.GlobToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewGrepTool(toolkit.GrepToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewListDirectoryTool(toolkit.ListDirectoryToolOptions{
			WorkspaceDir: workspaceDir,
		}),

		// Write tools (require approval)
		toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewEditTool(toolkit.EditToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewBashTool(toolkit.BashToolOptions{
			WorkspaceDir: workspaceDir,
		}),
	}
}

func createPermissionConfig() *dive.PermissionConfig {
	return &dive.PermissionConfig{
		Mode: dive.PermissionModeDefault,
		Rules: dive.PermissionRules{
			// Allow read-only tools without prompting
			dive.AllowRule("read_file"),
			dive.AllowRule("glob"),
			dive.AllowRule("grep"),
			dive.AllowRule("list_directory"),

			// Require approval for write operations
			dive.AskRule("write_file", "Write file"),
			dive.AskRule("edit", "Edit file"),
			dive.AskRule("bash", "Execute command"),
		},
	}
}

func systemInstructions(workspaceDir string) string {
	return fmt.Sprintf(`You are Dive, an AI coding assistant.

You help users with software engineering tasks including:
- Reading, understanding, and explaining code
- Writing new code and modifying existing code
- Debugging and fixing issues
- Running commands and tests

## Workspace

You are working in the following directory:
%s

All file operations are scoped to this workspace.

## Guidelines

- Be concise and direct in your responses
- When reading code, understand the context before suggesting changes
- Prefer small, focused changes over large refactors
- Always explain what you're doing and why
- If you're unsure about something, ask for clarification

## Tool Usage

- Use read_file to read file contents
- Use glob to find files by pattern
- Use grep to search file contents
- Use list_directory to explore directories
- Use edit for precise text replacements
- Use write_file to create new files
- Use bash to run commands (build, test, git, etc.)
`, workspaceDir)
}
