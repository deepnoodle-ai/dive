package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
	t.Run("missing prompt", func(t *testing.T) {
		params := imageGenerateParams{
			prompt:   "",
			provider: "openai",
			count:    1,
		}

		err := runImageGenerate(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("invalid provider", func(t *testing.T) {
		params := imageGenerateParams{
			prompt:   "test prompt",
			provider: "invalid",
			count:    1,
		}

		err := runImageGenerate(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid provider 'invalid', must be one of: openai, dalle, google")
	})

	t.Run("grok provider not supported", func(t *testing.T) {
		params := imageGenerateParams{
			prompt:   "test prompt",
			provider: "grok",
			count:    1,
		}

		err := runImageGenerate(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid provider 'grok', must be one of: openai, dalle, google")
	})
}

func TestEditImageValidationLogic(t *testing.T) {
	t.Run("missing prompt", func(t *testing.T) {
		params := imageEditParams{
			prompt:   "",
			input:    "test.png",
			provider: "openai",
		}

		err := runImageEdit(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("invalid provider", func(t *testing.T) {
		params := imageEditParams{
			prompt:   "test prompt",
			input:    "test.png",
			provider: "invalid",
		}

		err := runImageEdit(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid provider 'invalid', must be one of: openai, dalle (Google does not support image editing)")
	})
}
