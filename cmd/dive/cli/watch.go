package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

// WatchOptions holds configuration for the watch command
type WatchOptions struct {
	Patterns        []string
	OnChange        string
	Recursive       bool
	Debounce        time.Duration
	SystemPrompt    string
	AgentName       string
	Tools           []string
	ReasoningBudget int
	ExitOnError     bool
	LogFile         string
	OnlyExtensions  []string
	IgnorePatterns  []string
}

// FileWatcher manages file system watching and LLM action triggering
type FileWatcher struct {
	options   WatchOptions
	watcher   *fsnotify.Watcher
	agent     dive.Agent
	logger    slogger.Logger
	debouncer map[string]time.Time
}

// NewFileWatcher creates a new file watcher instance
func NewFileWatcher(options WatchOptions) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	logger := slogger.New(getLogLevel())

	// Create agent for LLM actions
	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %v", err)
	}

	var tools []dive.Tool
	if len(options.Tools) > 0 {
		for _, toolName := range options.Tools {
			tool, err := config.InitializeToolByName(toolName, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize tool %s: %w", toolName, err)
			}
			tools = append(tools, tool)
		}
	}

	modelSettings := &agent.ModelSettings{}
	if options.ReasoningBudget > 0 {
		modelSettings.ReasoningBudget = &options.ReasoningBudget
		maxTokens := 0
		if modelSettings.MaxTokens != nil {
			maxTokens = *modelSettings.MaxTokens
		}
		if options.ReasoningBudget > maxTokens+4096 {
			newLimit := options.ReasoningBudget + 4096
			modelSettings.MaxTokens = &newLimit
		}
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	watchAgent, err := agent.New(agent.Options{
		Name:             options.AgentName,
		Instructions:     options.SystemPrompt,
		Model:            model,
		Logger:           logger,
		Tools:            tools,
		ThreadRepository: agent.NewMemoryThreadRepository(),
		ModelSettings:    modelSettings,
		Confirmer:        confirmer,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating agent: %v", err)
	}

	return &FileWatcher{
		options:   options,
		watcher:   watcher,
		agent:     watchAgent,
		logger:    logger,
		debouncer: make(map[string]time.Time),
	}, nil
}

// Start begins watching for file changes
func (fw *FileWatcher) Start(ctx context.Context) error {
	defer fw.watcher.Close()

	// Add paths to watch based on patterns
	if err := fw.addWatchPaths(); err != nil {
		return fmt.Errorf("failed to add watch paths: %w", err)
	}

	fmt.Println(boldStyle.Sprint("🔍 File Watcher Started"))
	fmt.Printf("Watching patterns: %s\n", strings.Join(fw.options.Patterns, ", "))
	fmt.Printf("On change action: %s\n", fw.options.OnChange)
	fmt.Printf("Agent: %s\n", fw.options.AgentName)
	fmt.Println("Press Ctrl+C to stop...")
	fmt.Println()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n👋 File watcher stopped")
			return nil
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return nil
			}
			if err := fw.handleFileEvent(ctx, event); err != nil {
				fw.logger.Error("Error handling file event", "error", err, "file", event.Name)
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return nil
			}
			fw.logger.Error("File watcher error", "error", err)
		}
	}
}

// addWatchPaths adds file paths to the watcher based on the provided patterns
func (fw *FileWatcher) addWatchPaths() error {
	watchedDirs := make(map[string]bool)

	for _, pattern := range fw.options.Patterns {
		matches, err := doublestar.FilepathGlob(pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern %s: %w", pattern, err)
		}

		for _, match := range matches {
			// Get directory to watch
			dir := filepath.Dir(match)
			if !watchedDirs[dir] {
				if err := fw.watcher.Add(dir); err != nil {
					fw.logger.Warn("Failed to watch directory", "dir", dir, "error", err)
				} else {
					fw.logger.Debug("Watching directory", "dir", dir)
					watchedDirs[dir] = true
				}
			}

			// If recursive and it's a directory, add subdirectories
			if fw.options.Recursive {
				if info, err := os.Stat(match); err == nil && info.IsDir() {
					if err := fw.addRecursiveWatch(match, watchedDirs); err != nil {
						fw.logger.Warn("Failed to add recursive watch", "dir", match, "error", err)
					}
				}
			}
		}
	}

	if len(watchedDirs) == 0 {
		return fmt.Errorf("no directories found to watch for patterns: %s", strings.Join(fw.options.Patterns, ", "))
	}

	return nil
}

// addRecursiveWatch adds all subdirectories to the watcher
func (fw *FileWatcher) addRecursiveWatch(root string, watchedDirs map[string]bool) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if info.IsDir() && !watchedDirs[path] {
			if err := fw.watcher.Add(path); err != nil {
				fw.logger.Warn("Failed to watch directory", "dir", path, "error", err)
			} else {
				fw.logger.Debug("Watching directory", "dir", path)
				watchedDirs[path] = true
			}
		}
		return nil
	})
}

