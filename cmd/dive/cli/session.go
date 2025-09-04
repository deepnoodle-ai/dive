package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/agent"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage chat session files",
	Long:  "Manage chat session files including listing, inspecting, and cleaning up sessions.",
}

var sessionListCmd = &cobra.Command{
	Use:   "list [directory]",
	Short: "List chat session files",
	Long:  "List chat session files in the specified directory (defaults to current directory). Shows session file names, creation times, and message counts.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		directory := "."
		if len(args) > 0 {
			directory = args[0]
		}

		pattern := filepath.Join(directory, "*.json")
		files, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error finding session files: %v", err))
			os.Exit(1)
		}

		if len(files) == 0 {
			fmt.Println("No session files found in", directory)
			return
		}

		fmt.Println(boldStyle.Sprintf("Chat Sessions in %s:", directory))
		fmt.Println()

		// Collect session info
		type sessionInfo struct {
			path      string
			modTime   time.Time
			threads   int
			messages  int
			lastUser  string
			size      int64
		}

		var sessions []sessionInfo

		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}

			// Try to load and analyze the session
			repo := agent.NewFileThreadRepository(file)
			ctx := context.Background()
			if err := repo.Load(ctx); err != nil {
				// Skip files that can't be loaded as sessions
				continue
			}

			threads, err := repo.ListThreads(ctx)
			if err != nil {
				continue
			}

			totalMessages := 0
			var lastUser string
			for _, thread := range threads {
				totalMessages += len(thread.Messages)
				if thread.UserID != "" {
					lastUser = thread.UserID
				}
			}

			sessions = append(sessions, sessionInfo{
				path:     file,
				modTime:  info.ModTime(),
				threads:  len(threads),
				messages: totalMessages,
				lastUser: lastUser,
				size:     info.Size(),
			})
		}

		// Sort by modification time (newest first)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].modTime.After(sessions[j].modTime)
		})

		// Print session information
		for _, session := range sessions {
			relPath, _ := filepath.Rel(directory, session.path)
			fmt.Printf("%s %s\n", 
				boldStyle.Sprint(relPath),
				color.New(color.FgBlue).Sprintf("(%s)", formatFileSize(session.size)))
			
			fmt.Printf("  Last modified: %s\n", session.modTime.Format("2006-01-02 15:04:05"))
			
			if session.threads > 0 {
				fmt.Printf("  Threads: %d, Messages: %d\n", session.threads, session.messages)
				if session.lastUser != "" {
					fmt.Printf("  User: %s\n", session.lastUser)
				}
			} else {
				fmt.Printf("  %s\n", color.New(color.FgYellow).Sprint("Empty session"))
			}
			fmt.Println()
		}
	},
}

var sessionShowCmd = &cobra.Command{
	Use:   "show <session-file>",
	Short: "Show details of a chat session file",
	Long:  "Display detailed information about a chat session file including all messages and threads.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sessionFile := args[0]

		repo := agent.NewFileThreadRepository(sessionFile)
		ctx := context.Background()
		if err := repo.Load(ctx); err != nil {
			fmt.Println(errorStyle.Sprintf("Error loading session file: %v", err))
			os.Exit(1)
		}

		threads, err := repo.ListThreads(ctx)
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error listing threads: %v", err))
			os.Exit(1)
		}

		if len(threads) == 0 {
			fmt.Println("No threads found in session file")
			return
		}

		fmt.Println(boldStyle.Sprintf("Session: %s", sessionFile))
		fmt.Println()

		for i, thread := range threads {
			fmt.Printf("%s Thread %d: %s\n", boldStyle.Sprint("▶"), i+1, thread.ID)
			if thread.UserID != "" {
				fmt.Printf("  User: %s\n", thread.UserID)
			}
			fmt.Printf("  Created: %s\n", thread.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Updated: %s\n", thread.UpdatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Messages: %d\n", len(thread.Messages))

			showFull, _ := cmd.Flags().GetBool("full")
			if showFull {
				fmt.Println("  Content:")
				for j, msg := range thread.Messages {
					roleColor := successStyle
					if msg.Role == "user" {
						roleColor = boldStyle
					}
					fmt.Printf("    %d. %s: %s\n", j+1, roleColor.Sprint(strings.Title(string(msg.Role))), msg.Text())
				}
			}
			fmt.Println()
		}
	},
}

var sessionCleanCmd = &cobra.Command{
	Use:   "clean [directory]",
	Short: "Clean up old or empty session files",
	Long:  "Remove empty session files or session files older than the specified age from the directory.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		directory := "."
		if len(args) > 0 {
			directory = args[0]
		}

		maxAge, _ := cmd.Flags().GetDuration("older-than")
		removeEmpty, _ := cmd.Flags().GetBool("empty")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		pattern := filepath.Join(directory, "*.json")
		files, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error finding session files: %v", err))
			os.Exit(1)
		}

		var toDelete []string
		var totalSize int64

		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}

			// Check if file is too old
			if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
				toDelete = append(toDelete, file)
				totalSize += info.Size()
				continue
			}

			// Check if file is empty (if flag is set)
			if removeEmpty {
				repo := agent.NewFileThreadRepository(file)
				ctx := context.Background()
				if err := repo.Load(ctx); err != nil {
					continue // Skip files that can't be loaded
				}

				threads, err := repo.ListThreads(ctx)
				if err != nil {
					continue
				}

				isEmpty := len(threads) == 0
				for _, thread := range threads {
					if len(thread.Messages) > 0 {
						isEmpty = false
						break
					}
				}

				if isEmpty {
					toDelete = append(toDelete, file)
					totalSize += info.Size()
				}
			}
		}

		if len(toDelete) == 0 {
			fmt.Println("No session files to clean up")
			return
		}

		fmt.Printf("Found %d session files to remove (%s total):\n", len(toDelete), formatFileSize(totalSize))
		for _, file := range toDelete {
			relPath, _ := filepath.Rel(directory, file)
			fmt.Printf("  - %s\n", relPath)
		}

		if dryRun {
			fmt.Println(color.New(color.FgYellow).Sprint("\nDry run - no files were deleted"))
			return
		}

		fmt.Println()
		for _, file := range toDelete {
			if err := os.Remove(file); err != nil {
				fmt.Printf("Failed to remove %s: %v\n", file, err)
			} else {
				fmt.Printf("Removed %s\n", file)
			}
		}
	},
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func init() {
	rootCmd.AddCommand(sessionCmd)

	// Add subcommands
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	sessionCmd.AddCommand(sessionCleanCmd)

	// Flags for show command
	sessionShowCmd.Flags().BoolP("full", "f", false, "Show full conversation content")

	// Flags for clean command
	sessionCleanCmd.Flags().DurationP("older-than", "", 0, "Remove sessions older than this duration (e.g., 168h for 7 days)")
	sessionCleanCmd.Flags().BoolP("empty", "e", false, "Remove empty session files")
	sessionCleanCmd.Flags().BoolP("dry-run", "n", false, "Show what would be deleted without actually deleting")
}