package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/hooks"
	"github.com/deepnoodle-ai/dive/memory"
	"github.com/deepnoodle-ai/dive/settings"
	"github.com/spf13/cobra"
)

// InteractiveMode represents the current interactive mode
type InteractiveMode int

const (
	NormalMode InteractiveMode = iota
	AutoAcceptMode
	PlanMode
	BypassPermissionsMode
)

// InteractiveSession manages an interactive Dive session
type InteractiveSession struct {
	agent       dive.Agent
	ctx         context.Context
	threadID    string
	sessionID   string
	mode        InteractiveMode
	settings    *settings.Settings
	memory      *memory.Memory
	hookManager *hooks.HookManager
	history     []string
	historyIdx  int
	vimMode     bool
	statusLine  string
}

// SlashCommand represents a slash command
type SlashCommand struct {
	Name        string
	Description string
	Handler     func(session *InteractiveSession, args string) error
	Hidden      bool
}

var slashCommands []SlashCommand

func init() {
	slashCommands = []SlashCommand{
	{
		Name:        "help",
		Description: "Show available commands",
		Handler:     cmdHelp,
	},
	{
		Name:        "clear",
		Description: "Clear the current session and start fresh",
		Handler:     cmdClear,
	},
	{
		Name:        "memory",
		Description: "View or edit memory files (DIVE.md)",
		Handler:     cmdMemory,
	},
	{
		Name:        "config",
		Description: "View or edit configuration settings",
		Handler:     cmdConfig,
	},
	{
		Name:        "hooks",
		Description: "Manage hooks configuration",
		Handler:     cmdHooks,
	},
	{
		Name:        "agents",
		Description: "Manage subagents",
		Handler:     cmdAgents,
	},
	{
		Name:        "mcp",
		Description: "Manage MCP server connections",
		Handler:     cmdMCP,
	},
	{
		Name:        "mode",
		Description: "Switch between Normal, Auto-Accept, Plan, and Bypass modes",
		Handler:     cmdMode,
	},
	{
		Name:        "vim",
		Description: "Toggle vim editing mode",
		Handler:     cmdVim,
	},
	{
		Name:        "status",
		Description: "Show session status and configuration",
		Handler:     cmdStatus,
	},
	{
		Name:        "compact",
		Description: "Compact the current session to save context",
		Handler:     cmdCompact,
	},
	{
		Name:        "resume",
		Description: "Resume a previous session",
		Handler:     cmdResume,
	},
	{
		Name:        "threads",
		Description: "List and manage conversation threads",
		Handler:     cmdThreads,
	},
	{
		Name:        "init",
		Description: "Initialize project memory (DIVE.md)",
		Handler:     cmdInit,
	},
	{
		Name:        "allowed-tools",
		Description: "Manage allowed tools for the current session",
		Handler:     cmdAllowedTools,
	},
	{
		Name:        "terminal-setup",
		Description: "Configure terminal for better multiline support",
		Handler:     cmdTerminalSetup,
	},
	{
		Name:        "export",
		Description: "Export conversation to various formats",
		Handler:     cmdExport,
	},
	{
		Name:        "bug",
		Description: "Report a bug or issue",
		Handler:     cmdBug,
		Hidden:      true,
	},
	}
}

// NewInteractiveSession creates a new interactive session
func NewInteractiveSession(agent dive.Agent, settings *settings.Settings) *InteractiveSession {
	session := &InteractiveSession{
		agent:      agent,
		ctx:        context.Background(),
		sessionID:  generateSessionID(),
		mode:       NormalMode,
		settings:   settings,
		history:    []string{},
		historyIdx: -1,
	}

	// Initialize memory
	session.memory = memory.NewMemory()
	if err := session.memory.Load(); err != nil {
		fmt.Printf("Warning: Failed to load memory: %v\n", err)
	}

	// Initialize hooks
	if settings != nil {
		session.hookManager = hooks.NewHookManager(settings)
	}

	// Apply settings
	if settings != nil {
		// Set default mode
		if settings.Permissions != nil {
			switch settings.Permissions.DefaultMode {
			case "acceptEdits":
				session.mode = AutoAcceptMode
			case "plan":
				session.mode = PlanMode
			case "bypassPermissions":
				if settings.Permissions.DisableBypassPermissions != "disable" {
					session.mode = BypassPermissionsMode
				}
			}
		}

		// Apply environment variables
		for key, value := range settings.Env {
			os.Setenv(key, value)
		}
	}

	// Execute SessionStart hook
	if session.hookManager != nil {
		transcriptPath, _ := hooks.CreateTranscriptFile(session.sessionID)
		input := &hooks.HookInput{
			SessionID:      session.sessionID,
			TranscriptPath: transcriptPath,
			CWD:           getCurrentDir(),
			HookEventName:  string(hooks.SessionStart),
			Source:         "startup",
		}
		session.hookManager.ExecuteHooks(session.ctx, hooks.SessionStart, input)
	}

	// Update status line
	session.updateStatusLine()

	return session
}

