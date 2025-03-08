package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	userVarFlags  []string
	userVariables map[string]interface{}
	provider      string
	model         string
)

// getUserVariables returns the user variables for the Team, as set on the command line.
func getUserVariables() map[string]interface{} {
	return userVariables
}

var rootCmd = &cobra.Command{
	Use:   "dive",
	Short: "Dive runs teams of AI agents.",
	Long:  `Dive runs teams of AI agents.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		userVariables = make(map[string]interface{}, len(userVarFlags))
		for _, v := range userVarFlags {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				fmt.Printf("Warning: invalid variable format: %s\n", v)
				continue
			}
			userVariables[parts[0]] = parts[1]
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&provider, "provider", "", "",
		"LLM provider to use (e.g., 'anthropic', 'openai', 'groq')")

	rootCmd.PersistentFlags().StringVarP(
		&model, "model", "m", "",
		"Model to use (e.g. 'claude-3-7-sonnet-20250219')")

	rootCmd.PersistentFlags().StringArrayVarP(
		&userVarFlags, "var", "", []string{},
		"Set a variable (format: key=value). Can be specified multiple times")
}
