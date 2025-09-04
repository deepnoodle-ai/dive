package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	classifySuccessStyle = color.New(color.FgGreen)
	classifyErrorStyle   = color.New(color.FgRed)
	classifyBoldStyle    = color.New(color.Bold)
	classifyInfoStyle    = color.New(color.FgCyan)
)

// ClassificationResult represents the structured output from the classification task
type ClassificationResult struct {
	Text              string                       `json:"text"`
	Classifications   []ClassificationPrediction   `json:"classifications"`
	TopClassification ClassificationPrediction     `json:"top_classification"`
}

// ClassificationPrediction represents a single classification prediction with confidence
type ClassificationPrediction struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// createClassificationSchema creates a JSON schema for structured classification output
func createClassificationSchema(labels []string) *schema.Schema {
	return &schema.Schema{
		Type:        schema.Object,
		Description: "Classification result with confidence scores for each label",
		Properties: map[string]*schema.Property{
			"text": {
				Type:        schema.String,
				Description: "The original text that was classified",
			},
			"classifications": {
				Type:        schema.Array,
				Description: "Array of all classifications with confidence scores",
				Items: &schema.Property{
					Type: schema.Object,
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
					Required: []string{"label", "confidence"},
				},
			},
			"top_classification": {
				Type:        schema.Object,
				Description: "The classification with the highest confidence score",
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
				Required: []string{"label", "confidence"},
			},
		},
		Required: []string{"text", "classifications", "top_classification"},
	}
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

// runClassification performs the text classification using the LLM
func runClassification(ctx context.Context, text string, labels []string, model llm.LLM) (*ClassificationResult, error) {
	// Create the classification schema
	classificationSchema := createClassificationSchema(labels)
	
	// Create the system prompt
	systemPrompt := createClassificationPrompt(labels)
	
	// Create the user message
	userMessage := fmt.Sprintf("Please classify the following text:\n\n%s", text)
	
	// Configure the LLM options
	opts := []llm.Option{
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userMessage),
		llm.WithResponseFormat(&llm.ResponseFormat{
			Type:        llm.ResponseFormatTypeJSONSchema,
			Schema:      classificationSchema,
			Name:        "classification_result",
			Description: "Text classification with confidence scores",
		}),
	}
	
	// Generate the response
	response, err := model.Generate(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error generating classification: %v", err)
	}
	
	// Extract the JSON response
	if len(response.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}
	
	textContent, ok := response.Content[0].(*llm.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content type in response")
	}
	
	// Parse the JSON result
	var result ClassificationResult
	if err := json.Unmarshal([]byte(textContent.Text), &result); err != nil {
		return nil, fmt.Errorf("error parsing classification result: %v", err)
	}
	
	return &result, nil
}

var classifyCmd = &cobra.Command{
	Use:   "classify",
	Short: "Classify text into categories with confidence scores",
	Long: `Classify text into one or more categories using an LLM. 
Returns confidence scores for each label and identifies the most likely classification.
Useful for filtering data in scripts and automated workflows.

Examples:
  dive classify --text "This movie was amazing!" --labels "positive,negative,neutral"
  dive classify --text "Technical documentation" --labels "urgent,normal,low" --model "claude-3-5-sonnet-20241022"`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// Get required flags
		text, err := cmd.Flags().GetString("text")
		if err != nil {
			fmt.Println(classifyErrorStyle.Sprint(err))
			os.Exit(1)
		}
		if text == "" {
			fmt.Println(classifyErrorStyle.Sprint("Text is required. Use --text flag to provide input text"))
			os.Exit(1)
		}

		labelsStr, err := cmd.Flags().GetString("labels")
		if err != nil {
			fmt.Println(classifyErrorStyle.Sprint(err))
			os.Exit(1)
		}
		if labelsStr == "" {
			fmt.Println(classifyErrorStyle.Sprint("Labels are required. Use --labels flag to provide comma-separated labels"))
			os.Exit(1)
		}

		// Parse labels
		labels := strings.Split(labelsStr, ",")
		for i, label := range labels {
			labels[i] = strings.TrimSpace(label)
		}
		if len(labels) == 0 {
			fmt.Println(classifyErrorStyle.Sprint("At least one label must be provided"))
			os.Exit(1)
		}

		// Get optional model flag
		modelName, err := cmd.Flags().GetString("model")
		if err != nil {
			fmt.Println(classifyErrorStyle.Sprint(err))
			os.Exit(1)
		}

		// Get provider from global flag or default
		providerName := llmProvider
		if providerName == "" {
			providerName = config.DefaultProvider
		}

		// Create the LLM model
		model, err := config.GetModel(providerName, modelName)
		if err != nil {
			fmt.Println(classifyErrorStyle.Sprint(err))
			os.Exit(1)
		}

		// Perform classification
		result, err := runClassification(ctx, text, labels, model)
		if err != nil {
			fmt.Println(classifyErrorStyle.Sprint(err))
			os.Exit(1)
		}

		// Check for JSON output flag
		jsonOutput, err := cmd.Flags().GetBool("json")
		if err != nil {
			fmt.Println(classifyErrorStyle.Sprint(err))
			os.Exit(1)
		}

		if jsonOutput {
			// Output raw JSON for script integration
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				fmt.Println(classifyErrorStyle.Sprint(err))
				os.Exit(1)
			}
			fmt.Println(string(jsonBytes))
		} else {
			// Output human-readable format
			fmt.Printf("📝 %s: %s\n\n", classifyBoldStyle.Sprint("Text"), result.Text)
			
			fmt.Printf("🏆 %s: %s (%s)\n", 
				classifyBoldStyle.Sprint("Top Classification"),
				classifySuccessStyle.Sprint(result.TopClassification.Label), 
				classifySuccessStyle.Sprintf("%.2f%% confidence", result.TopClassification.Confidence*100))
			if result.TopClassification.Reasoning != "" {
				fmt.Printf("   %s: %s\n", classifyInfoStyle.Sprint("Reasoning"), result.TopClassification.Reasoning)
			}
			
			fmt.Printf("\n📊 %s:\n", classifyBoldStyle.Sprint("All Classifications"))
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
	},
}

func init() {
	rootCmd.AddCommand(classifyCmd)

	classifyCmd.Flags().StringP("text", "t", "", "Text to classify (required)")
	classifyCmd.Flags().String("labels", "", "Comma-separated list of classification labels (required)")
	classifyCmd.Flags().StringP("model", "m", "", "LLM model to use for classification")
	classifyCmd.Flags().BoolP("json", "j", false, "Output result as JSON for script integration")
	
	// Mark required flags
	classifyCmd.MarkFlagRequired("text")
	classifyCmd.MarkFlagRequired("labels")
}