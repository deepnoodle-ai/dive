package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/spf13/cobra"
)

func chatMessage(ctx context.Context, message string, agent dive.Agent, threadID string) error {
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

	if err := saveRecentThreadID(actualThreadID); err != nil {
		return fmt.Errorf("error saving recent thread: %v", err)
	}
	return nil
}

func runChat(systemPrompt, goal, instructions, threadID, configFlag, agentFlag string, noConfig bool, tools []dive.Tool) error {
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

	fmt.Println(boldStyle.Sprint("Chat Session"))
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(boldStyle.Sprint("You: "))
		if !scanner.Scan() {
			break
		}
		userInput := scanner.Text()

		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "quit" {
			fmt.Println()
			fmt.Println("Goodbye!")
			break
		}
		if strings.TrimSpace(userInput) == "" {
			continue
		}
		fmt.Println()
		fmt.Print(boldStyle.Sprintf("%s: ", chatAgent.Name()))

		if err := chatMessage(ctx, userInput, chatAgent, threadID); err != nil {
			return fmt.Errorf("error processing message: %v", err)
		}
		fmt.Println()
	}
	return nil
}

var chatCmd = &cobra.Command{
	Use:   "chat [message]",
	Short: "Start an interactive chat with an agent.",
	Long:  "Start an interactive chat with an agent.",
	Args:  cobra.ExactArgs(0),
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
		if err := runChat(systemPrompt, goal, instructions, thread, configFlag, agentFlag, noConfig, tools); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)

	chatCmd.Flags().StringP("system", "", "", "System prompt for the agent")
	chatCmd.Flags().StringP("goal", "", "", "Goal for the agent")
	chatCmd.Flags().StringP("instructions", "", "", "Instructions for the agent")
	chatCmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the agent")
	chatCmd.Flags().StringP("thread", "", "", "Name of the thread to use for the agent")
	chatCmd.Flags().StringP("config", "", "", "Path to configuration file or directory")
	chatCmd.Flags().StringP("agent", "", "", "Name of the agent to use from configuration")
	chatCmd.Flags().BoolP("no-config", "", false, "Disable automatic configuration loading")
}
