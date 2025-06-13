package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diveagents/dive/workflow"
	"github.com/spf13/cobra"
)

// getEventStore creates an event store instance for the given database path
func getEventStore(persistenceDB string) (workflow.ExecutionEventStore, error) {
	dbPath := persistenceDB
	if dbPath == "" {
		diveDir, err := getDiveConfigDir()
		if err != nil {
			return nil, fmt.Errorf("error getting dive config directory: %v", err)
		}
		dbPath = filepath.Join(diveDir, "executions.db")
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database not found: %s\nRun some workflows with 'dive run' to create execution history", dbPath)
	}

	eventStore, err := workflow.NewSQLiteExecutionEventStore(dbPath, workflow.DefaultSQLiteStoreOptions())
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	return eventStore, nil
}

func listExecutions(persistenceDB string, status, workflowName string, limit int) error {
	eventStore, err := getEventStore(persistenceDB)
	if err != nil {
		return err
	}

	ctx := context.Background()
	filter := workflow.ExecutionFilter{
		Limit: limit,
	}

	if status != "" {
		filter.Status = &status
	}
	if workflowName != "" {
		filter.WorkflowName = &workflowName
	}

	executions, err := eventStore.ListExecutions(ctx, filter)
	if err != nil {
		return fmt.Errorf("error listing executions: %v", err)
	}

	if len(executions) == 0 {
		fmt.Println("No executions found.")
		return nil
	}

	// Print header
	fmt.Printf("%-40s %-20s %-12s %-20s %-20s\n",
		"EXECUTION ID", "WORKFLOW", "STATUS", "STARTED", "DURATION")
	fmt.Println(strings.Repeat("-", 115))

	// Print executions
	for _, exec := range executions {
		duration := ""
		if !exec.EndTime.IsZero() && !exec.StartTime.IsZero() {
			duration = exec.EndTime.Sub(exec.StartTime).Round(time.Millisecond).String()
		} else if !exec.StartTime.IsZero() {
			duration = time.Since(exec.StartTime).Round(time.Second).String() + " (running)"
		}

		startTime := ""
		if !exec.StartTime.IsZero() {
			startTime = exec.StartTime.Format("2006-01-02 15:04:05")
		}

		statusText := exec.Status
		switch statusText {
		case "completed":
			statusText = successStyle.Sprint("✓ " + statusText)
		case "failed":
			statusText = errorStyle.Sprint("✗ " + statusText)
		case "running":
			statusText = warningStyle.Sprint("⚠ " + statusText)
		default:
			statusText = infoStyle.Sprint("• " + statusText)
		}

		fmt.Printf("%-40s %-20s %-12s %-20s %-20s\n",
			exec.ID,
			truncate(exec.WorkflowName, 20),
			statusText,
			startTime,
			duration)
	}

	return nil
}

func showExecution(persistenceDB, executionID string, showEvents bool) error {
	eventStore, err := getEventStore(persistenceDB)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Get execution snapshot
	snapshot, err := eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return fmt.Errorf("error getting execution: %v", err)
	}

	// Print execution details
	fmt.Printf("📋 Execution Details\n")
	fmt.Printf("ID:           %s\n", snapshot.ID)
	fmt.Printf("Workflow:     %s\n", snapshot.WorkflowName)

	statusText := snapshot.Status
	switch statusText {
	case "completed":
		statusText = successStyle.Sprint("✓ " + statusText)
	case "failed":
		statusText = errorStyle.Sprint("✗ " + statusText)
	case "running":
		statusText = warningStyle.Sprint("⚠ " + statusText)
	default:
		statusText = infoStyle.Sprint("• " + statusText)
	}
	fmt.Printf("Status:       %s\n", statusText)

	if !snapshot.StartTime.IsZero() {
		fmt.Printf("Started:      %s\n", snapshot.StartTime.Format("2006-01-02 15:04:05"))
	}
	if !snapshot.EndTime.IsZero() {
		fmt.Printf("Ended:        %s\n", snapshot.EndTime.Format("2006-01-02 15:04:05"))
		duration := snapshot.EndTime.Sub(snapshot.StartTime).Round(time.Millisecond)
		fmt.Printf("Duration:     %s\n", duration)
	}

	fmt.Printf("Events:       %d\n", snapshot.LastEventSeq)

	if snapshot.Error != "" {
		fmt.Printf("Error:        %s\n", errorStyle.Sprint(snapshot.Error))
	}

	// Print inputs
	if len(snapshot.Inputs) > 0 {
		fmt.Printf("\n📥 Inputs:\n")
		for key, value := range snapshot.Inputs {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	// Print outputs
	if len(snapshot.Outputs) > 0 {
		fmt.Printf("\n📤 Outputs:\n")
		for key, value := range snapshot.Outputs {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	// Show events if requested
	if showEvents {
		events, err := eventStore.GetEventHistory(ctx, executionID)
		if err != nil {
			return fmt.Errorf("error getting events: %v", err)
		}

		fmt.Printf("\n📜 Event History (%d events):\n", len(events))
		for _, event := range events {
			timestamp := event.Timestamp.Format("15:04:05.000")

			eventTypeDisplay := string(event.EventType)
			switch event.EventType {
			case "execution_started", "execution_completed":
				eventTypeDisplay = successStyle.Sprint(eventTypeDisplay)
			case "step_failed", "execution_failed", "path_failed":
				eventTypeDisplay = errorStyle.Sprint(eventTypeDisplay)
			case "step_started", "path_started":
				eventTypeDisplay = warningStyle.Sprint(eventTypeDisplay)
			default:
				eventTypeDisplay = infoStyle.Sprint(eventTypeDisplay)
			}

			stepInfo := ""
			if event.StepName != "" {
				stepInfo = fmt.Sprintf(" [%s]", event.StepName)
			}

			fmt.Printf("  %3d. [%s] %s%s\n",
				event.Sequence, timestamp, eventTypeDisplay, stepInfo)
		}
	}

	return nil
}

func deleteExecution(persistenceDB, executionID string, confirm bool) error {
	eventStore, err := getEventStore(persistenceDB)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Check if execution exists
	_, err = eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return fmt.Errorf("execution not found: %s", executionID)
	}

	if !confirm {
		fmt.Printf("Are you sure you want to delete execution %s? [y/N]: ", executionID)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	err = eventStore.DeleteExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("error deleting execution: %v", err)
	}

	fmt.Printf("✓ Execution %s deleted successfully.\n", executionID)
	return nil
}

func cleanupExecutions(persistenceDB string, olderThanDays int, confirm bool) error {
	eventStore, err := getEventStore(persistenceDB)
	if err != nil {
		return err
	}

	ctx := context.Background()
	olderThan := time.Now().AddDate(0, 0, -olderThanDays)

	// First, see what would be deleted
	filter := workflow.ExecutionFilter{
		Status: stringPtr("completed"),
		Limit:  1000,
	}
	allCompleted, err := eventStore.ListExecutions(ctx, filter)
	if err != nil {
		return fmt.Errorf("error listing executions: %v", err)
	}

	var toDelete []*workflow.ExecutionSnapshot
	for _, exec := range allCompleted {
		if exec.UpdatedAt.Before(olderThan) {
			toDelete = append(toDelete, exec)
		}
	}

	if len(toDelete) == 0 {
		fmt.Printf("No completed executions older than %d days found.\n", olderThanDays)
		return nil
	}

	fmt.Printf("Found %d completed executions older than %d days:\n", len(toDelete), olderThanDays)
	for _, exec := range toDelete {
		fmt.Printf("  - %s (%s) from %s\n",
			exec.ID, exec.WorkflowName, exec.UpdatedAt.Format("2006-01-02"))
	}

	if !confirm {
		fmt.Printf("\nDelete these %d executions? [y/N]: ", len(toDelete))
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Cleanup cancelled.")
			return nil
		}
	}

	err = eventStore.CleanupCompletedExecutions(ctx, olderThan)
	if err != nil {
		return fmt.Errorf("error cleaning up executions: %v", err)
	}

	fmt.Printf("✓ Cleaned up %d completed executions.\n", len(toDelete))
	return nil
}

func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	if length <= 3 {
		return s[:length]
	}
	return s[:length-3] + "..."
}

