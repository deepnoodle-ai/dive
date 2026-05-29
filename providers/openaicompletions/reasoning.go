package openaicompletions

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

func (p *Provider) resolveReasoningEffort(model string, config *llm.Config) (ReasoningEffort, bool, error) {
	effort := config.ReasoningEffort
	if effort == "" {
		return "", false, nil
	}

	modelLower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(modelLower, "openai/"):
		normalized, err := normalizeOpenAIReasoningEffort(strings.TrimPrefix(modelLower, "openai/"), effort)
		return ReasoningEffort(normalized), true, err
	case strings.HasPrefix(modelLower, "x-ai/"):
		normalized, err := normalizeGrokReasoningEffort(strings.TrimPrefix(modelLower, "x-ai/"), effort)
		return ReasoningEffort(normalized), true, err
	case strings.HasPrefix(modelLower, "gpt-") || strings.HasPrefix(modelLower, "o"):
		normalized, err := normalizeOpenAIReasoningEffort(modelLower, effort)
		return ReasoningEffort(normalized), true, err
	case strings.Contains(p.endpoint, "api.mistral.ai"):
		if config.Logger != nil {
			config.Logger.Warn("provider does not support reasoning effort; omitting option",
				"provider", "mistral", "model", model, "reasoning_effort", effort)
		}
		return "", false, nil
	default:
		return ReasoningEffort(effort), true, nil
	}
}

func normalizeOpenAIReasoningEffort(model string, effort llm.ReasoningEffort) (llm.ReasoningEffort, error) {
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
	switch {
	case strings.HasPrefix(model, "grok-4.3"):
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
