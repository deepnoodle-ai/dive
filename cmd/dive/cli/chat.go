package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/slogger"
	"github.com/spf13/cobra"
)

// Define chat-specific styles
var (
	chatTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0D6EFD")).
			Bold(true)

	botMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#198754"))

	// Note: errorStyle is already defined in run.go
)

// loadTeam loads a team from an HCL file
func loadTeam(ctx context.Context, filePath string) (dive.Team, error) {
	logger := slogger.New(slogger.LevelFromString("debug"))
	team, _, err := dive.LoadHCLTeam(ctx, filePath, nil, logger)
	if err != nil {
		return nil, fmt.Errorf("error loading team: %v", err)
	}
	return team, nil
}

// processMessage sends a message to the team and returns the response
func processMessage(ctx context.Context, message string, team dive.Team) string {
	// Create a user message for the LLM
	userMessage := llm.NewUserMessage(message)

	// If there's a supervisor agent, use it first
	for _, agent := range team.Agents() {
		if teamAgent, ok := agent.(dive.TeamAgent); ok && teamAgent.IsSupervisor() {
			response, err := agent.Generate(ctx, userMessage)
			if err != nil {
				return fmt.Sprintf("Error generating response: %v", err)
			}
			return response.Message().Text()
		}
	}

	// If no supervisor found, use the first agent
	if len(team.Agents()) > 0 {
		agent := team.Agents()[0]
		response, err := agent.Generate(ctx, userMessage)
		if err != nil {
			return fmt.Sprintf("Error generating response: %v", err)
		}
		return response.Message().Text()
	}

	// Fallback if no agents are available
	return "No agents are available to respond to your message."
}

// runChatSession starts an interactive chat session
func runChatSession(filePath string) error {
	ctx := context.Background()

	// Print welcome header
	fmt.Println(chatTitleStyle.Render(" Dive Team Chat "))
	fmt.Println()

	// Load the team
	fmt.Println("Loading team from", filePath, "...")
	team, err := loadTeam(ctx, filePath)
	if err != nil {
		return err
	}

	// Print welcome message
	welcomeMsg := fmt.Sprintf("Welcome to the Dive chat! I'm your assistant for the team defined in %s. How can I help you today?", filePath)
	fmt.Println(botMsgStyle.Render("Assistant: " + welcomeMsg))
	fmt.Println()

	// Create a scanner for user input
	scanner := bufio.NewScanner(os.Stdin)

	// Start the chat loop
	for {
		// Print prompt
		fmt.Print(userMsgStyle.Render("You: "))

		// Get user input
		if !scanner.Scan() {
			break
		}

		userInput := scanner.Text()

		// Check for exit commands
		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		// Skip empty messages
		if strings.TrimSpace(userInput) == "" {
			continue
		}

		fmt.Println() // Add a newline after user input

		// Process the message
		response := processMessage(ctx, userInput, team)

		// Print the response
		fmt.Println(botMsgStyle.Render("Assistant: " + response))
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
