package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/slogger"
	"github.com/spf13/cobra"
)

func listExecutions(databaseFlag string, status, workflowName string, limit int) error {
	// Validate status filter
	if err := validateExecutionStatus(status); err != nil {
		return err
	}

	eventStore, err := getEventStore(databaseFlag)
	if err != nil {
		return err
	}
	ctx := context.Background()

	executions, err := eventStore.ListExecutions(ctx, environment.ExecutionFilter{
		Limit:        limit,
		Status:       status,
		WorkflowName: workflowName,
	})
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
			duration = exec.EndTime.Sub(exec.StartTime).Round(time.Millisecond * 100).String()
		} else if !exec.StartTime.IsZero() {
			duration = time.Since(exec.StartTime).Round(time.Second).String() + " (running)"
		}

		startTime := ""
		if !exec.StartTime.IsZero() {
			startTime = exec.StartTime.Format("2006-01-02 15:04:05")
		}

		statusText := formatExecutionStatus(exec.Status)

		fmt.Printf("%-40s %-20s %-12s %-20s %-20s\n",
			exec.ID,
			truncate(exec.WorkflowName, 20),
			statusText,
			startTime,
			duration)
	}

	return nil
}

func showExecution(databaseFlag, executionID string, showEvents bool) error {
	eventStore, err := getEventStore(databaseFlag)
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

	fmt.Printf("Status:       %s\n", formatExecutionStatus(snapshot.Status))

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
			if event.Step != "" {
				stepInfo = fmt.Sprintf(" [%s]", event.Step)
			}

			fmt.Printf("  %3d. [%s] %s%s\n",
				event.Sequence, timestamp, eventTypeDisplay, stepInfo)
		}
	}

	return nil
}

