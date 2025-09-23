package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/tui"
	"github.com/spf13/cobra"
)

var askTUICmd = &cobra.Command{
	Use:   "ask-tui <question>",
	Short: "Ask a question with enhanced visual feedback",
	Long:  "Ask a question to an AI agent with beautiful visual feedback using the TUI library.",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		question := strings.Join(args, " ")

		// Initialize terminal
		terminal, err := tui.NewTerminal()
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Failed to initialize terminal: %s", err))
			os.Exit(1)
		}
		defer terminal.Close()

		// Get flags
		systemPrompt, _ := cmd.Flags().GetString("system")
		goal, _ := cmd.Flags().GetString("goal")
		instructions, _ := cmd.Flags().GetString("instructions")
		toolsSpec, _ := cmd.Flags().GetString("tools")
		configFlag, _ := cmd.Flags().GetString("config")
		agentFlag, _ := cmd.Flags().GetString("agent")
		noConfig, _ := cmd.Flags().GetBool("no-config")

		// Initialize tools
		var tools []dive.Tool
		if toolsSpec != "" {
			tools, err = initializeTools(strings.Split(toolsSpec, ","))
			if err != nil {
				fmt.Println(errorStyle.Sprintf("Failed to initialize tools: %s", err))
				os.Exit(1)
			}
		}

		// Show fancy header
		headerStyle := tui.NewStyle().WithForeground(tui.ColorCyan).WithBold()
		fmt.Println()
		fmt.Println(headerStyle.Apply("╭─────────────────────────────────────────╮"))
		fmt.Println(headerStyle.Apply("│          🤖 Dive AI Assistant           │"))
		fmt.Println(headerStyle.Apply("╰─────────────────────────────────────────╯"))
		fmt.Println()

		// Show the question with style
		questionStyle := tui.NewStyle().WithForeground(tui.ColorYellow).WithBold()
		fmt.Println(questionStyle.Apply("📝 Your Question:"))
		fmt.Println("   " + question)
		fmt.Println()

		// Show loading spinner while setting up
		spinner := tui.NewSpinner(terminal, tui.SpinnerDots).
			WithStyle(tui.NewStyle().WithForeground(tui.ColorMagenta)).
			WithMessage("Initializing AI agent...")
		spinner.Start()

		ctx := context.Background()

		// Try to discover and load configuration
		configResult, err := discoverConfiguration(ctx, configFlag, noConfig, agentFlag)
		if err != nil {
			spinner.Error("Failed to load configuration")
			os.Exit(1)
		}

		var agent dive.Agent
		if configResult != nil {
			// Use config-based agent
			if systemPrompt != "" || goal != "" || instructions != "" || len(tools) > 0 {
				agent, err = applyFlagOverrides(configResult.SelectedAgent, systemPrompt, goal, instructions, tools)
				if err != nil {
					spinner.Error("Failed to apply overrides")
					os.Exit(1)
				}
			} else {
				agent = configResult.SelectedAgent
			}
		} else {
			// Create agent from flags
			agent, err = createAgentFromFlags(systemPrompt, goal, instructions, tools)
			if err != nil {
				spinner.Error("Failed to create agent")
				os.Exit(1)
			}
		}

		spinner.Success("Agent initialized!")
		time.Sleep(500 * time.Millisecond)

		// Show thinking spinner
		thinkSpinner := tui.NewSpinner(terminal, tui.SpinnerCircle).
			WithStyle(tui.NewStyle().WithForeground(tui.ColorCyan)).
			WithMessage("Thinking...")
		thinkSpinner.Start()

		// Track response progress
		var responseText strings.Builder
		var hasToolUse bool

		// Generate response
		_, err = agent.CreateResponse(ctx,
			dive.WithInput(question),
			dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
				if item.Type == dive.ResponseItemTypeModelEvent {
					payload := item.Event
					if payload.ContentBlock != nil && payload.ContentBlock.Type == "tool_use" {
						if !hasToolUse {
							hasToolUse = true
							thinkSpinner.Stop()
							// Show tool usage
							toolStyle := tui.NewStyle().WithForeground(tui.ColorYellow)
							fmt.Println(toolStyle.Apply("🔧 Using tool: " + payload.ContentBlock.Name))
						}
					}
					if payload.Delta != nil && payload.Delta.Text != "" {
						if hasToolUse {
							// Switch back to response after tool use
							hasToolUse = false
							fmt.Println()
						}
						if thinkSpinner != nil {
							thinkSpinner.Stop()
							thinkSpinner = nil
							// Show response header
							responseStyle := tui.NewStyle().WithForeground(tui.ColorGreen).WithBold()
							fmt.Println(responseStyle.Apply("💡 Response:"))
							fmt.Println()
						}
						responseText.WriteString(payload.Delta.Text)
						fmt.Print(payload.Delta.Text)
					}
				}
				return nil
			}),
		)

		if thinkSpinner != nil {
			thinkSpinner.Stop()
		}

		if err != nil {
			errorStyle := tui.NewStyle().WithForeground(tui.ColorRed).WithBold()
			fmt.Println()
			fmt.Println(errorStyle.Apply("❌ Error: " + err.Error()))
			os.Exit(1)
		}

		// Show completion
		fmt.Println()
		fmt.Println()
		successStyle := tui.NewStyle().WithForeground(tui.ColorGreen)
		fmt.Println(successStyle.Apply("✅ Response complete!"))
		fmt.Println()

		// Show fancy footer
		footerStyle := tui.NewStyle().WithForeground(tui.ColorBrightBlack)
		fmt.Println(footerStyle.Apply("─────────────────────────────────────────"))
	},
}

func init() {
	rootCmd.AddCommand(askTUICmd)

	askTUICmd.Flags().StringP("system", "", "", "System prompt for the agent")
	askTUICmd.Flags().StringP("goal", "", "", "Goal for the agent")
	askTUICmd.Flags().StringP("instructions", "", "", "Instructions for the agent")
	askTUICmd.Flags().StringP("tools", "", "", "Comma-separated list of tools to use")
	askTUICmd.Flags().StringP("config", "", "", "Path to configuration file or directory")
	askTUICmd.Flags().StringP("agent", "", "", "Name of the agent to use from configuration")
	askTUICmd.Flags().BoolP("no-config", "", false, "Disable automatic configuration loading")
}