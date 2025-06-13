package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/spf13/cobra"
)

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("error creating destination file: %v", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("error copying file: %v", err)
	}
	return nil
}

// getDiveConfigDir returns the dive configuration directory, creating it if it doesn't exist
func getDiveConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	diveDir := filepath.Join(homeDir, ".dive")
	if err := os.MkdirAll(diveDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create dive config directory: %w", err)
	}

	return diveDir, nil
}

func runWorkflow(path, workflowName string, logLevel slogger.LogLevel, persistenceDB string) error {
	ctx := context.Background()
	startTime := time.Now()

	// Check if path is a directory or file
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error accessing path: %v", err)
	}

	configDir := path
	basePath := ""

	// If a single file is provided, copy it to a temporary directory
	// and use that as the config directory.
	if !fi.IsDir() {
		tmpDir, err := os.MkdirTemp("", "dive-config-*")
		if err != nil {
			return fmt.Errorf("error creating temp directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		dst := filepath.Join(tmpDir, filepath.Base(path))
		if err := copyFile(path, dst); err != nil {
			return err
		}
		configDir = tmpDir
		// base path should be the original directory containing the file
		basePath = filepath.Dir(path)
	} else {
		basePath = path
	}

	buildOpts := []config.BuildOption{}
	if logLevel != 0 {
		logger := slogger.New(logLevel)
		buildOpts = append(buildOpts, config.WithLogger(logger))
	}
	env, err := config.LoadDirectory(configDir, append(buildOpts, config.WithBasePath(basePath))...)
	if err != nil {
		return fmt.Errorf("error loading environment: %v", err)
	}
	if err := env.Start(ctx); err != nil {
		return fmt.Errorf("error starting environment: %v", err)
	}
	defer env.Stop(ctx)

	if workflowName == "" {
		workflows := env.Workflows()
		if len(workflows) != 1 {
			return fmt.Errorf("you must specify a workflow name")
		}
		workflowName = workflows[0].Name()
	}

	// Get the workflow for display
	wf, err := env.GetWorkflow(workflowName)
	if err != nil {
		return fmt.Errorf("error getting workflow: %v", err)
	}

	// Create formatter and display workflow header
	formatter := NewWorkflowFormatter()
	formatter.PrintWorkflowHeader(wf, getUserVariables())

	// Set up persistence with SQLite event store
	dbPath := persistenceDB
	if dbPath == "" {
		diveDir, err := getDiveConfigDir()
		if err != nil {
			return fmt.Errorf("error getting dive config directory: %v", err)
		}
		dbPath = filepath.Join(diveDir, "executions.db")
	}

	// Create SQLite event store
	eventStore, err := workflow.NewSQLiteExecutionEventStore(dbPath, workflow.DefaultSQLiteStoreOptions())
	if err != nil {
		return fmt.Errorf("error creating event store: %v", err)
	}
	defer eventStore.Close()

	// Create execution orchestrator
	orchestrator := environment.NewExecutionOrchestrator(eventStore, env)

	// Create execution with persistence
	execution, err := orchestrator.CreateExecution(ctx, environment.ExecutionOptions{
		WorkflowName: workflowName,
		Inputs:       getUserVariables(),
		Formatter:    formatter,
	})
	if err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		return fmt.Errorf("error creating execution: %v", err)
	}

	fmt.Printf("🔄 Execution started (ID: %s)\n", execution.ID())
	if persistenceDB != "" {
		fmt.Printf("📄 Database: %s\n", dbPath)
	}

	// Start the execution
	if err := execution.Run(ctx); err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		return fmt.Errorf("error running workflow: %v", err)
	}

	if err := execution.Wait(); err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		return fmt.Errorf("error waiting for workflow: %v", err)
	}

	duration := time.Since(startTime)
	formatter.PrintWorkflowComplete(duration)
	formatter.PrintExecutionStats(execution.GetStats())
	return nil
}

var runCmd = &cobra.Command{
	Use:   "run [file or directory]",
	Short: "Run a workflow",
	Long:  "Run a workflow with automatic persistence for retry and recovery",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		workflowName, err := cmd.Flags().GetString("workflow")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		persistenceDB, err := cmd.Flags().GetString("persist-db")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		if err := runWorkflow(filePath, workflowName, getLogLevel(), persistenceDB); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringP("workflow", "w", "", "Name of the workflow to run")
	runCmd.Flags().String("persist-db", "", "Path to SQLite database for persistence (default: ~/.dive/executions.db)")
}
