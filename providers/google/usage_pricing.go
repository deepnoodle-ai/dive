package google

import (
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"google.golang.org/genai"
)

// verifiedGoogleTextPricing limits automatic estimates to current text models
// whose standard online prices and caching rates are represented in the
// generated catalog. Retired, live, TTS, and image-generation rows require
// different billing shapes and intentionally remain unpriced here.
var verifiedGoogleTextPricing = map[string]struct{}{
	ModelGemini37Flash:                 {},
	ModelGemini36Flash:                 {},
	ModelGemini35Flash:                 {},
	ModelGemini35FlashLite:             {},
	ModelGemini31ProPreview:            {},
	ModelGemini31ProPreviewCustomTools: {},
	ModelGemini31FlashLite:             {},
	ModelGemini3FlashPreview:           {},
	ModelGemini25Pro:                   {},
	ModelGemini25Flash:                 {},
	ModelGemini25FlashLite:             {},
}

type googlePricingContext struct {
	vertexAI    bool
	location    string
	serviceTier genai.ServiceTier
}

func (p *Provider) pricingContext(serviceTier genai.ServiceTier) googlePricingContext {
	location := p.location
	if p.vertexAI && location == "" {
		location = os.Getenv("GOOGLE_CLOUD_LOCATION")
		if location == "" {
			location = os.Getenv("GOOGLE_CLOUD_REGION")
		}
		if location == "" {
			location = "global"
		}
	}
	return googlePricingContext{
		vertexAI:    p.vertexAI,
		location:    location,
		serviceTier: serviceTier,
	}
}

// populateGoogleCost attaches a standard online list-price estimate only when
// every price-bearing dimension is known. It leaves Cost nil and marks the
// usage unavailable for unsupported tiers, traffic types, models, or missing
// modality detail so generic model-only pricing cannot silently replace it.
func populateGoogleCost(model string, metadata *genai.GenerateContentResponseUsageMetadata, usage *llm.Usage, context googlePricingContext) {
	if usage == nil {
		return
	}
	usage.ServiceTier = googleServiceTierName(context.serviceTier)
	if context.serviceTier != "" &&
		context.serviceTier != genai.ServiceTierUnspecified &&
		context.serviceTier != genai.ServiceTierStandard {
		markGoogleCostUnavailable(usage)
		return
	}
	if context.vertexAI && metadata != nil {
		switch metadata.TrafficType {
		case "", genai.TrafficTypeUnspecified:
			// No confirmed traffic type; retain any explicitly requested tier.
		case genai.TrafficTypeOnDemand:
			usage.ServiceTier = "standard"
			// Standard pay-as-you-go pricing is represented below.
		case genai.TrafficTypeOnDemandPriority:
			usage.ServiceTier = "priority"
			markGoogleCostUnavailable(usage)
			return
		case genai.TrafficTypeOnDemandFlex:
			usage.ServiceTier = "flex"
			markGoogleCostUnavailable(usage)
			return
		case genai.TrafficTypeProvisionedThroughput:
			usage.ServiceTier = "provisioned"
			markGoogleCostUnavailable(usage)
			return
		default:
			usage.ServiceTier = strings.ToLower(string(metadata.TrafficType))
			markGoogleCostUnavailable(usage)
			return
		}
	}
	if usage.CostEstimateUnavailable {
		markGoogleCostUnavailable(usage)
		return
	}
	if _, ok := verifiedGoogleTextPricing[model]; !ok {
		markGoogleCostUnavailable(usage)
		return
	}
	pricing, ok := TextModelPricing[model]
	if !ok {
		markGoogleCostUnavailable(usage)
		return
	}
	if (len(pricing.InputPriceByModality) > 0 && usage.InputModalityTokenDetailsIncomplete) ||
		(len(pricing.OutputPriceByModality) > 0 && usage.OutputModalityTokenDetailsIncomplete) ||
		(len(pricing.CacheReadPriceByModality) > 0 && usage.CacheReadModalityTokenDetailsIncomplete) {
		markGoogleCostUnavailable(usage)
		return
	}
	if context.vertexAI && !isGlobalGoogleLocation(context.location) && pricing.NonGlobalPriceMultiplier > 0 {
		pricing = pricing.Scaled(pricing.NonGlobalPriceMultiplier)
	}
	cost := pricing.CostOf(usage)
	usage.Cost = &cost
}

func googleServiceTierName(tier genai.ServiceTier) string {
	switch tier {
	case genai.ServiceTierFlex:
		return "flex"
	case genai.ServiceTierPriority:
		return "priority"
	case genai.ServiceTierStandard:
		return "standard"
	default:
		return ""
	}
}

func markGoogleCostUnavailable(usage *llm.Usage) {
	usage.Cost = nil
	usage.CostEstimateUnavailable = true
}

func isGlobalGoogleLocation(location string) bool {
	return location == "" || strings.EqualFold(location, "global")
}
