package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [old_file] [new_file]",
	Short: "Semantic diff between texts using LLMs to explain changes",
	Long: `Semantic diff between texts using LLMs to explain changes, useful for output drift detection.

This command compares two text files and uses AI to provide a semantic analysis of the differences,
explaining what has changed in natural language rather than just showing line-by-line differences.

Files can be regular files or "-" to read from stdin.

Examples:
  dive diff old.txt new.txt                    # Basic size comparison
  dive diff old.txt new.txt --explain-changes # AI-powered semantic analysis
  dive diff - new.txt --explain-changes       # Compare stdin with file
  dive diff old.txt new.txt --explain-changes --format markdown
  dive diff old.txt new.txt --explain-changes --provider openai`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		
		oldFile := args[0]
		newFile := args[1]
		
		// Read the explain-changes flag
		explainChanges, err := cmd.Flags().GetBool("explain-changes")
		if err != nil {
			return fmt.Errorf("error getting explain-changes flag: %w", err)
		}
		
		// Get output format
		outputFormat, err := cmd.Flags().GetString("format")
		if err != nil {
			return fmt.Errorf("error getting format flag: %w", err)
		}
		
		// Get model configuration from global flags
		provider := llmProvider
		if provider == "" {
			provider = "anthropic" // Default provider
		}
		
		model := llmModel
		if model == "" {
			// Use default model for provider
		}
		
		// Run the diff
		return runDiff(ctx, oldFile, newFile, explainChanges, outputFormat, provider, model)
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
	
	diffCmd.Flags().Bool("explain-changes", false, "Provide detailed explanation of changes using AI analysis")
	diffCmd.Flags().StringP("format", "f", "text", "Output format (text, markdown, json)")
}

func runDiff(ctx context.Context, oldFile, newFile string, explainChanges bool, outputFormat, provider, modelName string) error {
	// Read file contents
	oldContent, err := readFileContent(oldFile)
	if err != nil {
		return fmt.Errorf("error reading old file %q: %w", oldFile, err)
	}
	
	newContent, err := readFileContent(newFile)
	if err != nil {
		return fmt.Errorf("error reading new file %q: %w", newFile, err)
	}
	
	// If files are identical, report that
	if oldContent == newContent {
		fmt.Println(successStyle.Sprint("✓ Files are identical - no changes detected"))
		return nil
	}
	
	// Show basic diff information
	fmt.Printf("%s Comparing files:\n", infoStyle.Sprint("📄"))
	fmt.Printf("  Old: %s\n", oldFile)
	fmt.Printf("  New: %s\n", newFile)
	fmt.Println()
	
	if !explainChanges {
		// Simple diff output without AI analysis
		fmt.Println(headerStyle.Sprint("Changes detected:"))
		fmt.Printf("Old file length: %d characters\n", len(oldContent))
		fmt.Printf("New file length: %d characters\n", len(newContent))
		
		if len(oldContent) != len(newContent) {
			diff := len(newContent) - len(oldContent)
			if diff > 0 {
				fmt.Printf("Size change: +%d characters\n", diff)
			} else {
				fmt.Printf("Size change: %d characters\n", diff)
			}
		}
		
		fmt.Println("\n💡 Use --explain-changes to get AI-powered semantic analysis of the differences")
		return nil
	}
	
	// Get LLM model for analysis
	model, err := config.GetModel(provider, modelName)
	if err != nil {
		return fmt.Errorf("error getting LLM model: %w", err)
	}
	
	// Create agent for diff analysis
	diffAgent, err := createDiffAgent(model)
	if err != nil {
		return fmt.Errorf("error creating diff agent: %w", err)
	}
	
	// Perform semantic diff analysis
	return performSemanticDiff(ctx, diffAgent, oldContent, newContent, oldFile, newFile, outputFormat)
}

func readFileContent(filePath string) (string, error) {
	// Handle stdin input
	if filePath == "-" {
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("error reading from stdin: %w", err)
		}
		return string(content), nil
	}
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", filePath)
	}
	
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}
	
	return string(content), nil
}

