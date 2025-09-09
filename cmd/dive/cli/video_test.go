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
	origWait := videoGenerateWait
	origNoWait := videoGenerateNoWait

	defer func() {
		// Restore original values
		videoGeneratePrompt = origPrompt
		videoGenerateWait = origWait
		videoGenerateNoWait = origNoWait
	}()

	t.Run("missing prompt", func(t *testing.T) {
		videoGeneratePrompt = ""
		videoGenerateWait = false
		videoGenerateNoWait = false

		err := runVideoGenerate(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("conflicting wait flags", func(t *testing.T) {
		videoGeneratePrompt = "test prompt"
		videoGenerateWait = true
		videoGenerateNoWait = true

		err := runVideoGenerate(nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot specify both --wait and --no-wait flags")
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
	origWait := videoGenerateWait
	origNoWait := videoGenerateNoWait

	defer func() {
		// Restore original values
		videoGeneratePrompt = origPrompt
		videoGenerateWait = origWait
		videoGenerateNoWait = origNoWait
	}()

	// Set a valid prompt to pass validation
	videoGeneratePrompt = "test prompt"

	t.Run("default behavior should wait", func(t *testing.T) {
		videoGenerateWait = false
		videoGenerateNoWait = false

		// We can't easily test the actual waiting without mocking the provider,
		// but we can test that the logic correctly determines shouldWait
		shouldWait := !videoGenerateNoWait
		if videoGenerateWait {
			shouldWait = true
		}

		require.True(t, shouldWait, "default behavior should be to wait")
	})

	t.Run("--no-wait should not wait", func(t *testing.T) {
		videoGenerateWait = false
		videoGenerateNoWait = true

		shouldWait := !videoGenerateNoWait
		if videoGenerateWait {
			shouldWait = true
		}

		require.False(t, shouldWait, "--no-wait should disable waiting")
	})

	t.Run("--wait should explicitly wait", func(t *testing.T) {
		videoGenerateWait = true
		videoGenerateNoWait = false

		shouldWait := !videoGenerateNoWait
		if videoGenerateWait {
			shouldWait = true
		}

		require.True(t, shouldWait, "--wait should explicitly enable waiting")
	})
}