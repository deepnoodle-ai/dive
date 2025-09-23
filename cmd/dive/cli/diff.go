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
	"github.com/deepnoodle-ai/dive/log"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [old_file] [new_file]",
	Short: "AI-powered semantic diff between files",
	Long: `AI-powered semantic diff that analyzes and explains changes between two files.

This command generates a unified diff and uses an LLM to provide semantic analysis,
explaining what has changed in natural language with context and significance.

Files can be regular files or "-" to read from stdin.

Examples:
  dive diff old.txt new.txt                    # Analyze changes with AI
  dive diff - new.txt                          # Compare stdin with file
  dive diff old.txt new.txt --format markdown  # Output in markdown format
  dive diff old.txt new.txt --provider openai  # Use specific LLM provider
  dive diff old.txt new.txt --context 5        # Include 5 lines of context in diff`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		oldFile := args[0]
		newFile := args[1]

		// Get output format
		outputFormat, err := cmd.Flags().GetString("format")
		if err != nil {
			return fmt.Errorf("error getting format flag: %w", err)
		}

		// Get context lines for diff
		contextLines, err := cmd.Flags().GetInt("context")
		if err != nil {
			return fmt.Errorf("error getting context flag: %w", err)
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

		// Run the diff with AI analysis
		return runDiff(ctx, oldFile, newFile, outputFormat, contextLines, provider, model)
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)

	diffCmd.Flags().StringP("format", "f", "text", "Output format (text, markdown, json)")
	diffCmd.Flags().IntP("context", "c", 3, "Number of context lines to show around changes in diff")
}

func runDiff(ctx context.Context, oldFile, newFile string, outputFormat string, contextLines int, provider, modelName string) error {
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

	// Generate unified diff
	diff := generateUnifiedDiff(oldContent, newContent, oldFile, newFile, contextLines)
	if diff == "" {
		fmt.Println(successStyle.Sprint("✓ Files are functionally identical (only whitespace differences)"))
		return nil
	}

	// Show basic diff information
	fmt.Printf("Analyzing differences between:\n")
	fmt.Printf("  Old: %s (%d lines)\n", oldFile, countLines(oldContent))
	fmt.Printf("  New: %s (%d lines)\n", newFile, countLines(newContent))
	fmt.Println()

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

	// Perform semantic diff analysis using the unified diff
	return performSemanticDiff(ctx, diffAgent, diff, oldFile, newFile, outputFormat)
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
	logger := log.New(getLogLevel())

	agentOpts := agent.Options{
		Name:            "DiffAnalyzer",
		Goal:            "Analyze and explain semantic differences between text files",
		Instructions:    buildDiffSystemPrompt(),
		Model:           model,
		Tools:           []dive.Tool{}, // No external tools needed for diff analysis
		Logger:          logger,
		ResponseTimeout: defaultDiffTimeout,
		ModelSettings: &agent.ModelSettings{
			Temperature: floatPtr(0.1), // Low temperature for consistent analysis
			MaxTokens:   intPtr(4000),  // Sufficient for detailed analysis
		},
	}

	return agent.New(agentOpts)
}

func buildDiffSystemPrompt() string {
	return `You are an expert diff analyzer. You will be provided with a unified diff (git-style) and your task is to explain the changes in a meaningful way.

When analyzing the diff:

1. **Understand the Format**: Lines starting with '+' are additions, '-' are deletions, ' ' are unchanged context lines
2. **Content Analysis**: Explain what was added, removed, or modified and why it matters
3. **Semantic Impact**: Describe the functional or logical impact of the changes
4. **Pattern Recognition**: Identify patterns in the changes (refactoring, bug fixes, feature additions, etc.)
5. **Code Quality**: If applicable, comment on improvements or potential issues introduced

Provide your analysis in a clear, structured format that helps users understand:
- The nature and purpose of the changes
- The scope and significance of modifications
- Any potential implications or side effects
- Recommendations if you spot issues or opportunities for improvement

Focus on the meaningful aspects of the changes rather than just restating the diff.
Be concise but thorough, and tailor your explanation to the type of content being diffed.`
}

func performSemanticDiff(ctx context.Context, diffAgent *agent.Agent, unifiedDiff, oldFile, newFile string, outputFormat string) error {
	// Prepare the prompt for the LLM
	prompt := buildDiffPrompt(unifiedDiff, oldFile, newFile, outputFormat)

	fmt.Println("Analyzing semantic differences...")
	fmt.Println()

	_, err := diffAgent.CreateResponse(ctx,
		dive.WithInput(prompt),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent {
				payload := item.Event
				if payload.Delta != nil {
					if payload.Delta.Text != "" {
						fmt.Print(successStyle.Sprint(payload.Delta.Text))
					} else if payload.Delta.Thinking != "" {
						fmt.Print(thinkingStyle.Sprint(payload.Delta.Thinking))
					}
				}
			}
			return nil
		}),
	)

	if err != nil {
		// Check if it's an authentication error and provide helpful message
		if strings.Contains(err.Error(), "authentication_error") || strings.Contains(err.Error(), "x-api-key header is required") {
			fmt.Println(errorStyle.Sprint("❌ Authentication required"))
			fmt.Println("💡 Set your API key using one of these methods:")
			fmt.Printf("   • Environment variable: export ANTHROPIC_API_KEY=your_key_here\n")
			fmt.Printf("   • For other providers, use: --provider openai (and set OPENAI_API_KEY)\n")
			fmt.Printf("   • See available providers: dive llm --help\n")
			return nil
		}
		return fmt.Errorf("error generating diff analysis: %w", err)
	}

	fmt.Println()
	return nil
}

func buildDiffPrompt(unifiedDiff, oldFile, newFile string, outputFormat string) string {
	var formatInstruction string
	switch outputFormat {
	case "markdown":
		formatInstruction = "Format your response using Markdown with clear headings, bullet points, and code blocks where appropriate."
	case "json":
		formatInstruction = "Format your response as a JSON object with fields: summary (string), changes (array of objects with 'type', 'description', 'impact'), patterns (array of strings), recommendations (array of strings)."
	default:
		formatInstruction = "Format your response as clear, readable text with appropriate sections."
	}

	return fmt.Sprintf(`Analyze this unified diff between two files and explain the changes:

Files being compared:
- Original: %s
- Modified: %s

Unified Diff:
%s

%s

Provide a comprehensive analysis that helps understand:
1. What changed and why it matters
2. The overall purpose/theme of the changes
3. Any potential issues or improvements
4. The significance and impact of the modifications`,
		oldFile, newFile, unifiedDiff, formatInstruction)
}

// generateUnifiedDiff creates a git-style unified diff between two strings
func generateUnifiedDiff(oldContent, newContent, oldFile, newFile string, contextLines int) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Generate the unified diff
	diff := difflib.UnifiedDiff{
		A:        oldLines,
		B:        newLines,
		FromFile: oldFile,
		ToFile:   newFile,
		FromDate: "original",
		ToDate:   "modified",
		Context:  contextLines,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		// If there's an error generating the diff, return a simple comparison
		return fmt.Sprintf("Error generating diff: %v\n\nFile sizes: old=%d bytes, new=%d bytes",
			err, len(oldContent), len(newContent))
	}

	return result
}

// countLines counts the number of lines in a string
func countLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Split(s, "\n")
	return len(lines)
}

// Helper functions
func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

var defaultDiffTimeout = time.Minute * 5
