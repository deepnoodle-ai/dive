package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/sandbox"
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
				Env("DIVE_MODEL").
				Help("Model to use (auto-detected from available API keys if not specified)"),
			cli.String("workspace", "w").
				Default("").
				Help("Workspace directory (defaults to current directory)"),
			cli.Float("temperature", "t").
				Env("DIVE_TEMPERATURE").
				Help("Sampling temperature (0.0-1.0)"),
			cli.Int("max-tokens", "").
				Default(16000).
				Env("DIVE_MAX_TOKENS").
				Help("Maximum tokens in response"),
			cli.String("system-prompt", "").
				Default("").
				Help("System prompt to use for the session"),
			cli.String("append-system-prompt", "").
				Default("").
				Help("Append a system prompt to the default system prompt"),
			cli.Bool("print", "p").
				Default(false).
				Help("Print response and exit (useful for pipes). Note: Permission prompts are skipped."),
			cli.String("output-format", "").
				Default("text").
				Help("Output format (only works with --print): \"text\" (default) or \"json\""),
			cli.String("api-endpoint", "").
				Default("").
				Env("DIVE_API_ENDPOINT").
				Help("Override the API endpoint URL for the provider"),
			cli.Bool("dangerously-skip-permissions", "").
				Default(false).
				Help("Skip all tool permission prompts (use with caution)"),
			cli.String("settings", "").
				Default("").
				Help("Path to a settings JSON file or a JSON string to load additional settings from"),
			cli.String("skills-dir", "").
				Default("").
				Help("Path to a skills directory"),
			cli.Bool("compaction", "").
				Default(true).
				Env("DIVE_COMPACTION").
				Help("Enable automatic context compaction when token limits are approached"),
			cli.Int("compaction-threshold", "").
				Default(100000).
				Env("DIVE_COMPACTION_THRESHOLD").
				Help("Token count that triggers compaction (default: 100000)"),
		).
		Run(runMain)

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

func runMain(ctx *cli.Context) error {
	printMode := ctx.Bool("print")
	if printMode {
		return runPrint(ctx)
	}
	return runInteractive(ctx)
}

// PrintOutputFormat defines the output format for print mode
type PrintOutputFormat string

const (
	PrintOutputText PrintOutputFormat = "text"
	PrintOutputJSON PrintOutputFormat = "json"
)

// PrintResult represents the final result for JSON output
type PrintResult struct {
	Output   string                `json:"output"`
	ThreadID string                `json:"thread_id,omitempty"`
	Usage    *llm.Usage            `json:"usage,omitempty"`
	Todos    []dive.TodoItem       `json:"todos,omitempty"`
	Error    string                `json:"error,omitempty"`
	Tools    []PrintToolCallResult `json:"tools,omitempty"`
}

