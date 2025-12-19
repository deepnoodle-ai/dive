package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/internal/random"
	"github.com/deepnoodle-ai/wonton/cli"
)

func chatMessage(ctx context.Context, message string, agent dive.Agent, threadID string) error {
	// Generate a thread ID if none was provided
	actualThreadID := threadID
	if actualThreadID == "" {
		actualThreadID = random.Integer()
	}

	var inToolUse bool
	toolUseAccum := ""
	toolName := ""
	toolID := ""

	_, err := agent.CreateResponse(ctx,
		dive.WithInput(message),
		dive.WithThreadID(actualThreadID),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent {
				payload := item.Event
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
			}
			return nil
		}),
	)

	if err != nil {
		return fmt.Errorf("error generating response: %v", err)
	}

	fmt.Println()

	if err := saveRecentThreadID(actualThreadID); err != nil {
		return fmt.Errorf("error saving recent thread: %v", err)
	}
	return nil
}

func runChat(ctx context.Context, systemPrompt, goal, instructions, threadID, configFlag, agentFlag string, noConfig bool, tools []dive.Tool) error {
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

func chatHandler(ctx *cli.Context) error {
	parseGlobalFlags(ctx)

	systemPrompt := ctx.String("system")
	goal := ctx.String("goal")
	instructions := ctx.String("instructions")
	thread := ctx.String("thread")
	toolsSpec := ctx.String("tools")
	configFlag := ctx.String("config")
	agentFlag := ctx.String("agent")
	noConfig := ctx.Bool("no-config")

	var tools []dive.Tool
	var err error
	if toolsSpec != "" {
		tools, err = initializeTools(strings.Split(toolsSpec, ","))
		if err != nil {
			return cli.Errorf("failed to initialize tools: %s", err)
		}
	}

	if err := runChat(ctx.Context(), systemPrompt, goal, instructions, thread, configFlag, agentFlag, noConfig, tools); err != nil {
		return cli.Errorf("%v", err)
	}
	return nil
}

func chatFlags() []cli.Flag {
	return []cli.Flag{
		cli.String("system", "").Help("System prompt for the agent"),
		cli.String("goal", "").Help("Goal for the agent"),
		cli.String("instructions", "").Help("Instructions for the agent"),
		cli.String("tools", "").Help("Comma-separated list of tools to use for the agent"),
		cli.String("thread", "").Help("Name of the thread to use for the agent"),
		cli.String("config", "").Help("Path to configuration file or directory"),
		cli.String("agent", "").Help("Name of the agent to use from configuration"),
		cli.Bool("no-config", "").Help("Disable automatic configuration loading"),
	}
}

func registerMainCommand(app *cli.App) {
	app.Main().
		NoArgs().
		Flags(chatFlags()...).
		Run(chatHandler)
}

func registerChatCommand(app *cli.App) {
	app.Command("chat").
		Description("Start an interactive chat with an agent").
		NoArgs().
		Flags(chatFlags()...).
		Run(chatHandler)
}
