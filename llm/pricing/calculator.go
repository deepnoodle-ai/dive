package pricing

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
	"github.com/deepnoodle-ai/dive/llm/providers/google"
	"github.com/deepnoodle-ai/dive/llm/providers/groq"
	"github.com/deepnoodle-ai/dive/llm/providers/grok"
	"github.com/deepnoodle-ai/dive/llm/providers/ollama"
	"github.com/deepnoodle-ai/dive/llm/providers/openai"
	"github.com/deepnoodle-ai/dive/llm/providers/openaicompletions"
	"github.com/deepnoodle-ai/dive/llm/providers/openrouter"
)

// ServiceType represents different types of AI services
type ServiceType int

const (
	ServiceTypeText ServiceType = iota
	ServiceTypeImage
	ServiceTypeEmbedding
)

// CostBreakdown provides detailed cost information
type CostBreakdown struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	ServiceType   string  `json:"service_type"`
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	TotalTokens   int     `json:"total_tokens,omitempty"`
	ImageCount    int     `json:"image_count,omitempty"`
	InputCost     float64 `json:"input_cost_usd"`
	OutputCost    float64 `json:"output_cost_usd"`
	TotalCost     float64 `json:"total_cost_usd"`
	PricePerUnit  string  `json:"price_per_unit"`
	Currency      string  `json:"currency"`
	EstimatedCost bool    `json:"estimated_cost"` // true if pricing was estimated/unavailable
}

// Calculator provides cost calculation functionality across providers
type Calculator struct{}

// NewCalculator creates a new pricing calculator instance
func NewCalculator() *Calculator {
	return &Calculator{}
}

// CalculateTextCost calculates the cost for text generation
func (c *Calculator) CalculateTextCost(provider, model string, inputTokens, outputTokens int) (*CostBreakdown, error) {
	// Normalize provider name
	provider = strings.ToLower(provider)
	
	var inputPrice, outputPrice float64
	var found bool
	var currency = "USD"

	// Look up pricing by provider
	switch provider {
	case "anthropic":
		if pricing, ok := anthropic.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "openai":
		if pricing, ok := openai.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "google":
		if pricing, ok := google.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "groq":
		if pricing, ok := groq.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "grok":
		if pricing, ok := grok.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "ollama":
		if pricing, ok := ollama.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "openai-completions", "openaicompletions":
		if pricing, ok := openaicompletions.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	case "openrouter":
		if pricing, ok := openrouter.TextModelPricing[model]; ok {
			inputPrice = pricing.InputPrice
			outputPrice = pricing.OutputPrice
			currency = pricing.Currency
			found = true
		}
	}

	// If pricing not found, use estimated pricing
	estimated := false
	if !found {
		inputPrice = 2.0   // $2 per 1M tokens (rough average)
		outputPrice = 6.0  // $6 per 1M tokens (rough average)
		estimated = true
	}

	// Calculate costs
	inputCost := float64(inputTokens) / 1_000_000.0 * inputPrice
	outputCost := float64(outputTokens) / 1_000_000.0 * outputPrice
	totalCost := inputCost + outputCost

	return &CostBreakdown{
		Provider:      provider,
		Model:         model,
		ServiceType:   "text",
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		TotalTokens:   inputTokens + outputTokens,
		InputCost:     inputCost,
		OutputCost:    outputCost,
		TotalCost:     totalCost,
		PricePerUnit:  fmt.Sprintf("$%.2f/$%.2f per 1M tokens", inputPrice, outputPrice),
		Currency:      currency,
		EstimatedCost: estimated,
	}, nil
}

// CalculateImageCost calculates the cost for image generation
func (c *Calculator) CalculateImageCost(provider, model string, imageCount int) (*CostBreakdown, error) {
	provider = strings.ToLower(provider)
	
	var pricePerImage float64
	var found bool
	var currency = "USD"
	var maxSize = "1024x1024"

	// Look up pricing by provider
	switch provider {
	case "openai":
		if pricing, ok := openai.ImageModelPricing[model]; ok {
			pricePerImage = pricing.Price
			currency = pricing.Currency
			maxSize = pricing.MaxSize
			found = true
		}
	case "google":
		if pricing, ok := google.ImageModelPricing[model]; ok {
			pricePerImage = pricing.Price
			currency = pricing.Currency
			maxSize = pricing.MaxSize
			found = true
		}
	case "openrouter":
		if pricing, ok := openrouter.ImageModelPricing[model]; ok {
			pricePerImage = pricing.Price
			currency = pricing.Currency
			maxSize = pricing.MaxSize
			found = true
		}
	}

	// If pricing not found, use estimated pricing
	estimated := false
	if !found {
		pricePerImage = 0.04 // $0.04 per image (rough average)
		estimated = true
	}

	// Calculate total cost
	totalCost := float64(imageCount) * pricePerImage

	return &CostBreakdown{
		Provider:      provider,
		Model:         model,
		ServiceType:   "image",
		ImageCount:    imageCount,
		TotalCost:     totalCost,
		PricePerUnit:  fmt.Sprintf("$%.3f per image (%s)", pricePerImage, maxSize),
		Currency:      currency,
		EstimatedCost: estimated,
	}, nil
}

