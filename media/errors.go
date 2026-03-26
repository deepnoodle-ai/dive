package media

import "errors"

var (
	// ErrNoModel is returned when no model is specified.
	ErrNoModel = errors.New("media: no model specified")

	// ErrProviderNotFound is returned when no provider matches the model name.
	ErrProviderNotFound = errors.New("media: no provider found for model")

	// ErrEditNotSupported is returned when a provider does not support editing.
	ErrEditNotSupported = errors.New("media: provider does not support image editing")

	// ErrTimeout is returned when generation exceeds the timeout.
	ErrTimeout = errors.New("media: generation timed out")

	// ErrNoResult is returned when a provider returns no results.
	ErrNoResult = errors.New("media: provider returned no results")
)
