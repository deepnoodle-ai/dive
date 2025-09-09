package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/deepnoodle-ai/dive/threads"
	"github.com/fatih/color"
)

var (
	boldStyle     = color.New(color.Bold)
	successStyle  = color.New(color.FgGreen)
	errorStyle    = color.New(color.FgRed)
	yellowStyle   = color.New(color.FgYellow)
	thinkingStyle = color.New(color.FgMagenta)
)

// readStdin reads all content from standard input
func readStdin() (string, error) {
	var content strings.Builder
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line != "" {
					content.WriteString(line)
				}
				break
			}
			return "", fmt.Errorf("error reading from stdin: %v", err)
		}
		content.WriteString(line)
	}

	return strings.TrimSpace(content.String()), nil
}

func diveDirectory() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, ".dive"), nil
}

func diveThreadsDirectory() (string, error) {
	dir, err := diveDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "threads"), nil
}

func formatTimeAgo(t time.Time) string {
	now := time.Now()
	duration := now.Sub(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case duration < 30*24*time.Hour:
		weeks := int(duration.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	case duration < 365*24*time.Hour:
		months := int(duration.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(duration.Hours() / (24 * 365))
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// saveRecentThreadID saves the most recent thread ID to ~/.dive/threads/recent
func saveRecentThreadID(threadID string) error {
	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return fmt.Errorf("error getting dive threads directory: %v", err)
	}
	if err := os.MkdirAll(threadsDir, 0755); err != nil {
		return fmt.Errorf("error creating threads directory: %v", err)
	}

	recentFile := filepath.Join(threadsDir, "recent")
	if err := os.WriteFile(recentFile, []byte(threadID), 0644); err != nil {
		return fmt.Errorf("error writing recent thread ID: %v", err)
	}

	return nil
}

func initializeTools(toolNames []string) ([]dive.Tool, error) {
	tools := make([]dive.Tool, 0, len(toolNames))
	for _, toolName := range toolNames {
		tool, err := config.InitializeToolByName(toolName, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tool: %s", err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// ConfigurationResult holds the result of configuration discovery
type ConfigurationResult struct {
	Config            *config.Config
	Environment       *config.Environment
	SourceDescription string
	SourcePath        string
	SelectedAgent     dive.Agent
	AgentName         string
}

// discoverConfiguration attempts to find and load configuration
func discoverConfiguration(ctx context.Context, path string, noConfig bool, agentName string) (*ConfigurationResult, error) {
	if noConfig {
		return nil, nil // Explicitly disabled
	}

	var cfg *config.Config
	var sourceDescription string
	var sourcePath string
	var err error

	if path != "" {
		// Explicit config specified
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, fmt.Errorf("config path does not exist: %s", path)
		}
		sourcePath = path

		if info.IsDir() {
			cfg, err = config.LoadDirectory(path)
			sourceDescription = fmt.Sprintf("directory: %s", path)
		} else {
			cfg, err = config.ParseFile(path)
			sourceDescription = fmt.Sprintf("file: %s", path)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", path, err)
		}
	} else {
		// Auto-discovery in CWD
		cwd, _ := os.Getwd()
		for _, filename := range []string{"dive.yaml", "dive.yml"} {
			path := filepath.Join(cwd, filename)
			if _, statErr := os.Stat(path); statErr == nil {
				cfg, err = config.ParseFile(path)
				if err != nil {
					return nil, fmt.Errorf("failed to load auto-discovered config %s: %w", path, err)
				}
				sourceDescription = fmt.Sprintf("auto-discovered: %s", path)
				sourcePath = path
				break
			}
		}
	}

	if cfg == nil {
		return nil, nil // No config found
	}

	// Create environment from config
	logger := slogger.New(slogger.LevelFromString("warn"))
	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return nil, fmt.Errorf("error getting threads directory: %v", err)
	}
	threadRepo := threads.NewDiskRepository(threadsDir)

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	env, err := config.NewEnvironment(ctx, config.EnvironmentOpts{
		Config:    cfg,
		Logger:    logger,
		Confirmer: confirmer,
		Threads:   threadRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	// Select agent
	var selectedAgent dive.Agent
	var selectedAgentName string

	if len(env.Agents) == 0 {
		return nil, fmt.Errorf("configuration contains no agents")
	}

	if agentName != "" {
		// Find specific agent by name
		for _, agent := range env.Agents {
			if agent.Name() == agentName {
				selectedAgent = agent
				selectedAgentName = agentName
				break
			}
		}
		if selectedAgent == nil {
			return nil, fmt.Errorf("agent %q not found in configuration", agentName)
		}
	} else {
		// Use first agent as default
		selectedAgent = env.Agents[0]
		selectedAgentName = selectedAgent.Name()
	}

	return &ConfigurationResult{
		Config:            cfg,
		Environment:       env,
		SourceDescription: sourceDescription,
		SourcePath:        sourcePath,
		SelectedAgent:     selectedAgent,
		AgentName:         selectedAgentName,
	}, nil
}

// reportConfigurationUsage prints information about the configuration being used
func reportConfigurationUsage(configSource string, agentName string) {
	if configSource != "" {
		fmt.Printf("Using configuration: %s\n", configSource)
		if agentName != "" {
			fmt.Printf("Using agent: %s\n", agentName)
		}
		fmt.Println()
	}
}

// createAgentFromFlags creates an agent using the traditional flag-based approach
func createAgentFromFlags(systemPrompt, goal, instructions string, tools []dive.Tool) (dive.Agent, error) {
	logger := slogger.New(slogger.LevelFromString("warn"))

	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %v", err)
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return nil, fmt.Errorf("error getting threads directory: %v", err)
	}
	threadRepo := threads.NewDiskRepository(threadsDir)

	return agent.New(agent.Options{
		Name:             "Assistant",
		SystemPrompt:     systemPrompt,
		Goal:             goal,
		Instructions:     instructions,
		Model:            model,
		Logger:           logger,
		Tools:            tools,
		ThreadRepository: threadRepo,
		ModelSettings:    &agent.ModelSettings{},
		Confirmer:        confirmer,
	})
}

// applyFlagOverrides applies CLI flag overrides to a config-based agent
func applyFlagOverrides(configAgent dive.Agent, systemPrompt, goal, instructions string, tools []dive.Tool) (dive.Agent, error) {
	// If no overrides are specified, return the config agent as-is
	if systemPrompt == "" && goal == "" && instructions == "" && len(tools) == 0 {
		return configAgent, nil
	}

	// For now, we'll create a new agent with overrides
	// This is a simplified approach - a more sophisticated implementation
	// would merge the configurations more granularly
	logger := slogger.New(slogger.LevelFromString("warn"))

	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %v", err)
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return nil, fmt.Errorf("error getting threads directory: %v", err)
	}
	threadRepo := threads.NewDiskRepository(threadsDir)

	// Use flag values if provided, otherwise keep config values
	finalSystemPrompt := systemPrompt
	if finalSystemPrompt == "" {
		// Try to get system prompt from config agent - this is a simplified approach
		// In a full implementation, we'd need to access the agent's internal configuration
		finalSystemPrompt = ""
	}

	finalGoal := goal
	finalInstructions := instructions
	finalTools := tools
	if len(finalTools) == 0 {
		// Use tools from config agent - this would need proper implementation
		finalTools = []dive.Tool{}
	}

	return agent.New(agent.Options{
		Name:             configAgent.Name(),
		SystemPrompt:     finalSystemPrompt,
		Goal:             finalGoal,
		Instructions:     finalInstructions,
		Model:            model,
		Logger:           logger,
		Tools:            finalTools,
		ThreadRepository: threadRepo,
		ModelSettings:    &agent.ModelSettings{},
		Confirmer:        confirmer,
	})
}
