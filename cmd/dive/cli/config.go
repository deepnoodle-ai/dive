package cli

import (
	"context"
	"fmt"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/slogger"
	"github.com/spf13/cobra"
)

// If there's a Bubbletea implementation in config.go, we would replace it with something like:

func showConfig() error {
	// Print title
	fmt.Println(titleStyle.Render(" Configuration "))

	// Print configuration details
	// ... implementation details ...

	return nil
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display configuration information",
	Long:  `Display configuration information for the Dive CLI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showConfig()
	},
}

// checkCmd represents the check subcommand of config
var checkCmd = &cobra.Command{
	Use:   "check [file]",
	Short: "Validate an HCL team definition file",
	Long: `Validate an HCL team definition file.
This will check the syntax and structure of the file without executing any tasks.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		fmt.Printf("Validating team definition file: %s\n", filePath)

		// Create context
		ctx := context.Background()

		// Validate the team definition
		logger := slogger.New(slogger.LevelFromString("debug"))
		_, _, err := dive.LoadHCLTeam(ctx, filePath, nil, logger)
		if err != nil {
			return fmt.Errorf("validation failed: %v", err)
		}

		fmt.Println("Validation successful! The team definition is valid.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(checkCmd)
}
