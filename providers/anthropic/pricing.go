package anthropic

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelClaude35Haiku20241022: {
		Model:       ModelClaude35Haiku20241022,
		InputPrice:  0.80,
		OutputPrice: 4.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaude35Sonnet20241022: {
		Model:       ModelClaude35Sonnet20241022,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaude37Sonnet20250219: {
		Model:       ModelClaude37Sonnet20250219,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeSonnet420250514: {
		Model:       ModelClaudeSonnet420250514,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeOpus420250514: {
		Model:       ModelClaudeOpus420250514,
		InputPrice:  15.00,
		OutputPrice: 75.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeOpus4120250805: {
		Model:       ModelClaudeOpus4120250805,
		InputPrice:  15.00,
		OutputPrice: 75.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeHaiku45: {
		Model:       ModelClaudeHaiku45,
		InputPrice:  1.00,
		OutputPrice: 5.00,
		Currency:    "USD",
		UpdatedAt:   "2025-12-24",
	},
	ModelClaudeSonnet45: {
		Model:       ModelClaudeSonnet45,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeOpus45: {
		Model:       ModelClaudeOpus45,
		InputPrice:  5.00,
		OutputPrice: 25.00,
		Currency:    "USD",
		UpdatedAt:   "2025-12-24",
	},
}
