package cli

import (
	"fmt"

	"github.com/getstingrai/dive/teamconf"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Validate Dive configuration files",
	Long:  `Validate Dive configuration files.`,
}

var checkCmd = &cobra.Command{
	Use:   "check [file]",
	Short: "Validate a Dive team definition file",
	Long:  `Validate a Dive team definition file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conf, err := teamconf.LoadFile(args[0])
		if err != nil {
			return fmt.Errorf("validation failed: %v", err)
		}
		fmt.Printf("Validation successful! Team %q is valid.\n", conf.Name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(checkCmd)
}
