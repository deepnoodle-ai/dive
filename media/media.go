package media

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// GenerateImage generates an image using a single provider.
// Returns the first result when config.Count is 1, or call with
// WithCount(n) to generate multiple images.
func GenerateImage(ctx context.Context, prompt string, opts ...Option) (*ImageResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if config.Model == "" {
		return nil, ErrNoModel
	}

	provider, err := defaultRegistry.ResolveImage(config.Model)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, config.Model)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results, err := provider.GenerateImage(ctx, prompt, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoResult
	}
	return results[0], nil
}

// GenerateImageBatch generates multiple images using a single provider.
// Use WithCount(n) to control how many images are generated.
func GenerateImageBatch(ctx context.Context, prompt string, opts ...Option) ([]*ImageResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if config.Model == "" {
		return nil, ErrNoModel
	}

	provider, err := defaultRegistry.ResolveImage(config.Model)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, config.Model)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results, err := provider.GenerateImage(ctx, prompt, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoResult
	}
	return results, nil
}

// GenerateImages fans out the same prompt to multiple models concurrently.
// Requires WithModels(). Returns one result per model. Individual provider
// failures are captured in ImageResult.Err rather than failing the entire call.
func GenerateImages(ctx context.Context, prompt string, opts ...Option) ([]*ImageResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if len(config.Models) == 0 {
		return nil, fmt.Errorf("media: WithModels is required for GenerateImages")
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results := make([]*ImageResult, len(config.Models))
	var wg sync.WaitGroup

	for i, model := range config.Models {
		wg.Add(1)
		go func(idx int, modelName string) {
			defer wg.Done()

			provider, err := defaultRegistry.ResolveImage(modelName)
			if err != nil {
				results[idx] = &ImageResult{
					Model: modelName,
					Err:   fmt.Errorf("%w: %s", err, modelName),
				}
				return
			}

			perModel := *config
			perModel.Model = modelName
			perModel.Models = nil

			generated, err := provider.GenerateImage(ctx, prompt, &perModel)
			if err != nil {
				results[idx] = &ImageResult{
					Model: modelName,
					Err:   err,
				}
				return
			}
			if len(generated) == 0 {
				results[idx] = &ImageResult{
					Model: modelName,
					Err:   ErrNoResult,
				}
				return
			}
			results[idx] = generated[0]
		}(i, model)
	}

	wg.Wait()
	return results, nil
}

// EditImage edits a reference image using a text prompt.
// Requires WithReferenceImage(). Returns ErrEditNotSupported if the
// provider does not implement image editing.
func EditImage(ctx context.Context, prompt string, opts ...Option) (*ImageResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if config.Model == "" {
		return nil, ErrNoModel
	}
	if len(config.ReferenceImages) == 0 {
		return nil, fmt.Errorf("media: WithReferenceImage is required for EditImage")
	}

	provider, err := defaultRegistry.ResolveImage(config.Model)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, config.Model)
	}

	editor, ok := provider.(ImageEditor)
	if !ok {
		return nil, ErrEditNotSupported
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results, err := editor.EditImage(ctx, prompt, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoResult
	}
	return results[0], nil
}

// GenerateVideo generates a video from a text prompt. Blocks until
// generation is complete or the context is cancelled.
func GenerateVideo(ctx context.Context, prompt string, opts ...Option) (*VideoResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if config.Model == "" {
		return nil, ErrNoModel
	}

	provider, err := defaultRegistry.ResolveVideo(config.Model)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, config.Model)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := provider.GenerateVideo(ctx, prompt, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	if result == nil {
		return nil, ErrNoResult
	}
	return result, nil
}

// GenerateSpeech generates spoken audio from text.
func GenerateSpeech(ctx context.Context, text string, opts ...Option) (*AudioResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if config.Model == "" {
		return nil, ErrNoModel
	}

	provider, err := defaultRegistry.ResolveSpeech(config.Model)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, config.Model)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := provider.GenerateSpeech(ctx, text, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	if result == nil || len(result.Data) == 0 {
		return nil, ErrNoResult
	}
	return result, nil
}

// TranscribeSpeech transcribes speech audio bytes into text.
func TranscribeSpeech(ctx context.Context, audio []byte, opts ...Option) (*TranscriptionResult, error) {
	config := &Config{}
	config.Apply(opts...)

	if config.Model == "" {
		return nil, ErrNoModel
	}

	provider, err := defaultRegistry.ResolveSpeechRecognition(config.Model)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, config.Model)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := provider.TranscribeSpeech(ctx, audio, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	if result == nil || result.Text == "" {
		return nil, ErrNoResult
	}
	return result, nil
}
