package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/agent"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var threadCmd = &cobra.Command{
	Use:   "threads",
	Short: "Manage chat threads in directory",
	Long:  "Manage chat threads stored in the threads directory including listing, inspecting, and cleaning up threads.",
}

var threadListCmd = &cobra.Command{
	Use:   "list [directory]",
	Short: "List chat threads",
	Long:  "List chat threads in the specified directory (defaults to ~/.dive/threads). Shows thread IDs, creation times, and message counts.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var directory string
		if len(args) > 0 {
			directory = args[0]
		} else {
			// Default to dive threads directory
			var err error
			directory, err = diveThreadsDirectory()
			if err != nil {
				fmt.Println(errorStyle.Sprintf("Error getting dive threads directory: %v", err))
				os.Exit(1)
			}
		}

		ctx := context.Background()
		repo := agent.NewDiskThreadRepository(directory)

		output, err := repo.ListThreads(ctx, nil)
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error listing threads: %v", err))
			os.Exit(1)
		}

		if len(output.Items) == 0 {
			fmt.Println("No thread files found in", directory)
			return
		}

		fmt.Println(boldStyle.Sprintf("Chat Threads in directory: %s", directory))
		fmt.Println()

		// Sort by modification time (newest first)
		sort.Slice(output.Items, func(i, j int) bool {
			return output.Items[i].UpdatedAt.After(output.Items[j].UpdatedAt)
		})

		// Print thread information
		for _, thread := range output.Items {
			fmt.Printf("Thread %s\n", thread.ID)
			if thread.Title != "" {
				fmt.Printf(" Title: %s\n", thread.Title)
			}
			if thread.AgentName != "" {
				fmt.Printf(" Agent: %s\n", thread.AgentName)
			}
			timeAgo := formatTimeAgo(thread.UpdatedAt)
			fmt.Printf("  %s %s\n",
				timeAgo,
				thread.UpdatedAt.Format("Jan 2, 2006 at 3:04 PM"))

			if len(thread.Messages) > 0 {
				// Don't show "Threads: 1" since there's always only 1 thread per file
				if len(thread.Messages) == 1 {
					fmt.Printf("  %d message", len(thread.Messages))
				} else {
					fmt.Printf("  %d messages", len(thread.Messages))
				}
				if thread.UserID != "" {
					fmt.Printf(" • User: %s", thread.UserID)
				}
				fmt.Println()
			} else {
				fmt.Printf("  %s\n", color.New(color.FgYellow).Sprint("Empty thread"))
			}
			fmt.Println()
		}
	},
}

var threadShowCmd = &cobra.Command{
	Use:   "show <thread-id>",
	Short: "Show details of a chat thread by ID",
	Long:  "Display detailed information about a chat thread by its ID.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		threadID := args[0]

		threadsDir, err := diveThreadsDirectory()
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error getting dive threads directory: %v", err))
			os.Exit(1)
		}

		ctx := context.Background()
		repo := agent.NewDiskThreadRepository(threadsDir)

		// Get the specific thread by ID
		thread, err := repo.GetThread(ctx, threadID)
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error getting thread: %v", err))
			os.Exit(1)
		}

		fmt.Println(boldStyle.Sprintf("Thread ID: %s", thread.ID))
		fmt.Println()

		if thread.UserID != "" {
			fmt.Printf("User: %s\n", thread.UserID)
		}
		fmt.Printf("Created: %s\n", thread.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated: %s\n", thread.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println("Messages:")
		for j, msg := range thread.Messages {
			roleColor := successStyle
			if msg.Role == "user" {
				roleColor = boldStyle
			}
			fmt.Printf("  %d. %s: %s\n", j+1, roleColor.Sprint(strings.Title(string(msg.Role))), msg.Text())
		}
		fmt.Println()
	},
}

var threadCleanCmd = &cobra.Command{
	Use:   "clean [directory]",
	Short: "Clean up old threads",
	Long:  "Remove empty thread files or thread files older than the specified age from the directory (defaults to ~/.dive/threads).",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var directory string
		if len(args) > 0 {
			directory = args[0]
		} else {
			// Default to dive threads directory
			var err error
			directory, err = diveThreadsDirectory()
			if err != nil {
				fmt.Println(errorStyle.Sprintf("Error getting dive threads directory: %v", err))
				os.Exit(1)
			}
		}

		maxAge, _ := cmd.Flags().GetDuration("older-than")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		force, _ := cmd.Flags().GetBool("force")

		ctx := context.Background()
		repo := agent.NewDiskThreadRepository(directory)

		output, err := repo.ListThreads(ctx, nil)
		if err != nil {
			fmt.Println(errorStyle.Sprintf("Error listing threads: %v", err))
			os.Exit(1)
		}

		var toDelete []string

		for _, thread := range output.Items {
			// Check if file is too old
			if maxAge > 0 && !thread.UpdatedAt.IsZero() && time.Since(thread.UpdatedAt) > maxAge {
				toDelete = append(toDelete, thread.ID)
				continue
			}
		}

		if len(toDelete) == 0 {
			fmt.Println("No threads to clean up")
			return
		}

		fmt.Printf("Found %d thread files to remove:\n", len(toDelete))
		for _, threadID := range toDelete {
			fmt.Printf("  - %s\n", threadID)
		}

		if dryRun {
			fmt.Println(color.New(color.FgYellow).Sprint("\nDry run - no threads were deleted"))
			return
		}

		// Ask for confirmation unless --force is specified
		if !force {
			fmt.Printf("\nAre you sure you want to delete these %d threads? [y/N]: ", len(toDelete))
			var response string
			fmt.Scanln(&response)
			response = strings.ToLower(strings.TrimSpace(response))
			if response != "y" && response != "yes" {
				fmt.Println("Operation cancelled")
				return
			}
		}

		fmt.Println()
		for _, threadID := range toDelete {
			if err := repo.DeleteThread(ctx, threadID); err != nil {
				fmt.Printf("Failed to remove %s: %v\n", threadID, err)
			} else {
				fmt.Printf("Removed %s\n", threadID)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(threadCmd)

	// Add subcommands
	threadCmd.AddCommand(threadListCmd)
	threadCmd.AddCommand(threadShowCmd)
	threadCmd.AddCommand(threadCleanCmd)

	// Flags for clean command
	threadCleanCmd.Flags().DurationP("older-than", "", 0, "Remove threads older than this duration (e.g., 168h for 7 days)")
	threadCleanCmd.Flags().BoolP("dry-run", "n", false, "Show what would be deleted without actually deleting")
	threadCleanCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt and delete files immediately")
}
