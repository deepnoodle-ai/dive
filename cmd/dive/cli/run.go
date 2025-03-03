package cli

import (
	"context"
	"fmt"

	"github.com/getstingrai/dive/slogger"
	"github.com/getstingrai/dive/teamconf"
	"github.com/spf13/cobra"
)

func runTeam(filePath string, logLevel string) error {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slogger.New(slogger.LevelFromString(logLevel))

	teamConf, err := teamconf.LoadFile(filePath, getUserVariables())
	if err != nil {
		return fmt.Errorf("error loading team: %v", err)
	}

	team, err := teamConf.Build(teamconf.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("error building team: %v", err)
	}

	if err := team.Start(ctx); err != nil {
		return fmt.Errorf("error starting team: %v", err)
	}
	defer team.Stop(ctx)

	fmt.Printf("Running team %s...\n", boldStyle.Sprint(team.Name()))
	fmt.Println()

	stream, err := team.Work(ctx)
	if err != nil {
		return fmt.Errorf("error running team: %v", err)
	}

	for event := range stream.Channel() {
		switch event.Type {
		case "task.result":
			resultText := "\n" + boldStyle.Sprint(event.TaskName+":") + "\n" + event.TaskResult.Content
			fmt.Println(resultText)
		case "task.error":
			fmt.Printf("Error: %s\n", event.Error)
		}
	}

	return nil
}

var runCmd = &cobra.Command{
	Use:   "run [file]",
	Short: "Run a team",
	Long:  `Run a team`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		logLevel, err := cmd.Flags().GetString("log-level")
		if err != nil {
			return fmt.Errorf("error getting log level: %v", err)
		}
		return runTeam(filePath, logLevel)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("log-level", "", "info", "Log level to use (debug, info, warn, error)")
}