func stringPtr(s string) *string {
	return &s
}

// Executions command
var executionsCmd = &cobra.Command{
	Use:   "executions",
	Short: "Manage workflow executions",
	Long:  "List, show, delete, and cleanup workflow executions from the persistence store",
}

// List executions
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List workflow executions",
	Long:  "List workflow executions with optional filtering by status and workflow name",
	Run: func(cmd *cobra.Command, args []string) {
		persistenceDB, _ := cmd.Flags().GetString("persist-db")
		status, _ := cmd.Flags().GetString("status")
		workflowName, _ := cmd.Flags().GetString("workflow")
		limit, _ := cmd.Flags().GetInt("limit")

		if err := listExecutions(persistenceDB, status, workflowName, limit); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

// Show execution details
var showCmd = &cobra.Command{
	Use:   "show <execution-id>",
	Short: "Show execution details",
	Long:  "Show detailed information about a specific execution including inputs, outputs, and optionally event history",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		executionID := args[0]
		persistenceDB, _ := cmd.Flags().GetString("persist-db")
		showEvents, _ := cmd.Flags().GetBool("events")

		if err := showExecution(persistenceDB, executionID, showEvents); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

// Delete execution
var deleteCmd = &cobra.Command{
	Use:   "delete <execution-id>",
	Short: "Delete an execution",
	Long:  "Delete a specific execution and all its associated events",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		executionID := args[0]
		persistenceDB, _ := cmd.Flags().GetString("persist-db")
		confirm, _ := cmd.Flags().GetBool("yes")

		if err := deleteExecution(persistenceDB, executionID, confirm); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

// Cleanup old executions
var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up old completed executions",
	Long:  "Remove completed executions older than the specified number of days",
	Run: func(cmd *cobra.Command, args []string) {
		persistenceDB, _ := cmd.Flags().GetString("persist-db")
		olderThanDays, _ := cmd.Flags().GetInt("older-than")
		confirm, _ := cmd.Flags().GetBool("yes")

		if err := cleanupExecutions(persistenceDB, olderThanDays, confirm); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(executionsCmd)

	// Add subcommands
	executionsCmd.AddCommand(listCmd)
	executionsCmd.AddCommand(showCmd)
	executionsCmd.AddCommand(deleteCmd)
	executionsCmd.AddCommand(cleanupCmd)

	// Global flags for all execution commands
	executionsCmd.PersistentFlags().String("persist-db", "", "Path to SQLite database (default: ~/.dive/executions.db)")

	// List command flags
	listCmd.Flags().StringP("status", "s", "", "Filter by status (pending, running, completed, failed)")
	listCmd.Flags().StringP("workflow", "w", "", "Filter by workflow name")
	listCmd.Flags().Int("limit", 50, "Maximum number of executions to show")

	// Show command flags
	showCmd.Flags().BoolP("events", "e", false, "Show event history")

	// Delete command flags
	deleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Cleanup command flags
	cleanupCmd.Flags().Int("older-than", 30, "Delete executions older than this many days")
	cleanupCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}
