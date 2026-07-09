package main

import (
	"bufio"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/google"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/kagi"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/permission"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/dive/skill"
	"github.com/deepnoodle-ai/dive/subagent"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/dive/toolkit/firecrawl"
	"github.com/deepnoodle-ai/dive/toolkit/orchestration"
	"github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/fetch"
)

func main() {
	app := cli.New("dive").
		Description("Interactive AI assistant for coding tasks").
		Version("0.1.0")

	// Image generation subcommand
	app.Command("image").
		Description("Generate or edit an image from a text prompt").
		Args("prompt").
		Flags(
			cli.String("model", "m").
				Default("").
				Help("Model to use (default: gpt-image-2)"),
			cli.String("aspect").
				Default("").
				Help("Aspect ratio: 1:1, 16:9, 9:16"),
			cli.String("format").
				Default("").
				Help("Output format: png, jpeg, webp"),
			cli.String("out", "o").
				Default("").
				Help("Output file path (auto-generated from prompt if omitted)"),
			cli.Int("count", "n").
				Default(1).
				Help("Number of images to generate"),
			cli.Strings("ref", "r").
				Help("Reference image file path (can be specified multiple times)"),
			cli.Bool("edit", "e").
				Default(false).
				Help("Edit reference images instead of generating new ones"),
			cli.Bool("open").
				Default(false).
				Help("Open result in default viewer"),
		).
		Run(runImage)

	// Video generation subcommand
	app.Command("video").
		Description("Generate a video from a text prompt").
		Args("prompt").
		Flags(
			cli.String("model", "m").
				Default("").
				Help("Model to use (default: veo-3.1-generate-preview)"),
			cli.String("aspect").
				Default("").
				Help("Aspect ratio: 16:9, 9:16, 1:1"),
			cli.String("duration", "d").
				Default("8s").
				Help("Video duration (e.g. 8s, 16s, 20s). Exact duration depends on provider."),
			cli.String("out", "o").
				Default("").
				Help("Output file path"),
			cli.Bool("open").
				Default(false).
				Help("Open result in default viewer"),
		).
		Run(runVideo)

	// Models subcommand
	app.Command("models").
		Description("List available models and providers").
		Flags(
			cli.Bool("available", "a").
				Default(false).
				Help("Only show providers with API keys configured"),
		).
		Run(runModels)

	app.Main().
		Args("prompt?").
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
			cli.Int("max-tokens").
				Default(16000).
				Env("DIVE_MAX_TOKENS").
				Help("Maximum tokens in response"),
			cli.Bool("show-thinking").
				Default(false).
				Env("DIVE_SHOW_THINKING").
				Help("Request and show summarized model thinking when supported"),
			cli.String("system-prompt").
				Default("").
				Help("System prompt to use for the session"),
			cli.Bool("print", "p").
				Default(false).
				Help("Print response and exit (useful for pipes)"),
			cli.String("output-format").
				Default("text").
				Help("Output format (only works with --print): \"text\" (default) or \"json\""),
			cli.String("api-endpoint").
				Default("").
				Env("DIVE_API_ENDPOINT").
				Help("Override the API endpoint URL for the provider"),
			cli.Bool("resume", "r").
				Default(false).
				Help("Resume a previous session"),
			cli.Bool("compaction").
				Default(true).
				Env("DIVE_COMPACTION").
				Help("Enable automatic context compaction"),
			cli.Int("compaction-threshold").
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

	// Create model
	model := createModel(modelName, ctx.String("api-endpoint"))

	// Build system prompt
	systemPrompt := ctx.String("system-prompt")
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt(workspaceDir, modelName)
	}

	// Create TUI dialog for interactive user prompts (AskUserQuestion tool)
	tuiDialog := &tuiDialog{}
	// monitorNotifier forwards monitor line batches to the app event loop
	monNotifier := &monitorNotifier{}

	// Create path validator for workspace enforcement
	pathValidator, err := toolkit.NewPathValidator(workspaceDir)
	if err != nil {
		return fmt.Errorf("failed to create path validator: %w", err)
	}

	// Create tools
	tools := createTools(pathValidator, tuiDialog)
	tools = append(tools, grokServerSideTools(modelName)...)

	// Set up the subagent catalog and orchestration tools. Runs is the shared
	// tracker that lets TaskStop cancel background spawns and monitors by id.
	subagents := map[string]*subagent.Definition{
		"GeneralPurpose": subagent.GeneralPurpose,
		"Explore":        subagent.Explore,
		"Plan":           subagent.Plan,
	}
	runs := orchestration.NewRuns()

	agentFactory := func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
		// Create sub-model (use parent model by default)
		subModel := model
		if def.Model != "" {
			subModel = createModel(def.Model, "")
		}

		return dive.NewAgent(dive.AgentOptions{
			Name:         name,
			SystemPrompt: def.Prompt,
			Model:        subModel,
			Tools:        subagent.FilterTools(def, parentTools),
		})
	}

	agentTool := orchestration.NewAgentTool(orchestration.AgentToolOptions{
		Subagents:    subagents,
		AgentFactory: agentFactory,
		ParentTools:  tools,
		Runs:         runs,
	})
	monitorTool := orchestration.NewMonitorTool(orchestration.MonitorToolOptions{
		Runs:           runs,
		NotifyCallback: monNotifier.notify,
	})
	taskStopTool := orchestration.NewTaskStopTool(orchestration.TaskStopToolOptions{
		Runs: runs,
	})
	tools = append(tools, agentTool, monitorTool, taskStopTool)

	// Create session store
	sessionStore, err := session.NewFileStore("~/.dive/sessions")
	if err != nil {
		return fmt.Errorf("failed to create session store: %w", err)
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
		result, err := RunSessionPicker(sessionStore, filter, workspaceDir)
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

	// Open the session
	bgCtx := context.Background()
	currentSession, err := sessionStore.Open(bgCtx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to open session: %w", err)
	}

	// Get initial prompt from args (if not resuming)
	var initialPrompt string
	if !ctx.Bool("resume") {
		args := ctx.Args()
		if len(args) > 0 {
			initialPrompt = strings.Join(args, " ")
		}
	}

	// Set up tool permission hook using the permission package
	permConfig := &permission.Config{
		Mode:  permission.ModeDefault,
		Rules: defaultPermissionRules(tools),
	}
	permManager := permission.NewManager(permConfig, tuiDialog)
	tuiDialog.perm = permManager
	permissionHook := permission.HookFromManager(permManager)

	// Set up compaction config
	var compactionConfig *compaction.CompactionConfig
	if ctx.Bool("compaction") {
		compactionConfig = &compaction.CompactionConfig{
			ContextTokenThreshold: ctx.Int("compaction-threshold"),
			Model:                 model,
		}
	}

	// Load skills and slash commands
	skills, err := skill.Load(bgCtx, skill.LoaderOptions{
		ProjectDir:     workspaceDir,
		ShellExpansion: true,
	})
	if err != nil {
		return fmt.Errorf("failed to load skills: %w", err)
	}

	// Allow read access to skill directories so the agent can read
	// reference files that live alongside skills (e.g., ~/.claude/skills/taste/reference/)
	for _, dir := range skills.BaseDirs() {
		_ = pathValidator.AllowReadPath(dir)
	}

	// Create model settings
	maxTokens := ctx.Int("max-tokens")
	modelSettings := &dive.ModelSettings{
		MaxTokens: &maxTokens,
	}
	if ctx.Bool("show-thinking") {
		modelSettings.Thinking = llm.ThinkingTypeAdaptive
		modelSettings.ThinkingDisplay = llm.ThinkingDisplaySummarized
	}
	if ctx.IsSet("temperature") {
		t := ctx.Float64("temperature")
		modelSettings.Temperature = &t
	}

	// Create agent options with hooks and extensions
	agentOpts := dive.AgentOptions{
		SystemPrompt:  systemPrompt,
		Model:         model,
		Tools:         tools,
		Extensions:    []dive.Extension{skills},
		ModelSettings: modelSettings,
		Hooks: dive.Hooks{
			PreToolUse: []dive.PreToolUseHook{permissionHook},
		},
	}

	// Mid-turn compaction: when compaction is enabled, summarize the working
	// context within a turn if it grows past the threshold, so a long tool loop
	// (many calls or large results) can't overflow the model's window before
	// the turn finishes. The summary is model-facing only — the full turn is
	// still saved (see compaction.MidTurnCompactionHook). app is assigned just
	// below; the notify closure only runs once the agent processes input, well
	// after that, so reading it here is safe.
	var app *App
	if compactionConfig != nil {
		midTurnThreshold := compactionConfig.ContextTokenThreshold
		if midTurnThreshold <= 0 {
			midTurnThreshold = compaction.DefaultContextTokenThreshold
		}
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration,
			compaction.MidTurnCompactionHook(
				compactionConfig.Model,
				midTurnThreshold,
				compaction.WithMidTurnSystemPrompt(systemPrompt),
				compaction.WithMidTurnNotify(func(e *compaction.CompactionEvent) {
					if app != nil {
						app.notifyMidTurnCompaction(e)
					}
				}),
			),
		)
	}

	agent, err := dive.NewAgent(agentOpts)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Create App
	app = NewApp(
		agent,
		sessionStore,
		workspaceDir,
		modelName,
		initialPrompt,
		compactionConfig,
		resumeSessionID,
		skills,
		ctx.String("api-endpoint"),
	)
	app.currentSession = currentSession

	attachment, err := loadStartupInstructionAttachment(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read startup instructions: %v\n", err)
	}
	app.startupAttachment = attachment

	// Wire up dialog and monitor notifier
	tuiDialog.app = app
	monNotifier.app = app

	return app.Run()
}

