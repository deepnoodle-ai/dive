// classify_cli is a standalone program for classifying text.
//
// Usage:
//
//	go run ./examples/programs/classify_cli --text "This is great!" --labels "positive,negative,neutral"
//	go run ./examples/programs/classify_cli --text "Technical doc" --labels "urgent,normal,low" --json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
)

type ClassificationResult struct {
	Text              string                     `json:"text"`
	Classifications   []ClassificationPrediction `json:"classifications"`
	TopClassification ClassificationPrediction   `json:"top_classification"`
}

type ClassificationPrediction struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

func main() {
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	text := flag.String("text", "", "Text to classify")
	labels := flag.String("labels", "", "Comma-separated list of classification labels")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")
	flag.Parse()

	if *text == "" {
		log.Fatal("--text is required")
	}
	if *labels == "" {
		log.Fatal("--labels is required")
	}

	labelList := strings.Split(*labels, ",")
	for i, label := range labelList {
		labelList[i] = strings.TrimSpace(label)
	}

	if err := runClassification(*provider, *model, *text, labelList, *jsonOutput); err != nil {
		log.Fatal(err)
	}
}

func createClassificationTool(labels []string) llm.Tool {
	additionalPropertiesFalse := false
	return llm.NewToolDefinition().
		WithName("classify_text").
		WithDescription("Classify the given text into one of the provided categories with confidence scores").
		WithSchema(&schema.Schema{
			Type:                 schema.Object,
			Description:          "Classification result with confidence scores for each label",
			AdditionalProperties: &additionalPropertiesFalse,
			Properties: map[string]*schema.Property{
				"text": {
					Type:        schema.String,
					Description: "The original text that was classified",
				},
				"classifications": {
					Type:        schema.Array,
					Description: "Array of all classifications with confidence scores",
					Items: &schema.Property{
						Type:                 schema.Object,
						AdditionalProperties: &additionalPropertiesFalse,
						Properties: map[string]*schema.Property{
							"label": {
								Type:        schema.String,
								Description: "The classification label",
								Enum:        labels,
							},
							"confidence": {
								Type:        schema.Number,
								Description: "Confidence score between 0.0 and 1.0",
								Minimum:     &[]float64{0.0}[0],
								Maximum:     &[]float64{1.0}[0],
							},
							"reasoning": {
								Type:        schema.String,
								Description: "Brief explanation for this classification",
							},
						},
						Required: []string{"label", "confidence", "reasoning"},
					},
				},
				"top_classification": {
					Type:                 schema.Object,
					Description:          "The classification with the highest confidence score",
					AdditionalProperties: &additionalPropertiesFalse,
					Properties: map[string]*schema.Property{
						"label": {
							Type:        schema.String,
							Description: "The most likely classification label",
							Enum:        labels,
						},
						"confidence": {
							Type:        schema.Number,
							Description: "Confidence score between 0.0 and 1.0",
							Minimum:     &[]float64{0.0}[0],
							Maximum:     &[]float64{1.0}[0],
						},
						"reasoning": {
							Type:        schema.String,
							Description: "Brief explanation for this classification",
						},
					},
					Required: []string{"label", "confidence", "reasoning"},
				},
			},
			Required: []string{"text", "classifications", "top_classification"},
		})
}

func runClassification(providerName, modelName, text string, labels []string, jsonOutput bool) error {
	ctx := context.Background()

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	classificationTool := createClassificationTool(labels)

	labelList := strings.Join(labels, ", ")
	systemPrompt := fmt.Sprintf(`You are a text classification expert. Classify the text into one of these categories: %s.
Assign confidence scores (0.0 to 1.0) for each label and provide brief reasoning.`, labelList)

	userMessage := fmt.Sprintf("Please classify the following text:\n\n%s", text)

	response, err := model.Generate(ctx,
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userMessage),
		llm.WithTools(classificationTool),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "classify_text",
		}),
	)
	if err != nil {
		return fmt.Errorf("error generating classification: %v", err)
	}

	var toolUseContent *llm.ToolUseContent
	for _, content := range response.Content {
		if tc, ok := content.(*llm.ToolUseContent); ok && tc.Name == "classify_text" {
			toolUseContent = tc
			break
		}
	}

	if toolUseContent == nil {
		return fmt.Errorf("no classification tool call found in response")
	}

	var result ClassificationResult
	if err := json.Unmarshal(toolUseContent.Input, &result); err != nil {
		return fmt.Errorf("error parsing tool call input: %v", err)
	}

	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %v", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Text: %s\n\n", result.Text)
		fmt.Printf("Top Classification: %s (%.2f%% confidence)\n",
			result.TopClassification.Label,
			result.TopClassification.Confidence*100)
		if result.TopClassification.Reasoning != "" {
			fmt.Printf("   Reasoning: %s\n", result.TopClassification.Reasoning)
		}
		fmt.Printf("\nAll Classifications:\n")
		for _, classification := range result.Classifications {
			fmt.Printf("   %s: %.2f%%", classification.Label, classification.Confidence*100)
			if classification.Reasoning != "" {
				fmt.Printf(" - %s", classification.Reasoning)
			}
			fmt.Println()
		}
	}
	return nil
}