func deleteExecution(databaseFlag, executionID string, confirm bool) error {
	eventStore, err := getEventStore(databaseFlag)
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
		if !confirmAction("delete execution", executionID) {
			fmt.Println("❌ Deletion cancelled.")
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

func cleanupExecutions(databaseFlag string, olderThanDays int, confirm bool) error {
	eventStore, err := getEventStore(databaseFlag)
	if err != nil {
		return err
	}

	ctx := context.Background()
	olderThan := time.Now().AddDate(0, 0, -olderThanDays)

	// First, see what would be deleted
	filter := environment.ExecutionFilter{
		Status: "completed",
		Limit:  1000,
	}
	allCompleted, err := eventStore.ListExecutions(ctx, filter)
	if err != nil {
		return fmt.Errorf("error listing executions: %v", err)
	}

	var toDelete []*environment.ExecutionSnapshot
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
		if !confirmAction("delete", fmt.Sprintf("these %d executions", len(toDelete))) {
			fmt.Println("❌ Cleanup cancelled.")
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

func resumeExecution(executionID, databasePath string, logLevel slogger.LogLevel) error {
	ctx := context.Background()
	startTime := time.Now()

	eventStore, err := getEventStore(databasePath)
	if err != nil {
		return err
	}

	snapshot, err := eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return fmt.Errorf("error getting execution snapshot: %v", err)
	}

	if snapshot.Status == "completed" {
		return fmt.Errorf("❌ Cannot resume a completed execution\n\n💡 Tip: Use 'dive executions show %s' to view the results", executionID)
	}

	// For now, we need to load the workflow from the original path.
	// This assumes the file is still available at the same location.
	// A future improvement could be to store the workflow definition itself.
	if snapshot.WorkflowPath == "" {
		return fmt.Errorf("❌ Cannot resume execution: workflow path not recorded\n\n💡 This execution was created with an older version of Dive.\n   Try running the workflow again with 'dive run <workflow-path>'%s", suggestWorkflowPaths())
	}

	fi, err := os.Stat(snapshot.WorkflowPath)
	if err != nil {
		return fmt.Errorf("❌ Cannot access workflow file '%s': %v\n\n💡 The workflow file may have been moved or deleted.\n   Make sure the file exists at the original location, or run the workflow again%s", snapshot.WorkflowPath, err, suggestWorkflowPaths())
	}

	configDir := snapshot.WorkflowPath
	basePath := ""
	if !fi.IsDir() {
		configDir = filepath.Dir(snapshot.WorkflowPath)
		basePath = filepath.Dir(snapshot.WorkflowPath)
	} else {
		basePath = snapshot.WorkflowPath
	}

	var logger slogger.Logger
	buildOpts := []config.BuildOption{}
	if logLevel != 0 {
		logger = slogger.New(logLevel)
		buildOpts = append(buildOpts, config.WithLogger(logger))
	}
	env, err := config.LoadDirectory(configDir, append(buildOpts, config.WithBasePath(basePath))...)
	if err != nil {
		return fmt.Errorf("error loading environment from '%s': %v", configDir, err)
	}
	if err := env.Start(ctx); err != nil {
		return fmt.Errorf("error starting environment: %v", err)
	}
	defer env.Stop(ctx)

	wf, err := env.GetWorkflow(snapshot.WorkflowName)
	if err != nil {
		return fmt.Errorf("❌ Workflow '%s' not found: %v\n\n💡 The workflow may have been renamed or removed since this execution was created", snapshot.WorkflowName, err)
	}

	// Validate that the workflow hasn't changed significantly since the execution was started
	hasher := environment.NewBasicWorkflowHasher()
	workflowHash, err := hasher.HashWorkflow(wf)
	if err != nil {
		fmt.Printf("⚠️  Warning: Unable to verify if workflow has changed: %v\n", err)
	} else if snapshot.WorkflowHash != "" && workflowHash != snapshot.WorkflowHash {
		fmt.Printf("⚠️  Warning: The workflow definition has changed since this execution was started.\n")
		fmt.Printf("   Original hash: %s\n", snapshot.WorkflowHash)
		fmt.Printf("   Current hash:  %s\n", workflowHash)
		fmt.Printf("\n")
		if !confirmAction("resume with changed workflow", "this execution") {
			return fmt.Errorf("❌ Resume cancelled due to workflow changes")
		}
	}

	formatter := NewWorkflowFormatter()
	fmt.Printf("🔄 Resuming execution %s for workflow %s...\n", executionID, wf.Name())

	execution, err := environment.NewExecution(environment.ExecutionOptions{
		Workflow:        wf,
		Environment:     env,
		Inputs:          snapshot.Inputs,
		EventStore:      eventStore,
		Logger:          logger,
		ReplayMode:      true,
		ExecutionID:     executionID,
		Formatter:       formatter,
		InitialSnapshot: snapshot,
	})
	if err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		return fmt.Errorf("error creating execution for resume: %v", err)
	}

	formatter.PrintExecutionID(execution.ID())

	if err := execution.Run(ctx); err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		formatter.PrintExecutionNextSteps(execution.ID())
		return fmt.Errorf("error resuming workflow: %v", err)
	}

	duration := time.Since(startTime)
	formatter.PrintWorkflowComplete(duration)

	return nil
}

func showExecutionStats(databaseFlag string) error {
	eventStore, err := getEventStore(databaseFlag)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Get all executions to calculate statistics
	executions, err := eventStore.ListExecutions(ctx, environment.ExecutionFilter{
		Limit: 1000, // Get a reasonable number for stats
	})
	if err != nil {
		return fmt.Errorf("error listing executions: %v", err)
	}

	if len(executions) == 0 {
		fmt.Println("📊 No executions found.")
		return nil
	}

	// Calculate statistics
	statusCounts := make(map[string]int)
	workflowCounts := make(map[string]int)
	var totalDuration time.Duration
	var completedCount int

	for _, exec := range executions {
		statusCounts[exec.Status]++
		workflowCounts[exec.WorkflowName]++

		if !exec.EndTime.IsZero() && !exec.StartTime.IsZero() {
			totalDuration += exec.EndTime.Sub(exec.StartTime)
			completedCount++
		}
	}

	// Print statistics
	fmt.Printf("📊 Execution Statistics\n\n")

	fmt.Printf("📈 Total Executions: %d\n", len(executions))

	fmt.Printf("\n📋 Status Breakdown:\n")
	for status, count := range statusCounts {
		percentage := float64(count) / float64(len(executions)) * 100
		fmt.Printf("   %s: %d (%.1f%%)\n", formatExecutionStatus(status), count, percentage)
	}

	if completedCount > 0 {
		avgDuration := totalDuration / time.Duration(completedCount)
		fmt.Printf("\n⏱️  Average Duration: %s\n", avgDuration.Round(time.Millisecond))
	}

	fmt.Printf("\n🔧 Most Used Workflows:\n")
	// Sort workflows by usage count
	type workflowUsage struct {
		name  string
		count int
	}
	var workflows []workflowUsage
	for name, count := range workflowCounts {
		workflows = append(workflows, workflowUsage{name, count})
	}

	// Simple sort by count (descending)
	for i := 0; i < len(workflows)-1; i++ {
		for j := i + 1; j < len(workflows); j++ {
			if workflows[j].count > workflows[i].count {
				workflows[i], workflows[j] = workflows[j], workflows[i]
			}
		}
	}

	// Show top 5 workflows
	limit := 5
	if len(workflows) < limit {
		limit = len(workflows)
	}
	for i := 0; i < limit; i++ {
		wf := workflows[i]
		percentage := float64(wf.count) / float64(len(executions)) * 100
		fmt.Printf("   %s: %d executions (%.1f%%)\n", wf.name, wf.count, percentage)
	}

	return nil
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
		databasePath, _ := cmd.Flags().GetString("database")
		status, _ := cmd.Flags().GetString("status")
		workflowName, _ := cmd.Flags().GetString("workflow")
		limit, _ := cmd.Flags().GetInt("limit")

		if err := listExecutions(databasePath, status, workflowName, limit); err != nil {
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
		databasePath, _ := cmd.Flags().GetString("database")
		showEvents, _ := cmd.Flags().GetBool("events")

		if err := showExecution(databasePath, executionID, showEvents); err != nil {
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
		databasePath, _ := cmd.Flags().GetString("database")
		confirm, _ := cmd.Flags().GetBool("yes")

		if err := deleteExecution(databasePath, executionID, confirm); err != nil {
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
		databasePath, _ := cmd.Flags().GetString("database")
		olderThanDays, _ := cmd.Flags().GetInt("older-than")
		confirm, _ := cmd.Flags().GetBool("yes")

		if err := cleanupExecutions(databasePath, olderThanDays, confirm); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

// Resume execution
var resumeCmd = &cobra.Command{
	Use:   "resume <execution-id>",
	Short: "Resume a failed or pending execution",
	Long:  "Resumes a workflow from its last known state based on the execution history",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		executionID := args[0]
		databasePath, _ := cmd.Flags().GetString("database")

		if err := resumeExecution(executionID, databasePath, getLogLevel()); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

// Stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show execution statistics",
	Long:  "Display statistical information about workflow executions including status breakdown and usage patterns",
	Run: func(cmd *cobra.Command, args []string) {
		databasePath, _ := cmd.Flags().GetString("database")

		if err := showExecutionStats(databasePath); err != nil {
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
	executionsCmd.AddCommand(resumeCmd)
	executionsCmd.AddCommand(statsCmd)

	// Global flags for all execution commands
	executionsCmd.PersistentFlags().String("database", "", "Path to SQLite database (default: ~/.dive/executions.db)")

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
