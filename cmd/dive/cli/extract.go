package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract structured data from text/images/PDFs using schemas",
	Long: `Extract structured data from various input types (text, images, PDFs) using JSON schemas or simple field lists.

The extract command uses AI to analyze input files and extract structured information
according to a provided JSON schema or a simple comma-separated list of fields.
Input can be provided via file path or piped from stdin.

Field Types:
  When using --fields, you can optionally specify types using the format "name:type".
  Supported types:
    - bool, boolean    : Boolean values (true/false)
    - int, integer     : Integer numbers
    - float, number    : Floating point numbers
    - string           : Text strings (default if no type specified)
    - object           : JSON objects
    - array of <type>  : Arrays of the specified type (e.g., "array of string")
    - array<type>      : Alternative array syntax (e.g., "array<int>")
    - [type]           : Bracket array syntax (e.g., "[bool]")

Examples:
  # Extract using JSON schema file
  dive extract --schema entity.json --input report.pdf --output extracted.json

  # Extract using simple field list (all strings by default)
  dive extract --fields "name,age,color" --input person.txt --output person.json

  # Extract with typed fields
  dive extract --fields "name:string,age:int,active:bool" --input person.txt

  # Extract with array types
  dive extract --fields "name:string,scores:array of int,tags:[string]" --input data.txt

  # Read from stdin with schema file
  cat document.txt | dive extract --schema data.json --output extracted.json

  # Read from stdin with typed fields
  echo "John,25,true" | dive extract --fields "name:string,age:int,verified:bool"

  # With bias filtering
  dive extract --schema person.json --input image.jpg --bias-filter "avoid gender assumptions"

  # With custom instructions
  dive extract --schema data.json --input document.txt --instructions "focus on financial data"`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		schemaPath, err := cmd.Flags().GetString("schema")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fields, err := cmd.Flags().GetString("fields")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if schemaPath == "" && fields == "" {
			fmt.Println(errorStyle.Sprint("Error: either --schema or --fields flag is required"))
			os.Exit(1)
		}

		if schemaPath != "" && fields != "" {
			fmt.Println(errorStyle.Sprint("Error: cannot specify both --schema and --fields flags"))
			os.Exit(1)
		}

		inputPath, err := cmd.Flags().GetString("input")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
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

		// Handle stdin input if no input file provided
		var inputContent string
		var stdinErr error

		if inputPath == "" {
			inputContent, stdinErr = readFromStdin()
			if stdinErr != nil {
				fmt.Println(errorStyle.Sprint(stdinErr))
				os.Exit(1)
			}
		} else {
			inputContent = inputPath
		}

		if runErr := runExtract(schemaPath, fields, inputContent, outputPath, biasFilter, instructions, inputPath == ""); runErr != nil {
			fmt.Println(errorStyle.Sprint(runErr))
			os.Exit(1)
		}
	},
}

func runExtract(schemaPath, fields, inputContent, outputPath, biasFilter, instructions string, isStdin bool) error {
	ctx := context.Background()

	// Load schema from file or create from fields
	var extractionSchema *schema.Schema
	var schemaErr error
	if schemaPath != "" {
		extractionSchema, schemaErr = loadSchemaFromFile(schemaPath)
		if schemaErr != nil {
			return fmt.Errorf("error loading schema: %v", schemaErr)
		}
	} else {
		extractionSchema, schemaErr = createSchemaFromFields(fields)
		if schemaErr != nil {
			return fmt.Errorf("error creating schema from fields: %v", schemaErr)
		}
	}

	// Get model for extraction
	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	// Execute extraction using LLM tools
	extractedData, err := performExtraction(ctx, model, extractionSchema, inputContent, biasFilter, instructions)
	if err != nil {
		return fmt.Errorf("error during extraction: %v", err)
	}

	// The extracted data should already be valid JSON from the tool
	var jsonData interface{}
	if err := json.Unmarshal([]byte(extractedData), &jsonData); err != nil {
		return fmt.Errorf("extraction did not produce valid JSON: %v", err)
	}

	// Pretty print the result
	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("error formatting JSON: %v", err)
	}

	// Output only JSON to stdout
	fmt.Println(string(prettyJSON))

	// Save to output file if specified
	if outputPath != "" {
		if err := saveExtractedData(outputPath, prettyJSON); err != nil {
			return fmt.Errorf("error saving output: %v", err)
		}
	}

	return nil
}