// PrintToolCallResult represents a tool call result for JSON output
type PrintToolCallResult struct {
	Name   string `json:"name"`
	Input  any    `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// sessionConfig holds common configuration for both print and interactive modes.
// This includes model settings, tools, and feature flags like compaction.
type sessionConfig struct {
	modelName            string
	workspaceDir         string
	temperature          float64
	maxTokens            int
	instructions         string
	model                llm.LLM
	tools                []dive.Tool
	skillLoader          *skill.Loader
	taskRegistry         *toolkit.TaskRegistry
	sandboxConfig        *sandbox.Config
	settings             *dive.Settings
	apiEndpoint          string
	compactionEnabled    bool // Enable automatic context compaction
	compactionThreshold  int  // Token count that triggers compaction (default: 100000)
}

// parseSessionConfig extracts common configuration from CLI context
func parseSessionConfig(ctx *cli.Context) (*sessionConfig, error) {
	modelName := ctx.String("model")
	if modelName == "" {
		modelName = getDefaultModel()
	}

	cfg := &sessionConfig{
		modelName:           modelName,
		temperature:         ctx.Float64("temperature"),
		maxTokens:           ctx.Int("max-tokens"),
		apiEndpoint:         ctx.String("api-endpoint"),
		compactionEnabled:   ctx.Bool("compaction"),
		compactionThreshold: ctx.Int("compaction-threshold"),
	}

	// Workspace setup
	workspaceDir := ctx.String("workspace")
	if workspaceDir == "" {
		var err error
		workspaceDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
	}
	workspaceDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workspace path: %w", err)
	}
	cfg.workspaceDir = workspaceDir

	// Load settings (errors handled differently per mode, so just attempt load)
	cfg.settings, _ = dive.LoadSettings(workspaceDir)

	// Load additional settings from --settings flag if provided
	if settingsArg := ctx.String("settings"); settingsArg != "" {
		additionalSettings, err := loadSettingsFromArg(settingsArg)
		if err != nil {
			return nil, fmt.Errorf("failed to load settings from --settings: %w", err)
		}
		cfg.settings = mergeSettings(cfg.settings, additionalSettings)
	}

	// Sandbox config
	if cfg.settings != nil && cfg.settings.Sandbox != nil {
		cfg.sandboxConfig = cfg.settings.Sandbox
		if cfg.sandboxConfig.WorkDir == "" {
			cfg.sandboxConfig.WorkDir = workspaceDir
		}
	}

	// Build instructions
	systemPrompt := ctx.String("system-prompt")
	appendSystemPrompt := ctx.String("append-system-prompt")
	if systemPrompt != "" {
		cfg.instructions = systemPrompt
	} else {
		cfg.instructions = systemInstructions(workspaceDir)
	}
	if appendSystemPrompt != "" {
		cfg.instructions += "\n\n" + appendSystemPrompt
	}

	// Create model and tools
	cfg.model = createModel(cfg.modelName, cfg.apiEndpoint)
	cfg.tools = createTools(workspaceDir, cfg.sandboxConfig)

	// Skill loader
	loaderOpts := skill.LoaderOptions{
		ProjectDir: workspaceDir,
	}
	if skillsDir := ctx.String("skills-dir"); skillsDir != "" {
		loaderOpts.AdditionalPaths = []string{skillsDir}
	}
	cfg.skillLoader = skill.NewLoader(loaderOpts)
	_ = cfg.skillLoader.LoadSkills()

	// Task registry
	cfg.taskRegistry = toolkit.NewTaskRegistry()

	return cfg, nil
}

// addSkillAndTaskTools adds skill and task tools to the session config
func (cfg *sessionConfig) addSkillAndTaskTools() {
	// Add skill tool
	skillTool := toolkit.NewSkillTool(toolkit.SkillToolOptions{
		Loader: cfg.skillLoader,
	})
	cfg.tools = append(cfg.tools, dive.ToolAdapter(skillTool))

	// Add task tools
	cfg.tools = append(cfg.tools,
		dive.ToolAdapter(toolkit.NewTaskTool(toolkit.TaskToolOptions{
			Registry:     cfg.taskRegistry,
			ParentTools:  cfg.tools,
			AgentFactory: createTaskAgentFactory(cfg.model, cfg.apiEndpoint),
		})),
		dive.ToolAdapter(toolkit.NewTaskOutputTool(toolkit.TaskOutputToolOptions{
			Registry: cfg.taskRegistry,
		})),
	)
}

func runPrint(ctx *cli.Context) error {
	outputFormat := PrintOutputFormat(ctx.String("output-format"))

	// Validate output format
	switch outputFormat {
	case PrintOutputText, PrintOutputJSON:
		// Valid
	default:
		return fmt.Errorf("invalid output format: %s (must be text or json)", outputFormat)
	}

	// Get input from args or stdin
	input, err := getInput(ctx.Args())
	if err != nil {
		return fmt.Errorf("failed to get input: %w", err)
	}
	if input == "" {
		return fmt.Errorf("no input provided; provide a prompt as an argument or pipe content to stdin")
	}

	// Parse common session configuration
	cfg, err := parseSessionConfig(ctx)
	if err != nil {
		return err
	}

	// Add skill and task tools
	cfg.addSkillAndTaskTools()

	// Create the agent (skip all permissions in print mode)
	// Note: Compaction is not used in print mode (single request)
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Dive",
		Instructions: cfg.instructions,
		Model:        cfg.model,
		Tools:        cfg.tools,
		Permission: &dive.PermissionConfig{
			Mode: dive.PermissionModeBypassPermissions,
		},
		ModelSettings: &dive.ModelSettings{
			Temperature: &cfg.temperature,
			MaxTokens:   &cfg.maxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Run the agent
	bgCtx := context.Background()

	switch outputFormat {
	case PrintOutputJSON:
		return runPrintJSON(bgCtx, agent, input)
	default:
		return runPrintText(bgCtx, agent, input)
	}
}

func readStdinInput() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}

	// Check if stdin has data (piped input)
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}

	return "", nil
}

// getInput returns input from command-line args or stdin.
// If args are provided, exactly one arg is expected.
// If no args, stdin is read for piped input.
func getInput(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		return strings.TrimSpace(args[0]), nil
	}
	// No args provided, try stdin
	return readStdinInput()
}

func runPrintText(ctx context.Context, agent dive.Agent, input string) error {
	var outputText strings.Builder

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(input),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent && item.Event != nil {
				if item.Event.Delta != nil && item.Event.Delta.Text != "" {
					fmt.Print(item.Event.Delta.Text)
					outputText.WriteString(item.Event.Delta.Text)
				}
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	// Ensure newline at end
	if outputText.Len() > 0 && !strings.HasSuffix(outputText.String(), "\n") {
		fmt.Println()
	} else if outputText.Len() == 0 {
		// If no streaming output was captured, print the final output
		fmt.Println(resp.OutputText())
	}

	return nil
}

func runPrintJSON(ctx context.Context, agent dive.Agent, input string) error {
	result := PrintResult{}
	var toolResults []PrintToolCallResult

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(input),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			switch item.Type {
			case dive.ResponseItemTypeInit:
				if item.Init != nil {
					result.ThreadID = item.Init.ThreadID
				}
			case dive.ResponseItemTypeTodo:
				if item.Todo != nil {
					result.Todos = item.Todo.Todos
				}
			case dive.ResponseItemTypeToolCallResult:
				if item.ToolCallResult != nil {
					tr := PrintToolCallResult{
						Name: item.ToolCallResult.Name,
					}
					if item.ToolCallResult.Result != nil {
						tr.Output = item.ToolCallResult.Result.Display
					}
					if item.ToolCallResult.Error != nil {
						tr.Error = item.ToolCallResult.Error.Error()
					}
					toolResults = append(toolResults, tr)
				}
			}
			if item.Usage != nil {
				result.Usage = item.Usage
			}
			return nil
		}),
	)
	if err != nil {
		result.Error = err.Error()
	} else {
		result.Output = resp.OutputText()
		if resp.Usage != nil {
			result.Usage = resp.Usage
		}
	}
	result.Tools = toolResults

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func runInteractive(ctx *cli.Context) error {
	skipPermissions := ctx.Bool("dangerously-skip-permissions")

	// Get initial prompt from args (at most 1 arg allowed)
	args := ctx.Args()
	if len(args) > 1 {
		return fmt.Errorf("expected at most 1 argument, got %d", len(args))
	}
	var initialPrompt string
	if len(args) == 1 {
		initialPrompt = strings.TrimSpace(args[0])
	}

	// Parse common session configuration
	cfg, err := parseSessionConfig(ctx)
	if err != nil {
		return err
	}

	// Create interactor and add AskUserQuestion tool
	interactor := NewAppInteractor()
	cfg.tools = append(cfg.tools, toolkit.NewAskUserTool(toolkit.AskUserToolOptions{
		Interactor: interactor,
	}))

	// Add skill and task tools
	cfg.addSkillAndTaskTools()

	// Create permission config
	var permissionConfig *dive.PermissionConfig
	if skipPermissions {
		permissionConfig = &dive.PermissionConfig{
			Mode: dive.PermissionModeBypassPermissions,
		}
	} else {
		permissionConfig = createPermissionConfig()
	}

	if cfg.settings != nil {
		// Add rules from settings file
		settingsRules := cfg.settings.ToPermissionRules()
		if len(settingsRules) > 0 {
			// Prepend settings rules so they're checked first
			permissionConfig.Rules = append(settingsRules, permissionConfig.Rules...)
		}
	}
	applySandboxPermissionMode(permissionConfig, cfg.sandboxConfig)

	// Create thread repository for conversation memory
	threadRepo := dive.NewMemoryThreadRepository()

	// Create the agent
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:             "Dive",
		Instructions:     cfg.instructions,
		Model:            cfg.model,
		Tools:            cfg.tools,
		Permission:       permissionConfig,
		Interactor:       interactor,
		ThreadRepository: threadRepo,
		ModelSettings: &dive.ModelSettings{
			Temperature: &cfg.temperature,
			MaxTokens:   &cfg.maxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Create compaction config for external management
	var compactionConfig *dive.CompactionConfig
	if cfg.compactionEnabled {
		compactionConfig = &dive.CompactionConfig{
			ContextTokenThreshold: cfg.compactionThreshold,
			Model:                 cfg.model,
		}
	}

	// Create and run the app
	app := NewApp(agent, threadRepo, cfg.workspaceDir, cfg.modelName, initialPrompt, compactionConfig)
	interactor.SetApp(app)

	return app.Run()
}

// createTaskAgentFactory returns an AgentFactory for creating subagents
func createTaskAgentFactory(parentModel llm.LLM, apiEndpoint string) toolkit.AgentFactory {
	return func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
		// Use the parent model by default, or create a new one if specified
		model := parentModel
		if def.Model != "" {
			model = createModel(def.Model, apiEndpoint)
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

func createTools(workspaceDir string, sandboxConfig *sandbox.Config) []dive.Tool {
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
			WorkspaceDir:  workspaceDir,
			SandboxConfig: sandboxConfig,
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
			dive.AllowRule("Read"),
			dive.AllowRule("Glob"),
			dive.AllowRule("Grep"),
			dive.AllowRule("ListDirectory"),
			dive.AllowRule("WebFetch"),
			dive.AllowRule("WebSearch"),
			dive.AllowRule("TodoWrite"),
			dive.AllowRule("AskUserQuestion"),
			dive.AllowRule("Skill"),
			dive.AllowRule("Task"),
			dive.AllowRule("TaskOutput"),

			// Require approval for write operations
			dive.AskRule("Write", "Write file"),
			dive.AskRule("Edit", "Edit file"),
			dive.AskRule("Bash", "Execute command"),
		},
	}
}

func applySandboxPermissionMode(permissionConfig *dive.PermissionConfig, sandboxConfig *sandbox.Config) {
	if permissionConfig == nil || sandboxConfig == nil || !sandboxConfig.Enabled {
		return
	}
	if sandboxConfig.Mode != sandbox.SandboxModeAutoAllow {
		return
	}

	// Drop the default Bash ask rule so sandboxed commands can auto-allow.
	filtered := make(dive.PermissionRules, 0, len(permissionConfig.Rules))
	for _, rule := range permissionConfig.Rules {
		if rule.Type == dive.PermissionRuleAsk && rule.Tool == "Bash" && rule.Command == "" && rule.Message == "Execute command" {
			continue
		}
		filtered = append(filtered, rule)
	}
	permissionConfig.Rules = filtered

	prevCanUse := permissionConfig.CanUseTool
	permissionConfig.CanUseTool = func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent) (*dive.ToolHookResult, error) {
		if tool != nil && strings.EqualFold(tool.Name(), "Bash") {
			command := extractCommand(call)
			if matchesAnyCommand(command, sandboxConfig.ExcludedCommands) {
				if sandboxConfig.AllowUnsandboxedCommands {
					return dive.AskResult("Command is excluded from sandbox; run unsandboxed?"), nil
				}
				return dive.DenyResult("Command is excluded from sandbox and unsandboxed commands are disabled"), nil
			}
			return dive.AllowResult(), nil
		}
		if prevCanUse != nil {
			return prevCanUse(ctx, tool, call)
		}
		return dive.ContinueResult(), nil
	}
}

func extractCommand(call *llm.ToolUseContent) string {
	if call == nil || call.Input == nil {
		return ""
	}
	var inputMap map[string]any
	if err := json.Unmarshal(call.Input, &inputMap); err != nil {
		return ""
	}
	for _, field := range []string{"command", "cmd", "script", "code"} {
		if value, ok := inputMap[field].(string); ok {
			return value
		}
	}
	return ""
}

func matchesAnyCommand(command string, patterns []string) bool {
	for _, pattern := range patterns {
		if sandbox.MatchesCommandPattern(pattern, command) {
			return true
		}
	}
	return false
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

// loadSettingsFromArg loads settings from either a file path or a JSON string.
// If the argument starts with '{', it's treated as inline JSON.
// Otherwise, it's treated as a file path.
func loadSettingsFromArg(arg string) (*dive.Settings, error) {
	arg = strings.TrimSpace(arg)

	// Check if it looks like JSON (starts with '{')
	if strings.HasPrefix(arg, "{") {
		var settings dive.Settings
		if err := json.Unmarshal([]byte(arg), &settings); err != nil {
			return nil, fmt.Errorf("parsing JSON string: %w", err)
		}
		return &settings, nil
	}

	// Treat as file path
	data, err := os.ReadFile(arg)
	if err != nil {
		return nil, fmt.Errorf("reading settings file: %w", err)
	}

	var settings dive.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing settings file: %w", err)
	}

	return &settings, nil
}

// mergeSettings merges two Settings objects, with the second taking precedence.
// If base is nil, returns override. If override is nil, returns base.
func mergeSettings(base, override *dive.Settings) *dive.Settings {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}

	// Create a new merged settings object
	merged := &dive.Settings{}

	// Merge permissions - concatenate allow and deny lists
	merged.Permissions.Allow = append([]string{}, base.Permissions.Allow...)
	merged.Permissions.Allow = append(merged.Permissions.Allow, override.Permissions.Allow...)

	merged.Permissions.Deny = append([]string{}, base.Permissions.Deny...)
	merged.Permissions.Deny = append(merged.Permissions.Deny, override.Permissions.Deny...)

	// Sandbox config - override takes precedence
	if override.Sandbox != nil {
		merged.Sandbox = override.Sandbox
	} else {
		merged.Sandbox = base.Sandbox
	}

	return merged
}

// getDefaultModel returns the default model based on available API keys.
// Priority order: anthropic, google, openai, grok, mistral.
// Falls back to anthropic if no API keys are found.
func getDefaultModel() string {
	// Check for API keys in priority order
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "claude-haiku-4-5"
	}
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return "gemini-3-flash-preview"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "gpt-5.2"
	}
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" {
		return "grok-code-fast-1"
	}
	if os.Getenv("MISTRAL_API_KEY") != "" {
		return "mistral-small-latest"
	}

	// Default fallback to anthropic (will prompt for API key)
	return "claude-haiku-4-5"
}
