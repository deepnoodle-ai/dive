package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/experimental/session"
	"github.com/deepnoodle-ai/dive/experimental/slashcmd"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/firecrawl"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/google"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/kagi"
	"github.com/deepnoodle-ai/dive/permission"
	"github.com/deepnoodle-ai/dive/toolkit"
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
			cli.Bool("print", "p").
				Default(false).
				Help("Print response and exit (useful for pipes)"),
			cli.String("output-format", "").
				Default("text").
				Help("Output format (only works with --print): \"text\" (default) or \"json\""),
			cli.String("api-endpoint", "").
				Default("").
				Env("DIVE_API_ENDPOINT").
				Help("Override the API endpoint URL for the provider"),
			cli.Bool("resume", "r").
				Default(false).
				Help("Resume a previous session"),
			cli.Bool("compaction", "").
				Default(true).
				Env("DIVE_COMPACTION").
				Help("Enable automatic context compaction"),
			cli.Int("compaction-threshold", "").
				Default(100000).
				Env("DIVE_COMPACTION_THRESHOLD").
				Help("Token threshold for automatic context compaction"),
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

func runMain(ctx *cli.Context) error {
	printMode := ctx.Bool("print")
	if printMode {
		return runPrint(ctx)
	}
	return runInteractive(ctx)
}

func runInteractive(ctx *cli.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Parse workspace
	workspaceDir := ctx.String("workspace")
	if workspaceDir == "" {
		workspaceDir = cwd
	}

	// Parse model
	modelName := ctx.String("model")
	if modelName == "" {
		modelName = getDefaultModel()
	}

	// Build system prompt
	systemPrompt := ctx.String("system-prompt")
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt(workspaceDir)
	}

	// Create model
	model := createModel(modelName, ctx.String("api-endpoint"))

	// Create TUI dialog for interactive user prompts (AskUserQuestion tool)
	tuiDialog := &tuiDialog{}

	// Create tools
	tools := createTools(workspaceDir, tuiDialog)

	// Create session repository
	sessionRepo, err := session.NewFileRepository("~/.dive/sessions")
	if err != nil {
		return fmt.Errorf("failed to create session repository: %w", err)
	}

	// Handle --resume flag
	var resumeSessionID string
	if ctx.Bool("resume") {
		// If an argument was provided, use it as a session ID or filter
		args := ctx.Args()
		var filter string
		if len(args) > 0 {
			filter = args[0]
		}
		result, err := RunSessionPicker(sessionRepo, filter, workspaceDir)
		if err != nil {
			return fmt.Errorf("session picker failed: %w", err)
		}
		if result.Canceled {
			return nil // User canceled, exit gracefully
		}
		resumeSessionID = result.SessionID
	}

	// Generate session ID
	sessionID := resumeSessionID
	if sessionID == "" {
		sessionID = newSessionID()
	}

	// Get initial prompt from args (if not resuming)
	var initialPrompt string
	if !ctx.Bool("resume") {
		args := ctx.Args()
		if len(args) > 0 {
			initialPrompt = strings.Join(args, " ")
		}
	}

	// appPtr is set after App creation; closures below capture it by pointer.
	var appPtr *App

	// Set up session ID hook (injects session_id into generation state).
	// Reads from app.currentSessionID dynamically so /clear can reset it.
	// If currentSessionID is empty, generates a new one (fresh conversation).
	sessionIDHook := func(_ context.Context, hctx *dive.HookContext) error {
		if hctx.Values == nil {
			hctx.Values = map[string]any{}
		}
		if appPtr != nil && appPtr.currentSessionID == "" {
			// Generate a new session ID after /clear
			appPtr.currentSessionID = newSessionID()
		}
		if appPtr != nil {
			hctx.Values["session_id"] = appPtr.currentSessionID
		} else {
			hctx.Values["session_id"] = sessionID
		}
		return nil
	}

	// Set up session hooks for multi-turn conversation
	sessionLoader := session.Loader(sessionRepo)
	sessionSaver := session.Saver(sessionRepo)

	// Set up tool permission hook using the permission package
	permConfig := &permission.Config{
		Mode:  permission.ModeDefault,
		Rules: defaultPermissionRules(tools),
	}
	permManager := permission.NewManager(permConfig, tuiDialog)
	permissionHook := permission.HookFromManager(permManager)

	// Set up compaction config
	var compactionConfig *compaction.CompactionConfig
	if ctx.Bool("compaction") {
		compactionConfig = &compaction.CompactionConfig{
			ContextTokenThreshold: ctx.Int("compaction-threshold"),
			Model:                 model,
		}
	}

	// Load slash commands
	commandLoader := slashcmd.NewLoader(slashcmd.LoaderOptions{
		ProjectDir: workspaceDir,
	})
	_ = commandLoader.LoadCommands() // Ignore errors, commands are optional

	// Create model settings
	temperature := ctx.Float64("temperature")
	maxTokens := ctx.Int("max-tokens")

	// Create agent with hooks
	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: systemPrompt,
		Model:        model,
		Tools:        tools,
		ModelSettings: &dive.ModelSettings{
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
		},
		Hooks: dive.Hooks{
			PreGeneration:  []dive.PreGenerationHook{sessionIDHook, sessionLoader},
			PostGeneration: []dive.PostGenerationHook{sessionSaver},
			PreToolUse:     []dive.PreToolUseHook{permissionHook},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Create App
	app := NewApp(
		agent,
		sessionRepo,
		workspaceDir,
		modelName,
		initialPrompt,
		compactionConfig,
		resumeSessionID,
		commandLoader,
	)

	attachment, err := loadStartupInstructionAttachment(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read startup instructions: %v\n", err)
	}
	app.startupAttachment = attachment

	// Set the session ID on the app and wire up closures
	app.currentSessionID = sessionID
	appPtr = app
	tuiDialog.app = app

	return app.Run()
}

