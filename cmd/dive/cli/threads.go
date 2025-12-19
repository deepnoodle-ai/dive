package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/threads"
	"github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/color"
)

func registerThreadsCommand(app *cli.App) {
	threadsGroup := app.Group("threads").
		Description("Manage chat threads in directory")

	threadsGroup.Command("list").
		Description("List chat threads").
		Long("List chat threads in the specified directory (defaults to ~/.dive/threads). Shows thread IDs, creation times, and message counts.").
		Args("directory?").
		Run(func(ctx *cli.Context) error {
			parseGlobalFlags(ctx)

			var directory string
			if ctx.NArg() > 0 {
				directory = ctx.Arg(0)
			} else {
				var err error
				directory, err = diveThreadsDirectory()
				if err != nil {
					return cli.Errorf("error getting dive threads directory: %v", err)
				}
			}

			goCtx := context.Background()
			repo := threads.NewDiskRepository(directory)

			output, err := repo.ListThreads(goCtx, nil)
			if err != nil {
				return cli.Errorf("error listing threads: %v", err)
			}

			if len(output.Items) == 0 {
				fmt.Println("No thread files found in", directory)
				return nil
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
					if len(thread.Messages) == 1 {
						fmt.Printf("  %d message", len(thread.Messages))
					} else {
						fmt.Printf("  %d messages", len(thread.Messages))
					}
					if thread.UserID != "" {
						fmt.Printf(" - User: %s", thread.UserID)
					}
					fmt.Println()
				} else {
					fmt.Printf("  %s\n", color.Yellow.Sprint("Empty thread"))
				}
				fmt.Println()
			}
			return nil
		})

	threadsGroup.Command("show").
		Description("Show details of a chat thread by ID").
		Args("thread-id").
		Run(func(ctx *cli.Context) error {
			parseGlobalFlags(ctx)

			threadID := ctx.Arg(0)

			threadsDir, err := diveThreadsDirectory()
			if err != nil {
				return cli.Errorf("error getting dive threads directory: %v", err)
			}

			goCtx := context.Background()
			repo := threads.NewDiskRepository(threadsDir)

			thread, err := repo.GetThread(goCtx, threadID)
			if err != nil {
				return cli.Errorf("error getting thread: %v", err)
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
				roleStyle := successStyle
				if msg.Role == "user" {
					roleStyle = boldStyle
				}
				fmt.Printf("  %d. %s: %s\n", j+1, roleStyle.Sprint(strings.Title(string(msg.Role))), msg.Text())
			}
			fmt.Println()
			return nil
		})

	threadsGroup.Command("clean").
		Description("Clean up old threads").
		Long("Remove empty thread files or thread files older than the specified age from the directory (defaults to ~/.dive/threads).").
		Args("directory?").
		Flags(
			cli.String("older-than", "").Help("Remove threads older than this duration (e.g., 168h for 7 days)"),
			cli.Bool("dry-run", "n").Help("Show what would be deleted without actually deleting"),
			cli.Bool("force", "f").Help("Skip confirmation prompt and delete files immediately"),
		).
		Run(func(ctx *cli.Context) error {
			parseGlobalFlags(ctx)

			var directory string
			if ctx.NArg() > 0 {
				directory = ctx.Arg(0)
			} else {
				var err error
				directory, err = diveThreadsDirectory()
				if err != nil {
					return cli.Errorf("error getting dive threads directory: %v", err)
				}
			}

			var maxAge time.Duration
			if olderThanStr := ctx.String("older-than"); olderThanStr != "" {
				var err error
				maxAge, err = time.ParseDuration(olderThanStr)
				if err != nil {
					return cli.Errorf("invalid duration format: %v", err)
				}
			}
			dryRun := ctx.Bool("dry-run")
			force := ctx.Bool("force")

			goCtx := context.Background()
			repo := threads.NewDiskRepository(directory)

			output, err := repo.ListThreads(goCtx, nil)
			if err != nil {
				return cli.Errorf("error listing threads: %v", err)
			}

			var toDelete []string

			for _, thread := range output.Items {
				if maxAge > 0 && !thread.UpdatedAt.IsZero() && time.Since(thread.UpdatedAt) > maxAge {
					toDelete = append(toDelete, thread.ID)
					continue
				}
			}

			if len(toDelete) == 0 {
				fmt.Println("No threads to clean up")
				return nil
			}

			fmt.Printf("Found %d thread files to remove:\n", len(toDelete))
			for _, threadID := range toDelete {
				fmt.Printf("  - %s\n", threadID)
			}

			if dryRun {
				fmt.Println(color.Yellow.Sprint("\nDry run - no threads were deleted"))
				return nil
			}

			// Ask for confirmation unless --force is specified
			if !force {
				confirmed, err := ctx.Confirm(fmt.Sprintf("Delete %d threads?", len(toDelete)))
				if err != nil || !confirmed {
					fmt.Println("Operation cancelled")
					return nil
				}
			}

			fmt.Println()
			for _, threadID := range toDelete {
				if err := repo.DeleteThread(goCtx, threadID); err != nil {
					fmt.Printf("Failed to remove %s: %v\n", threadID, err)
				} else {
					fmt.Printf("Removed %s\n", threadID)
				}
			}
			return nil
		})
}