// CalculateEmbeddingCost calculates the cost for text embeddings
func (c *Calculator) CalculateEmbeddingCost(provider, model string, tokens int) (*CostBreakdown, error) {
	provider = strings.ToLower(provider)
	
	var pricePerMillion float64
	var found bool
	var currency = "USD"

	// Look up pricing by provider
	switch provider {
	case "openai":
		if pricing, ok := openai.EmbeddingModelPricing[model]; ok {
			pricePerMillion = pricing.Price
			currency = pricing.Currency
			found = true
		}
	case "google":
		if pricing, ok := google.EmbeddingModelPricing[model]; ok {
			pricePerMillion = pricing.Price
			currency = pricing.Currency
			found = true
		}
	case "ollama":
		if pricing, ok := ollama.EmbeddingModelPricing[model]; ok {
			pricePerMillion = pricing.Price
			currency = pricing.Currency
			found = true
		}
	case "openrouter":
		if pricing, ok := openrouter.EmbeddingModelPricing[model]; ok {
			pricePerMillion = pricing.Price
			currency = pricing.Currency
			found = true
		}
	}

	// If pricing not found, use estimated pricing
	estimated := false
	if !found {
		pricePerMillion = 0.1 // $0.1 per 1M tokens (rough average)
		estimated = true
	}

	// Calculate cost
	totalCost := float64(tokens) / 1_000_000.0 * pricePerMillion

	return &CostBreakdown{
		Provider:      provider,
		Model:         model,
		ServiceType:   "embedding",
		TotalTokens:   tokens,
		TotalCost:     totalCost,
		PricePerUnit:  fmt.Sprintf("$%.3f per 1M tokens", pricePerMillion),
		Currency:      currency,
		EstimatedCost: estimated,
	}, nil
}

// GetProviderModels returns available models for a provider by service type
func (c *Calculator) GetProviderModels(provider string, serviceType ServiceType) []string {
	provider = strings.ToLower(provider)
	var models []string

	switch serviceType {
	case ServiceTypeText:
		switch provider {
		case "anthropic":
			for model := range anthropic.TextModelPricing {
				models = append(models, model)
			}
		case "openai":
			for model := range openai.TextModelPricing {
				models = append(models, model)
			}
		case "google":
			for model := range google.TextModelPricing {
				models = append(models, model)
			}
		case "groq":
			for model := range groq.TextModelPricing {
				models = append(models, model)
			}
		case "grok":
			for model := range grok.TextModelPricing {
				models = append(models, model)
			}
		case "ollama":
			for model := range ollama.TextModelPricing {
				models = append(models, model)
			}
		case "openai-completions", "openaicompletions":
			for model := range openaicompletions.TextModelPricing {
				models = append(models, model)
			}
		case "openrouter":
			for model := range openrouter.TextModelPricing {
				models = append(models, model)
			}
		}
	case ServiceTypeImage:
		switch provider {
		case "openai":
			for model := range openai.ImageModelPricing {
				models = append(models, model)
			}
		case "google":
			for model := range google.ImageModelPricing {
				models = append(models, model)
			}
		case "openrouter":
			for model := range openrouter.ImageModelPricing {
				models = append(models, model)
			}
		}
	case ServiceTypeEmbedding:
		switch provider {
		case "openai":
			for model := range openai.EmbeddingModelPricing {
				models = append(models, model)
			}
		case "google":
			for model := range google.EmbeddingModelPricing {
				models = append(models, model)
			}
		case "ollama":
			for model := range ollama.EmbeddingModelPricing {
				models = append(models, model)
			}
		case "openrouter":
			for model := range openrouter.EmbeddingModelPricing {
				models = append(models, model)
			}
		}
	}

	return models
}

// CompareCosts compares costs across multiple providers for the same usage
func (c *Calculator) CompareCosts(serviceType ServiceType, inputTokens, outputTokens, imageCount, embeddingTokens int) ([]*CostBreakdown, error) {
	var results []*CostBreakdown
	providers := []string{"anthropic", "openai", "google", "groq", "grok", "ollama", "openai-completions", "openrouter"}

	for _, provider := range providers {
		models := c.GetProviderModels(provider, serviceType)
		
		// Use first available model for comparison
		if len(models) > 0 {
			model := models[0]
			var breakdown *CostBreakdown
			var err error

			switch serviceType {
			case ServiceTypeText:
				breakdown, err = c.CalculateTextCost(provider, model, inputTokens, outputTokens)
			case ServiceTypeImage:
				breakdown, err = c.CalculateImageCost(provider, model, imageCount)
			case ServiceTypeEmbedding:
				breakdown, err = c.CalculateEmbeddingCost(provider, model, embeddingTokens)
			}

			if err == nil {
				results = append(results, breakdown)
			}
		}
	}

	return results, nil
}