package anthropic_test

import (
	"fmt"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
)

func ExampleCatalog() {
	model := anthropic.New(anthropic.WithModel(anthropic.ModelClaudeSonnet55))
	fmt.Println(model.Name())
	for _, entry := range anthropic.Catalog().RecommendedModels() {
		if entry.ID == anthropic.ModelClaudeSonnet55 {
			fmt.Println(entry.ID, entry.ContextWindow)
		}
	}
	price := anthropic.TextModelPricing[anthropic.ModelClaudeSonnet55]
	cost := price.CostOf(&llm.Usage{InputTokens: 1000, OutputTokens: 1000})
	fmt.Printf("Standard token cost: $%.3f\n", cost.Total)
	// Output:
	// anthropic
	// claude-sonnet-5-5 1000000
	// Standard token cost: $0.012
}
