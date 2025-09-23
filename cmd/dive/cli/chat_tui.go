package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/internal/random"
	"github.com/deepnoodle-ai/dive/tui"
	"github.com/spf13/cobra"
)

func runChatTUI(systemPrompt, goal, instructions, threadID, configFlag, agentFlag string, noConfig bool, tools []dive.Tool) error {
	ctx := context.Background()

	// Initialize terminal
	terminal, err := tui.NewTerminal()
	if err != nil {
		return fmt.Errorf("error initializing terminal: %v", err)
	}
	defer terminal.Close()

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

	// Create layout with header and footer
	layout := tui.NewLayout(terminal)

	// Set header with agent name and branding
	header := &tui.Header{
		Left:   " 🤖 " + chatAgent.Name(),
		Center: "Dive AI Chat",
		Right:  time.Now().Format("15:04") + " ",
		Style:  tui.NewStyle().WithForeground(tui.ColorWhite).WithBold(),
		Background: tui.NewStyle().WithBackground(tui.ColorBlue),
		Height: 1,
	}
	layout.SetHeader(header)

	// Set footer with status information
	footer := &tui.Footer{
		StatusBar: true,
		StatusItems: []tui.StatusItem{
			{Icon: "💬", Key: "Thread", Value: threadID, Style: tui.NewStyle().WithForeground(tui.ColorCyan)},
			{Icon: "⚡", Key: "Status", Value: "Ready", Style: tui.NewStyle().WithForeground(tui.ColorGreen)},
			{Key: "ESC", Value: "Exit", Style: tui.NewStyle().WithForeground(tui.ColorRed)},
			{Key: "Ctrl+C", Value: "Clear", Style: tui.NewStyle().WithForeground(tui.ColorYellow)},
		},
		Height: 1,
	}
	layout.SetFooter(footer)

	// Draw initial layout
	layout.Draw()

	// Print welcome message
	y, _ := layout.ContentArea()
	terminal.MoveCursor(2, y+1)

	// Welcome box
	welcomeBox := tui.NewBox([]string{
		fmt.Sprintf("Welcome to %s Chat Session!", chatAgent.Name()),
		"",
		"Type your message and press Enter to send.",
		"Type 'exit' or 'quit' to end the session.",
		"",
		"Let's begin!",
	})
	welcomeBox.Border = tui.RoundedBorder
	welcomeBox.BorderStyle = tui.NewStyle().WithForeground(tui.ColorCyan)
	welcomeBox.Draw(terminal, 2, y+1)

	terminal.MoveCursor(0, y+10)

	// Create input handler with beautiful styling
	input := tui.NewInput(terminal).
		WithPrompt("You > ", tui.NewStyle().WithForeground(tui.ColorCyan).WithBold()).
		WithLines(true, false, tui.NewStyle().WithForeground(tui.ColorBrightBlack))

	// Main chat loop
	for {
		// Update status
		footer.StatusItems[1].Value = "Waiting"
		footer.StatusItems[1].Style = tui.NewStyle().WithForeground(tui.ColorYellow)
		layout.Refresh()

		// Get user input
		fmt.Println()
		userInput, err := input.ReadBasic()
		if err != nil {
			break
		}

		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "quit" {
			// Show goodbye message with animation
			_, currentY := terminal.Size()
			terminal.MoveCursor(2, currentY-10)
			spinner := tui.NewSpinner(terminal, tui.SpinnerDots).
				WithStyle(tui.NewStyle().WithForeground(tui.ColorGreen)).
				WithMessage("Saving session...")
			spinner.Start()
			time.Sleep(1 * time.Second)
			spinner.Success("Session saved! Goodbye! 👋")
			time.Sleep(500 * time.Millisecond)
			break
		}

		if strings.TrimSpace(userInput) == "" {
			continue
		}

		// Update status to processing
		footer.StatusItems[1].Value = "Processing"
		footer.StatusItems[1].Style = tui.NewStyle().WithForeground(tui.ColorMagenta)
		layout.Refresh()

		// Show agent response header
		fmt.Println()
		agentStyle := tui.NewStyle().WithForeground(tui.ColorGreen).WithBold()
		fmt.Print(agentStyle.Apply(fmt.Sprintf("%s > ", chatAgent.Name())))

		// Process message with visual feedback
		if err := chatMessageTUI(ctx, userInput, chatAgent, threadID, terminal, layout, footer); err != nil {
			errorStyle := tui.NewStyle().WithForeground(tui.ColorRed)
			fmt.Println(errorStyle.Apply("Error: " + err.Error()))
		}

		// Update status back to ready
		footer.StatusItems[1].Value = "Ready"
		footer.StatusItems[1].Style = tui.NewStyle().WithForeground(tui.ColorGreen)
		layout.Refresh()
	}

	return nil
}

