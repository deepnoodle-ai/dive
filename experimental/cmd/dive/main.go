package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/firecrawl"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/google"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/kagi"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/fetch"
)

func main() {
	app := cli.New("dive").
		Description("Interactive AI assistant for coding tasks").
		Version("0.1.0")

	app.Main().
		Flags(
			cli.String("model", "m").
				Env("DIVE_MODEL").
				Help("Model to use (auto-detected from available API keys if not specified)"),
			cli.String("workspace", "w").
				Default("").
				Help("Workspace directory (defaults to current directory)"),
			cli.Float("temperature", "t").
				Env("DIVE_TEMPERATURE").
				Help("Sampling temperature (0.0-1.0)"),
			cli.Int("max-tokens", "").
				Default(16000).
				Env("DIVE_MAX_TOKENS").
				Help("Maximum tokens in response"),
			cli.String("system-prompt", "").
				Default("").
				Help("System prompt to use for the session"),
			cli.Bool("print", "p").
				Default(false).
				Help("Print response and exit (useful for pipes)"),
			cli.String("output-format", "").
				Default("text").
				Help("Output format (only works with --print): \"text\" (default) or \"json\""),
			cli.String("api-endpoint", "").
				Default("").
				Env("DIVE_API_ENDPOINT").
				Help("Override the API endpoint URL for the provider"),
		).
		Run(runMain)

	if err := app.Execute(); err != nil {
		if cli.IsHelpRequested(err) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(cli.GetExitCode(err))
	}
}

func runMain(ctx *cli.Context) error {
	printMode := ctx.Bool("print")
	if printMode {
		return runPrint(ctx)
	}
	return fmt.Errorf("interactive mode not implemented yet")
}

func runPrint(ctx *cli.Context) error {
	outputFormat := ctx.String("output-format")

	// Get input from args or stdin
	input, err := getInput(ctx.Args())
	if err != nil {
		return fmt.Errorf("failed to get input: %w", err)
	}
	if input == "" {
		return fmt.Errorf("no input provided; provide a prompt as an argument or pipe content to stdin")
	}

	// Get workspace
	workspaceDir := ctx.String("workspace")
	if workspaceDir == "" {
		workspaceDir, _ = os.Getwd()
	}

	// Get model
	modelName := ctx.String("model")
	if modelName == "" {
		modelName = getDefaultModel()
	}

	// Build system prompt
	systemPrompt := ctx.String("system-prompt")
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf("You are Dive, an AI coding assistant working in: %s", workspaceDir)
	}

	// Create model
	model := createModel(modelName, ctx.String("api-endpoint"))

	// Create tools
	tools := createTools(workspaceDir)

	// Create agent
	temperature := ctx.Float64("temperature")
	maxTokens := ctx.Int("max-tokens")

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: systemPrompt,
		Model:        model,
		Tools:        tools,
		ModelSettings: &dive.ModelSettings{
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Run agent
	bgCtx := context.Background()

	if outputFormat == "json" {
		return runPrintJSON(bgCtx, agent, input)
	}
	return runPrintText(bgCtx, agent, input)
}

func getInput(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		return strings.TrimSpace(args[0]), nil
	}
	// Try stdin
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

func runPrintText(ctx context.Context, agent dive.Agent, input string) error {
	var outputText strings.Builder

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(input),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent && item.Event != nil {
				if item.Event.Delta != nil && item.Event.Delta.Text != "" {
					fmt.Print(item.Event.Delta.Text)
					outputText.WriteString(item.Event.Delta.Text)
				}
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	if outputText.Len() > 0 && !strings.HasSuffix(outputText.String(), "\n") {
		fmt.Println()
	} else if outputText.Len() == 0 {
		fmt.Println(resp.OutputText())
	}

	return nil
}

func runPrintJSON(ctx context.Context, agent dive.Agent, input string) error {
	result := map[string]interface{}{}

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(input),
	)
	if err != nil {
		result["error"] = err.Error()
	} else {
		result["output"] = resp.OutputText()
		if resp.Usage != nil {
			result["usage"] = resp.Usage
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func createTools(workspaceDir string) []dive.Tool {
	tools := []dive.Tool{
		// Read-only file tools
		toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewGlobTool(toolkit.GlobToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewGrepTool(toolkit.GrepToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewListDirectoryTool(toolkit.ListDirectoryToolOptions{
			WorkspaceDir: workspaceDir,
		}),

		// Write tools
		toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewEditTool(toolkit.EditToolOptions{
			WorkspaceDir: workspaceDir,
		}),
		toolkit.NewBashTool(toolkit.BashToolOptions{
			WorkspaceDir: workspaceDir,
		}),

		// User interaction
		toolkit.NewAskUserTool(toolkit.AskUserToolOptions{
			Dialog: &dive.AutoApproveDialog{}, // Auto-approve for now
		}),
	}

	// Add web fetch if available
	if firecrawlClient, err := firecrawl.New(); err == nil {
		tools = append(tools, toolkit.NewFetchTool(toolkit.FetchToolOptions{
			Fetcher: firecrawlClient,
		}))
	} else {
		tools = append(tools, toolkit.NewFetchTool(toolkit.FetchToolOptions{
			Fetcher: fetch.NewHTTPFetcher(fetch.HTTPFetcherOptions{}),
		}))
	}

	// Add web search if available
	if kagiClient, err := kagi.New(); err == nil {
		tools = append(tools, toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
			Searcher: kagiClient,
		}))
	} else if googleSearchClient, err := google.New(); err == nil {
		tools = append(tools, toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
			Searcher: googleSearchClient,
		}))
	}

	return tools
}

func getDefaultModel() string {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "claude-haiku-4-5"
	}
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return "gemini-3-flash-preview"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "gpt-5.2"
	}
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" {
		return "grok-code-fast-1"
	}
	if os.Getenv("MISTRAL_API_KEY") != "" {
		return "mistral-small-latest"
	}
	return "claude-haiku-4-5"
}
