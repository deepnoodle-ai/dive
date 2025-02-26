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
	"github.com/getstingrai/dive/slogger"
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
	stateStreaming // New state for streaming events and results
)

// Model represents the application state
type runModel struct {
	state    int
	spinner  spinner.Model
	viewport viewport.Model
	help     help.Model
	keys     keyMap
	results  []*dive.TaskResult
	events   []string // Store events
	logs     []string // Store combined logs of events and results
	err      error
	width    int
	height   int
	filePath string
	vars     string
	verbose  bool

	// Channels for async communication
	eventCh  chan eventMsg
	resultCh chan taskResultMsg
	doneCh   chan completionMsg
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
		events:   []string{},
		logs:     []string{},
		results:  []*dive.TaskResult{},

		// Initialize channels
		eventCh:  make(chan eventMsg),
		resultCh: make(chan taskResultMsg),
		doneCh:   make(chan completionMsg),
	}
}

func (m runModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		runTeam(m.filePath, m.vars, m.verbose, m.eventCh, m.resultCh, m.doneCh),
		waitForActivity(m.eventCh, m.resultCh, m.doneCh),
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

		if m.state == stateResults || m.state == stateStreaming {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 10 // Adjust for header and footer
		}

		m.help.Width = msg.Width

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case eventMsg:
		// Add event to logs
		logEntry := fmt.Sprintf("[%s] EVENT: %s", msg.timestamp.Format("15:04:05"), msg.eventType)
		m.logs = append(m.logs, logEntry)
		m.events = append(m.events, msg.eventType)

		// If we're already in streaming state, update the viewport
		if m.state == stateStreaming {
			m.viewport.SetContent(formatLogs(m.logs, m.verbose))
		} else {
			// Transition to streaming state
			m.state = stateStreaming
			vp := viewport.New(m.width, m.height-10)
			vp.SetContent(formatLogs(m.logs, m.verbose))
			m.viewport = vp
		}

		// Continue waiting for more activity
		cmds = append(cmds, waitForActivity(m.eventCh, m.resultCh, m.doneCh))
		return m, tea.Batch(cmds...)

	case taskResultMsg:
		// Add result to logs and results
		m.results = append(m.results, msg.result)
		logEntry := fmt.Sprintf("[%s] RESULT: Task completed with content: %s",
			msg.result.FinishedAt.Format("15:04:05"),
			// msg.result.Task.Name(),
			msg.result.Content)
		m.logs = append(m.logs, logEntry)

		// If we're already in streaming state, update the viewport
		if m.state == stateStreaming {
			m.viewport.SetContent(formatLogs(m.logs, m.verbose))
		} else {
			// Transition to streaming state
			m.state = stateStreaming
			vp := viewport.New(m.width, m.height-10)
			vp.SetContent(formatLogs(m.logs, m.verbose))
			m.viewport = vp
		}

		// Continue waiting for more activity
		cmds = append(cmds, waitForActivity(m.eventCh, m.resultCh, m.doneCh))
		return m, tea.Batch(cmds...)

	case completionMsg:
		// Handle completion
		m.state = stateResults
		m.err = msg.err

		// Create viewport for final results
		vp := viewport.New(m.width, m.height-10)
		vp.SetContent(formatResults(m.results, m.err, m.verbose))
		m.viewport = vp

		return m, nil

	case resultMsg:
		// For backward compatibility
		m.state = stateResults
		m.results = msg.results
		m.err = msg.err

		// Create viewport for results
		vp := viewport.New(m.width, m.height-10)
		vp.SetContent(formatResults(msg.results, msg.err, m.verbose))
		m.viewport = vp

		return m, nil
	}

	if m.state == stateResults || m.state == stateStreaming {
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
	case stateStreaming:
		return streamingView(m)
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

func streamingView(m runModel) string {
	title := titleStyle.Render("Dive Team Runner - Live Stream")

	status := infoStyle.Render(fmt.Sprintf("Streaming events and results (%d events, %d results)",
		len(m.events), len(m.results)))

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

// Format logs for display
func formatLogs(logs []string, verbose bool) string {
	if len(logs) == 0 {
		return "No logs yet..."
	}

	var sb strings.Builder

	// If verbose, show all logs
	if verbose {
		for _, log := range logs {
			sb.WriteString(log)
			sb.WriteString("\n")
		}
	} else {
		// Otherwise, show only the last 20 logs
		startIdx := 0
		if len(logs) > 20 {
			startIdx = len(logs) - 20
		}

		for i := startIdx; i < len(logs); i++ {
			sb.WriteString(logs[i])
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Message for team run results
type resultMsg struct {
	results []*dive.TaskResult
	err     error
}

// Message for a single event
type eventMsg struct {
	eventType string
	timestamp time.Time
}

// Message for a single result
type taskResultMsg struct {
	result *dive.TaskResult
}

// Message for completion
type completionMsg struct {
	err error
}

// Command to wait for activity from the runTeam command
func waitForActivity(eventCh chan eventMsg, resultCh chan taskResultMsg, doneCh chan completionMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-eventCh:
			return msg
		case msg := <-resultCh:
			return msg
		case msg := <-doneCh:
			return msg
		}
	}
}

// Command to run the team
func runTeam(filePath string, varsFlag string, verbose bool, eventCh chan eventMsg, resultCh chan taskResultMsg, doneCh chan completionMsg) tea.Cmd {
	return func() tea.Msg {
		// Parse variables
		vars := dive.VariableValues{}
		if varsFlag != "" {
			varPairs := strings.Split(varsFlag, ",")
			for _, pair := range varPairs {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					doneCh <- completionMsg{
						err: fmt.Errorf("invalid variable format: %s", pair),
					}
					return nil
				}
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				vars[key] = cty.StringVal(value)
			}
		}

		// Create context
		ctx := context.Background()

		// Run the team
		logger := slogger.New(slogger.LevelFromString("debug"))
		team, tasks, err := dive.LoadHCLTeam(ctx, filePath, vars, logger)
		if err != nil {
			doneCh <- completionMsg{
				err: fmt.Errorf("failed to load HCL team: %w", err),
			}
			return nil
		}

		if err := team.Start(ctx); err != nil {
			doneCh <- completionMsg{
				err: fmt.Errorf("failed to start team: %w", err),
			}
			return nil
		}
		defer team.Stop(ctx)

		stream, err := team.Work(ctx, tasks...)
		if err != nil {
			doneCh <- completionMsg{
				err: fmt.Errorf("failed to execute work: %w", err),
			}
			return nil
		}

		// Start a goroutine to process events and results
		go func() {
			for {
				select {
				case result, ok := <-stream.Results():
					if !ok {
						break
					}
					resultCh <- taskResultMsg{
						result: result,
					}
				}
			}
			// var results []*dive.TaskResult
			// var streamErr error

			// // Process events
			// for event := range stream.Events() {
			// 	eventCh <- eventMsg{
			// 		eventType: event.Type,
			// 		timestamp: time.Now(),
			// 	}
			// }

			// // Process results
			// for result := range stream.Results() {
			// 	results = append(results, result)
			// 	resultCh <- taskResultMsg{
			// 		result: result,
			// 	}
			// }

			// // Send completion message when both channels are closed
			// doneCh <- completionMsg{
			// 	err: streamErr,
			// }
		}()

		// Return nil as we're using channels for communication
		return nil
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