func createDiffAgent(model llm.LLM) (*agent.Agent, error) {
	logger := slogger.New(getLogLevel())
	
	agentOpts := agent.Options{
		Name:         "DiffAnalyzer",
		Goal:         "Analyze and explain semantic differences between text files",
		Instructions: buildDiffSystemPrompt(),
		Model:        model,
		Tools:        []dive.Tool{}, // No external tools needed for diff analysis
		Logger:       logger,
		ResponseTimeout: defaultDiffTimeout,
		ModelSettings: &agent.ModelSettings{
			Temperature: floatPtr(0.1), // Low temperature for consistent analysis
			MaxTokens:   intPtr(4000),  // Sufficient for detailed analysis
		},
	}
	
	return agent.New(agentOpts)
}

func buildDiffSystemPrompt() string {
	return `You are a semantic diff analyzer. Your job is to compare two text files and explain the meaningful differences between them.

When analyzing differences, focus on:

1. **Content Changes**: What information was added, removed, or modified
2. **Structural Changes**: How the organization or format changed
3. **Semantic Meaning**: What the changes mean in context
4. **Impact Assessment**: How significant these changes are

Provide your analysis in a clear, structured format that helps users understand:
- What changed and where
- Why the changes might be significant
- Any patterns or themes in the modifications

Be concise but thorough. Focus on meaningful differences rather than minor formatting changes unless they affect readability or structure significantly.

If the files are very similar with only minor changes, acknowledge this but still highlight what did change.
If the files are completely different, provide a high-level summary of the major differences.`
}

func performSemanticDiff(ctx context.Context, diffAgent *agent.Agent, oldContent, newContent, oldFile, newFile string, outputFormat string) error {
	// Prepare the prompt for the LLM
	prompt := buildDiffPrompt(oldContent, newContent, oldFile, newFile, outputFormat)
	
	fmt.Println(headerStyle.Sprint("🔍 Analyzing semantic differences..."))
	fmt.Println()
	
	// Stream the response
	stream, err := diffAgent.StreamResponse(ctx, dive.WithInput(prompt))
	if err != nil {
		return fmt.Errorf("error generating diff analysis: %w", err)
	}
	defer stream.Close()
	
	// Process the streaming response
	var incremental bool
	for stream.Next(ctx) {
		event := stream.Event()
		if event.Type == dive.EventTypeLLMEvent {
			incremental = true
			payload := event.Item.Event
			if payload.Delta != nil {
				if payload.Delta.Text != "" {
					fmt.Print(successStyle.Sprint(payload.Delta.Text))
				} else if payload.Delta.Thinking != "" {
					fmt.Print(thinkingStyle.Sprint(payload.Delta.Thinking))
				}
			}
		} else if event.Type == dive.EventTypeResponseCompleted {
			if !incremental {
				text := strings.TrimSpace(event.Response.OutputText())
				fmt.Println(successStyle.Sprint(text))
			}
		} else if event.Type == dive.EventTypeResponseFailed {
			// Handle failed response event
			if event.Error != nil {
				// Check if it's an authentication error and provide helpful message
				if strings.Contains(event.Error.Error(), "authentication_error") || strings.Contains(event.Error.Error(), "x-api-key header is required") {
					fmt.Println(errorStyle.Sprint("❌ Authentication required"))
					fmt.Println("💡 Set your API key using one of these methods:")
					fmt.Printf("   • Environment variable: export ANTHROPIC_API_KEY=your_key_here\n")
					fmt.Printf("   • For other providers, use: --provider openai (and set OPENAI_API_KEY)\n")
					fmt.Printf("   • See available providers: dive llm --help\n")
					return nil
				}
				return fmt.Errorf("diff analysis failed: %w", event.Error)
			}
		}
	}
	
	if err := stream.Err(); err != nil {
		return fmt.Errorf("error during diff analysis: %w", err)
	}
	
	fmt.Println() // Add final newline
	return nil
}

func buildDiffPrompt(oldContent, newContent, oldFile, newFile string, outputFormat string) string {
	var formatInstruction string
	switch outputFormat {
	case "markdown":
		formatInstruction = "Format your response using Markdown with clear headings and structure."
	case "json":
		formatInstruction = "Format your response as a JSON object with fields: summary, changes (array), impact_assessment."
	default:
		formatInstruction = "Format your response as clear, readable text."
	}
	
	return fmt.Sprintf(`Please analyze the semantic differences between these two files:

OLD FILE (%s):
%s

NEW FILE (%s):
%s

%s

Provide a comprehensive analysis of the differences, focusing on semantic meaning rather than just textual changes.`, 
		oldFile, oldContent, newFile, newContent, formatInstruction)
}

// Helper functions
func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

var defaultDiffTimeout = time.Minute * 5