// performExtraction executes the extraction using LLM tools
func performExtraction(ctx context.Context, model llm.LLM, extractionSchema *schema.Schema, content, biasFilter, instructions string) (string, error) {
	// Create the extraction tool
	extractTool := createExtractTool(extractionSchema)

	// Create system prompt
	systemPrompt := "You are a data extraction specialist. Your task is to analyze the provided content and extract structured information using the extract_data tool. Extract only the information that is clearly present in the content. Do not make assumptions or add placeholder values like '<UNKNOWN>'. If information is not available, simply omit those fields from the result."

	if biasFilter != "" {
		systemPrompt += fmt.Sprintf(" Apply the following bias filter: %s", biasFilter)
	}

	if instructions != "" {
		systemPrompt += fmt.Sprintf(" Additional instructions: %s", instructions)
	}

	// Create user message with the actual content to extract from
	userMessage := fmt.Sprintf("Please extract structured data from the following content using the extract_data tool:\n\n%s", content)

	// Generate response from LLM with the tool
	// Force the model to use the extraction tool
	toolChoice := &llm.ToolChoice{
		Type: llm.ToolChoiceTypeTool,
		Name: "extract_data",
	}

	response, err := model.Generate(ctx,
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userMessage),
		llm.WithTools(extractTool),
		llm.WithToolChoice(toolChoice),
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %v", err)
	}

	// Check if the model used the extraction tool
	for _, contentBlock := range response.Content {
		if toolUse, ok := contentBlock.(*llm.ToolUseContent); ok && toolUse.Name == "extract_data" {
			// The tool input should contain the extracted data structured according to the schema
			return string(toolUse.Input), nil
		}
	}

	return "", fmt.Errorf("model did not use the extraction tool")
}

func loadSchemaFromFile(schemaPath string) (*schema.Schema, error) {
	absPath, err := filepath.Abs(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve schema path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	var s schema.Schema
	if err := json.Unmarshal(content, &s); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}

	return &s, nil
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

func readFromStdin() (string, error) {
	// Check if stdin has data available
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("error checking stdin: %v", err)
	}

	// If stdin is not a pipe/redirect, return error
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", fmt.Errorf("no input provided via stdin. Use --input flag or pipe text to stdin")
	}

	// Read all data from stdin
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		return "", fmt.Errorf("error reading from stdin: %v", err)
	}

	if len(data) == 0 {
		return "", fmt.Errorf("no input provided via stdin")
	}

	return string(data), nil
}

func createSchemaFromFields(fieldsStr string) (*schema.Schema, error) {
	if fieldsStr == "" {
		return nil, fmt.Errorf("fields string cannot be empty")
	}

	// Split fields by comma and clean whitespace
	fieldSpecs := strings.Split(fieldsStr, ",")
	properties := make(map[string]*schema.Property)
	var fieldNames []string

	for _, fieldSpec := range fieldSpecs {
		fieldSpec = strings.TrimSpace(fieldSpec)
		if fieldSpec == "" {
			continue
		}

		// Parse field:type syntax
		fieldName, fieldType, err := parseFieldSpec(fieldSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid field specification '%s': %v", fieldSpec, err)
		}

		// Create property with parsed type
		property, err := createPropertyFromType(fieldType)
		if err != nil {
			return nil, fmt.Errorf("invalid type for field '%s': %v", fieldName, err)
		}

		properties[fieldName] = property
		fieldNames = append(fieldNames, fieldName)
	}

	if len(properties) == 0 {
		return nil, fmt.Errorf("no valid fields found in: %s", fieldsStr)
	}

	// Create a schema with the properties
	s := &schema.Schema{
		Type:       schema.Object,
		Properties: properties,
		Required:   fieldNames,
	}

	return s, nil
}