func chatMessageTUI(ctx context.Context, message string, agent dive.Agent, threadID string, terminal *tui.Terminal, layout *tui.Layout, footer *tui.Footer) error {
	// Generate a thread ID if none was provided
	actualThreadID := threadID
	if actualThreadID == "" {
		actualThreadID = random.Integer()
	}

	var inToolUse bool
	toolUseAccum := ""
	toolName := ""

	// Style definitions
	toolStyle := tui.NewStyle().WithForeground(tui.ColorYellow)
	textStyle := tui.NewStyle().WithForeground(tui.ColorWhite)
	thinkingStyle := tui.NewStyle().WithForeground(tui.ColorBrightBlack).WithItalic()

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
						// toolID = cb.ID // not used in TUI version
						// Update status to show tool usage
						footer.StatusItems[1].Value = "Using " + toolName
						footer.StatusItems[1].Style = tui.NewStyle().WithForeground(tui.ColorYellow)
						layout.Refresh()
					}
				}
				if payload.Delta != nil {
					delta := payload.Delta
					if delta.PartialJSON != "" {
						if !inToolUse {
							inToolUse = true
							fmt.Println()
							// Draw a nice border for tool use
							fmt.Println(toolStyle.Apply("┌─ Tool: " + toolName + " ─────────────────"))
						}
						toolUseAccum += delta.PartialJSON
					} else if delta.Text != "" {
						if inToolUse {
							fmt.Println(toolStyle.Apply("│ " + toolUseAccum))
							fmt.Println(toolStyle.Apply("└──────────────────────────────────────"))
							inToolUse = false
							toolUseAccum = ""
							// Update status back to processing
							footer.StatusItems[1].Value = "Processing"
							footer.StatusItems[1].Style = tui.NewStyle().WithForeground(tui.ColorMagenta)
							layout.Refresh()
						}
						fmt.Print(textStyle.Apply(delta.Text))
					} else if delta.Thinking != "" {
						fmt.Print(thinkingStyle.Apply(delta.Thinking))
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

var chatTUICmd = &cobra.Command{
	Use:   "chat-tui",
	Short: "Start an interactive chat with enhanced TUI interface",
	Long:  "Start an interactive chat session with an agent using the beautiful TUI interface.",
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
		if thread == "" {
			thread = random.Integer()
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
		if err := runChatTUI(systemPrompt, goal, instructions, thread, configFlag, agentFlag, noConfig, tools); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(chatTUICmd)

	chatTUICmd.Flags().StringP("system", "", "", "System prompt for the agent")
	chatTUICmd.Flags().StringP("goal", "", "", "Goal for the agent")
	chatTUICmd.Flags().StringP("instructions", "", "", "Instructions for the agent")
	chatTUICmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use for the agent")
	chatTUICmd.Flags().StringP("thread", "", "", "Name of the thread to use for the agent")
	chatTUICmd.Flags().StringP("config", "", "", "Path to configuration file or directory")
	chatTUICmd.Flags().StringP("agent", "", "", "Name of the agent to use from configuration")
	chatTUICmd.Flags().BoolP("no-config", "", false, "Disable automatic configuration loading")
}