package anthropic

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelClaude35Haiku: {
		Model:       ModelClaude35Haiku,
		InputPrice:  0.80,
		OutputPrice: 4.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaude35Sonnet: {
		Model:       ModelClaude35Sonnet,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaude37Sonnet: {
		Model:       ModelClaude37Sonnet,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeSonnet4: {
		Model:       ModelClaudeSonnet4,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeOpus4: {
		Model:       ModelClaudeOpus4,
		InputPrice:  15.00,
		OutputPrice: 75.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeOpus41: {
		Model:       ModelClaudeOpus41,
		InputPrice:  15.00,
		OutputPrice: 75.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}
