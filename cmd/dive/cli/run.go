package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/getstingrai/dive"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

// Define styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#0D6EFD")).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#DC3545")).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#198754")).
			Padding(0, 1)

	taskNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))

	taskContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				MarginLeft(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))
)

// Define key mappings
type keyMap struct {
	Help key.Binding
	Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Quit},
	}
}

var keys = keyMap{
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// Model states
const (
	stateRunning = iota
	stateResults
)

// Model represents the application state
type runModel struct {
	state    int
	spinner  spinner.Model
	viewport viewport.Model
	help     help.Model
	keys     keyMap
	results  []*dive.TaskResult
	err      error
	width    int
	height   int
	filePath string
	vars     string
	verbose  bool
}

func initialRunModel(filePath string, vars string, verbose bool) runModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	h := help.New()
	h.Width = 50

	return runModel{
		state:    stateRunning,
		spinner:  s,
		help:     h,
		keys:     keys,
		filePath: filePath,
		vars:     vars,
		verbose:  verbose,
	}
}

func (m runModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		runTeam(m.filePath, m.vars, m.verbose),
	)
}

func (m runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.state == stateResults {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 10 // Adjust for header and footer
		}

		m.help.Width = msg.Width

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case resultMsg:
		m.state = stateResults
		m.results = msg.results
		m.err = msg.err

		// Create viewport for results
		vp := viewport.New(m.width, m.height-10)
		vp.SetContent(formatResults(msg.results, msg.err, m.verbose))
		m.viewport = vp

		return m, nil
	}

	if m.state == stateResults {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m runModel) View() string {
	switch m.state {
	case stateRunning:
		return runningView(m)
	case stateResults:
		return resultsView(m)
	default:
		return "Unknown state"
	}
}

func runningView(m runModel) string {
	title := titleStyle.Render("Dive Team Runner")

	spinner := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.spinner.View(),
		" Running team...",
	)

	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		spinner,
	)
}

func resultsView(m runModel) string {
	title := titleStyle.Render("Dive Team Runner - Results")

	var status string
	if m.err != nil {
		status = errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		status = successStyle.Render(fmt.Sprintf("Successfully ran %d tasks", len(m.results)))
	}

	help := m.help.View(m.keys)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		status,
		"",
		m.viewport.View(),
		"",
		help,
	)
}

// Format results for display
func formatResults(results []*dive.TaskResult, err error, verbose bool) string {
	if err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", err))
	}

	var sb strings.Builder

	for _, result := range results {
		sb.WriteString(taskNameStyle.Render(fmt.Sprintf("Task: %s", result.Task.Name())))
		sb.WriteString("\n")

		if verbose {
			sb.WriteString(fmt.Sprintf("Started: %s\n", result.StartedAt.Format(time.RFC3339)))
			sb.WriteString(fmt.Sprintf("Finished: %s\n", result.FinishedAt.Format(time.RFC3339)))
			if result.Error != nil {
				sb.WriteString(fmt.Sprintf("Error: %v\n", result.Error))
			}
		}

		sb.WriteString(taskContentStyle.Render(fmt.Sprintf("Output: %s", result.Content)))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// Message for team run results
type resultMsg struct {
	results []*dive.TaskResult
	err     error
}

// Command to run the team
func runTeam(filePath string, varsFlag string, verbose bool) tea.Cmd {
	return func() tea.Msg {
		// Parse variables
		vars := dive.VariableValues{}
		if varsFlag != "" {
			varPairs := strings.Split(varsFlag, ",")
			for _, pair := range varPairs {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					return resultMsg{
						err: fmt.Errorf("invalid variable format: %s", pair),
					}
				}
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				vars[key] = cty.StringVal(value)
			}
		}

		// Create context
		ctx := context.Background()

		// Run the team
		team, tasks, err := dive.LoadHCLTeam(ctx, filePath, vars)
		if err != nil {
			return resultMsg{
				err: fmt.Errorf("failed to load HCL team: %w", err),
			}
		}

		if err := team.Start(ctx); err != nil {
			return resultMsg{
				err: fmt.Errorf("failed to start team: %w", err),
			}
		}
		defer team.Stop(ctx)

		stream, err := team.Work(ctx, tasks...)
		if err != nil {
			return resultMsg{
				err: fmt.Errorf("failed to execute work: %w", err),
			}
		}

		for {
			select {
			case event, ok := <-stream.Events():
				if !ok {
					continue
				}
				print("EVENT", event.Type)
			case result, ok := <-stream.Results():
				if !ok {
					continue
				}
				print("RESULT", result.Content)
			}
		}

		return resultMsg{
			results: nil,
			err:     err,
		}
	}
}

var varsFlag string
var verboseFlag bool

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [file]",
	Short: "Run a team defined in an HCL file",
	Long: `Run a team defined in an HCL file. 
This will execute all tasks defined in the team in the order specified.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// Start the Bubbletea program
		p := tea.NewProgram(initialRunModel(filePath, varsFlag, verboseFlag), tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error running program: %v", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Add flags
	runCmd.Flags().StringVar(&varsFlag, "vars", "", "Comma-separated list of variables in format key=value")
	runCmd.Flags().BoolVar(&verboseFlag, "verbose", false, "Enable verbose output")
}
