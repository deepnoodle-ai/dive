package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract structured data from text/images/PDFs using schemas",
	Long: `Extract structured data from various input types (text, images, PDFs) using JSON schemas.
	
The extract command uses AI to analyze input files and extract structured information
according to a provided JSON schema. It supports bias filtering and custom instructions.

Examples:
  dive extract --schema entity.json --input report.pdf --output extracted.json
  dive extract --schema person.json --input image.jpg --bias-filter "avoid gender assumptions"
  dive extract --schema data.json --input document.txt --instructions "focus on financial data"`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		schemaPath, err := cmd.Flags().GetString("schema")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if schemaPath == "" {
			fmt.Println(errorStyle.Sprint("Error: --schema flag is required"))
			os.Exit(1)
		}

		inputPath, err := cmd.Flags().GetString("input")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if inputPath == "" {
			fmt.Println(errorStyle.Sprint("Error: --input flag is required"))
			os.Exit(1)
		}

		outputPath, err := cmd.Flags().GetString("output")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		biasFilter, err := cmd.Flags().GetString("bias-filter")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		instructions, err := cmd.Flags().GetString("instructions")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		if err := runExtract(schemaPath, inputPath, outputPath, biasFilter, instructions); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func runExtract(schemaPath, inputPath, outputPath, biasFilter, instructions string) error {
	ctx := context.Background()

	// Load schema from file
	schema, err := loadSchemaFromFile(schemaPath)
	if err != nil {
		return fmt.Errorf("error loading schema: %v", err)
	}

	// Get model for extraction
	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	// Initialize extract tool
	extractTool, err := config.InitializeToolByName("extract", nil)
	if err != nil {
		return fmt.Errorf("error initializing extract tool: %v", err)
	}

	// Create logger
	logger := slogger.New(getLogLevel())

	// Create confirmer
	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	// Create agent for extraction
	extractAgent, err := agent.New(agent.Options{
		Name:         "DataExtractor",
		Instructions: buildExtractionInstructions(schema, biasFilter, instructions),
		Model:        model,
		Logger:       logger,
		Tools:        []dive.Tool{extractTool},
		ThreadRepository: agent.NewMemoryThreadRepository(),
		Confirmer:    confirmer,
	})
	if err != nil {
		return fmt.Errorf("error creating extraction agent: %v", err)
	}

	// Build extraction message
	extractionMessage := fmt.Sprintf(
		"Please extract structured data from the file '%s' according to the provided schema. First, use the extract tool to analyze the file, then provide the extracted data as valid JSON that conforms to the schema structure.",
		inputPath,
	)

	// Execute extraction
	fmt.Println(boldStyle.Sprint("🔍 Starting data extraction..."))
	fmt.Println()

	response, err := extractAgent.CreateResponse(ctx, dive.WithInput(extractionMessage))
	if err != nil {
		return fmt.Errorf("error during extraction: %v", err)
	}

	// Get the extracted data from the response
	extractedData := response.OutputText()

	// Try to parse as JSON to validate
	var jsonData interface{}
	if err := json.Unmarshal([]byte(extractedData), &jsonData); err != nil {
		// If it's not valid JSON, try to extract JSON from the response
		extractedData = extractJSONFromResponse(extractedData)
		if err := json.Unmarshal([]byte(extractedData), &jsonData); err != nil {
			return fmt.Errorf("extraction did not produce valid JSON: %v", err)
		}
	}

	// Pretty print the result
	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("error formatting JSON: %v", err)
	}

	fmt.Println(successStyle.Sprint("✅ Extraction completed successfully!"))
	fmt.Println()
	fmt.Println("Extracted data:")
	fmt.Println(string(prettyJSON))

	// Save to output file if specified
	if outputPath != "" {
		if err := saveExtractedData(outputPath, prettyJSON); err != nil {
			return fmt.Errorf("error saving output: %v", err)
		}
		fmt.Println()
		fmt.Printf("💾 Data saved to: %s\n", outputPath)
	}

	return nil
}

func loadSchemaFromFile(schemaPath string) (map[string]interface{}, error) {
	absPath, err := filepath.Abs(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve schema path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(content, &schema); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}

	return schema, nil
}

func buildExtractionInstructions(schema map[string]interface{}, biasFilter, instructions string) string {
	baseInstructions := `You are a data extraction specialist. Your task is to extract structured information from various types of documents (text, images, PDFs) according to provided JSON schemas.

Key responsibilities:
1. Use the extract tool to analyze the input file and understand its structure
2. Extract data that precisely matches the provided schema structure
3. Return ONLY valid JSON that conforms to the schema - no additional text or explanations
4. Handle missing data gracefully with null values
5. Maintain high accuracy and avoid hallucination
6. Apply bias filtering when specified to ensure fair and unbiased extraction
7. Follow any additional extraction instructions provided

CRITICAL: Your final response must be valid JSON that can be directly parsed. Do not include any markdown formatting, explanations, or additional text around the JSON.`

	if biasFilter != "" {
		baseInstructions += fmt.Sprintf("\n\nBias Filtering Requirements: %s", biasFilter)
	}

	if instructions != "" {
		baseInstructions += fmt.Sprintf("\n\nAdditional Extraction Instructions: %s", instructions)
	}

	return baseInstructions
}

func extractJSONFromResponse(response string) string {
	// Try to find JSON in the response by looking for { or [
	start := -1
	end := -1
	
	for i, char := range response {
		if char == '{' || char == '[' {
			start = i
			break
		}
	}
	
	if start == -1 {
		return response // No JSON found, return original
	}
	
	// Find the matching closing brace/bracket
	braceCount := 0
	bracketCount := 0
	
	for i := start; i < len(response); i++ {
		switch response[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
		case '[':
			bracketCount++
		case ']':
			bracketCount--
		}
		
		if braceCount == 0 && bracketCount == 0 {
			end = i + 1
			break
		}
	}
	
	if end == -1 {
		return response // No complete JSON found
	}
	
	return response[start:end]
}

func saveExtractedData(outputPath string, data []byte) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write the file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(extractCmd)

	extractCmd.Flags().StringP("schema", "s", "", "Path to JSON schema file (required)")
	extractCmd.Flags().StringP("input", "i", "", "Path to input file to extract from (required)")
	extractCmd.Flags().StringP("output", "o", "", "Path to save extracted JSON data (optional)")
	extractCmd.Flags().StringP("bias-filter", "b", "", "Instructions for filtering or avoiding bias in extraction")
	extractCmd.Flags().StringP("instructions", "", "", "Additional instructions for the extraction process")

	// Mark required flags
	extractCmd.MarkFlagRequired("schema")
	extractCmd.MarkFlagRequired("input")
}