func defaultSystemPrompt(workspaceDir string) string {
	return fmt.Sprintf(`You are Dive, an AI coding assistant. You help users with software engineering tasks including writing code, debugging, explaining code, and more.

You are working in: %s

Be concise and helpful. When modifying code, explain what you're changing and why. Use the available tools to read, write, and search files as needed.`, workspaceDir)
}

func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a simple timestamp-based ID
		return fmt.Sprintf("session-%d", os.Getpid())
	}
	return hex.EncodeToString(b)
}

func runPrint(ctx *cli.Context) error {
	outputFormat := ctx.String("output-format")

	// Get input from args or stdin
	input, err := getInput(ctx.Args())
	if err != nil {
		return fmt.Errorf("failed to get input: %w", err)
	}
	if input == "" {
		return fmt.Errorf("no input provided; provide a prompt as an argument or pipe content to stdin")
	}

	// Get workspace
	workspaceDir := ctx.String("workspace")
	if workspaceDir == "" {
		var err error
		workspaceDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Get model
	modelName := ctx.String("model")
	if modelName == "" {
		modelName = getDefaultModel()
	}

	// Build system prompt
	systemPrompt := ctx.String("system-prompt")
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf("You are Dive, an AI coding assistant working in: %s", workspaceDir)
	}

	// Create model
	model := createModel(modelName, ctx.String("api-endpoint"))

	// Create tools (auto-approve dialog for non-interactive print mode)
	tools := createTools(workspaceDir, nil)

	// Create agent
	temperature := ctx.Float64("temperature")
	maxTokens := ctx.Int("max-tokens")

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: systemPrompt,
		Model:        model,
		Tools:        tools,
		ModelSettings: &dive.ModelSettings{
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Run agent
	bgCtx := context.Background()
	if cwd, err := os.Getwd(); err == nil {
		if attachment, readErr := loadStartupInstructionAttachment(cwd); readErr == nil {
			input = appendAttachedContent(input, attachment)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: failed to read startup instructions: %v\n", readErr)
		}
	}

	switch outputFormat {
	case "json":
		return runPrintJSON(bgCtx, agent, input)
	case "text":
		return runPrintText(bgCtx, agent, input)
	default:
		return fmt.Errorf("unsupported --output-format %q; valid values are: json, text", outputFormat)
	}
}

func loadStartupInstructionAttachment(cwd string) (string, error) {
	candidates := []string{"AGENTS.md", "CLAUDE.md"}
	for _, name := range candidates {
		path := filepath.Join(cwd, name)
		content, err := os.ReadFile(path)
		if err == nil {
			return fmt.Sprintf("\n<file path=\"%s\">\n%s\n</file>\n", name, string(content)), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

func appendAttachedContent(input, attachment string) string {
	if attachment == "" {
		return input
	}
	trimmed := strings.TrimRight(input, "\n")
	return trimmed + attachment
}

func getInput(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		return strings.TrimSpace(args[0]), nil
	}
	// Try stdin
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
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

func runPrintText(ctx context.Context, agent *dive.Agent, input string) error {
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

	if outputText.Len() > 0 && !strings.HasSuffix(outputText.String(), "\n") {
		fmt.Println()
	} else if outputText.Len() == 0 {
		fmt.Println(resp.OutputText())
	}

	return nil
}

func runPrintJSON(ctx context.Context, agent *dive.Agent, input string) error {
	result := map[string]interface{}{}

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(input),
	)
	if err != nil {
		result["error"] = err.Error()
	} else {
		result["output"] = resp.OutputText()
		if resp.Usage != nil {
			result["usage"] = resp.Usage
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// tuiDialog implements dive.Dialog by routing to the App's TUI dialog system.
// The app field is set after App creation (same pattern as the confirmer closure).
type tuiDialog struct {
	app *App
}

func (d *tuiDialog) Show(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	if d.app == nil || d.app.runner == nil {
		// Fallback before app is initialized
		return (&dive.AutoApproveDialog{}).Show(ctx, in)
	}

	if in.Confirm {
		return d.showConfirm(ctx, in)
	}
	if len(in.Options) > 0 && in.MultiSelect {
		return d.showMultiSelect(ctx, in)
	}
	if len(in.Options) > 0 {
		return d.showSelect(ctx, in)
	}
	return d.showInput(ctx, in)
}

func (d *tuiDialog) showConfirm(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	// Build tool-specific title and preview
	title := in.Title
	preview := in.Message
	if in.Call != nil {
		t, p := buildToolPreview(in.Call.Name, in.Call.Input)
		if t != "" {
			title = t
		}
		if p != "" {
			preview = p
		}
	}

	// Get tool category for "allow all X this session" option
	categoryLabel := "this action"
	if in.Tool != nil {
		category := permission.GetToolCategory(in.Tool.Name())
		categoryLabel = category.Label
	}

	confirmChan := make(chan ConfirmResult, 1)
	d.app.runner.SendEvent(showDialogEvent{
		baseEvent: newBaseEvent(),
		dialog: &DialogState{
			Type:                     DialogTypeConfirm,
			Active:                   true,
			Title:                    title,
			ContentPreview:           preview,
			ConfirmChan:              confirmChan,
			ConfirmToolCategoryLabel: categoryLabel,
		},
	})
	select {
	case result := <-confirmChan:
		return &dive.DialogOutput{
			Confirmed:    result.Approved,
			AllowSession: result.AllowSession,
			Feedback:     result.Feedback,
		}, nil
	case <-ctx.Done():
		d.app.runner.SendEvent(hideDialogEvent{baseEvent: newBaseEvent()})
		return &dive.DialogOutput{Canceled: true}, ctx.Err()
	}
}

func (d *tuiDialog) showSelect(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	options := make([]DialogOption, len(in.Options))
	defaultIdx := 0
	for i, opt := range in.Options {
		options[i] = DialogOption{
			Label:       opt.Label,
			Description: opt.Description,
			Value:       opt.Value,
		}
		if opt.Value == in.Default {
			defaultIdx = i
		}
	}

	selectChan := make(chan SelectResult, 1)
	d.app.runner.SendEvent(showDialogEvent{
		baseEvent: newBaseEvent(),
		dialog: &DialogState{
			Type:         DialogTypeSelect,
			Active:       true,
			Title:        in.Title,
			Message:      in.Message,
			Options:      options,
			DefaultIndex: defaultIdx,
			SelectIndex:  defaultIdx,
			SelectChan:   selectChan,
		},
	})
	select {
	case result := <-selectChan:
		if result.OtherText != "" {
			return &dive.DialogOutput{Text: result.OtherText}, nil
		}
		if result.Index < 0 {
			return &dive.DialogOutput{Canceled: true}, nil
		}
		return &dive.DialogOutput{Values: []string{in.Options[result.Index].Value}}, nil
	case <-ctx.Done():
		d.app.runner.SendEvent(hideDialogEvent{baseEvent: newBaseEvent()})
		return &dive.DialogOutput{Canceled: true}, ctx.Err()
	}
}

func (d *tuiDialog) showMultiSelect(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	options := make([]DialogOption, len(in.Options))
	checked := make([]bool, len(in.Options))
	for i, opt := range in.Options {
		options[i] = DialogOption{
			Label:       opt.Label,
			Description: opt.Description,
			Value:       opt.Value,
		}
		if opt.Value == in.Default {
			checked[i] = true
		}
	}

	multiSelectChan := make(chan []int, 1)
	d.app.runner.SendEvent(showDialogEvent{
		baseEvent: newBaseEvent(),
		dialog: &DialogState{
			Type:               DialogTypeMultiSelect,
			Active:             true,
			Title:              in.Title,
			Message:            in.Message,
			Options:            options,
			MultiSelectChan:    multiSelectChan,
			MultiSelectChecked: checked,
			MultiSelectCursor:  0,
		},
	})
	select {
	case indices := <-multiSelectChan:
		if indices == nil {
			return &dive.DialogOutput{Canceled: true}, nil
		}
		var values []string
		for _, idx := range indices {
			values = append(values, in.Options[idx].Value)
		}
		return &dive.DialogOutput{Values: values}, nil
	case <-ctx.Done():
		d.app.runner.SendEvent(hideDialogEvent{baseEvent: newBaseEvent()})
		return &dive.DialogOutput{Canceled: true}, ctx.Err()
	}
}

func (d *tuiDialog) showInput(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	inputChan := make(chan string, 1)
	d.app.runner.SendEvent(showDialogEvent{
		baseEvent: newBaseEvent(),
		dialog: &DialogState{
			Type:         DialogTypeInput,
			Active:       true,
			Title:        in.Title,
			Message:      in.Message,
			DefaultValue: in.Default,
			InputValue:   "",
			InputChan:    inputChan,
		},
	})
	select {
	case value := <-inputChan:
		if value == "" && in.Default == "" {
			return &dive.DialogOutput{Canceled: true}, nil
		}
		return &dive.DialogOutput{Text: value}, nil
	case <-ctx.Done():
		d.app.runner.SendEvent(hideDialogEvent{baseEvent: newBaseEvent()})
		return &dive.DialogOutput{Canceled: true}, ctx.Err()
	}
}

// buildToolPreview generates a human-readable title and content preview
// for a tool confirmation dialog based on the tool name and its JSON input.
func buildToolPreview(toolName string, input json.RawMessage) (title, preview string) {
	var parsed map[string]interface{}
	if len(input) > 0 {
		json.Unmarshal(input, &parsed)
	}

	switch toolName {
	case "Bash":
		if cmd, ok := parsed["command"].(string); ok {
			if len(cmd) > 80 {
				cmd = cmd[:77] + "..."
			}
			title = fmt.Sprintf("Run: %s", cmd)
		}

	case "Edit":
		if filePath, ok := parsed["file_path"].(string); ok {
			title = fmt.Sprintf("Edit %s", filePath)
		} else if filePath, ok := parsed["filePath"].(string); ok {
			title = fmt.Sprintf("Edit %s", filePath)
		}
		if oldStr, ok := parsed["old_string"].(string); ok {
			if newStr, ok := parsed["new_string"].(string); ok {
				if len(oldStr) > 50 {
					oldStr = oldStr[:47] + "..."
				}
				if len(newStr) > 50 {
					newStr = newStr[:47] + "..."
				}
				preview = fmt.Sprintf("Replace:\n  %q\nWith:\n  %q", oldStr, newStr)
			}
		}

	case "Write":
		if filePath, ok := parsed["file_path"].(string); ok {
			title = fmt.Sprintf("Write to %s", filePath)
		} else if filePath, ok := parsed["filePath"].(string); ok {
			title = fmt.Sprintf("Write to %s", filePath)
		}
		if content, ok := parsed["content"].(string); ok {
			lines := strings.Split(content, "\n")
			if len(lines) > 10 {
				preview = strings.Join(lines[:10], "\n") + "\n..."
			} else {
				preview = content
			}
		}

	case "Read":
		if filePath, ok := parsed["file_path"].(string); ok {
			title = fmt.Sprintf("Read %s", filePath)
		} else if filePath, ok := parsed["filePath"].(string); ok {
			title = fmt.Sprintf("Read %s", filePath)
		}

	default:
		if len(parsed) > 0 {
			var params []string
			for k, v := range parsed {
				valStr := fmt.Sprintf("%v", v)
				if len(valStr) > 50 {
					valStr = valStr[:47] + "..."
				}
				params = append(params, fmt.Sprintf("%s: %s", k, valStr))
			}
			if len(params) > 0 {
				preview = strings.Join(params, "\n")
			}
		}
	}

	return title, preview
}

func createTools(workspaceDir string, dialog dive.Dialog) []dive.Tool {
	if dialog == nil {
		dialog = &dive.AutoApproveDialog{}
	}
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

		// Write tools
		toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewEditTool(toolkit.EditToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewBashTool(toolkit.BashToolOptions{
			WorkspaceDir: workspaceDir,
		}),

		// User interaction
		toolkit.NewAskUserTool(toolkit.AskUserToolOptions{
			Dialog: dialog,
		}),
	}

	// Add web fetch if available
	if firecrawlClient, err := firecrawl.New(); err == nil {
		tools = append(tools, toolkit.NewFetchTool(toolkit.FetchToolOptions{
			Fetcher: firecrawlClient,
		}))
	} else {
		tools = append(tools, toolkit.NewFetchTool(toolkit.FetchToolOptions{
			Fetcher: fetch.NewHTTPFetcher(fetch.HTTPFetcherOptions{}),
		}))
	}

	// Add web search if available
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

// defaultPermissionRules builds the CLI's default permission rules.
// Read-only tools are auto-allowed; other tools continue through normal
// permission flow (prompted unless explicitly allowed/denied elsewhere).
func defaultPermissionRules(tools []dive.Tool) permission.Rules {
	rules := make(permission.Rules, 0, len(tools))
	seen := make(map[string]bool, len(tools))

	for _, tool := range tools {
		if tool == nil {
			continue
		}
		annotations := tool.Annotations()
		if annotations == nil || !annotations.ReadOnlyHint {
			continue
		}
		name := tool.Name()
		if name == "" || seen[name] {
			continue
		}
		rules = append(rules, permission.AllowRule(name))
		seen[name] = true
	}

	return rules
}

func getDefaultModel() string {
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
	return "claude-haiku-4-5"
}
