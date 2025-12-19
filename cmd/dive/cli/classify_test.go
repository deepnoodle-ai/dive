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