// Run starts the interactive session
func (s *InteractiveSession) Run() error {
	// Show welcome message
	s.showWelcome()

	// Create input scanner
	scanner := bufio.NewScanner(os.Stdin)

	for {
		// Show prompt
		s.showPrompt()

		// Read input
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()

		// Handle special inputs
		if input == "" {
			continue
		}
		if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
			break
		}

		// Add to history
		s.addToHistory(input)

		// Process input
		if err := s.processInput(input); err != nil {
			fmt.Printf("%s Error: %v\n", errorStyle.Sprint("✗"), err)
		}
	}

	// Execute SessionEnd hook
	if s.hookManager != nil {
		transcriptPath, _ := hooks.CreateTranscriptFile(s.sessionID)
		input := &hooks.HookInput{
			SessionID:      s.sessionID,
			TranscriptPath: transcriptPath,
			CWD:           getCurrentDir(),
			HookEventName:  string(hooks.SessionEnd),
			Reason:         "exit",
		}
		s.hookManager.ExecuteHooks(s.ctx, hooks.SessionEnd, input)
	}

	fmt.Println("\nGoodbye!")
	return nil
}

// processInput processes user input
func (s *InteractiveSession) processInput(input string) error {
	// Handle slash commands
	if strings.HasPrefix(input, "/") {
		return s.handleSlashCommand(input)
	}

	// Handle memory shortcut
	if strings.HasPrefix(input, "#") {
		return s.handleMemoryShortcut(input[1:])
	}

	// Handle bash mode
	if strings.HasPrefix(input, "!") {
		return s.handleBashMode(input[1:])
	}

	// Execute UserPromptSubmit hook
	if s.hookManager != nil {
		transcriptPath, _ := hooks.CreateTranscriptFile(s.sessionID)
		hookInput := &hooks.HookInput{
			SessionID:      s.sessionID,
			TranscriptPath: transcriptPath,
			CWD:           getCurrentDir(),
			HookEventName:  string(hooks.UserPromptSubmit),
			Prompt:         input,
		}
		output, err := s.hookManager.ExecuteHooks(s.ctx, hooks.UserPromptSubmit, hookInput)
		if err != nil {
			return err
		}
		if !output.Continue {
			if output.StopReason != "" {
				fmt.Println(output.StopReason)
			}
			return nil
		}
		// Add any additional context from hooks
		if output.HookSpecificOutput != nil && output.HookSpecificOutput.AdditionalContext != "" {
			input = fmt.Sprintf("%s\n\n[Context from hooks: %s]", input, output.HookSpecificOutput.AdditionalContext)
		}
	}

	// Regular chat message
	return s.sendMessage(input)
}

// handleSlashCommand processes slash commands
func (s *InteractiveSession) handleSlashCommand(input string) error {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	for _, slashCmd := range slashCommands {
		if slashCmd.Name == cmd {
			return slashCmd.Handler(s, args)
		}
	}

	// Check for MCP prompt commands (format: /mcp__servername__promptname)
	if strings.HasPrefix(cmd, "mcp__") {
		return s.handleMCPPrompt(cmd, args)
	}

	return fmt.Errorf("unknown command: /%s (use /help for available commands)", cmd)
}

