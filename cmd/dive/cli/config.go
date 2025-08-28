package cli

import (
	"fmt"
	"os"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Work with Dive configurations",
	Long:  "Work with Dive configurations",
}

var checkCmd = &cobra.Command{
	Use:   "check [file]",
	Short: "Validate a Dive configuration",
	Long:  "Validate a Dive configuration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		env, err := config.LoadDirectory(args[0])
		if err != nil {
			fmt.Printf("❌ %s\n", errorStyle.Sprint(err))
			os.Exit(1)
		}
		fmt.Printf("✅ %q is valid\n", env.Name())
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(checkCmd)
}
