package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/agent"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/slogger"
	"github.com/spf13/cobra"
)

var (
	boldStyle    = color.New(color.Bold)
	successStyle = color.New(color.FgGreen)
	errorStyle   = color.New(color.FgRed)
)

func chatMessage(ctx context.Context, message string, agent dive.Agent) error {
	fmt.Print(boldStyle.Sprintf("%s: ", agent.Name()))

	iterator, err := agent.Stream(ctx,
		llm.NewUserMessage(message),
		dive.WithThreadID("chat"),
	)
	if err != nil {
		return fmt.Errorf("error generating response: %v", err)
	}
	defer iterator.Close()

	var inToolUse bool
	toolUseAccum := ""
	toolName := ""
	toolID := ""

	for iterator.Next(ctx) {
		event := iterator.Event()
		switch payload := event.Payload.(type) {
		case *llm.Event:
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
						fmt.Println("\n----")
					}
					toolUseAccum += delta.PartialJSON
				} else if delta.Text != "" {
					if inToolUse {
						fmt.Println(toolName, toolID)
						fmt.Println(toolUseAccum)
						fmt.Println("----")
						inToolUse = false
						toolUseAccum = ""
					}
					fmt.Print(successStyle.Sprint(delta.Text))
				}
			}
		}
	}

	fmt.Println()
	return nil
}

func getProvider() (llm.LLM, error) {
	switch llmProvider {
	case "", "anthropic":
		return anthropic.New(anthropic.WithModel(llmModel)), nil
	case "openai":
		return openai.New(openai.WithModel(llmModel)), nil
	case "groq":
		return groq.New(groq.WithModel(llmModel)), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", llmProvider)
	}
}

var DefaultChatSystemPrompt = `You are a helpful AI assistant. You aim to be direct, clear, and helpful in your responses.`

func runChat(systemPrompt, agentName string) error {
	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString("warn"))

	llmProvider, err := getProvider()
	if err != nil {
		return fmt.Errorf("error getting provider: %v", err)
	}
	chatAgent, err := agent.NewAgent(agent.AgentOptions{
		Name:         agentName,
		Backstory:    systemPrompt,
		LLM:          llmProvider,
		CacheControl: "ephemeral",
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("error creating agent: %v", err)
	}
	if err := chatAgent.Start(ctx); err != nil {
		return fmt.Errorf("error starting agent: %v", err)
	}
	defer chatAgent.Stop(ctx)

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
		if systemPrompt == "" {
			systemPrompt = DefaultChatSystemPrompt
		}
		agentName, err := cmd.Flags().GetString("agent-name")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if err := runChat(systemPrompt, agentName); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)

	chatCmd.Flags().StringP("agent-name", "n", "Assistant", "Name of the chat agent")
	chatCmd.Flags().StringP("system-prompt", "s", "", "System prompt for the chat agent")
}
