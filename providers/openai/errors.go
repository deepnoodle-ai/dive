package openai

import (
	"errors"

	"github.com/deepnoodle-ai/dive/providers"
	openaisdk "github.com/openai/openai-go/v3"
)

func normalizeOpenAIError(err error) error {
	var apiErr *openaisdk.Error
	if errors.As(err, &apiErr) {
		return providers.NewError(apiErr.StatusCode, apiErr.Message)
	}
	return err
}
