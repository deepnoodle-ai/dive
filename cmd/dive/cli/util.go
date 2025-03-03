package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/getstingrai/dive"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#198754"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DC3545"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0D6EFD"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFC107"))
)

// formatVars formats variables for display
func formatVars(vars dive.VariableValues) string {
	var parts []string
	for k, v := range vars {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v.AsString()))
	}
	return strings.Join(parts, ", ")
}