//go:embed system_prompt.txt
var defaultSystemPromptTemplate string

func defaultSystemPrompt(workspaceDir, modelName string) string {
	var b strings.Builder
	b.WriteString(defaultSystemPromptTemplate)
	b.WriteString("\n\n# Environment\nYou have been invoked in the following environment:\n")

	// Working directory and git status
	b.WriteString(fmt.Sprintf("- Working directory: %s\n", workspaceDir))
	if isGitRepo(workspaceDir) {
		b.WriteString("  - Is a git repository: true\n")
	}

	// Platform and OS
	b.WriteString(fmt.Sprintf("- Platform: %s\n", runtime.GOOS))
	if shell := os.Getenv("SHELL"); shell != "" {
		b.WriteString(fmt.Sprintf("- Shell: %s\n", filepath.Base(shell)))
	}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		b.WriteString(fmt.Sprintf("- OS version: %s %s\n", runtime.GOOS, strings.TrimSpace(string(out))))
	}

	// Model
	if modelName != "" {
		b.WriteString(fmt.Sprintf("- Model: %s\n", modelName))
	}

	return b.String()
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
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
		systemPrompt = defaultSystemPrompt(workspaceDir, modelName)
	}

	// Create model
	model := createModel(modelName, ctx.String("api-endpoint"))

	// Create tools (auto-approve dialog for non-interactive print mode)
	printValidator, err := toolkit.NewPathValidator(workspaceDir)
	if err != nil {
		return fmt.Errorf("failed to create path validator: %w", err)
	}
	tools := createTools(printValidator, nil)
	tools = append(tools, grokServerSideTools(modelName)...)

	// Create agent
	maxTokens := ctx.Int("max-tokens")
	printModelSettings := &dive.ModelSettings{
		MaxTokens: &maxTokens,
	}
	showThinking := ctx.Bool("show-thinking")
	if showThinking {
		printModelSettings.Thinking = llm.ThinkingTypeAdaptive
		printModelSettings.ThinkingDisplay = llm.ThinkingDisplaySummarized
	}
	if ctx.IsSet("temperature") {
		t := ctx.Float64("temperature")
		printModelSettings.Temperature = &t
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt:  systemPrompt,
		Model:         model,
		Tools:         tools,
		ModelSettings: printModelSettings,
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
		return runPrintText(bgCtx, agent, input, showThinking)
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
	// The main command declares a single optional positional ("prompt?"), so the
	// parser caps args at one element (extras are rejected as "unexpected
	// argument" before we get here).
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

func runPrintText(ctx context.Context, agent *dive.Agent, input string, showThinking bool) error {
	var outputText strings.Builder
	var thinkingText strings.Builder
	thinkingStarted := false
	textStarted := false

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(input),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent && item.Event != nil {
				if item.Event.Delta != nil {
					if showThinking && item.Event.Delta.Thinking != "" {
						if !thinkingStarted {
							fmt.Println("Thinking:")
							thinkingStarted = true
						}
						fmt.Print(item.Event.Delta.Thinking)
						thinkingText.WriteString(item.Event.Delta.Thinking)
					}
					if item.Event.Delta.Text != "" {
						if showThinking && thinkingStarted && !textStarted {
							if thinkingText.Len() > 0 && !strings.HasSuffix(thinkingText.String(), "\n") {
								fmt.Println()
							}
							fmt.Println()
							fmt.Println("Response:")
							textStarted = true
						}
						fmt.Print(item.Event.Delta.Text)
						outputText.WriteString(item.Event.Delta.Text)
					}
				}
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	if showThinking && thinkingStarted && outputText.Len() == 0 {
		if thinkingText.Len() > 0 && !strings.HasSuffix(thinkingText.String(), "\n") {
			fmt.Println()
		}
		fmt.Println()
		fmt.Println("Response:")
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
		if thinking := responseThinkingText(resp); thinking != "" {
			result["thinking"] = thinking
		}
		if resp.Usage != nil {
			result["usage"] = resp.Usage
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func responseThinkingText(resp *dive.Response) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, item := range resp.Items {
		if item.Type == dive.ResponseItemTypeMessage && item.Message != nil {
			if thinking := messageThinkingText(item.Message); thinking != "" {
				parts = append(parts, thinking)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func messageThinkingText(msg *llm.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, content := range msg.Content {
		if thinking, ok := content.(*llm.ThinkingContent); ok {
			text := strings.TrimSpace(thinking.Thinking)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// monitorNotifier sends monitor line batches to the app's event loop.
// The app field is set after App creation (same deferred-init pattern as tuiDialog).
type monitorNotifier struct {
	app *App
}

func (n *monitorNotifier) notify(description string, lines []string) {
	if n.app == nil || n.app.runner == nil {
		return
	}
	n.app.runner.SendEvent(monitorNotificationEvent{
		baseEvent:   newBaseEvent(),
		description: description,
		lines:       lines,
	})
}

// tuiDialog implements dive.Dialog by routing to the App's TUI dialog system.
// The app and perm fields are set after App and Manager creation (same
// pattern as the confirmer closure).
type tuiDialog struct {
	app  *App
	perm *permission.Manager
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
	// Build tool-specific title, subtitle, preview, and question
	tp := buildToolPreview(in.Call)
	title := tp.title
	if title == "" {
		title = in.Title
	}
	subtitle := tp.subtitle
	preview := tp.preview
	if preview == "" {
		preview = in.Message
	}
	question := tp.question

	// Describe what an "allow all X this session" approval would grant
	categoryLabel := "this action"
	if d.perm != nil {
		categoryLabel = d.perm.SessionGrantLabel(in.Tool, in.Call)
	} else if in.Tool != nil {
		categoryLabel = in.Tool.Name() + " operations"
	}

	confirmChan := make(chan ConfirmResult, 1)
	d.app.runner.SendEvent(showDialogEvent{
		baseEvent: newBaseEvent(),
		dialog: &DialogState{
			Type:                     DialogTypeConfirm,
			Active:                   true,
			Title:                    title,
			Message:                  subtitle,
			ContentPreview:           preview,
			ConfirmChan:              confirmChan,
			ConfirmToolCategoryLabel: categoryLabel,
			ConfirmQuestion:          question,
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
type toolPreview struct {
	title    string
	subtitle string
	preview  string
	question string
}

func buildToolPreview(call *llm.ToolUseContent) toolPreview {
	if call == nil {
		return toolPreview{}
	}
	var parsed map[string]interface{}
	if len(call.Input) > 0 {
		json.Unmarshal(call.Input, &parsed)
	}

	filePath, _ := parsed["file_path"].(string)
	if filePath == "" {
		filePath, _ = parsed["filePath"].(string)
	}
	fileName := filepath.Base(filePath)

	switch call.Name {
	case "Bash":
		cmd, _ := parsed["command"].(string)
		if len(cmd) > 80 {
			cmd = cmd[:77] + "..."
		}
		return toolPreview{
			title:    "Run command",
			preview:  cmd,
			question: "Do you want to run this command?",
		}

	case "Edit":
		tp := toolPreview{
			title:    "Edit file",
			subtitle: filePath,
			question: fmt.Sprintf("Do you want to make this edit to %s?", fileName),
		}
		oldStr, _ := parsed["old_string"].(string)
		newStr, _ := parsed["new_string"].(string)
		replaceAll, _ := parsed["replace_all"].(bool)
		if oldStr != "" || newStr != "" {
			tp.preview = buildEditDiffPreview(filePath, oldStr, newStr, replaceAll)
		}
		return tp

	case "Write":
		tp := toolPreview{
			title:    "Write file",
			subtitle: filePath,
			question: fmt.Sprintf("Do you want to write to %s?", fileName),
		}
		if content, ok := parsed["content"].(string); ok {
			lines := strings.Split(content, "\n")
			if len(lines) > 10 {
				tp.preview = strings.Join(lines[:10], "\n") + "\n..."
			} else {
				tp.preview = content
			}
		}
		return tp

	case "Read":
		return toolPreview{
			title:    "Read file",
			subtitle: filePath,
			question: fmt.Sprintf("Do you want to read %s?", fileName),
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
			return toolPreview{preview: strings.Join(params, "\n")}
		}
		return toolPreview{}
	}
}

// buildEditDiffPreview generates a diff preview for the permission dialog
// using only the old/new strings from the tool payload (no file I/O).
// When replaceAll is true, it shows a summary indicating all matches will be replaced.
func buildEditDiffPreview(filePath, oldStr, newStr string, replaceAll bool) string {
	var b strings.Builder

	// Removed lines (skip when empty to avoid stray "  - " line)
	if oldStr != "" {
		for _, line := range strings.Split(oldStr, "\n") {
			b.WriteString(fmt.Sprintf("  - %s\n", line))
		}
	}
	// Added lines (skip when empty to avoid stray "  + " line)
	if newStr != "" {
		for _, line := range strings.Split(newStr, "\n") {
			b.WriteString(fmt.Sprintf("  + %s\n", line))
		}
	}

	if replaceAll {
		b.WriteString("\n  (replace all occurrences)")
	}

	return strings.TrimRight(b.String(), "\n")
}

func formatSimpleEditPreview(oldStr, newStr string) string {
	if len(oldStr) > 50 {
		oldStr = oldStr[:47] + "..."
	}
	if len(newStr) > 50 {
		newStr = newStr[:47] + "..."
	}
	return fmt.Sprintf("Replace:\n  %q\nWith:\n  %q", oldStr, newStr)
}

func createTools(validator *toolkit.PathValidator, dialog dive.Dialog) []dive.Tool {
	if dialog == nil {
		dialog = &dive.AutoApproveDialog{}
	}
	tools := []dive.Tool{
		// Read-only file tools
		toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{
			Validator: validator,
		}),
		toolkit.NewGlobTool(toolkit.GlobToolOptions{
			Validator: validator,
		}),
		toolkit.NewGrepTool(toolkit.GrepToolOptions{
			Validator: validator,
		}),
		toolkit.NewListDirectoryTool(toolkit.ListDirectoryToolOptions{
			Validator: validator,
		}),

		// Write tools
		toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{
			Validator: validator,
		}),
		toolkit.NewEditTool(toolkit.EditToolOptions{
			Validator: validator,
		}),
		toolkit.NewBashTool(toolkit.BashToolOptions{
			Validator: validator,
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

	// Add image generation tool using the best available provider
	if imageModel := getDefaultImageModel(); imageModel != "" {
		tools = append(tools, toolkit.NewImageGenerationTool(imageModel,
			toolkit.WithImageToolWorkDir(validator.WorkspaceDir),
		))
	}

	// Add video generation tool using the best available provider
	if videoModel := getDefaultVideoModel(); videoModel != "" {
		tools = append(tools, toolkit.NewVideoGenerationTool(videoModel,
			toolkit.WithVideoToolWorkDir(validator.WorkspaceDir),
		))
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

func getDefaultImageModel() string {
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "gpt-image-2"
	}
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return "imagen-4.0-generate-001"
	}
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" {
		return "grok-imagine-image"
	}
	return ""
}

func getDefaultVideoModel() string {
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return "veo-3.1-generate-preview"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "sora-2"
	}
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" {
		return "grok-imagine-video"
	}
	return ""
}

func getDefaultModel() string {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "claude-haiku-4-5"
	}
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return "gemini-3-flash-preview"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "gpt-5.5"
	}
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" {
		return defaultGrokModel
	}
	if os.Getenv("MISTRAL_API_KEY") != "" {
		return "mistral-small-latest"
	}
	return "claude-haiku-4-5"
}
