package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/embedding"
	"github.com/deepnoodle-ai/dive/llm/providers/openai"
	"github.com/spf13/cobra"
)

// OutputFormat defines the output format for embeddings
type EmbeddingOutputFormat string

const (
	EmbeddingOutputJSON   EmbeddingOutputFormat = "json"
	EmbeddingOutputVector EmbeddingOutputFormat = "vector"
)

func runEmbedding(model, outputFormat string, inputs []string) error {
	ctx := context.Background()

	// Create embedding provider
	provider := openai.NewEmbeddingProvider()

	// Set up embedding options
	var opts []embedding.Option
	opts = append(opts, embedding.WithInputs(inputs))

	if model != "" {
		opts = append(opts, embedding.WithModel(model))
	}

	// Always use float encoding format for consistency
	opts = append(opts, embedding.WithDimensions(1536))

	// Generate embedding
	response, err := provider.Embed(ctx, opts...)
	if err != nil {
		return fmt.Errorf("error generating embedding: %w", err)
	}

	// Output based on format
	switch EmbeddingOutputFormat(outputFormat) {
	case EmbeddingOutputJSON:
		// Output full JSON response
		jsonData, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling response to JSON: %w", err)
		}
		fmt.Println(string(jsonData))

	case EmbeddingOutputVector:
		// Output JSON representation of the vector(s)
		if len(response.Floats) > 0 {
			if len(response.Floats) == 1 {
				// Single vector: output as JSON array
				jsonData, err := json.Marshal(response.Floats[0])
				if err != nil {
					return fmt.Errorf("error marshaling float vector to JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			} else {
				// Multiple vectors: output as JSON array of arrays
				jsonData, err := json.Marshal(response.Floats)
				if err != nil {
					return fmt.Errorf("error marshaling float vectors to JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			}
		} else if len(response.Ints) > 0 {
			if len(response.Ints) == 1 {
				// Single vector: output as JSON array
				jsonData, err := json.Marshal(response.Ints[0])
				if err != nil {
					return fmt.Errorf("error marshaling int vector to JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			} else {
				// Multiple vectors: output as JSON array of arrays
				jsonData, err := json.Marshal(response.Ints)
				if err != nil {
					return fmt.Errorf("error marshaling int vectors to JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			}
		} else {
			return fmt.Errorf("no embeddings returned")
		}
	default:
		return fmt.Errorf("unsupported output format: %s (supported: json, vector)", outputFormat)
	}
	return nil
}

func readStdin() (string, error) {
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading from stdin: %w", err)
	}

	return strings.Join(lines, "\n"), nil
}

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate embeddings from text input",
	Long: `Generate embeddings from text input using OpenAI's embedding models.

The text input can be provided via the --text flag or through stdin.
Output can be formatted as JSON (full response) or vector (just the embedding values).

Examples:
  dive embed --text "Hello, world!" --output json
  echo "Hello, world!" | dive embed --model text-embedding-3-small --output vector
  dive embed --text "Some text" --output json`,
	Run: func(cmd *cobra.Command, args []string) {
		text, err := cmd.Flags().GetString("text")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		model, err := cmd.Flags().GetString("model")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		outputFormat, err := cmd.Flags().GetString("output")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// If no text provided via flag, try to read from stdin
		if text == "" {
			// Check if stdin has data
			stat, err := os.Stdin.Stat()
			if err != nil {
				fmt.Println(errorStyle.Sprintf("Error checking stdin: %v", err))
				os.Exit(1)
			}

			// If stdin is a pipe or redirect (not a terminal)
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				text, err = readStdin()
				if err != nil {
					fmt.Println(errorStyle.Sprintf("Error reading from stdin: %v", err))
					os.Exit(1)
				}
			}
		}

		// Validate that we have text input
		if text == "" {
			fmt.Println(errorStyle.Sprint("Error: no text input provided. Use --text flag or pipe text via stdin."))
			os.Exit(1)
		}

		// Trim whitespace from input
		text = strings.TrimSpace(text)
		if text == "" {
			fmt.Println(errorStyle.Sprint("Error: text input is empty after trimming whitespace."))
			os.Exit(1)
		}

		// Run embedding generation
		if err := runEmbedding(model, outputFormat, []string{text}); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(embedCmd)

	embedCmd.Flags().StringP("text", "t", "", "Input text to embed (if not provided, reads from stdin)")
	embedCmd.Flags().StringP("model", "m", "", "Embedding model to use (if not provided, uses the default model)")
	embedCmd.Flags().StringP("output", "o", "json", "Output format: json (full response) or vector (embedding values only)")
}
