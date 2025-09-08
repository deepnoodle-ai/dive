package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	boldStyle     = color.New(color.Bold)
	successStyle  = color.New(color.FgGreen)
	errorStyle    = color.New(color.FgRed)
	yellowStyle   = color.New(color.FgYellow)
	thinkingStyle = color.New(color.FgMagenta)
)

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

func chatMessage(ctx context.Context, message string, agent dive.Agent, threadID string, showName bool) error {
	if showName {
		fmt.Print(boldStyle.Sprintf("%s: ", agent.Name()))
	}

	// Generate a thread ID if none was provided
	actualThreadID := threadID
	if actualThreadID == "" {
		actualThreadID = dive.NewID()
	}

	stream, err := agent.StreamResponse(ctx, dive.WithInput(message), dive.WithThreadID(actualThreadID))
	if err != nil {
		return fmt.Errorf("error generating response: %v", err)
	}
	defer stream.Close()

	var inToolUse, incremental bool
	toolUseAccum := ""
	toolName := ""
	toolID := ""

	for stream.Next(ctx) {
		event := stream.Event()
		switch event.Type {
		case dive.EventTypeLLMEvent:
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
				if delta.PartialJSON != "" {
					if !inToolUse {
						inToolUse = true
						fmt.Print("\n----\n")
					}
					toolUseAccum += delta.PartialJSON
				} else if delta.Text != "" {
					if inToolUse {
						fmt.Println(yellowStyle.Sprint(toolName), yellowStyle.Sprint(toolID))
						fmt.Println(yellowStyle.Sprint(toolUseAccum))
						fmt.Print("----\n")
						inToolUse = false
						toolUseAccum = ""
					}
					fmt.Print(successStyle.Sprint(delta.Text))
				} else if delta.Thinking != "" {
					fmt.Print(thinkingStyle.Sprint(delta.Thinking))
				}
			}
		case dive.EventTypeResponseCompleted:
			if !incremental {
				text := strings.TrimSpace(event.Response.OutputText())
				fmt.Println(successStyle.Sprint(text))
			}
		}
	}

	fmt.Println()

	// Save the thread ID for future use
	if err := saveRecentThreadID(actualThreadID); err != nil {
		// Don't fail the whole operation if we can't save the thread ID
		fmt.Fprintf(os.Stderr, "Warning: Failed to save recent thread ID: %v\n", err)
	}

	return nil
}

func runChat(instructions, agentName, threadID string, tools []dive.Tool) error {
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

	fmt.Println(boldStyle.Sprint("Chat Session"))
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(boldStyle.Sprint("You: "))
		if !scanner.Scan() {
			break
		}
		userInput := scanner.Text()

		if strings.ToLower(userInput) == "exit" ||
			strings.ToLower(userInput) == "quit" {
			fmt.Println()
			fmt.Println("Goodbye!")
			break
		}
		if strings.TrimSpace(userInput) == "" {
			continue
		}
		fmt.Println()

		if err := chatMessage(ctx, userInput, chatAgent, "", true); err != nil {
			return fmt.Errorf("error processing message: %v", err)
		}
		fmt.Println()
	}
	return nil
}

var chatCmd = &cobra.Command{
	Use:   "chat [message]",
	Short: "Start an interactive chat with an AI agent or send a single message",
	Long:  "Start an interactive chat with an AI agent. If a message is provided as an argument, send that message, show the response, and exit. Use --session to persist conversation history to a file for resuming later.",
	Args:  cobra.MaximumNArgs(1),
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

		// If a message argument is provided, send it and exit
		if len(args) > 0 {
			message := args[0]
			if err := runSingleMessage(message, systemPrompt, agentName, thread, tools); err != nil {
				fmt.Println(errorStyle.Sprint(err))
				os.Exit(1)
			}
			return
		}

		// Otherwise, start interactive chat
		if err := runChat(systemPrompt, agentName, thread, tools); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)

	chatCmd.Flags().StringP("agent-name", "", "Assistant", "Name of the chat agent")
	chatCmd.Flags().StringP("system", "", "", "System prompt for the chat agent")
	chatCmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the chat agent")
	chatCmd.Flags().StringP("thread", "", "", "ID or name of the thread to use for the chat agent")
}
