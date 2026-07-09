package openai

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

func normalizeResponsesReasoningEffort(providerName, model string, effort llm.ReasoningEffort) (llm.ReasoningEffort, error) {
	if strings.EqualFold(providerName, "grok") {
		return normalizeGrokReasoningEffort(model, effort)
	}
	return normalizeOpenAIReasoningEffort(model, effort)
}

func normalizeOpenAIReasoningEffort(model string, effort llm.ReasoningEffort) (llm.ReasoningEffort, error) {
	model = strings.ToLower(model)
	model = strings.TrimPrefix(model, "openai/")
	if strings.Contains(model, "codex") {
		return effort, nil
	}

	switch {
	case strings.HasPrefix(model, "gpt-5.5"),
		strings.HasPrefix(model, "gpt-5.4"),
		strings.HasPrefix(model, "gpt-5.3"),
		strings.HasPrefix(model, "gpt-5.2"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{
				llm.ReasoningEffortNone,
				llm.ReasoningEffortLow,
				llm.ReasoningEffortMedium,
				llm.ReasoningEffortHigh,
				llm.ReasoningEffortXHigh,
			},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal: llm.ReasoningEffortLow,
				llm.ReasoningEffortMax:     llm.ReasoningEffortXHigh,
			})
	case strings.HasPrefix(model, "gpt-5.1"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{
				llm.ReasoningEffortNone,
				llm.ReasoningEffortLow,
				llm.ReasoningEffortMedium,
				llm.ReasoningEffortHigh,
			},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal: llm.ReasoningEffortLow,
				llm.ReasoningEffortXHigh:   llm.ReasoningEffortHigh,
				llm.ReasoningEffortMax:     llm.ReasoningEffortHigh,
			})
	case strings.HasPrefix(model, "gpt-5-pro"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{llm.ReasoningEffortHigh},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal: llm.ReasoningEffortHigh,
				llm.ReasoningEffortLow:     llm.ReasoningEffortHigh,
				llm.ReasoningEffortMedium:  llm.ReasoningEffortHigh,
				llm.ReasoningEffortXHigh:   llm.ReasoningEffortHigh,
				llm.ReasoningEffortMax:     llm.ReasoningEffortHigh,
			})
	case strings.HasPrefix(model, "gpt-5"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal,
				llm.ReasoningEffortLow,
				llm.ReasoningEffortMedium,
				llm.ReasoningEffortHigh,
			},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortXHigh: llm.ReasoningEffortHigh,
				llm.ReasoningEffortMax:   llm.ReasoningEffortHigh,
			})
	case strings.HasPrefix(model, "o"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{
				llm.ReasoningEffortLow,
				llm.ReasoningEffortMedium,
				llm.ReasoningEffortHigh,
			},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal: llm.ReasoningEffortLow,
				llm.ReasoningEffortXHigh:   llm.ReasoningEffortHigh,
				llm.ReasoningEffortMax:     llm.ReasoningEffortHigh,
			})
	default:
		return effort, nil
	}
}

func normalizeGrokReasoningEffort(model string, effort llm.ReasoningEffort) (llm.ReasoningEffort, error) {
	model = strings.ToLower(strings.TrimPrefix(model, "x-ai/"))

	switch {
	case strings.HasPrefix(model, "grok-4.5"),
		strings.HasPrefix(model, "grok-4.3"),
		strings.HasPrefix(model, "grok-build-latest"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{
				llm.ReasoningEffortNone,
				llm.ReasoningEffortLow,
				llm.ReasoningEffortMedium,
				llm.ReasoningEffortHigh,
			},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal: llm.ReasoningEffortLow,
				llm.ReasoningEffortXHigh:   llm.ReasoningEffortHigh,
				llm.ReasoningEffortMax:     llm.ReasoningEffortHigh,
			})
	case strings.Contains(model, "multi-agent"):
		return mapReasoningEffort(model, effort,
			[]llm.ReasoningEffort{
				llm.ReasoningEffortLow,
				llm.ReasoningEffortMedium,
				llm.ReasoningEffortHigh,
				llm.ReasoningEffortXHigh,
			},
			map[llm.ReasoningEffort]llm.ReasoningEffort{
				llm.ReasoningEffortMinimal: llm.ReasoningEffortLow,
				llm.ReasoningEffortMax:     llm.ReasoningEffortXHigh,
			})
	default:
		return effort, nil
	}
}

func mapReasoningEffort(model string, effort llm.ReasoningEffort, allowed []llm.ReasoningEffort, aliases map[llm.ReasoningEffort]llm.ReasoningEffort) (llm.ReasoningEffort, error) {
	for _, value := range allowed {
		if effort == value {
			return effort, nil
		}
	}
	if mapped, ok := aliases[effort]; ok {
		return mapped, nil
	}
	if !effort.IsValid() {
		return effort, nil
	}
	return "", fmt.Errorf("reasoning effort %q is not supported by model %s", effort, model)
}
