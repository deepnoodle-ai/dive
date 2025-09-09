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
	"github.com/deepnoodle-ai/dive/threads"
	"github.com/spf13/cobra"
)

func runAsk(message, systemPrompt, goal, instructions, threadID string, tools []dive.Tool) error {
	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString("warn"))

	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return fmt.Errorf("error getting threads directory: %v", err)
	}
	threadRepo := threads.NewDiskRepository(threadsDir)

	chatAgent, err := agent.New(agent.Options{
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
	if err != nil {
		return fmt.Errorf("error creating agent: %v", err)
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
		var tools []dive.Tool
		if toolsSpec != "" {
			tools, err = initializeTools(strings.Split(toolsSpec, ","))
			if err != nil {
				fmt.Println(errorStyle.Sprintf("Failed to initialize tools: %s", err))
				os.Exit(1)
			}
		}
		message := args[0]
		if err := runAsk(message, systemPrompt, goal, instructions, thread, tools); err != nil {
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
}
