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
	wontoncli "github.com/deepnoodle-ai/wonton/cli"
)

func runExtract(schemaPath, fields, inputContent, outputPath, biasFilter, instructions string, multi, isStdin bool) error {
	ctx := context.Background()

	// Load schema from file or create from fields
	var itemSchema *schema.Schema
	var schemaErr error
	if schemaPath != "" {
		itemSchema, schemaErr = loadSchemaFromFile(schemaPath)
		if schemaErr != nil {
			return fmt.Errorf("error loading schema: %v", schemaErr)
		}
	} else {
		itemSchema, schemaErr = createSchemaFromFields(fields)
		if schemaErr != nil {
			return fmt.Errorf("error creating schema from fields: %v", schemaErr)
		}
	}

	// Wrap in array schema if multi mode is enabled
	var extractionSchema *schema.Schema
	if multi {
		extractionSchema = &schema.Schema{
			Type: schema.Object,
			Properties: map[string]*schema.Property{
				"data": {
					Type: schema.Array,
					Items: &schema.Property{
						Type:                 schema.Object,
						Properties:           itemSchema.Properties,
						Required:             itemSchema.Required,
						AdditionalProperties: itemSchema.AdditionalProperties,
					},
				},
			},
			Required: []string{"data"},
		}
	} else {
		extractionSchema = itemSchema
	}

	// Get model for extraction
	model, err := config.GetModel(llmProvider, llmModel)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	// Execute extraction using LLM tools
	extractedData, err := performExtraction(ctx, model, extractionSchema, inputContent, biasFilter, instructions, multi)
	if err != nil {
		return fmt.Errorf("error during extraction: %v", err)
	}

	var jsonData interface{}
	if err := json.Unmarshal([]byte(extractedData), &jsonData); err != nil {
		return fmt.Errorf("extraction did not produce valid JSON: %v", err)
	}

	// For multi mode, extract the data array from the wrapper object
	if multi {
		if objData, ok := jsonData.(map[string]interface{}); ok {
			if dataArray, exists := objData["data"]; exists {
				jsonData = dataArray
			} else {
				jsonData = []interface{}{jsonData}
			}
		} else {
			jsonData = []interface{}{jsonData}
		}

		if _, isArray := jsonData.([]interface{}); !isArray {
			jsonData = []interface{}{jsonData}
		}
	}

	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("error formatting JSON: %v", err)
	}

	fmt.Println(string(prettyJSON))

	if outputPath != "" {
		if err := saveExtractedData(outputPath, prettyJSON); err != nil {
			return fmt.Errorf("error saving output: %v", err)
		}
	}

	return nil
}

func performExtraction(ctx context.Context, model llm.LLM, extractionSchema *schema.Schema, content, biasFilter, instructions string, multi bool) (string, error) {
	extractTool := createExtractTool(extractionSchema)

	systemPrompt := "You are a data extraction specialist. Your task is to analyze the provided content and extract structured information using the extract_data tool. Extract only the information that is clearly present in the content. Do not make assumptions or add placeholder values like '<UNKNOWN>'. If information is not available, simply omit those fields from the result."

	if multi {
		systemPrompt += " You are extracting multiple items from the content - identify all relevant instances and return them as an array of objects. Each object should follow the specified schema structure."
	}

	if biasFilter != "" {
		systemPrompt += fmt.Sprintf(" Apply the following bias filter: %s", biasFilter)
	}

	if instructions != "" {
		systemPrompt += fmt.Sprintf(" Additional instructions: %s", instructions)
	}

	userMessage := fmt.Sprintf("Please extract structured data from the following content using the extract_data tool:\n\n%s", content)

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

	for _, contentBlock := range response.Content {
		if toolUse, ok := contentBlock.(*llm.ToolUseContent); ok && toolUse.Name == "extract_data" {
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
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

func readFromStdin() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("error checking stdin: %v", err)
	}

	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", fmt.Errorf("no input provided via stdin. Use --input flag or pipe text to stdin")
	}

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

	fieldSpecs := strings.Split(fieldsStr, ",")
	properties := make(map[string]*schema.Property)
	var fieldNames []string

	for _, fieldSpec := range fieldSpecs {
		fieldSpec = strings.TrimSpace(fieldSpec)
		if fieldSpec == "" {
			continue
		}

		fieldName, fieldType, err := parseFieldSpec(fieldSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid field specification '%s': %v", fieldSpec, err)
		}

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

	s := &schema.Schema{
		Type:       schema.Object,
		Properties: properties,
		Required:   fieldNames,
	}

	return s, nil
}

func parseFieldSpec(fieldSpec string) (name, fieldType string, err error) {
	if strings.TrimSpace(fieldSpec) == "" {
		return "", "", fmt.Errorf("field specification cannot be empty")
	}

	parts := strings.SplitN(fieldSpec, ":", 2)
	if len(parts) == 1 {
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
			AdditionalProperties: &[]bool{true}[0],
		}, nil
	default:
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

func parseArrayType(typeStr string) string {
	if strings.HasPrefix(typeStr, "array of ") {
		return strings.TrimSpace(strings.TrimPrefix(typeStr, "array of "))
	}

	if strings.HasPrefix(typeStr, "array<") && strings.HasSuffix(typeStr, ">") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(typeStr, "array<"), ">"))
	}

	if strings.HasPrefix(typeStr, "[") && strings.HasSuffix(typeStr, "]") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(typeStr, "["), "]"))
	}

	return ""
}

