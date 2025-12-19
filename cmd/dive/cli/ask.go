package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/internal/random"
	"github.com/deepnoodle-ai/wonton/cli"
)

func runAsk(ctx context.Context, message, systemPrompt, goal, instructions, threadID, configFlag, agentFlag string, noConfig bool, tools []dive.Tool) error {
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
		threadID = random.Integer()
	}
	if err := chatMessage(ctx, message, chatAgent, threadID); err != nil {
		return err
	}
	if err := saveRecentThreadID(threadID); err != nil {
		return fmt.Errorf("error saving recent thread: %v", err)
	}
	return nil
}

func registerAskCommand(app *cli.App) {
	app.Command("ask").
		Description("Ask an agent a question").
		Args("message").
		Flags(
			cli.String("system", "").Help("System prompt for the agent"),
			cli.String("goal", "").Help("Goal for the agent"),
			cli.String("instructions", "").Help("Instructions for the agent"),
			cli.String("tools", "").Help("Comma-separated list of tools to use for the agent"),
			cli.String("thread", "").Help("Name of the thread to use for the agent"),
			cli.String("config", "").Help("Path to configuration file or directory"),
			cli.String("agent", "").Help("Name of the agent to use from configuration"),
			cli.Bool("no-config", "").Help("Disable automatic configuration loading"),
		).
		Run(func(ctx *cli.Context) error {
			parseGlobalFlags(ctx)

			message := ctx.Arg(0)
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

			if err := runAsk(ctx.Context(), message, systemPrompt, goal, instructions, thread, configFlag, agentFlag, noConfig, tools); err != nil {
				return cli.Errorf("%v", err)
			}
			return nil
		})
}
