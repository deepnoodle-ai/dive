package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/spf13/cobra"
)

func runSingleMessage(message, instructions, agentName, threadID string, tools []dive.Tool) error {
	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString("warn"))

	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	modelSettings := &agent.ModelSettings{}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return fmt.Errorf("error getting dive threads directory: %v", err)
	}
	threadRepo := agent.NewDiskThreadRepository(threadsDir)

	chatAgent, err := agent.New(agent.Options{
		Name:             agentName,
		Instructions:     instructions,
		Model:            model,
		Logger:           logger,
		Tools:            tools,
		ThreadRepository: threadRepo,
		ModelSettings:    modelSettings,
		Confirmer:        confirmer,
	})
	if err != nil {
		return fmt.Errorf("error creating agent: %v", err)
	}
	// Generate a thread ID if none was provided
	actualThreadID := threadID
	if actualThreadID == "" {
		actualThreadID = dive.NewID()
	}

	err = chatMessage(ctx, message, chatAgent, actualThreadID, false)
	if err != nil {
		return err
	}

	// Save the thread ID for future use
	if err := saveRecentThreadID(actualThreadID); err != nil {
		// Don't fail the whole operation if we can't save the thread ID
		fmt.Fprintf(os.Stderr, "Warning: Failed to save recent thread ID: %v\n", err)
	}

	return nil
}

var askCmd = &cobra.Command{
	Use:   "ask [message]",
	Short: "Ask an AI agent a question and show the response",
	Long:  "Ask an AI agent a question and show the response",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		systemPrompt, err := cmd.Flags().GetString("system")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		agentName, err := cmd.Flags().GetString("agent-name")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		thread, err := cmd.Flags().GetString("thread")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		var tools []dive.Tool
		toolsStr, err := cmd.Flags().GetString("tools")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if toolsStr != "" {
			toolNames := strings.Split(toolsStr, ",")
			for _, toolName := range toolNames {
				tool, err := config.InitializeToolByName(toolName, nil)
				if err != nil {
					fmt.Println(errorStyle.Sprintf("Failed to initialize tool: %s", err))
					os.Exit(1)
				}
				tools = append(tools, tool)
			}
		}

		message := args[0]
		if err := runSingleMessage(message, systemPrompt, agentName, thread, tools); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(askCmd)

	askCmd.Flags().StringP("agent-name", "", "Assistant", "Name of the agent")
	askCmd.Flags().StringP("system", "", "", "System prompt for the agent")
	askCmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the agent")
	askCmd.Flags().StringP("thread", "t", "", "Name of the thread to use for the agent")
}
