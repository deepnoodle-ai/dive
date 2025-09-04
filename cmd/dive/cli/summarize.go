package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/spf13/cobra"
)

// summarizationPrompts returns system prompts for different summary lengths
func summarizationPrompts(length string) string {
	switch length {
	case "short":
		return "You are a concise summarization assistant. Create a brief, focused summary that captures only the most essential points. Aim for 1-3 sentences that distill the core message or findings. Be direct and eliminate any redundancy."
	case "medium":
		return "You are a balanced summarization assistant. Create a well-structured summary that covers the main points while maintaining important context and details. Aim for a paragraph or two that provides a comprehensive overview without being verbose."
	case "long":
		return "You are a detailed summarization assistant. Create a thorough summary that preserves important details, context, and nuances while organizing the information clearly. Include key supporting points and maintain the structure of the original content."
	default:
		return "You are a summarization assistant. Create a clear, well-organized summary of the provided text that captures the main points and key details."
	}
}

// runSummarize executes the summarization process
func runSummarize(length string) error {
	ctx := context.Background()

	// Read input from stdin
	input, err := readStdin()
	if err != nil {
		return fmt.Errorf("failed to read input: %v", err)
	}

	if input == "" {
		return fmt.Errorf("no input provided - please pipe text to this command (e.g., cat document.txt | dive summarize)")
	}

	// Get the model using global flags
	llmInstance, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	// Create the summarization prompt
	systemPrompt := summarizationPrompts(length)
	userPrompt := fmt.Sprintf("Please summarize the following text:\n\n%s", input)

	// Generate the summary
	opts := []llm.Option{
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userPrompt),
	}

	if streamingLLM, ok := llmInstance.(llm.StreamingLLM); ok {
		// Use streaming if available for better user experience
		_, err = streamLLM(ctx, streamingLLM, opts...)
	} else {
		// Fall back to non-streaming
		response, err := generateLLM(ctx, llmInstance, opts...)
		if err == nil && len(response.Content) > 0 {
			if textContent, ok := response.Content[0].(*llm.TextContent); ok {
				fmt.Print(textContent.Text)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("error generating summary: %v", err)
	}

	fmt.Println() // Add a final newline
	return nil
}

var summarizeCmd = &cobra.Command{
	Use:   "summarize",
	Short: "Summarize text from stdin using AI",
	Long:  "Reads text from standard input and generates a summary using the specified AI model. Ideal for unix-style processing pipelines.",
	Example: `  # Summarize a document
  cat document.txt | dive summarize

  # Create a short summary with a specific model
  cat report.md | dive summarize --length short --model claude-sonnet-4

  # Summarize with custom provider and model
  echo "Long text here..." | dive summarize --length long --provider openai --model gpt-4`,
	Run: func(cmd *cobra.Command, args []string) {
		length, err := cmd.Flags().GetString("length")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		if err := runSummarize(length); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(summarizeCmd)

	summarizeCmd.Flags().StringP("length", "", "medium", "Summary length: short, medium, or long")
}
