package cli

import (
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/log"
	"github.com/deepnoodle-ai/wonton/cli"
)

var (
	userVarFlags  []string
	userVariables map[string]interface{}
	llmProvider   string
	llmModel      string
	logLevel      string
	app           *cli.App
)

func getLogLevel() log.Level {
	return log.LevelFromString(logLevel)
}

func Execute() {
	app = cli.New("dive").
		Description("Dive runs AI agent workflows").
		Version("1.0.0").
		GlobalFlags(
			cli.String("provider", "").
				Env("DIVE_PROVIDER").
				Help("LLM provider to use (e.g., 'anthropic', 'openai', 'openrouter', 'groq', 'grok', 'mistral', 'ollama', 'google')"),
			cli.String("model", "m").
				Env("DIVE_MODEL").
				Help("Model to use (e.g. 'claude-sonnet-4-20250514')"),
			cli.Strings("var", "").
				Help("Set a variable (format: key=value). Can be specified multiple times"),
			cli.String("log-level", "").
				Default("warn").
				Help("Log level to use (none, debug, info, warn, error)"),
		)

	// Register all commands
	registerAskCommand(app)
	registerChatCommand(app)
	registerClassifyCommand(app)
	registerCompareCommand(app)
	registerConfigCommand(app)
	registerDiffCommand(app)
	registerEmbedCommand(app)
	registerExtractCommand(app)
	registerImageCommand(app)
	registerLLMCommand(app)
	registerMCPCommand(app)
	registerSummarizeCommand(app)
	registerThreadsCommand(app)
	registerVideoCommand(app)
	registerWatchCommand(app)

	if err := app.Execute(); err != nil {
		if cli.IsHelpRequested(err) {
			os.Exit(0)
		}
		os.Exit(cli.GetExitCode(err))
	}
}

// parseGlobalFlags extracts global flag values from context
func parseGlobalFlags(ctx *cli.Context) {
	llmProvider = ctx.String("provider")
	llmModel = ctx.String("model")
	logLevel = ctx.String("log-level")
	userVarFlags = ctx.Strings("var")

	// Parse user variables
	userVariables = make(map[string]interface{}, len(userVarFlags))
	for _, v := range userVarFlags {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			userVariables[parts[0]] = parts[1]
		}
	}
}
