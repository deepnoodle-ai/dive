package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/getstingrai/dive"
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

	chatHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))
)

// Define key mappings for chat
type chatKeyMap struct {
	Help  key.Binding
	Quit  key.Binding
	Send  key.Binding
	Clear key.Binding
}

func (k chatKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k chatKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.Clear},
		{k.Help, k.Quit},
	}
}

var chatKeys = chatKeyMap{
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	),
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send message"),
	),
	Clear: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "clear input"),
	),
}

// Message represents a chat message
type Message struct {
	Content string
	IsUser  bool
}

// Model represents the chat application state
type chatModel struct {
	input      textinput.Model
	messages   []Message
	viewport   viewport.Model
	help       help.Model
	keys       chatKeyMap
	width      int
	height     int
	team       dive.Team
	teamLoaded bool
	filePath   string
}

func initialChatModel(filePath string) chatModel {
	input := textinput.New()
	input.Placeholder = "Type your message here..."
	input.Focus()
	input.CharLimit = 256
	input.Width = 80

	vp := viewport.New(80, 20)
	vp.SetContent("")

	h := help.New()
	h.Width = 80

	return chatModel{
		input:      input,
		messages:   []Message{},
		viewport:   vp,
		help:       h,
		keys:       chatKeys,
		teamLoaded: false,
		filePath:   filePath,
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		loadTeam(m.filePath),
	)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil

		case key.Matches(msg, m.keys.Clear):
			m.input.SetValue("")
			return m, nil

		case key.Matches(msg, m.keys.Send):
			if m.input.Value() != "" {
				userMsg := m.input.Value()
				m.messages = append(m.messages, Message{Content: userMsg, IsUser: true})
				m.input.SetValue("")

				// Only process message if team is loaded
				if m.teamLoaded {
					return m, processMessage(userMsg, m.team)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 10 // Adjust for input and help
		m.help.Width = msg.Width

		m.viewport.SetContent(formatMessages(m.messages))

		return m, nil

	case teamLoadedMsg:
		m.team = msg.team
		m.teamLoaded = true

		// Add welcome message
		welcomeMsg := "Welcome to the Dive chat! I'm your assistant for the team defined in " + m.filePath + ". How can I help you today?"
		m.messages = append(m.messages, Message{Content: welcomeMsg, IsUser: false})
		m.viewport.SetContent(formatMessages(m.messages))

		return m, nil

	case chatResponseMsg:
		m.messages = append(m.messages, Message{Content: msg.response, IsUser: false})
		m.viewport.SetContent(formatMessages(m.messages))

		// Scroll to bottom
		m.viewport.GotoBottom()

		return m, nil
	}

	// Handle input updates
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	// Handle viewport updates
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m chatModel) View() string {
	// Update viewport content with messages
	chatContent := m.viewport.View()

	// Input area
	inputArea := fmt.Sprintf("\n%s\n", m.input.View())

	// Help
	helpView := m.help.View(m.keys)

	// Title
	title := chatTitleStyle.Render("Dive Team Chat")

	// Combine all elements
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		chatContent,
		inputArea,
		helpView,
	)
}

// Format messages for display
func formatMessages(messages []Message) string {
	var sb strings.Builder

	for _, msg := range messages {
		if msg.IsUser {
			sb.WriteString(userMsgStyle.Render("You: " + msg.Content))
		} else {
			sb.WriteString(botMsgStyle.Render("Assistant: " + msg.Content))
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// Message for team loading
type teamLoadedMsg struct {
	team  dive.Team
	tasks []*dive.Task
}

// Command to load the team
func loadTeam(filePath string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		team, tasks, err := dive.LoadHCLTeam(ctx, filePath, nil)
		if err != nil {
			// Return error message
			return chatResponseMsg{
				response: fmt.Sprintf("Error loading team: %v", err),
			}
		}

		return teamLoadedMsg{
			team:  team,
			tasks: tasks,
		}
	}
}

// Message for chat responses
type chatResponseMsg struct {
	response string
}

// Command to process a message
func processMessage(message string, team dive.Team) tea.Cmd {
	return func() tea.Msg {
		// In a real implementation, this would interact with the team's AI capabilities
		// For now, we'll just echo back a simple response
		response := fmt.Sprintf("You asked about: %s\n\nThis is a placeholder response. In a real implementation, this would use the team's AI capabilities to generate a meaningful response.", message)

		return chatResponseMsg{
			response: response,
		}
	}
}

// chatCmd represents the chat command
var chatCmd = &cobra.Command{
	Use:   "chat [file]",
	Short: "Start a chat session with a team",
	Long: `Start an interactive chat session with a team defined in an HCL file.
This allows you to ask questions and interact with the team's AI capabilities.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// Start the Bubbletea program
		p := tea.NewProgram(initialChatModel(filePath), tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error running chat: %v", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
