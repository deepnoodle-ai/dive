package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/getstingrai/dive/teamconf"
	"github.com/spf13/cobra"
)

func runTeam(filePath string, logLevel string) error {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	teamConf, err := teamconf.LoadFile(filePath, getUserVariables())
	if err != nil {
		return fmt.Errorf("error loading team: %v", err)
	}

	if logLevel != "" {
		teamConf.Config.LogLevel = logLevel
	}

	team, err := teamConf.Build()
	if err != nil {
		return fmt.Errorf("error building team: %v", err)
	}

	if err := team.Start(ctx); err != nil {
		return fmt.Errorf("error starting team: %v", err)
	}
	defer team.Stop(ctx)

	fmt.Printf("Running %s\n", boldStyle.Sprint(team.Name()))
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
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		logLevel, err := cmd.Flags().GetString("log-level")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if err := runTeam(filePath, logLevel); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("log-level", "", "info", "Log level to use (debug, info, warn, error)")
}
