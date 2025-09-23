package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/log"
	"github.com/deepnoodle-ai/dive/threads"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

// WatchOptions holds configuration for the watch command
type WatchOptions struct {
	Patterns        []string
	OnChange        string
	Recursive       bool
	Debounce        time.Duration
	BatchTimer      time.Duration
	SystemPrompt    string
	AgentName       string
	Tools           []string
	ReasoningBudget int
	ExitOnError     bool
	LogFile         string
	OnlyExtensions  []string
	IgnorePatterns  []string
}

// FileChange represents a file change event
type FileChange struct {
	FilePath  string
	Operation string
	Timestamp time.Time
}

// FileWatcher manages file system watching and LLM action triggering
type FileWatcher struct {
	options     WatchOptions
	watcher     *fsnotify.Watcher
	agent       dive.Agent
	logger      log.Logger
	debouncer   map[string]time.Time
	changeBatch []FileChange
	batchMutex  sync.Mutex
	batchTimer  *time.Timer
	threadID    string
}

// NewFileWatcher creates a new file watcher instance
func NewFileWatcher(options WatchOptions) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	logger := log.New(getLogLevel())

	// Generate a random thread ID for this watch session
	threadID := dive.NewID()

	// Create agent for LLM actions
	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %v", err)
	}

	// Always include read_file and list_directory tools
	requiredTools := []string{"read_file", "list_directory"}
	allToolNames := make(map[string]bool)
	for _, toolName := range requiredTools {
		allToolNames[toolName] = true
	}
	for _, toolName := range options.Tools {
		allToolNames[toolName] = true
	}

	var tools []dive.Tool
	for toolName := range allToolNames {
		tool, err := config.InitializeToolByName(toolName, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tool %s: %w", toolName, err)
		}
		tools = append(tools, tool)
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
		ThreadRepository: threads.NewMemoryRepository(),
		ModelSettings:    modelSettings,
		Confirmer:        confirmer,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating agent: %v", err)
	}

	return &FileWatcher{
		options:     options,
		watcher:     watcher,
		agent:       watchAgent,
		logger:      logger,
		debouncer:   make(map[string]time.Time),
		changeBatch: make([]FileChange, 0),
		threadID:    threadID,
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
	fmt.Printf("Agent: %s (Thread ID: %s)\n", fw.options.AgentName, fw.threadID)
	fmt.Printf("Batch timer: %v\n", fw.options.BatchTimer)
	fmt.Println("Press Ctrl+C to stop...")
	fmt.Println()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n👋 File watcher stopped")
			return nil
		case event, ok := <-fw.watcher.Events:
			// fmt.Println("Event:", event)
			if !ok {
				return nil
			}
			if err := fw.handleFileEvent(ctx, event); err != nil {
				fw.logger.Error("Error handling file event", "error", err, "file", event.Name)
			}
		case err, ok := <-fw.watcher.Errors:
			fmt.Println("Error:", err)
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

// handleFileEvent processes a file system event and adds it to the batch
func (fw *FileWatcher) handleFileEvent(ctx context.Context, event fsnotify.Event) error {
	// Only handle write and create events - filter early to avoid unnecessary processing
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
		fw.logger.Debug("Ignoring non-write/create event", "file", event.Name, "op", event.Op.String())
		return nil
	}

	// Check if the file matches any of our patterns
	if !fw.matchesPatterns(event.Name) {
		fw.logger.Debug("File does not match patterns", "file", event.Name)
		return nil
	}

	// Debounce rapid file changes
	now := time.Now()
	if lastTime, exists := fw.debouncer[event.Name]; exists {
		timeSinceLastEvent := now.Sub(lastTime)
		if timeSinceLastEvent < fw.options.Debounce {
			fw.logger.Debug("Event debounced", "file", event.Name, "time_since_last", timeSinceLastEvent)
			return nil
		}
	}
	fw.debouncer[event.Name] = now

	fw.addToBatch(ctx, event.Name, event.Op.String())
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

// addToBatch adds a file change to the current batch and manages the timer
func (fw *FileWatcher) addToBatch(ctx context.Context, filePath, operation string) {
	fw.batchMutex.Lock()
	defer fw.batchMutex.Unlock()

	// Add the change to the batch
	change := FileChange{
		FilePath:  filePath,
		Operation: operation,
		Timestamp: time.Now(),
	}
	fw.changeBatch = append(fw.changeBatch, change)

	// Reset or start the batch timer
	if fw.batchTimer != nil {
		fw.batchTimer.Stop()
	}
	fw.batchTimer = time.AfterFunc(fw.options.BatchTimer, func() {
		fw.processBatch(ctx)
	})
}

// processBatch processes all changes in the current batch
func (fw *FileWatcher) processBatch(ctx context.Context) {
	fw.batchMutex.Lock()
	batch := make([]FileChange, len(fw.changeBatch))
	copy(batch, fw.changeBatch)
	fw.changeBatch = fw.changeBatch[:0] // Clear the batch
	fw.batchMutex.Unlock()

	if len(batch) == 0 {
		return
	}

	fw.logger.Info("Processing batch of file changes", "count", len(batch))
	if err := fw.triggerLLMAction(ctx, batch); err != nil {
		fw.logger.Error("Error processing batch", "error", err)
	}
}

// triggerLLMAction invokes the LLM agent with the batch of file changes
func (fw *FileWatcher) triggerLLMAction(ctx context.Context, changes []FileChange) error {
	// Log to file if specified
	if fw.options.LogFile != "" {
		for _, change := range changes {
			fw.logToFile(fmt.Sprintf("File changed: %s (%s)", change.FilePath, change.Operation))
		}
	}

	// Build message with all file changes
	var changeMessage strings.Builder
	changeMessage.WriteString(fmt.Sprintf("File changes detected (%d files):\n\n", len(changes)))

	for i, change := range changes {
		changeMessage.WriteString(fmt.Sprintf("## Change %d: %s %s\n", i+1, change.Operation, change.FilePath))
		changeMessage.WriteString(fmt.Sprintf("Timestamp: %s\n", change.Timestamp.Format(time.RFC3339)))
		// Get file stats for additional context
		fileInfo, err := os.Stat(change.FilePath)
		if err == nil {
			changeMessage.WriteString(fmt.Sprintf("Last modified: %s\n\n", fileInfo.ModTime().Format(time.RFC3339)))
			changeMessage.WriteString(fmt.Sprintf("File size: %d bytes\n", fileInfo.Size()))
		}
	}

	message := &llm.Message{
		Role: llm.User,
		Content: []llm.Content{
			llm.NewTextContent(changeMessage.String()),
			llm.NewTextContent(fw.options.OnChange),
		},
	}

	// Generate response from agent using persistent thread ID
	response, err := fw.agent.CreateResponse(ctx, dive.WithThreadID(fw.threadID), dive.WithMessages(message))
	if err != nil {
		errorMsg := fmt.Sprintf("error generating response: %v", err)
		fw.logger.Error("LLM response error", "error", err, "changes", len(changes))
		if fw.options.LogFile != "" {
			fw.logToFile(fmt.Sprintf("ERROR: %s", errorMsg))
		}
		if fw.options.ExitOnError {
			return fmt.Errorf("%s", errorMsg)
		}
		fmt.Println(errorStyle.Sprint(errorMsg))
		return nil
	}

	// Extract response text
	responseText := strings.TrimSpace(response.OutputText())
	fmt.Println(successStyle.Sprint(responseText))

	// Log response to file if specified
	if fw.options.LogFile != "" && responseText != "" {
		fw.logToFile(fmt.Sprintf("Response for batch:\n%s", responseText))
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
Changes are batched together and processed as a group with configurable timing.
The agent automatically has access to read_file and list_directory tools and can decide
whether to read file contents based on the task requirements.

Examples:
  # Basic file watching with default 1-second batching
  dive watch src/*.go --on-change "Lint and suggest fixes"
  
  # Custom batch timing (500ms)
  dive watch . --recursive --batch-timer 500 --on-change "Review changes for security issues"
  
  # With additional tools and filtering
  dive watch "**/*.py" --on-change "Check for PEP8 compliance" --tools "web_search"
  
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

		batchTimerMs, err := cmd.Flags().GetInt("batch-timer")
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
			BatchTimer:      time.Duration(batchTimerMs) * time.Millisecond,
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
	watchCmd.Flags().IntP("batch-timer", "", 1000, "Batch timer in milliseconds to group file changes")
	watchCmd.Flags().StringP("system-prompt", "", "", "System prompt for the watch agent")
	watchCmd.Flags().StringP("agent-name", "", "FileWatcher", "Name of the watch agent")
	watchCmd.Flags().StringP("tools", "", "", "Comma-separated list of additional tools to use (read_file and list_directory are always included)")
	watchCmd.Flags().IntP("reasoning-budget", "", 0, "Reasoning budget for the watch agent")
	watchCmd.Flags().BoolP("exit-on-error", "", false, "Exit on LLM errors (useful for CI/CD)")
	watchCmd.Flags().StringP("log-file", "", "", "Log file path for watch events and responses")
	watchCmd.Flags().StringP("only-extensions", "", "", "Comma-separated list of file extensions to watch (e.g., 'go,js,py')")
	watchCmd.Flags().StringP("ignore", "", "", "Comma-separated list of patterns to ignore")

	// Mark on-change as required
	watchCmd.MarkFlagRequired("on-change")
}
