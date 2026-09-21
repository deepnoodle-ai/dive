package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3"
)

// The image rates in the catalog only apply as deltas over Usage.ModalityTokens,
// so a request priced without that split silently bills image input at the text
// rate. This walks the real conversion rather than a hand-built Usage.
func TestConvertImageUsageAppliesImageRates(t *testing.T) {
	usage := convertImageUsage(openai.ImagesResponseUsage{
		InputTokens: 2_000_000,
		InputTokensDetails: openai.ImagesResponseUsageInputTokensDetails{
			TextTokens:  1_000_000,
			ImageTokens: 1_000_000,
		},
		OutputTokens: 1_000_000,
		OutputTokensDetails: openai.ImagesResponseUsageOutputTokensDetails{
			ImageTokens: 1_000_000,
		},
		TotalTokens: 3_000_000,
	}, ModelGPTImage25Sunburst)

	assert.NotNil(t, usage)
	assert.Equal(t, 2_000_000, usage.InputTokens)
	assert.Equal(t, 1_000_000, usage.ModalityTokens["image"].InputTokens)
	assert.Equal(t, 1_000_000, usage.ModalityTokens["text"].InputTokens)
	assert.Equal(t, 1_000_000, usage.ModalityTokens["image"].OutputTokens)
	assert.False(t, usage.InputModalityTokenDetailsIncomplete)
	assert.False(t, usage.OutputModalityTokenDetailsIncomplete)

	assert.NotNil(t, usage.Cost, "token-billed image model should resolve through the registry")
	// 1M text input at $5 plus 1M image input at $8, not 2M at $5.
	assert.Equal(t, 13.0, usage.Cost.Input)
	assert.Equal(t, 30.0, usage.Cost.Output)
	assert.Equal(t, 43.0, usage.Cost.Total)
}

// The endpoint reports nothing about cache hits, and a split that misses tokens
// would quietly bill the remainder at the base rate.
func TestConvertImageUsageMarksMissingDetail(t *testing.T) {
	usage := convertImageUsage(openai.ImagesResponseUsage{
		InputTokens:  1_000,
		OutputTokens: 2_000,
	}, ModelGPTImage25Flare)

	assert.NotNil(t, usage)
	assert.Nil(t, usage.ModalityTokens)
	assert.True(t, usage.InputModalityTokenDetailsIncomplete)
	assert.True(t, usage.OutputModalityTokenDetailsIncomplete)
	assert.True(t, usage.CacheCreationInputTokensUnavailable)
	assert.Equal(t, 0, usage.CacheReadInputTokens)
}

// Per-image models report no usage at all, and inventing a zero-token Usage
// for them would read as a free request.
func TestConvertImageUsageEmpty(t *testing.T) {
	assert.Nil(t, convertImageUsage(openai.ImagesResponseUsage{}, "dall-e-3"))
}

// The endpoint bills the request, not each image, so repeating usage across a
// fan-out would multiply the reported cost by the image count.
func TestAttachImageUsageOnlyFirstResult(t *testing.T) {
	results := []*media.ImageResult{{}, {}, {}}
	attachImageUsage(results, openai.ImagesResponseUsage{
		InputTokens:  100,
		OutputTokens: 200,
	}, ModelGPTImage25Sunburst)

	assert.NotNil(t, results[0].Usage)
	assert.Nil(t, results[1].Usage)
	assert.Nil(t, results[2].Usage)

	var total llm.Usage
	for _, r := range results {
		if r.Usage != nil {
			total.Add(r.Usage)
		}
	}
	assert.Equal(t, 100, total.InputTokens)
}
