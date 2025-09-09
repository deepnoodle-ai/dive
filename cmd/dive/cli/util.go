package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/fatih/color"
)

var (
	boldStyle     = color.New(color.Bold)
	successStyle  = color.New(color.FgGreen)
	errorStyle    = color.New(color.FgRed)
	yellowStyle   = color.New(color.FgYellow)
	thinkingStyle = color.New(color.FgMagenta)
)

// readStdin reads all content from standard input
func readStdin() (string, error) {
	var content strings.Builder
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line != "" {
					content.WriteString(line)
				}
				break
			}
			return "", fmt.Errorf("error reading from stdin: %v", err)
		}
		content.WriteString(line)
	}

	return strings.TrimSpace(content.String()), nil
}

func diveDirectory() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, ".dive"), nil
}

func diveThreadsDirectory() (string, error) {
	dir, err := diveDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "threads"), nil
}

func formatTimeAgo(t time.Time) string {
	now := time.Now()
	duration := now.Sub(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case duration < 30*24*time.Hour:
		weeks := int(duration.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	case duration < 365*24*time.Hour:
		months := int(duration.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(duration.Hours() / (24 * 365))
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// saveRecentThreadID saves the most recent thread ID to ~/.dive/threads/recent
func saveRecentThreadID(threadID string) error {
	threadsDir, err := diveThreadsDirectory()
	if err != nil {
		return fmt.Errorf("error getting dive threads directory: %v", err)
	}
	if err := os.MkdirAll(threadsDir, 0755); err != nil {
		return fmt.Errorf("error creating threads directory: %v", err)
	}

	recentFile := filepath.Join(threadsDir, "recent")
	if err := os.WriteFile(recentFile, []byte(threadID), 0644); err != nil {
		return fmt.Errorf("error writing recent thread ID: %v", err)
	}

	return nil
}

func initializeTools(toolNames []string) ([]dive.Tool, error) {
	tools := make([]dive.Tool, 0, len(toolNames))
	for _, toolName := range toolNames {
		tool, err := config.InitializeToolByName(toolName, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tool: %s", err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}