// handleMemoryShortcut handles the # memory shortcut
func (s *InteractiveSession) handleMemoryShortcut(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("empty memory content")
	}

	// Show selection menu
	fmt.Println("\nSelect memory location:")
	fmt.Println("1. User memory (~/.dive/DIVE.md)")
	fmt.Println("2. Project memory (.dive/DIVE.md)")
	fmt.Print("Choice (1-2): ")

	var choice string
	fmt.Scanln(&choice)

	var memoryType memory.MemoryType
	switch choice {
	case "1":
		memoryType = memory.UserMemory
	case "2":
		memoryType = memory.ProjectMemory
	default:
		return fmt.Errorf("invalid choice")
	}

	if err := s.memory.AddMemory(content, memoryType); err != nil {
		return err
	}

	fmt.Println(successStyle.Sprint("✓ Memory added successfully"))
	return nil
}

// handleBashMode executes bash commands directly
func (s *InteractiveSession) handleBashMode(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	fmt.Printf("%s Executing: %s\n", yellowStyle.Sprint("$"), command)

	cmd := exec.CommandContext(s.ctx, "sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// handleMCPPrompt handles MCP server prompts as slash commands
func (s *InteractiveSession) handleMCPPrompt(cmd string, args string) error {
	// Parse MCP prompt command
	parts := strings.Split(cmd, "__")
	if len(parts) != 3 {
		return fmt.Errorf("invalid MCP prompt command format")
	}

	serverName := parts[1]
	promptName := parts[2]

	// TODO: Execute MCP prompt through the appropriate server
	fmt.Printf("Executing MCP prompt '%s' on server '%s' with args: %s\n", promptName, serverName, args)

	return fmt.Errorf("MCP prompts not yet implemented")
}

// sendMessage sends a message to the agent
func (s *InteractiveSession) sendMessage(message string) error {
	// Add memory context
	if s.memory != nil {
		memoryContext := s.memory.GetCombinedMemory()
		if memoryContext != "" {
			message = fmt.Sprintf("[Memory Context:\n%s]\n\n%s", memoryContext, message)
		}
	}

	fmt.Printf("\n%s: ", boldStyle.Sprint(s.agent.Name()))

	// Send message to agent
	_, err := s.agent.CreateResponse(s.ctx,
		dive.WithInput(message),
		dive.WithThreadID(s.threadID),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			// Display response
			if item.Type == dive.ResponseItemTypeModelEvent {
				payload := item.Event
				if payload.Delta != nil {
					delta := payload.Delta
					if delta.Text != "" {
						fmt.Print(successStyle.Sprint(delta.Text))
					} else if delta.Thinking != "" {
						if s.mode == NormalMode || s.mode == PlanMode {
							fmt.Print(thinkingStyle.Sprint(delta.Thinking))
						}
					}
				}
			}
			return nil
		}),
	)

	fmt.Println()
	return err
}

// Command handlers
func cmdHelp(s *InteractiveSession, args string) error {
	fmt.Println("\nAvailable commands:")
	for _, cmd := range slashCommands {
		if !cmd.Hidden {
			fmt.Printf("  /%s - %s\n", boldStyle.Sprint(cmd.Name), cmd.Description)
		}
	}
	fmt.Println("\nShortcuts:")
	fmt.Println("  # - Add memory to DIVE.md")
	fmt.Println("  ! - Execute bash command")
	fmt.Println("\nKeyboard shortcuts:")
	fmt.Println("  Ctrl+C - Cancel current operation")
	fmt.Println("  Ctrl+D - Exit session")
	fmt.Println("  Ctrl+L - Clear screen")
	fmt.Println("  Shift+Tab - Toggle mode")
	fmt.Println("  Esc+Esc - Edit previous message")
	return nil
}

func cmdClear(s *InteractiveSession, args string) error {
	// Clear the screen
	fmt.Print("\033[2J\033[H")

	// Reset session
	s.threadID = ""
	s.sessionID = generateSessionID()

	// Execute SessionStart hook with "clear" source
	if s.hookManager != nil {
		transcriptPath, _ := hooks.CreateTranscriptFile(s.sessionID)
		input := &hooks.HookInput{
			SessionID:      s.sessionID,
			TranscriptPath: transcriptPath,
			CWD:           getCurrentDir(),
			HookEventName:  string(hooks.SessionStart),
			Source:         "clear",
		}
		s.hookManager.ExecuteHooks(s.ctx, hooks.SessionStart, input)
	}

	fmt.Println(successStyle.Sprint("✓ Session cleared"))
	return nil
}

