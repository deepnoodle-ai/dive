package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/spf13/cobra"
)

func runAsk(message, systemPrompt, goal, instructions, threadID, configFlag, agentFlag string, noConfig bool, tools []dive.Tool) error {
	ctx := context.Background()

	// Try to discover and load configuration
	configResult, err := discoverConfiguration(ctx, configFlag, noConfig, agentFlag)
	if err != nil {
		return fmt.Errorf("error loading configuration: %v", err)
	}
	var chatAgent dive.Agent

	if configResult != nil {
		// Report configuration usage
		reportConfigurationUsage(configResult.SourcePath, configResult.AgentName)

		// Use config-based agent with potential flag overrides
		if systemPrompt != "" || goal != "" || instructions != "" || len(tools) > 0 {
			// Apply flag overrides
			chatAgent, err = applyFlagOverrides(configResult.SelectedAgent, systemPrompt, goal, instructions, tools)
			if err != nil {
				return fmt.Errorf("error applying flag overrides: %v", err)
			}
		} else {
			// Use config agent as-is
			chatAgent = configResult.SelectedAgent
		}
	} else {
		// No configuration found, use traditional flag-based approach
		chatAgent, err = createAgentFromFlags(systemPrompt, goal, instructions, tools)
		if err != nil {
			return fmt.Errorf("error creating agent: %v", err)
		}
	}

	if threadID == "" {
		threadID = dive.NewID()
	}
	if err := chatMessage(ctx, message, chatAgent, threadID); err != nil {
		return err
	}
	if err := saveRecentThreadID(threadID); err != nil {
		return fmt.Errorf("error saving recent thread: %v", err)
	}
	return nil
}

var askCmd = &cobra.Command{
	Use:   "ask [message]",
	Short: "Ask an agent a question",
	Long:  "Ask an agent a question",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		systemPrompt, err := cmd.Flags().GetString("system")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		goal, err := cmd.Flags().GetString("goal")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		instructions, err := cmd.Flags().GetString("instructions")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		thread, err := cmd.Flags().GetString("thread")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		toolsSpec, err := cmd.Flags().GetString("tools")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		configFlag, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		agentFlag, err := cmd.Flags().GetString("agent")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		noConfig, err := cmd.Flags().GetBool("no-config")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		var tools []dive.Tool
		if toolsSpec != "" {
			tools, err = initializeTools(strings.Split(toolsSpec, ","))
			if err != nil {
				fmt.Println(errorStyle.Sprintf("Failed to initialize tools: %s", err))
				os.Exit(1)
			}
		}
		message := args[0]
		if err := runAsk(message, systemPrompt, goal, instructions, thread, configFlag, agentFlag, noConfig, tools); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(askCmd)

	askCmd.Flags().StringP("system", "", "", "System prompt for the agent")
	askCmd.Flags().StringP("goal", "", "", "Goal for the agent")
	askCmd.Flags().StringP("instructions", "", "", "Instructions for the agent")
	askCmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the agent")
	askCmd.Flags().StringP("thread", "", "", "Name of the thread to use for the agent")
	askCmd.Flags().StringP("config", "", "", "Path to configuration file or directory")
	askCmd.Flags().StringP("agent", "", "", "Name of the agent to use from configuration")
	askCmd.Flags().BoolP("no-config", "", false, "Disable automatic configuration loading")
}
