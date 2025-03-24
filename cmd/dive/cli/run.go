package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/getstingrai/dive/config"
	"github.com/getstingrai/dive/slogger"
	"github.com/spf13/cobra"
)

func runWorkflow(filePath string, logLevel string) error {
	ctx := context.Background()

	// Build environment from config file
	buildOpts := []config.BuildOption{}
	if logLevel != "" {
		buildOpts = append(buildOpts, config.WithLogger(slogger.New(slogger.LevelFromString(logLevel))))
	}

	env, err := config.LoadDirectory(filePath, buildOpts...)
	if err != nil {
		return fmt.Errorf("error loading environment: %v", err)
	}

	// Get the default workflow
	workflows := env.Workflows()
	if len(workflows) == 0 {
		return fmt.Errorf("no workflows defined")
	}
	workflow := workflows[0]

	// Execute the workflow
	execution, err := env.ExecuteWorkflow(ctx, workflow.Name(), map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("error executing workflow: %v", err)
	}

	if err := execution.Wait(); err != nil {
		return fmt.Errorf("error waiting for workflow: %v", err)
	}

	// // Monitor execution events
	// for event := range execution.Events() {
	// 	switch e := event.(type) {
	// 	case *dive.StepStartEvent:
	// 		fmt.Printf("\nStarting step: %s\n", boldStyle.Sprint(e.Name))
	// 	case *dive.StepCompleteEvent:
	// 		fmt.Printf("\nCompleted step: %s\n", boldStyle.Sprint(e.Name))
	// 		if e.Result != nil {
	// 			fmt.Printf("\nResult:\n%s\n", e.Result)
	// 		}
	// 	case *dive.StepErrorEvent:
	// 		fmt.Printf("\nStep failed: %s\nError: %v\n", errorStyle.Sprint(e.Name), e.Error)
	// 	}
	// }

	return nil
}

var runCmd = &cobra.Command{
	Use:   "run [file]",
	Short: "Run a workflow",
	Long:  `Run a workflow defined in a YAML file`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		logLevel, err := cmd.Flags().GetString("log-level")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if err := runWorkflow(filePath, logLevel); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("log-level", "", "", "Log level to use (debug, info, warn, error)")
}
