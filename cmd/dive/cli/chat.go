package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/slogger"
	"github.com/getstingrai/dive/teamconf"
	"github.com/spf13/cobra"
)

var (
	boldStyle    = color.New(color.Bold)
	successStyle = color.New(color.FgGreen)
	errorStyle   = color.New(color.FgRed)
	infoStyle    = color.New(color.FgBlue)
)

func chatMessage(ctx context.Context, message string, agent dive.Agent) (string, error) {
	fmt.Print(boldStyle.Sprint("Assistant: "))

	stream, err := agent.Stream(ctx,
		llm.NewUserMessage(message),
		dive.WithThreadID("chat"),
	)
	if err != nil {
		return "", fmt.Errorf("error generating response: %v", err)
	}
	defer stream.Close()

	for event := range stream.Channel() {
		switch event.Type {
		case "llm.event":
			switch event.LLMEvent.Type {
			case llm.EventContentBlockDelta:
				delta := event.LLMEvent.Delta
				if delta.Text != "" {
					fmt.Print(successStyle.Sprint(delta.Text))
				} else if delta.PartialJSON != "" {
					fmt.Print(infoStyle.Sprint(delta.PartialJSON))
				}
			}
		}
	}

	fmt.Println()
	return "", nil
}

func getChatAgent(team dive.Team) (dive.Agent, bool) {
	agents := team.Agents()
	// Chat with the supervisor if there is one
	for _, agent := range agents {
		if teamAgent, ok := agent.(dive.TeamAgent); ok && teamAgent.IsSupervisor() {
			return agent, true
		}
	}
	// Otherwise, just pick the first agent
	if len(agents) > 0 {
		return agents[0], true
	}
	return nil, false
}

func runChatSession(teamConfPath string) error {
	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString("warn"))

	team, err := teamconf.TeamFromFile(teamConfPath, teamconf.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("error loading team: %v", err)
	}

	chatAgent, ok := getChatAgent(team)
	if !ok {
		return fmt.Errorf("no agent available to chat with")
	}

	if err := team.Start(ctx); err != nil {
		return fmt.Errorf("error starting team: %v", err)
	}
	defer team.Stop(ctx)

	fmt.Println(boldStyle.Sprint("Dive Chat"))
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

		if _, err := chatMessage(ctx, userInput, chatAgent); err != nil {
			return fmt.Errorf("error processing message: %v", err)
		}

		fmt.Println()
	}
	return nil
}

// chatCmd represents the chat command
var chatCmd = &cobra.Command{
	Use:   "chat [file]",
	Short: "Start a chat session with a team",
	Long: `Start an interactive chat session with a team defined in an HCL file.
This allows you to ask questions and interact with the team's AI capabilities.

To exit the chat, type 'exit' or 'quit'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		return runChatSession(filePath)
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
