package dive

import (
	"net/http"

	"github.com/deepnoodle-ai/dive/llm"
)

// ModelSettings are used to configure details of the LLM for an Agent.
type ModelSettings struct {
	Temperature       *float64
	PresencePenalty   *float64
	FrequencyPenalty  *float64
	ParallelToolCalls *bool
	Caching           *bool
	MaxTokens         *int
	ReasoningBudget   *int
	ReasoningEffort   llm.ReasoningEffort
	ToolChoice        *llm.ToolChoice
	Features          []string
	RequestHeaders    http.Header
	MCPServers        []llm.MCPServerConfig
}

// Options returns the LLM options corresponding to the model settings.
func (m *ModelSettings) Options() []llm.Option {
	if m == nil {
		return nil
	}
	var opts []llm.Option
	if m.Temperature != nil {
		opts = append(opts, llm.WithTemperature(*m.Temperature))
	}
	if m.PresencePenalty != nil {
		opts = append(opts, llm.WithPresencePenalty(*m.PresencePenalty))
	}
	if m.FrequencyPenalty != nil {
		opts = append(opts, llm.WithFrequencyPenalty(*m.FrequencyPenalty))
	}
	if m.ReasoningBudget != nil {
		opts = append(opts, llm.WithReasoningBudget(*m.ReasoningBudget))
	}
	if m.ReasoningEffort != "" {
		opts = append(opts, llm.WithReasoningEffort(m.ReasoningEffort))
	}
	if m.MaxTokens != nil {
		opts = append(opts, llm.WithMaxTokens(*m.MaxTokens))
	}
	if m.ToolChoice != nil {
		opts = append(opts, llm.WithToolChoice(m.ToolChoice))
	}
	if m.ParallelToolCalls != nil {
		opts = append(opts, llm.WithParallelToolCalls(*m.ParallelToolCalls))
	}
	if len(m.Features) > 0 {
		opts = append(opts, llm.WithFeatures(m.Features...))
	}
	if len(m.RequestHeaders) > 0 {
		opts = append(opts, llm.WithRequestHeaders(m.RequestHeaders))
	}
	if len(m.MCPServers) > 0 {
		opts = append(opts, llm.WithMCPServers(m.MCPServers...))
	}
	if m.Caching != nil {
		opts = append(opts, llm.WithCaching(*m.Caching))
	}
	return opts
}