// handleFileEvent processes a file system event
func (fw *FileWatcher) handleFileEvent(ctx context.Context, event fsnotify.Event) error {
	// Check if the file matches any of our patterns
	if !fw.matchesPatterns(event.Name) {
		return nil
	}

	// Debounce rapid file changes
	now := time.Now()
	if lastTime, exists := fw.debouncer[event.Name]; exists {
		if now.Sub(lastTime) < fw.options.Debounce {
			return nil
		}
	}
	fw.debouncer[event.Name] = now

	// Only handle write and create events
	if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
		return fw.triggerLLMAction(ctx, event.Name, event.Op.String())
	}

	return nil
}

// matchesPatterns checks if a file path matches any of the watch patterns
func (fw *FileWatcher) matchesPatterns(filePath string) bool {
	// Check ignore patterns first
	for _, ignorePattern := range fw.options.IgnorePatterns {
		if matched, _ := doublestar.PathMatch(ignorePattern, filePath); matched {
			return false
		}
	}

	// Check file extension filter
	if len(fw.options.OnlyExtensions) > 0 {
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext != "" {
			ext = ext[1:] // Remove the dot
		}
		found := false
		for _, allowedExt := range fw.options.OnlyExtensions {
			if strings.ToLower(allowedExt) == ext {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check main patterns
	for _, pattern := range fw.options.Patterns {
		if matched, _ := doublestar.PathMatch(pattern, filePath); matched {
			return true
		}
	}
	return false
}

// triggerLLMAction invokes the LLM agent with the specified action
func (fw *FileWatcher) triggerLLMAction(ctx context.Context, filePath, operation string) error {
	fmt.Printf("📁 %s %s\n", operation, filePath)
	
	// Log to file if specified
	if fw.options.LogFile != "" {
		fw.logToFile(fmt.Sprintf("File changed: %s (%s)", filePath, operation))
	}

	fmt.Print(boldStyle.Sprintf("%s: ", fw.agent.Name()))

	// Read file content for context
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		fw.logger.Warn("Failed to read file content", "file", filePath, "error", err)
		fileContent = []byte("(unable to read file content)")
	}

	// Get file stats for additional context
	fileInfo, _ := os.Stat(filePath)
	var fileSize int64
	var modTime string
	if fileInfo != nil {
		fileSize = fileInfo.Size()
		modTime = fileInfo.ModTime().Format(time.RFC3339)
	}

	// Create enhanced message with file context and metadata
	message := fmt.Sprintf(`File changed: %s
Operation: %s
File size: %d bytes
Modified: %s

File content:
%s

Task: %s

Please provide actionable feedback and suggestions. If you find issues, be specific about line numbers and provide concrete fixes.`, 
		filePath, operation, fileSize, modTime, string(fileContent), fw.options.OnChange)

	// Stream response from agent
	stream, err := fw.agent.StreamResponse(ctx, dive.WithInput(message))
	if err != nil {
		errorMsg := fmt.Sprintf("error generating response: %v", err)
		fw.logger.Error("LLM response error", "error", err, "file", filePath)
		
		if fw.options.LogFile != "" {
			fw.logToFile(fmt.Sprintf("ERROR: %s", errorMsg))
		}
		
		if fw.options.ExitOnError {
			return fmt.Errorf(errorMsg)
		}
		
		fmt.Println(errorStyle.Sprint(errorMsg))
		return nil
	}
	defer stream.Close()

	var inToolUse, incremental bool
	var hasError bool
	toolUseAccum := ""
	toolName := ""
	toolID := ""
	responseText := ""

	for stream.Next(ctx) {
		event := stream.Event()
		if event.Type == dive.EventTypeLLMEvent {
			incremental = true
			payload := event.Item.Event
			if payload.ContentBlock != nil {
				cb := payload.ContentBlock
				if cb.Type == "tool_use" {
					toolName = cb.Name
					toolID = cb.ID
				}
			}
			if payload.Delta != nil {
				delta := payload.Delta
				if delta.Type == "tool_use" {
					inToolUse = true
					toolUseAccum += delta.PartialJSON
				} else if delta.Text != "" {
					if inToolUse {
						fmt.Println(yellowStyle.Sprint(toolName), yellowStyle.Sprint(toolID))
						fmt.Println(yellowStyle.Sprint(toolUseAccum))
						fmt.Print("----\n")
						inToolUse = false
						toolUseAccum = ""
					}
					text := delta.Text
					responseText += text
					fmt.Print(successStyle.Sprint(text))
				} else if delta.Thinking != "" {
					fmt.Print(thinkingStyle.Sprint(delta.Thinking))
				}
			}
		} else if event.Type == dive.EventTypeResponseCompleted {
			if !incremental {
				text := strings.TrimSpace(event.Response.OutputText())
				responseText = text
				fmt.Println(successStyle.Sprint(text))
			}
		} else if event.Type == dive.EventTypeError {
			hasError = true
			fw.logger.Error("Response stream error", "error", event.Error)
		}
	}

	// Log response to file if specified
	if fw.options.LogFile != "" && responseText != "" {
		fw.logToFile(fmt.Sprintf("Response for %s:\n%s", filePath, responseText))
	}

	fmt.Println()
	fmt.Println("---")

	// Exit on error if configured for CI/CD
	if hasError && fw.options.ExitOnError {
		return fmt.Errorf("LLM response contained errors")
	}

	return nil
}

// logToFile appends a message to the log file with timestamp
func (fw *FileWatcher) logToFile(message string) {
	if fw.options.LogFile == "" {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)
	
	file, err := os.OpenFile(fw.options.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fw.logger.Warn("Failed to open log file", "file", fw.options.LogFile, "error", err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(logEntry); err != nil {
		fw.logger.Warn("Failed to write to log file", "file", fw.options.LogFile, "error", err)
	}
}

var watchCmd = &cobra.Command{
	Use:   "watch [patterns...] --on-change [action]",
	Short: "Monitor files/directories and trigger LLM actions on changes",
	Long: `Monitor files/directories for changes and trigger LLM actions when changes occur.

Examples:
  # Basic file watching
  dive watch src/*.go --on-change "Lint and suggest fixes"
  
  # Recursive directory watching
  dive watch . --recursive --on-change "Review changes for security issues"
  
  # With tools and filtering
  dive watch "**/*.py" --on-change "Check for PEP8 compliance" --tools "Web.Search"
  
  # CI/CD integration with logging
  dive watch src/ --recursive --on-change "Run code review" --exit-on-error --log-file ci.log
  
  # Filter by extensions and ignore patterns
  dive watch . --only-extensions "go,js" --ignore "*.test.go,node_modules/**" --on-change "Code review"`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		onChange, err := cmd.Flags().GetString("on-change")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if onChange == "" {
			fmt.Println(errorStyle.Sprint("--on-change action is required"))
			os.Exit(1)
		}

		recursive, err := cmd.Flags().GetBool("recursive")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		debounceMs, err := cmd.Flags().GetInt("debounce")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		systemPrompt, err := cmd.Flags().GetString("system-prompt")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		agentName, err := cmd.Flags().GetString("agent-name")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		toolsStr, err := cmd.Flags().GetString("tools")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		var tools []string
		if toolsStr != "" {
			tools = strings.Split(toolsStr, ",")
		}

		reasoningBudget, err := cmd.Flags().GetInt("reasoning-budget")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		exitOnError, err := cmd.Flags().GetBool("exit-on-error")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		logFile, err := cmd.Flags().GetString("log-file")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		onlyExtensionsStr, err := cmd.Flags().GetString("only-extensions")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		var onlyExtensions []string
		if onlyExtensionsStr != "" {
			onlyExtensions = strings.Split(onlyExtensionsStr, ",")
		}

		ignorePatternsStr, err := cmd.Flags().GetString("ignore")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		var ignorePatterns []string
		if ignorePatternsStr != "" {
			ignorePatterns = strings.Split(ignorePatternsStr, ",")
		}

		options := WatchOptions{
			Patterns:        args,
			OnChange:        onChange,
			Recursive:       recursive,
			Debounce:        time.Duration(debounceMs) * time.Millisecond,
			SystemPrompt:    systemPrompt,
			AgentName:       agentName,
			Tools:           tools,
			ReasoningBudget: reasoningBudget,
			ExitOnError:     exitOnError,
			LogFile:         logFile,
			OnlyExtensions:  onlyExtensions,
			IgnorePatterns:  ignorePatterns,
		}

		if err := runWatch(cmd.Context(), options); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func runWatch(ctx context.Context, options WatchOptions) error {
	watcher, err := NewFileWatcher(options)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	return watcher.Start(ctx)
}

func init() {
	rootCmd.AddCommand(watchCmd)

	watchCmd.Flags().StringP("on-change", "", "", "Action to perform when files change (required)")
	watchCmd.Flags().BoolP("recursive", "r", false, "Watch directories recursively")
	watchCmd.Flags().IntP("debounce", "", 500, "Debounce time in milliseconds to avoid rapid triggers")
	watchCmd.Flags().StringP("system-prompt", "", "", "System prompt for the watch agent")
	watchCmd.Flags().StringP("agent-name", "", "FileWatcher", "Name of the watch agent")
	watchCmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the watch agent")
	watchCmd.Flags().IntP("reasoning-budget", "", 0, "Reasoning budget for the watch agent")
	watchCmd.Flags().BoolP("exit-on-error", "", false, "Exit on LLM errors (useful for CI/CD)")
	watchCmd.Flags().StringP("log-file", "", "", "Log file path for watch events and responses")
	watchCmd.Flags().StringP("only-extensions", "", "", "Comma-separated list of file extensions to watch (e.g., 'go,js,py')")
	watchCmd.Flags().StringP("ignore", "", "", "Comma-separated list of patterns to ignore")

	// Mark on-change as required
	watchCmd.MarkFlagRequired("on-change")
}