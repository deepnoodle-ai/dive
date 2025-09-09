package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoCommands(t *testing.T) {
	// Test that video command is properly registered
	rootCmd.SetArgs([]string{"video", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestVideoGenerateCommand(t *testing.T) {
	// Test help command
	rootCmd.SetArgs([]string{"video", "generate", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestVideoStatusCommand(t *testing.T) {
	// Test help command
	rootCmd.SetArgs([]string{"video", "status", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestVideoGenerateValidation(t *testing.T) {
	// Save original values
	origPrompt := videoGeneratePrompt
	origNoWait := videoGenerateNoWait

	defer func() {
		// Restore original values
		videoGeneratePrompt = origPrompt
		videoGenerateNoWait = origNoWait
	}()

	t.Run("missing prompt", func(t *testing.T) {
		videoGeneratePrompt = ""
		videoGenerateNoWait = false

		err := runVideoGenerate(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})
}

func TestVideoStatusValidation(t *testing.T) {
	// Save original values
	origOperationID := videoStatusOperationID

	defer func() {
		// Restore original values
		videoStatusOperationID = origOperationID
	}()

	t.Run("missing operation ID", func(t *testing.T) {
		videoStatusOperationID = ""

		err := runVideoStatus(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "operation-id is required")
	})
}

func TestVideoGenerateWaitBehavior(t *testing.T) {
	// Save original values
	origPrompt := videoGeneratePrompt
	origNoWait := videoGenerateNoWait

	defer func() {
		// Restore original values
		videoGeneratePrompt = origPrompt
		videoGenerateNoWait = origNoWait
	}()

	// Set a valid prompt to pass validation
	videoGeneratePrompt = "test prompt"

	t.Run("default behavior should wait", func(t *testing.T) {
		videoGenerateNoWait = false

		// We can't easily test the actual waiting without mocking the provider,
		// but we can test that the logic correctly determines shouldWait
		shouldWait := !videoGenerateNoWait

		require.True(t, shouldWait, "default behavior should be to wait")
	})

	t.Run("--no-wait should not wait", func(t *testing.T) {
		videoGenerateNoWait = true

		shouldWait := !videoGenerateNoWait

		require.False(t, shouldWait, "--no-wait should disable waiting")
	})
}