// parseFieldSpec parses a field specification in the format "name" or "name:type"
func parseFieldSpec(fieldSpec string) (name, fieldType string, err error) {
	if strings.TrimSpace(fieldSpec) == "" {
		return "", "", fmt.Errorf("field specification cannot be empty")
	}

	parts := strings.SplitN(fieldSpec, ":", 2)
	if len(parts) == 1 {
		// No type specified, default to string
		name = strings.TrimSpace(parts[0])
		if name == "" {
			return "", "", fmt.Errorf("field name cannot be empty")
		}
		return name, "string", nil
	}

	name = strings.TrimSpace(parts[0])
	fieldType = strings.TrimSpace(parts[1])

	if name == "" {
		return "", "", fmt.Errorf("field name cannot be empty")
	}
	if fieldType == "" {
		return "", "", fmt.Errorf("field type cannot be empty")
	}

	return name, fieldType, nil
}

// createPropertyFromType creates a schema property based on the type string
func createPropertyFromType(typeStr string) (*schema.Property, error) {
	switch typeStr {
	case "bool", "boolean":
		return &schema.Property{Type: schema.Boolean}, nil
	case "int", "integer":
		return &schema.Property{Type: schema.Integer}, nil
	case "float", "number":
		return &schema.Property{Type: schema.Number}, nil
	case "string":
		return &schema.Property{Type: schema.String}, nil
	case "object":
		return &schema.Property{
			Type:                 schema.Object,
			AdditionalProperties: &[]bool{true}[0], // Allow additional properties
		}, nil
	default:
		// Check for array syntax: "array of <type>" or "array<type>" or "[type]"
		if arrayType := parseArrayType(typeStr); arrayType != "" {
			itemProperty, err := createPropertyFromType(arrayType)
			if err != nil {
				return nil, fmt.Errorf("invalid array item type '%s': %v", arrayType, err)
			}
			return &schema.Property{
				Type:  schema.Array,
				Items: itemProperty,
			}, nil
		}

		return nil, fmt.Errorf("unsupported type: %s", typeStr)
	}
}

// parseArrayType extracts the item type from array syntax
// Supports: "array of string", "array<string>", "[string]"
func parseArrayType(typeStr string) string {
	// Handle "array of <type>" format
	if strings.HasPrefix(typeStr, "array of ") {
		return strings.TrimSpace(strings.TrimPrefix(typeStr, "array of "))
	}

	// Handle "array<type>" format
	if strings.HasPrefix(typeStr, "array<") && strings.HasSuffix(typeStr, ">") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(typeStr, "array<"), ">"))
	}

	// Handle "[type]" format
	if strings.HasPrefix(typeStr, "[") && strings.HasSuffix(typeStr, "]") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(typeStr, "["), "]"))
	}

	return ""
}

// createExtractTool creates a tool definition for data extraction
func createExtractTool(extractionSchema *schema.Schema) llm.Tool {
	tool := llm.NewToolDefinition().
		WithName("extract_data").
		WithDescription("Extract structured data from the provided content. Return the extracted data as JSON structured according to the required schema. Only include fields that have actual values found in the content. If a field is not found or cannot be determined from the content, omit it entirely from the result.").
		WithSchema(extractionSchema)

	return tool
}

func init() {
	rootCmd.AddCommand(extractCmd)

	extractCmd.Flags().StringP("schema", "s", "", "Path to JSON schema file")
	extractCmd.Flags().StringP("fields", "f", "", "Comma-separated list of fields to extract with optional types (e.g., 'name:string,age:int,active:bool')")
	extractCmd.Flags().StringP("input", "i", "", "Path to input file to extract from (optional, reads from stdin if not provided)")
	extractCmd.Flags().StringP("output", "o", "", "Path to save extracted JSON data (optional)")
	extractCmd.Flags().StringP("bias-filter", "b", "", "Instructions for filtering or avoiding bias in extraction")
	extractCmd.Flags().StringP("instructions", "", "", "Additional instructions for the extraction process")

	// schema and fields are mutually exclusive, but at least one is required
	// input flag is now optional
}
