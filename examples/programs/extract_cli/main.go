// extract_cli is a standalone program for extracting structured data from text.
//
// Usage:
//
//	echo "John is 25 years old" | go run ./examples/programs/extract_cli --fields "name,age:int"
//	go run ./examples/programs/extract_cli --input document.txt --fields "title,author,date"
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
)

func main() {
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	fields := flag.String("fields", "", "Comma-separated list of fields to extract (e.g., 'name,age:int,active:bool')")
	schemaPath := flag.String("schema", "", "Path to JSON schema file")
	input := flag.String("input", "", "Input file path (reads from stdin if not provided)")
	multi := flag.Bool("multi", false, "Extract multiple items as JSON array")
	instructions := flag.String("instructions", "", "Additional instructions for extraction")
	flag.Parse()

	if *fields == "" && *schemaPath == "" {
		log.Fatal("either --fields or --schema is required")
	}

	var inputContent string
	var err error

	if *input != "" {
		inputContent = *input
	} else {
		inputContent, err = readStdin()
		if err != nil {
			log.Fatal(err)
		}
	}

	if inputContent == "" {
		log.Fatal("no input provided")
	}

	if err := runExtract(*provider, *model, *schemaPath, *fields, inputContent, *instructions, *multi); err != nil {
		log.Fatal(err)
	}
}

func readStdin() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("error checking stdin: %v", err)
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}

	var content strings.Builder
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line != "" {
					content.WriteString(line)
				}
				break
			}
			return "", fmt.Errorf("error reading from stdin: %v", err)
		}
		content.WriteString(line)
	}
	return strings.TrimSpace(content.String()), nil
}

func createSchemaFromFields(fieldsStr string) (*schema.Schema, error) {
	fieldSpecs := strings.Split(fieldsStr, ",")
	properties := make(map[string]*schema.Property)
	var fieldNames []string

	for _, fieldSpec := range fieldSpecs {
		fieldSpec = strings.TrimSpace(fieldSpec)
		if fieldSpec == "" {
			continue
		}

		parts := strings.SplitN(fieldSpec, ":", 2)
		fieldName := strings.TrimSpace(parts[0])
		fieldType := "string"
		if len(parts) == 2 {
			fieldType = strings.TrimSpace(parts[1])
		}

		var prop *schema.Property
		switch fieldType {
		case "bool", "boolean":
			prop = &schema.Property{Type: schema.Boolean}
		case "int", "integer":
			prop = &schema.Property{Type: schema.Integer}
		case "float", "number":
			prop = &schema.Property{Type: schema.Number}
		default:
			prop = &schema.Property{Type: schema.String}
		}

		properties[fieldName] = prop
		fieldNames = append(fieldNames, fieldName)
	}

	return &schema.Schema{
		Type:       schema.Object,
		Properties: properties,
		Required:   fieldNames,
	}, nil
}

func loadSchemaFromFile(path string) (*schema.Schema, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	var s schema.Schema
	if err := json.Unmarshal(content, &s); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}
	return &s, nil
}

func runExtract(providerName, modelName, schemaPath, fields, inputContent, instructions string, multi bool) error {
	ctx := context.Background()

	var itemSchema *schema.Schema
	var err error

	if schemaPath != "" {
		itemSchema, err = loadSchemaFromFile(schemaPath)
	} else {
		itemSchema, err = createSchemaFromFields(fields)
	}
	if err != nil {
		return err
	}

	var extractionSchema *schema.Schema
	if multi {
		extractionSchema = &schema.Schema{
			Type: schema.Object,
			Properties: map[string]*schema.Property{
				"data": {
					Type: schema.Array,
					Items: &schema.Property{
						Type:       schema.Object,
						Properties: itemSchema.Properties,
						Required:   itemSchema.Required,
					},
				},
			},
			Required: []string{"data"},
		}
	} else {
		extractionSchema = itemSchema
	}

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	extractTool := llm.NewToolDefinition().
		WithName("extract_data").
		WithDescription("Extract structured data from the provided content.").
		WithSchema(extractionSchema)

	systemPrompt := "You are a data extraction specialist. Extract structured information using the extract_data tool. Only extract information clearly present in the content."
	if multi {
		systemPrompt += " Extract multiple items and return them as an array."
	}
	if instructions != "" {
		systemPrompt += " " + instructions
	}

	userMessage := fmt.Sprintf("Please extract structured data from:\n\n%s", inputContent)

	response, err := model.Generate(ctx,
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userMessage),
		llm.WithTools(extractTool),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "extract_data",
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to generate response: %v", err)
	}

	for _, content := range response.Content {
		if toolUse, ok := content.(*llm.ToolUseContent); ok && toolUse.Name == "extract_data" {
			var jsonData interface{}
			if err := json.Unmarshal(toolUse.Input, &jsonData); err != nil {
				return fmt.Errorf("invalid JSON: %v", err)
			}

			if multi {
				if objData, ok := jsonData.(map[string]interface{}); ok {
					if dataArray, exists := objData["data"]; exists {
						jsonData = dataArray
					}
				}
			}

			prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
			if err != nil {
				return fmt.Errorf("error formatting JSON: %v", err)
			}
			fmt.Println(string(prettyJSON))
			return nil
		}
	}

	return fmt.Errorf("model did not use the extraction tool")
}
