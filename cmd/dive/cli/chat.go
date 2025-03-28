package cli

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
	"github.com/fatih/color"
	"github.com/mendableai/firecrawl-go"
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

	iterator, err := agent.Chat(ctx, llm.NewSingleUserMessage(message), dive.WithThreadID("chat"))
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
			if payload.Type == llm.EventContentBlockStop {
				fmt.Printf("\n\n")
			}
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
						fmt.Println("\n\n----")
					}
					toolUseAccum += delta.PartialJSON
				} else if delta.Text != "" {
					if inToolUse {
						fmt.Println(yellowStyle.Sprint(toolName), yellowStyle.Sprint(toolID))
						fmt.Println(yellowStyle.Sprint(toolUseAccum))
						fmt.Println("----")
						fmt.Println()
						inToolUse = false
						toolUseAccum = ""
					}
					fmt.Print(successStyle.Sprint(delta.Text))
				} else if delta.Thinking != "" {
					fmt.Print(thinkingStyle.Sprint(delta.Thinking))
				}
			}
		}
	}

	fmt.Println()
	return nil
}

var DefaultChatBackstory = `You are a helpful AI assistant. You aim to be direct, clear, and helpful in your responses.`

func runChat(backstory, agentName string, reasoningBudget int) error {
	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString("warn"))

	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	var theTools []llm.Tool

	if key := os.Getenv("FIRECRAWL_API_KEY"); key != "" {
		app, err := firecrawl.NewFirecrawlApp(key, "")
		if err != nil {
			log.Fatal(err)
		}
		scraper := toolkit.NewFirecrawlScrapeTool(toolkit.FirecrawlScrapeToolOptions{
			App: app,
		})
		theTools = append(theTools, scraper)
	}

	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			log.Fatal(err)
		}
		theTools = append(theTools, toolkit.NewGoogleSearch(googleClient))
	}

	modelSettings := &agent.ModelSettings{}
	if reasoningBudget > 0 {
		modelSettings.ReasoningBudget = &reasoningBudget
		if reasoningBudget > modelSettings.MaxTokens+4096 {
			modelSettings.MaxTokens = reasoningBudget + 4096
		}
	}

	chatAgent, err := agent.New(agent.Options{
		Name:             agentName,
		Backstory:        backstory,
		Model:            model,
		Logger:           logger,
		Tools:            theTools,
		ThreadRepository: agent.NewMemoryThreadRepository(),
		AutoStart:        true,
		ModelSettings:    modelSettings,
	})
	if err != nil {
		return fmt.Errorf("error creating agent: %v", err)
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
			systemPrompt = DefaultChatBackstory
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

		if err := runChat(systemPrompt, agentName, reasoningBudget); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)

	chatCmd.Flags().StringP("agent-name", "", "Assistant", "Name of the chat agent")
	chatCmd.Flags().StringP("system-prompt", "", "", "System prompt for the chat agent")
	chatCmd.Flags().IntP("reasoning-budget", "", 0, "Reasoning budget for the chat agent")
}
