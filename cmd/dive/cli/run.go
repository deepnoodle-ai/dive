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

func runWorkflow(path, workflowName string, logLevel slogger.LogLevel, databasePath string) error {
	ctx := context.Background()
	startTime := time.Now()

	// Check if path is a directory or file
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("❌ Cannot access workflow path '%s': %v%s", path, err, suggestWorkflowPaths())
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

	var logger slogger.Logger
	buildOpts := []config.BuildOption{}
	if logLevel != 0 {
		logger = slogger.New(logLevel)
		buildOpts = append(buildOpts, config.WithLogger(logger))
	}
	env, err := config.LoadDirectory(configDir, append(buildOpts, config.WithBasePath(basePath))...)
	if err != nil {
		return fmt.Errorf("❌ Failed to load workflow configuration: %v\n\n💡 Check that your YAML syntax is correct and all required fields are present", err)
	}
	if err := env.Start(ctx); err != nil {
		return fmt.Errorf("error starting environment: %v", err)
	}
	defer env.Stop(ctx)

	if workflowName == "" {
		workflows := env.Workflows()
		if len(workflows) != 1 {
			if len(workflows) == 0 {
				return fmt.Errorf("❌ No workflows found in the configuration\n\n💡 Make sure your YAML file contains a workflow definition")
			}
			var workflowNames []string
			for _, wf := range workflows {
				workflowNames = append(workflowNames, wf.Name())
			}
			return fmt.Errorf("❌ Multiple workflows found. Specify which one to run with --workflow:\n   Available workflows: %v", workflowNames)
		}
		workflowName = workflows[0].Name()
	}

	wf, err := env.GetWorkflow(workflowName)
	if err != nil {
		workflows := env.Workflows()
		var workflowNames []string
		for _, wf := range workflows {
			workflowNames = append(workflowNames, wf.Name())
		}
		if len(workflowNames) > 0 {
			return fmt.Errorf("❌ Workflow '%s' not found\n\n💡 Available workflows: %v", workflowName, workflowNames)
		}
		return fmt.Errorf("❌ Workflow '%s' not found: %v", workflowName, err)
	}

	formatter := NewWorkflowFormatter()
	formatter.PrintWorkflowHeader(wf, getUserVariables())

	// Create a new event store. We don't use getEventStore because we want to
	// be able to create the database if it doesn't exist.
	dbPath, err := getDatabasePath(databasePath)
	if err != nil {
		return fmt.Errorf("error getting database path: %v", err)
	}
	eventStore, err := environment.NewSQLiteExecutionEventStore(dbPath, environment.DefaultSQLiteStoreOptions())
	if err != nil {
		return fmt.Errorf("error creating event store: %v", err)
	}
	defer eventStore.Close()

	execution, err := environment.NewExecution(environment.ExecutionOptions{
		Workflow:    wf,
		Environment: env,
		Inputs:      getUserVariables(),
		EventStore:  eventStore,
		Logger:      logger,
		ReplayMode:  false,
		Formatter:   formatter,
	})
	if err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		return fmt.Errorf("error creating execution: %v", err)
	}

	formatter.PrintExecutionID(execution.ID())

	if err := execution.Run(ctx); err != nil {
		duration := time.Since(startTime)
		formatter.PrintWorkflowError(err, duration)
		formatter.PrintExecutionNextSteps(execution.ID())
		return fmt.Errorf("error running workflow: %v", err)
	}

	duration := time.Since(startTime)
	formatter.PrintWorkflowComplete(duration)
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
		databasePath, err := cmd.Flags().GetString("database")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if err := runWorkflow(filePath, workflowName, getLogLevel(), databasePath); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringP("workflow", "w", "", "Name of the workflow to run")
	runCmd.Flags().String("database", "", "Path to SQLite database (default: ~/.dive/executions.db)")
}
