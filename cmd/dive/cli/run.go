package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func runWorkflow(path, workflowName string, logLevel slogger.LogLevel, showStepOutput bool) error {
	ctx := context.Background()

	// Check if path is a directory or file
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error accessing path: %v", err)
	}

	configDir := path

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
	}

	buildOpts := []config.BuildOption{}
	if logLevel != 0 {
		logger := slogger.New(logLevel)
		buildOpts = append(buildOpts, config.WithLogger(logger))
	}
	env, err := config.LoadDirectory(configDir, buildOpts...)
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

	execution, err := env.ExecuteWorkflow(ctx, environment.ExecutionOptions{
		WorkflowName:   workflowName,
		Inputs:         getUserVariables(),
		ShowStepOutput: showStepOutput,
	})
	if err != nil {
		return fmt.Errorf("error executing workflow: %v", err)
	}
	if err := execution.Wait(); err != nil {
		return fmt.Errorf("error waiting for workflow: %v", err)
	}
	return nil
}

var runCmd = &cobra.Command{
	Use:   "run [file or directory]",
	Short: "Run a workflow",
	Long:  "Run a workflow",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		workflowName, err := cmd.Flags().GetString("workflow")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		showStepOutput, err := cmd.Flags().GetBool("show-step-output")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if err := runWorkflow(filePath, workflowName, getLogLevel(), showStepOutput); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringP("workflow", "w", "", "Name of the workflow to run")
	runCmd.Flags().BoolP("show-step-output", "", false, "Show output from workflow steps")
}
