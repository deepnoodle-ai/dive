package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/slogger"
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

func chatMessage(ctx context.Context, message string, agent dive.Agent) error {
	fmt.Print(boldStyle.Sprintf("%s: ", agent.Name()))

	stream, err := agent.StreamResponse(ctx, dive.WithInput(message), dive.WithThreadID("cli-chat"))
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
		} else if event.Type == dive.EventTypeResponseCompleted {
			if !incremental {
				text := strings.TrimSpace(event.Response.OutputText())
				fmt.Println(successStyle.Sprint(text))
			}
		}
	}

	fmt.Println()
	return nil
}

func runChat(instructions, agentName string, reasoningBudget int, tools []dive.Tool) error {
	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString("warn"))

	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	modelSettings := &agent.ModelSettings{}
	if reasoningBudget > 0 {
		modelSettings.ReasoningBudget = &reasoningBudget
		maxTokens := 0
		if modelSettings.MaxTokens != nil {
			maxTokens = *modelSettings.MaxTokens
		}
		if reasoningBudget > maxTokens+4096 {
			newLimit := reasoningBudget + 4096
			modelSettings.MaxTokens = &newLimit
		}
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	chatAgent, err := agent.New(agent.Options{
		Name:             agentName,
		Instructions:     instructions,
		Model:            model,
		Logger:           logger,
		Tools:            tools,
		ThreadRepository: agent.NewMemoryThreadRepository(),
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

		if err := chatMessage(ctx, userInput, chatAgent); err != nil {
			return fmt.Errorf("error processing message: %v", err)
		}
		fmt.Println()
	}
	return nil
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat with an AI agent",
	Long:  "Start an interactive chat with an AI agent",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

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

		var reasoningBudget int
		if value, err := cmd.Flags().GetInt("reasoning-budget"); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		} else {
			reasoningBudget = value
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

		if err := runChat(systemPrompt, agentName, reasoningBudget, tools); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)

	chatCmd.Flags().StringP("agent-name", "", "Assistant", "Name of the chat agent")
	chatCmd.Flags().StringP("system-prompt", "", "", "System prompt for the chat agent")
	chatCmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the chat agent")
	chatCmd.Flags().IntP("reasoning-budget", "", 0, "Reasoning budget for the chat agent")
}