func cmdMemory(s *InteractiveSession, args string) error {
	if s.memory == nil {
		return fmt.Errorf("memory system not initialized")
	}

	files := s.memory.GetMemoryFiles()
	if len(files) == 0 {
		fmt.Println("No memory files loaded")
		return nil
	}

	fmt.Println("\nLoaded memory files:")
	for _, file := range files {
		fmt.Printf("  - %s (%v)\n", file.Path, file.Type)
	}

	if args == "edit" {
		// Open editor for project memory
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		return exec.Command(editor, ".dive/DIVE.md").Run()
	}

	return nil
}

func cmdConfig(s *InteractiveSession, args string) error {
	// TODO: Implement config viewing/editing
	fmt.Println("Configuration management coming soon!")
	return nil
}

func cmdHooks(s *InteractiveSession, args string) error {
	if s.settings == nil || s.settings.Hooks == nil {
		fmt.Println("No hooks configured")
		return nil
	}

	fmt.Println("\nConfigured hooks:")
	for event, configs := range s.settings.Hooks {
		fmt.Printf("\n%s:\n", boldStyle.Sprint(event))
		for _, config := range configs {
			if config.Matcher != "" {
				fmt.Printf("  Matcher: %s\n", config.Matcher)
			}
			for _, hook := range config.Hooks {
				fmt.Printf("    - %s: %s\n", hook.Type, hook.Command)
			}
		}
	}

	return nil
}

func cmdAgents(s *InteractiveSession, args string) error {
	// TODO: Implement subagent management
	fmt.Println("Subagent management coming soon!")
	return nil
}

func cmdMCP(s *InteractiveSession, args string) error {
	if s.settings == nil || s.settings.MCPServers == nil {
		fmt.Println("No MCP servers configured")
		return nil
	}

	fmt.Println("\nConfigured MCP servers:")
	for name, server := range s.settings.MCPServers {
		fmt.Printf("  - %s (%s)\n", boldStyle.Sprint(name), server.Type)
		if server.URL != "" {
			fmt.Printf("    URL: %s\n", server.URL)
		}
		if server.Command != "" {
			fmt.Printf("    Command: %s\n", server.Command)
		}
	}

	return nil
}

func cmdMode(s *InteractiveSession, args string) error {
	if args == "" {
		// Show current mode
		modeNames := map[InteractiveMode]string{
			NormalMode:            "Normal",
			AutoAcceptMode:        "Auto-Accept",
			PlanMode:              "Plan",
			BypassPermissionsMode: "Bypass Permissions",
		}
		fmt.Printf("Current mode: %s\n", boldStyle.Sprint(modeNames[s.mode]))
		return nil
	}

	// Switch mode
	switch strings.ToLower(args) {
	case "normal":
		s.mode = NormalMode
	case "auto", "auto-accept":
		s.mode = AutoAcceptMode
	case "plan":
		s.mode = PlanMode
	case "bypass":
		if s.settings == nil || s.settings.Permissions == nil || s.settings.Permissions.DisableBypassPermissions != "disable" {
			s.mode = BypassPermissionsMode
		} else {
			return fmt.Errorf("bypass mode is disabled by policy")
		}
	default:
		return fmt.Errorf("unknown mode: %s", args)
	}

	s.updateStatusLine()
	fmt.Printf("Switched to %s mode\n", boldStyle.Sprint(args))
	return nil
}

func cmdVim(s *InteractiveSession, args string) error {
	s.vimMode = !s.vimMode
	if s.vimMode {
		fmt.Println(successStyle.Sprint("✓ Vim mode enabled"))
	} else {
		fmt.Println(successStyle.Sprint("✓ Vim mode disabled"))
	}
	return nil
}

func cmdStatus(s *InteractiveSession, args string) error {
	fmt.Println("\nSession Status:")
	fmt.Printf("  Session ID: %s\n", s.sessionID)
	fmt.Printf("  Thread ID: %s\n", s.threadID)
	fmt.Printf("  Mode: %v\n", s.mode)
	fmt.Printf("  Vim Mode: %v\n", s.vimMode)

	if s.memory != nil {
		files := s.memory.GetMemoryFiles()
		fmt.Printf("  Memory files loaded: %d\n", len(files))
	}

	if s.settings != nil {
		if s.settings.Model != "" {
			fmt.Printf("  Model: %s\n", s.settings.Model)
		}
		if s.settings.DefaultProvider != "" {
			fmt.Printf("  Provider: %s\n", s.settings.DefaultProvider)
		}
	}

	return nil
}

