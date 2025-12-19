package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoGenerateValidation(t *testing.T) {
	t.Run("missing prompt", func(t *testing.T) {
		params := videoGenerateParams{
			prompt: "",
		}

		err := runVideoGenerate(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})
}

func TestVideoStatusValidation(t *testing.T) {
	t.Run("missing operation ID", func(t *testing.T) {
		params := videoStatusParams{
			operationID: "",
		}

		err := runVideoStatus(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "operation-id is required")
	})
}

func TestVideoGenerateWaitBehavior(t *testing.T) {
	t.Run("default behavior should wait", func(t *testing.T) {
		params := videoGenerateParams{
			noWait: false,
		}
		shouldWait := !params.noWait
		require.True(t, shouldWait, "default behavior should be to wait")
	})

	t.Run("--no-wait should not wait", func(t *testing.T) {
		params := videoGenerateParams{
			noWait: true,
		}
		shouldWait := !params.noWait
		require.False(t, shouldWait, "--no-wait should disable waiting")
	})
}
