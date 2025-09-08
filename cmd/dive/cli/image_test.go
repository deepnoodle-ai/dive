package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageCommands(t *testing.T) {
	// Test that image command is properly registered
	rootCmd.SetArgs([]string{"image", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestImageGenerateCommand(t *testing.T) {
	// Test help command
	rootCmd.SetArgs([]string{"image", "generate", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestImageEditCommand(t *testing.T) {
	// Test help command
	rootCmd.SetArgs([]string{"image", "edit", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestImageGenerateValidation(t *testing.T) {
	// Test that prompt is required - this should be caught by cobra's required flag validation
	// We'll test this by checking if the command would fail when executed without setting the flag

	// Reset the command to clear any previous state
	imageGenerateCmd.ResetFlags()
	imageGenerateCmd.Flags().StringVarP(&generatePrompt, "prompt", "p", "", "Text description of the desired image (required)")
	imageGenerateCmd.MarkFlagRequired("prompt")

	// Test execution without prompt
	err := runImageGenerate(imageGenerateCmd, []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt is required")
}

func TestImageEditValidation(t *testing.T) {
	// Test that prompt is required

	// Reset the command to clear any previous state
	imageEditCmd.ResetFlags()
	imageEditCmd.Flags().StringVarP(&editPrompt, "prompt", "p", "", "Text instructions for editing the image (required)")
	imageEditCmd.MarkFlagRequired("prompt")

	// Test execution without prompt
	err := runImageEdit(imageEditCmd, []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt is required")
}

func TestIsBase64(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid base64",
			input:    "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			expected: true,
		},
		{
			name:     "invalid base64",
			input:    "not base64!",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: false,
		},
		{
			name:     "valid base64 with whitespace",
			input:    "  SGVsbG8gV29ybGQ=  ",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBase64(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateImageValidationLogic(t *testing.T) {
	// Save original values
	origPrompt := generatePrompt
	origProvider := generateProvider
	origSize := generateSize
	origModel := generateModel
	origOutput := generateOutput
	origStdout := generateStdout

	defer func() {
		// Restore original values
		generatePrompt = origPrompt
		generateProvider = origProvider
		generateSize = origSize
		generateModel = origModel
		generateOutput = origOutput
		generateStdout = origStdout
	}()

	t.Run("missing prompt", func(t *testing.T) {
		generatePrompt = ""
		generateProvider = "dalle"

		err := runImageGenerate(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("invalid provider", func(t *testing.T) {
		generatePrompt = "test prompt"
		generateProvider = "invalid"

		err := runImageGenerate(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider invalid not found")
	})

	t.Run("grok provider not supported", func(t *testing.T) {
		generatePrompt = "test prompt"
		generateProvider = "grok"

		err := runImageGenerate(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider grok not found")
	})
}

func TestEditImageValidationLogic(t *testing.T) {
	// Save original values
	origPrompt := editPrompt
	origInput := editInput
	origProvider := editProvider

	defer func() {
		// Restore original values
		editPrompt = origPrompt
		editInput = origInput
		editProvider = origProvider
	}()

	t.Run("missing prompt", func(t *testing.T) {
		editPrompt = ""
		editInput = "test.png"
		editProvider = "dalle"

		err := runImageEdit(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("invalid provider", func(t *testing.T) {
		editPrompt = "test prompt"
		editInput = "test.png"
		editProvider = "invalid"

		err := runImageEdit(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider invalid not found")
	})
}
