package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/providers/grok"
	"github.com/deepnoodle-ai/dive/providers/groq"
	"github.com/deepnoodle-ai/dive/providers/ollama"
	"github.com/deepnoodle-ai/dive/providers/openrouter"
	"github.com/deepnoodle-ai/dive/skill"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/dive/toolkit/firecrawl"
	"github.com/deepnoodle-ai/dive/toolkit/google"
	"github.com/deepnoodle-ai/dive/toolkit/kagi"
	"github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/fetch"
)

func main() {
	app := cli.New("dive").
		Description("Interactive AI assistant for coding tasks").
		Version("0.1.0")

	app.Main().
		Flags(
			cli.String("model", "m").
				Default("claude-opus-4-5").
				Env("DIVE_MODEL").
				Help("Model to use"),
			cli.String("workspace", "w").
				Default("").
				Help("Workspace directory (defaults to current directory)"),
			cli.Float("temperature", "t").
				Default(1.0).
				Env("DIVE_TEMPERATURE").
				Help("Sampling temperature (0.0-2.0)"),
			cli.Int("max-tokens", "").
				Default(16000).
				Env("DIVE_MAX_TOKENS").
				Help("Maximum tokens in response"),
			cli.String("system-prompt", "s").
				Default("").
				Help("Path to custom system prompt file"),
			cli.Bool("dangerously-skip-permissions", "").
				Default(false).
				Help("Skip all tool permission prompts (use with caution)"),
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

// AppInteractor implements UserInteractor to work with the App
type AppInteractor struct {
	mu  sync.RWMutex
	app *App
}

func NewAppInteractor() *AppInteractor {
	return &AppInteractor{}
}

func (i *AppInteractor) SetApp(app *App) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.app = app
}

func (i *AppInteractor) Confirm(ctx context.Context, req *dive.ConfirmRequest) (bool, error) {
	i.mu.RLock()
	app := i.app
	i.mu.RUnlock()

	if app == nil {
		return true, nil // Auto-approve if no app
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

func (i *AppInteractor) Select(ctx context.Context, req *dive.SelectRequest) (*dive.SelectResponse, error) {
	i.mu.RLock()
	app := i.app
	i.mu.RUnlock()

	if app == nil {
		// Return default or first option
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

	return app.SelectTool(ctx, req)
}

func (i *AppInteractor) MultiSelect(ctx context.Context, req *dive.MultiSelectRequest) (*dive.MultiSelectResponse, error) {
	i.mu.RLock()
	app := i.app
	i.mu.RUnlock()

	if app == nil {
		var values []string
		for _, opt := range req.Options {
			if opt.Default {
				values = append(values, opt.Value)
			}
		}
		return &dive.MultiSelectResponse{Values: values}, nil
	}

	return app.MultiSelectTool(ctx, req)
}

func (i *AppInteractor) Input(ctx context.Context, req *dive.InputRequest) (*dive.InputResponse, error) {
	i.mu.RLock()
	app := i.app
	i.mu.RUnlock()

	if app == nil {
		return &dive.InputResponse{Value: req.Default}, nil
	}

	return app.InputTool(ctx, req)
}

var _ dive.UserInteractor = (*AppInteractor)(nil)

func runInteractive(ctx *cli.Context) error {
	modelName := ctx.String("model")
	workspaceDir := ctx.String("workspace")
	temperature := ctx.Float64("temperature")
	maxTokens := ctx.Int("max-tokens")
	systemPromptFile := ctx.String("system-prompt")
	skipPermissions := ctx.Bool("dangerously-skip-permissions")

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

	// Load custom system prompt if provided
	instructions := systemInstructions(workspaceDir)
	if systemPromptFile != "" {
		data, err := os.ReadFile(systemPromptFile)
		if err != nil {
			return fmt.Errorf("failed to read system prompt file: %w", err)
		}
		instructions = string(data)
	}

	// Create LLM provider based on model name
	model := createModel(modelName)

	// Create standard tools with workspace validation
	tools := createTools(workspaceDir)

	// Create interactor
	interactor := NewAppInteractor()

	// Add ask_user tool with interactor
	tools = append(tools, toolkit.NewAskUserTool(toolkit.AskUserToolOptions{
		Interactor: interactor,
	}))

	// Create skill loader and add skill tool
	skillLoader := skill.NewLoader(skill.LoaderOptions{
		ProjectDir: workspaceDir,
	})
	_ = skillLoader.LoadSkills() // Ignore error, skills are optional
	skillTool := toolkit.NewSkillTool(toolkit.SkillToolOptions{
		Loader: skillLoader,
	})
	tools = append(tools, dive.ToolAdapter(skillTool))

	// Create task registry and task tools
	taskRegistry := toolkit.NewTaskRegistry()
	tools = append(tools,
		dive.ToolAdapter(toolkit.NewTaskTool(toolkit.TaskToolOptions{
			Registry:     taskRegistry,
			ParentTools:  tools,
			AgentFactory: createTaskAgentFactory(model),
		})),
		dive.ToolAdapter(toolkit.NewTaskOutputTool(toolkit.TaskOutputToolOptions{
			Registry: taskRegistry,
		})),
	)

	// Create permission config
	var permissionConfig *dive.PermissionConfig
	if skipPermissions {
		permissionConfig = &dive.PermissionConfig{
			Mode: dive.PermissionModeBypassPermissions,
		}
	} else {
		permissionConfig = createPermissionConfig()
	}

	// Load project settings from .dive/settings.json
	settings, err := dive.LoadSettings(workspaceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load settings: %v\n", err)
	} else if settings != nil {
		// Add rules from settings file
		settingsRules := settings.ToPermissionRules()
		if len(settingsRules) > 0 {
			// Prepend settings rules so they're checked first
			permissionConfig.Rules = append(settingsRules, permissionConfig.Rules...)
		}
	}

	// Create thread repository for conversation memory
	threadRepo := dive.NewMemoryThreadRepository()

	// Create the agent
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:             "Dive",
		Instructions:     instructions,
		Model:            model,
		Tools:            tools,
		Permission:       permissionConfig,
		Interactor:       interactor,
		ThreadRepository: threadRepo,
		ModelSettings: &dive.ModelSettings{
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Create and run the app
	app := NewApp(agent, workspaceDir, modelName)
	interactor.SetApp(app)

	return app.Run()
}

// createModel creates the appropriate LLM provider based on model name
func createModel(modelName string) llm.LLM {
	lower := strings.ToLower(modelName)

	switch {
	case strings.HasPrefix(lower, "claude-"):
		return anthropic.New(anthropic.WithModel(modelName))

	case strings.HasPrefix(lower, "grok-"):
		return grok.New(grok.WithModel(modelName))

	case strings.HasPrefix(lower, "llama") || strings.HasPrefix(lower, "mixtral") || strings.HasPrefix(lower, "gemma"):
		// Check for Groq API key first, otherwise use Ollama
		if os.Getenv("GROQ_API_KEY") != "" {
			return groq.New(groq.WithModel(modelName))
		}
		return ollama.New(ollama.WithModel(modelName))

	case strings.Contains(modelName, "/"):
		// Models with "/" are OpenRouter format (e.g., "openai/gpt-4", "google/gemini-pro")
		return openrouter.New(openrouter.WithModel(modelName))

	default:
		// Default to Anthropic for unknown models
		return anthropic.New(anthropic.WithModel(modelName))
	}
}

// createTaskAgentFactory returns an AgentFactory for creating subagents
func createTaskAgentFactory(parentModel llm.LLM) toolkit.AgentFactory {
	return func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
		// Use the parent model by default, or create a new one if specified
		model := parentModel
		if def.Model != "" {
			model = createModel(def.Model)
		}

		// Filter tools based on subagent definition
		var tools []dive.Tool
		if len(def.Tools) > 0 {
			toolMap := make(map[string]dive.Tool)
			for _, t := range parentTools {
				toolMap[t.Name()] = t
			}
			for _, toolName := range def.Tools {
				if t, ok := toolMap[toolName]; ok {
					tools = append(tools, t)
				}
			}
		} else {
			tools = parentTools
		}

		return dive.NewAgent(dive.AgentOptions{
			Name:         name,
			Instructions: def.Prompt,
			Model:        model,
			Tools:        tools,
			Permission: &dive.PermissionConfig{
				Mode: dive.PermissionModeBypassPermissions,
			},
		})
	}
}

func createTools(workspaceDir string) []dive.Tool {
	tools := []dive.Tool{
		// Read-only file tools
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

		// Todo tool for task management
		toolkit.NewTodoWriteTool(),

		// TODO: Re-enable once Anthropic API supports memory_* tool type
		// // Memory tool for persistent notes
		// toolkit.NewMemoryTool(),
	}

	// Add web fetch if Firecrawl credentials are available, otherwise use a
	// fallback HTTP fetcher
	if firecrawlClient, err := firecrawl.New(); err == nil {
		tools = append(tools, toolkit.NewFetchTool(toolkit.FetchToolOptions{
			Fetcher: firecrawlClient,
		}))
	} else {
		tools = append(tools, toolkit.NewFetchTool(toolkit.FetchToolOptions{
			Fetcher: fetch.NewHTTPFetcher(fetch.HTTPFetcherOptions{}),
		}))
	}

	// Add web search if Kagi or Google credentials are available
	if kagiClient, err := kagi.New(); err == nil {
		tools = append(tools, toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
			Searcher: kagiClient,
		}))
	} else if googleSearchClient, err := google.New(); err == nil {
		tools = append(tools, toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
			Searcher: googleSearchClient,
		}))
	}

	return tools
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
			dive.AllowRule("fetch"),
			dive.AllowRule("web_search"),
			dive.AllowRule("TodoWrite"),
			dive.AllowRule("memory"),
			dive.AllowRule("ask_user"),
			dive.AllowRule("Skill"),
			dive.AllowRule("Task"),
			dive.AllowRule("TaskOutput"),

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
- Researching documentation and APIs

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

File operations:
- read_file: Read file contents
- glob: Find files by pattern
- grep: Search file contents
- list_directory: Explore directories
- edit: Make precise text replacements
- write_file: Create new files

Shell:
- bash: Run commands (build, test, git, etc.)

Web:
- fetch: Fetch and read web page contents
- web_search: Search the web (if available)

Organization:
- todo_write: Track tasks and progress
- memory: Store and retrieve notes
`, workspaceDir)
}
