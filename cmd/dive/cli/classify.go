package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	wontoncli "github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/color"
)

var (
	classifySuccessStyle = color.Green
	classifyErrorStyle   = color.Red
	classifyInfoStyle    = color.Cyan
)

// ClassificationResult represents the structured output from the classification task
type ClassificationResult struct {
	Text              string                     `json:"text"`
	Classifications   []ClassificationPrediction `json:"classifications"`
	TopClassification ClassificationPrediction   `json:"top_classification"`
}

// ClassificationPrediction represents a single classification prediction with confidence
type ClassificationPrediction struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// createClassificationTool creates a tool for text classification
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

// createClassificationPrompt creates a system prompt for the classification task
func createClassificationPrompt(labels []string) string {
	labelList := strings.Join(labels, ", ")

	return fmt.Sprintf(`You are a text classification expert. Your task is to classify the given text into one or more of the provided categories.

Available labels: %s

Instructions:
1. Analyze the text carefully
2. Assign confidence scores (0.0 to 1.0) for each label based on how well it fits
3. Provide brief reasoning for your classifications
4. Ensure confidence scores are realistic and sum to approximately 1.0 across all meaningful classifications
5. The top_classification should be the label with the highest confidence score

Be objective and base your classifications on the actual content and context of the text.`, labelList)
}

// parseTextResponse attempts to extract classification data from a text/markdown response
func parseTextResponse(text string, labels []string, response string) (*ClassificationResult, error) {
	result := &ClassificationResult{
		Text:            text,
		Classifications: make([]ClassificationPrediction, 0, len(labels)),
	}

	// Try to extract label scores using regex patterns
	scorePattern := regexp.MustCompile(`(?i)\*?\*?` + `([^:]+):\s*([0-9.]+)`)
	matches := scorePattern.FindAllStringSubmatch(response, -1)

	labelScores := make(map[string]float64)
	for _, match := range matches {
		if len(match) >= 3 {
			label := strings.TrimSpace(strings.ToLower(match[1]))
			scoreStr := strings.TrimSpace(match[2])

			// Check if this matches one of our labels
			for _, validLabel := range labels {
				if strings.ToLower(validLabel) == label {
					if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
						labelScores[validLabel] = score
					}
					break
				}
			}
		}
	}

	// If we didn't find scores, try to infer from the response
	if len(labelScores) == 0 {
		// Simple heuristic: look for positive/negative keywords
		responseUpper := strings.ToUpper(response)
		for _, label := range labels {
			switch strings.ToLower(label) {
			case "positive":
				if strings.Contains(responseUpper, "POSITIVE") || strings.Contains(responseUpper, "AMAZING") {
					labelScores[label] = 0.8
				} else {
					labelScores[label] = 0.1
				}
			case "negative":
				if strings.Contains(responseUpper, "NEGATIVE") {
					labelScores[label] = 0.8
				} else {
					labelScores[label] = 0.1
				}
			case "neutral":
				if strings.Contains(responseUpper, "NEUTRAL") {
					labelScores[label] = 0.8
				} else {
					labelScores[label] = 0.1
				}
			default:
				labelScores[label] = 0.33 // Default score
			}
		}
	}

	// Create classifications and find the top one
	var topClassification ClassificationPrediction
	maxConfidence := 0.0

	for _, label := range labels {
		confidence := labelScores[label]
		classification := ClassificationPrediction{
			Label:      label,
			Confidence: confidence,
			Reasoning:  fmt.Sprintf("Extracted from text response for %s", label),
		}

		result.Classifications = append(result.Classifications, classification)

		if confidence > maxConfidence {
			maxConfidence = confidence
			topClassification = classification
		}
	}

	result.TopClassification = topClassification
	return result, nil
}

