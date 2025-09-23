package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/log"
	"github.com/spf13/cobra"
)

var (
	userVarFlags  []string
	userVariables map[string]interface{}
	llmProvider   string
	llmModel      string
	logLevel      string
)

func getLogLevel() log.Level {
	return log.LevelFromString(logLevel)
}

var rootCmd = &cobra.Command{
	Use:   "dive",
	Short: "Dive runs AI agent workflows.",
	Long:  "Dive runs AI agent workflows.",

	// Extract user-provided variables from --var flags.
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
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.PersistentFlags().StringVarP(
		&llmProvider, "provider", "", "",
		"LLM provider to use (e.g., 'anthropic', 'openai', 'openrouter', 'groq', 'grok', 'ollama', 'google')")

	rootCmd.PersistentFlags().StringVarP(
		&llmModel, "model", "m", "",
		"Model to use (e.g. 'claude-sonnet-4-20250514')")

	rootCmd.PersistentFlags().StringArrayVarP(
		&userVarFlags, "var", "", []string{},
		"Set a variable (format: key=value). Can be specified multiple times")

	rootCmd.PersistentFlags().StringVarP(
		&logLevel, "log-level", "", "warn",
		"Log level to use (none, debug, info, warn, error)")
}