func cmdCompact(s *InteractiveSession, args string) error {
	// Execute PreCompact hook
	if s.hookManager != nil {
		transcriptPath, _ := hooks.CreateTranscriptFile(s.sessionID)
		input := &hooks.HookInput{
			SessionID:      s.sessionID,
			TranscriptPath: transcriptPath,
			HookEventName:  string(hooks.PreCompact),
			Trigger:        "manual",
			CustomInstructions: args,
		}
		s.hookManager.ExecuteHooks(s.ctx, hooks.PreCompact, input)
	}

	// TODO: Implement actual compaction
	fmt.Println("Session compaction coming soon!")
	return nil
}

func cmdResume(s *InteractiveSession, args string) error {
	// TODO: Implement session resumption
	fmt.Println("Session resumption coming soon!")
	return nil
}

func cmdThreads(s *InteractiveSession, args string) error {
	// TODO: Implement thread management
	fmt.Println("Thread management coming soon!")
	return nil
}

func cmdInit(s *InteractiveSession, args string) error {
	if err := memory.InitProjectMemory(); err != nil {
		return err
	}
	fmt.Println(successStyle.Sprint("✓ Project memory initialized at .dive/DIVE.md"))

	// Reload memory
	if s.memory != nil {
		s.memory.Load()
	}

	return nil
}

func cmdAllowedTools(s *InteractiveSession, args string) error {
	// TODO: Implement tool management
	fmt.Println("Tool management coming soon!")
	return nil
}

func cmdTerminalSetup(s *InteractiveSession, args string) error {
	// TODO: Implement terminal setup
	fmt.Println("Terminal setup coming soon!")
	return nil
}

func cmdExport(s *InteractiveSession, args string) error {
	// TODO: Implement export functionality
	fmt.Println("Export functionality coming soon!")
	return nil
}

func cmdBug(s *InteractiveSession, args string) error {
	fmt.Println("Report issues at: https://github.com/deepnoodle-ai/dive/issues")
	return nil
}

// Helper functions
func (s *InteractiveSession) showWelcome() {
	fmt.Println(boldStyle.Sprint("Welcome to Dive Interactive Mode!"))
	fmt.Println("Type /help for available commands, or start chatting.")
	fmt.Println()
}

func (s *InteractiveSession) showPrompt() {
	modeIndicator := ""
	switch s.mode {
	case AutoAcceptMode:
		modeIndicator = yellowStyle.Sprint("[AUTO] ")
	case PlanMode:
		modeIndicator = greenStyle.Sprint("[PLAN] ")
	case BypassPermissionsMode:
		modeIndicator = errorStyle.Sprint("[BYPASS] ")
	}

	fmt.Printf("%s%s ", modeIndicator, boldStyle.Sprint("You:"))
}

func (s *InteractiveSession) updateStatusLine() {
	// TODO: Implement status line update based on settings
	if s.settings != nil && s.settings.StatusLine != nil {
		// Execute status line command if configured
	}
}

func (s *InteractiveSession) addToHistory(input string) {
	s.history = append(s.history, input)
	s.historyIdx = len(s.history)
}

func generateSessionID() string {
	return fmt.Sprintf("dive-%d", os.Getpid())
}

func getCurrentDir() string {
	dir, _ := os.Getwd()
	return dir
}

// InteractiveCmd is the main interactive command
var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Start an enhanced interactive session with Dive",
	Long:  "Start an enhanced interactive session with slash commands, hooks, and memory support",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load settings
		settingsManager := settings.NewSettingsManager()
		if err := settingsManager.Load(); err != nil {
			fmt.Printf("Warning: Failed to load settings: %v\n", err)
		}
		settings := settingsManager.GetMerged()

		// Create agent (simplified for now)
		agent, err := createAgentFromFlags("", "", "", nil)
		if err != nil {
			return err
		}

		// Create and run session
		session := NewInteractiveSession(agent, settings)
		return session.Run()
	},
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}