// runClassification performs the text classification using the LLM
func runClassification(ctx context.Context, text string, labels []string, model llm.LLM) (*ClassificationResult, error) {
	// Create the classification tool
	classificationTool := createClassificationTool(labels)

	// Create the system prompt
	systemPrompt := createClassificationPrompt(labels)

	// Create the user message
	userMessage := fmt.Sprintf("Please classify the following text:\n\n%s", text)

	// Configure the LLM options with tool choice to force using the tool
	opts := []llm.Option{
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userMessage),
		llm.WithTools(classificationTool),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "classify_text",
		}),
	}

	// Generate the response
	response, err := model.Generate(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error generating classification: %v", err)
	}

	// Extract tool calls from the response
	if len(response.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	// Look for tool use content
	var toolUseContent *llm.ToolUseContent
	for _, content := range response.Content {
		if tc, ok := content.(*llm.ToolUseContent); ok && tc.Name == "classify_text" {
			toolUseContent = tc
			break
		}
	}

	if toolUseContent == nil {
		return nil, fmt.Errorf("no classification tool call found in response")
	}

	// Parse the tool call input as JSON
	var result ClassificationResult
	if err := json.Unmarshal(toolUseContent.Input, &result); err != nil {
		return nil, fmt.Errorf("error parsing tool call input: %v", err)
	}

	return &result, nil
}

func registerClassifyCommand(app *wontoncli.App) {
	app.Command("classify").
		Description("Classify text into categories with confidence scores").
		Long(`Classify text into one or more categories using an LLM.
Returns confidence scores for each label and identifies the most likely classification.
Useful for filtering data in scripts and automated workflows.

Examples:
  dive classify --text "This movie was amazing!" --labels "positive,negative,neutral"
  dive classify --text "Technical documentation" --labels "urgent,normal,low" --model "claude-3-5-sonnet-20241022"`).
		NoArgs().
		Flags(
			wontoncli.String("text", "t").Required().Help("Text to classify"),
			wontoncli.String("labels", "").Required().Help("Comma-separated list of classification labels"),
			wontoncli.Bool("json", "j").Help("Output result as JSON for script integration"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)
			goCtx := context.Background()

			text := ctx.String("text")
			labelsStr := ctx.String("labels")
			jsonOutput := ctx.Bool("json")

			// Parse labels
			labels := strings.Split(labelsStr, ",")
			for i, label := range labels {
				labels[i] = strings.TrimSpace(label)
			}
			if len(labels) == 0 {
				return wontoncli.Errorf("at least one label must be provided")
			}

			// Get provider from global flag or default
			providerName := llmProvider
			if providerName == "" {
				providerName = config.DefaultProvider
			}

			// Create the LLM model
			model, err := config.GetModel(providerName, llmModel)
			if err != nil {
				return wontoncli.Errorf("%v", err)
			}

			// Perform classification
			result, err := runClassification(goCtx, text, labels, model)
			if err != nil {
				return wontoncli.Errorf("%v", err)
			}

			if jsonOutput {
				// Output raw JSON for script integration
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return wontoncli.Errorf("%v", err)
				}
				fmt.Println(string(jsonBytes))
			} else {
				// Output human-readable format
				fmt.Printf("%s: %s\n\n", boldStyle.Sprint("Text"), result.Text)

				fmt.Printf("%s: %s (%s)\n",
					boldStyle.Sprint("Top Classification"),
					classifySuccessStyle.Sprint(result.TopClassification.Label),
					classifySuccessStyle.Sprintf("%.2f%% confidence", result.TopClassification.Confidence*100))
				if result.TopClassification.Reasoning != "" {
					fmt.Printf("   %s: %s\n", classifyInfoStyle.Sprint("Reasoning"), result.TopClassification.Reasoning)
				}

				fmt.Printf("\n%s:\n", boldStyle.Sprint("All Classifications"))
				for _, classification := range result.Classifications {
					confidenceColor := classifySuccessStyle
					if classification.Confidence < 0.5 {
						confidenceColor = classifyInfoStyle
					}
					fmt.Printf("   %s: %s",
						classification.Label,
						confidenceColor.Sprintf("%.2f%%", classification.Confidence*100))
					if classification.Reasoning != "" {
						fmt.Printf(" - %s", classification.Reasoning)
					}
					fmt.Println()
				}
			}
			return nil
		})
}
