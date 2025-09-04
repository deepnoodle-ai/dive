package cli

import (
	"testing"

	"github.com/deepnoodle-ai/dive/schema"
	"github.com/stretchr/testify/require"
)

func TestCreateClassificationSchema(t *testing.T) {
	labels := []string{"positive", "negative", "neutral"}
	classificationSchema := createClassificationSchema(labels)

	require.Equal(t, schema.Object, classificationSchema.Type)
	require.Equal(t, "Classification result with confidence scores for each label", classificationSchema.Description)
	require.Contains(t, classificationSchema.Properties, "text")
	require.Contains(t, classificationSchema.Properties, "classifications")
	require.Contains(t, classificationSchema.Properties, "top_classification")
	require.Equal(t, []string{"text", "classifications", "top_classification"}, classificationSchema.Required)

	// Test classifications array structure
	classificationsProperty := classificationSchema.Properties["classifications"]
	require.Equal(t, schema.Array, classificationsProperty.Type)
	require.NotNil(t, classificationsProperty.Items)
	require.Equal(t, schema.Object, classificationsProperty.Items.Type)
	require.Contains(t, classificationsProperty.Items.Properties, "label")
	require.Contains(t, classificationsProperty.Items.Properties, "confidence")

	// Test label enum
	labelProperty := classificationsProperty.Items.Properties["label"]
	require.Equal(t, labels, labelProperty.Enum)

	// Test confidence bounds
	confidenceProperty := classificationsProperty.Items.Properties["confidence"]
	require.Equal(t, schema.Number, confidenceProperty.Type)
	require.NotNil(t, confidenceProperty.Minimum)
	require.Equal(t, 0.0, *confidenceProperty.Minimum)
	require.NotNil(t, confidenceProperty.Maximum)
	require.Equal(t, 1.0, *confidenceProperty.Maximum)
}

func TestCreateClassificationPrompt(t *testing.T) {
	labels := []string{"urgent", "normal", "low"}
	prompt := createClassificationPrompt(labels)

	require.Contains(t, prompt, "text classification expert")
	require.Contains(t, prompt, "urgent, normal, low")
	require.Contains(t, prompt, "confidence scores (0.0 to 1.0)")
	require.Contains(t, prompt, "top_classification")
}

func TestClassifyCommandFlags(t *testing.T) {
	// Test that flags exist
	require.NotNil(t, classifyCmd.Flag("text"))
	require.NotNil(t, classifyCmd.Flag("labels"))
	require.NotNil(t, classifyCmd.Flag("model"))
	require.NotNil(t, classifyCmd.Flag("json"))

	// Test flag types
	require.Equal(t, "string", classifyCmd.Flag("text").Value.Type())
	require.Equal(t, "string", classifyCmd.Flag("labels").Value.Type())
	require.Equal(t, "string", classifyCmd.Flag("model").Value.Type())
	require.Equal(t, "bool", classifyCmd.Flag("json").Value.Type())
}

func TestClassifyCommandUsage(t *testing.T) {
	require.Equal(t, "classify", classifyCmd.Use)
	require.Equal(t, "Classify text into categories with confidence scores", classifyCmd.Short)
	require.Contains(t, classifyCmd.Long, "Classify text into one or more categories using an LLM")
	require.Contains(t, classifyCmd.Long, "Examples:")
	require.Contains(t, classifyCmd.Long, "positive,negative,neutral")
}