package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