func createExtractTool(extractionSchema *schema.Schema) llm.Tool {
	tool := llm.NewToolDefinition().
		WithName("extract_data").
		WithDescription("Extract structured data from the provided content. Return the extracted data as JSON structured according to the required schema. Only include fields that have actual values found in the content. If a field is not found or cannot be determined from the content, omit it entirely from the result.").
		WithSchema(extractionSchema)

	return tool
}

func registerExtractCommand(app *wontoncli.App) {
	app.Command("extract").
		Description("Extract structured data from text/images/PDFs using schemas").
		Long(`Extract structured data from various input types (text, images, PDFs) using JSON schemas or simple field lists.

The extract command uses AI to analyze input files and extract structured information
according to a provided JSON schema or a simple comma-separated list of fields.
Input can be provided via file path or piped from stdin.

Examples:
  dive extract --schema entity.json --input report.pdf --output extracted.json
  dive extract --fields "name,age,color" --input person.txt --output person.json
  dive extract --fields "name:string,age:int,active:bool" --input person.txt
  cat document.txt | dive extract --schema data.json --output extracted.json`).
		NoArgs().
		Flags(
			wontoncli.String("schema", "s").Help("Path to JSON schema file"),
			wontoncli.String("fields", "f").Help("Comma-separated list of fields to extract with optional types (e.g., 'name:string,age:int,active:bool')"),
			wontoncli.String("input", "i").Help("Path to input file to extract from (optional, reads from stdin if not provided)"),
			wontoncli.String("output", "o").Help("Path to save extracted JSON data (optional)"),
			wontoncli.String("bias-filter", "b").Help("Instructions for filtering or avoiding bias in extraction"),
			wontoncli.String("instructions", "").Help("Additional instructions for the extraction process"),
			wontoncli.Bool("multi", "").Help("Extract multiple items from the input and return as JSON array"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			schemaPath := ctx.String("schema")
			fields := ctx.String("fields")
			inputPath := ctx.String("input")
			outputPath := ctx.String("output")
			biasFilter := ctx.String("bias-filter")
			instructions := ctx.String("instructions")
			multi := ctx.Bool("multi")

			if schemaPath == "" && fields == "" {
				return wontoncli.Errorf("either --schema or --fields flag is required")
			}

			if schemaPath != "" && fields != "" {
				return wontoncli.Errorf("cannot specify both --schema and --fields flags")
			}

			var inputContent string
			var stdinErr error

			if inputPath == "" {
				inputContent, stdinErr = readFromStdin()
				if stdinErr != nil {
					return wontoncli.Errorf("%v", stdinErr)
				}
			} else {
				inputContent = inputPath
			}

			if err := runExtract(schemaPath, fields, inputContent, outputPath, biasFilter, instructions, multi, inputPath == ""); err != nil {
				return wontoncli.Errorf("%v", err)
			}
			return nil
		})
}
