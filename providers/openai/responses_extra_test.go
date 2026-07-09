package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/openai/openai-go/v3/responses"
)

func TestDecodeFileSearchCallContent(t *testing.T) {
	call := responses.ResponseFileSearchToolCall{
		ID:      "fs_1",
		Queries: []string{"tesla production 2024"},
		Status:  "completed",
		Results: []responses.ResponseFileSearchToolCallResult{
			{FileID: "file_1", Filename: "tesla-10k.pdf", Score: 0.91, Text: "produced 1,773,443 vehicles"},
		},
	}
	contents, err := decodeFileSearchCallContent(call)
	assert.NoError(t, err)
	assert.Len(t, contents, 1)

	fsc, ok := contents[0].(*FileSearchCallContent)
	assert.True(t, ok)
	assert.Equal(t, "fs_1", fsc.ID)
	assert.Equal(t, "completed", fsc.Status)
	assert.Len(t, fsc.Queries, 1)
	assert.Len(t, fsc.Results, 1)
	assert.Equal(t, "file_1", fsc.Results[0].FileID)
	assert.Equal(t, "tesla-10k.pdf", fsc.Results[0].Filename)
	assert.Equal(t, "produced 1,773,443 vehicles", fsc.Results[0].Text)
}

func TestDecodeAssistantResponse_ReasoningTokens(t *testing.T) {
	resp := &responses.Response{
		ID: "resp_1",
		Usage: responses.ResponseUsage{
			InputTokens:         100,
			OutputTokens:        50,
			InputTokensDetails:  responses.ResponseUsageInputTokensDetails{CachedTokens: 10},
			OutputTokensDetails: responses.ResponseUsageOutputTokensDetails{ReasoningTokens: 20},
		},
	}
	out, err := decodeAssistantResponse(resp)
	assert.NoError(t, err)
	assert.Equal(t, 100, out.Usage.InputTokens)
	assert.Equal(t, 50, out.Usage.OutputTokens)
	assert.Equal(t, 10, out.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, out.Usage.ReasoningTokens)
}

// fakeIncludeTool implements llm.Tool, ResponsesToolProvider, and
// ResponsesIncludeProvider for testing include wiring.
type fakeIncludeTool struct {
	includes []responses.ResponseIncludable
}

func (f *fakeIncludeTool) Name() string           { return "fake" }
func (f *fakeIncludeTool) Description() string    { return "fake tool" }
func (f *fakeIncludeTool) Schema() *schema.Schema { return nil }
func (f *fakeIncludeTool) ResponsesToolParam() responses.ToolUnionParam {
	return responses.ToolUnionParam{OfWebSearch: &responses.WebSearchToolParam{Type: "web_search"}}
}
func (f *fakeIncludeTool) ResponsesIncludes() []responses.ResponseIncludable { return f.includes }

func TestBuildRequestParams_ToolIncludes(t *testing.T) {
	provider := New(WithAPIKey("test"))

	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithTools(&fakeIncludeTool{
			includes: []responses.ResponseIncludable{"file_search_call.results"},
		}),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)

	found := false
	for _, inc := range params.Include {
		if inc == responses.ResponseIncludable("file_search_call.results") {
			found = true
		}
	}
	assert.True(t, found, "expected file_search_call.results in params.Include")
}

func TestBuildRequestParams_NoIncludesWhenToolOptsOut(t *testing.T) {
	provider := New(WithAPIKey("test"))

	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithTools(&fakeIncludeTool{includes: nil}),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Empty(t, params.Include)
}

func TestProviderDefaultModel(t *testing.T) {
	provider := New(WithAPIKey("test"))

	config := &llm.Config{}
	config.Apply(llm.WithMessages(llm.NewUserTextMessage("hi")))

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Equal(t, ModelGPT56Sol, string(params.Model))
}

func TestBuildRequestParams_ReasoningEffortNone(t *testing.T) {
	provider := New(WithAPIKey("test"))

	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffortNone),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Equal(t, responses.ReasoningEffort("none"), params.Reasoning.Effort)
}

func TestBuildRequestParams_NormalizesOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort llm.ReasoningEffort
		want   responses.ReasoningEffort
	}{
		{
			name:   "max passes through on gpt-5.6",
			model:  ModelGPT56Sol,
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("max"),
		},
		{
			name:   "minimal maps to low on gpt-5.6",
			model:  ModelGPT56Terra,
			effort: llm.ReasoningEffortMinimal,
			want:   responses.ReasoningEffort("low"),
		},
		{
			name:   "max maps to xhigh on gpt-5.5",
			model:  ModelGPT55,
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("xhigh"),
		},
		{
			name:   "xhigh maps to high on gpt-5.1",
			model:  ModelGPT51,
			effort: llm.ReasoningEffortXHigh,
			want:   responses.ReasoningEffort("high"),
		},
		{
			name:   "max maps to high on o-series",
			model:  ModelO3,
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("high"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(WithAPIKey("test"), WithModel(tt.model))
			config := &llm.Config{}
			config.Apply(
				llm.WithMessages(llm.NewUserTextMessage("hi")),
				llm.WithReasoningEffort(tt.effort),
			)

			params, err := provider.buildRequestParams(config)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, params.Reasoning.Effort)
		})
	}
}

func TestBuildRequestParams_ReasoningEffortUnsupportedForOpenAIModel(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel(ModelGPT5))
	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffortNone),
	)

	_, err := provider.buildRequestParams(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestBuildRequestParams_NormalizesGrokReasoningEffort(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		effort  llm.ReasoningEffort
		want    responses.ReasoningEffort
		wantErr bool
	}{
		{
			name:   "grok 4.5 max maps to high",
			model:  "grok-4.5",
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("high"),
		},
		{
			name:   "grok build latest alias minimal maps to low",
			model:  "grok-build-latest",
			effort: llm.ReasoningEffortMinimal,
			want:   responses.ReasoningEffort("low"),
		},
		{
			name:   "grok multi-agent max maps to xhigh",
			model:  "grok-4.20-multi-agent-0309",
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("xhigh"),
		},
		{
			name:    "grok multi-agent rejects none",
			model:   "grok-4.20-multi-agent-0309",
			effort:  llm.ReasoningEffortNone,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(WithAPIKey("test"), WithName("grok"), WithModel(tt.model))
			config := &llm.Config{}
			config.Apply(
				llm.WithMessages(llm.NewUserTextMessage("hi")),
				llm.WithReasoningEffort(tt.effort),
			)

			params, err := provider.buildRequestParams(config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not supported")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, params.Reasoning.Effort)
		})
	}
}

func TestBuildRequestParams_UnknownModelPassesReasoningEffortThrough(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel("custom-reasoning-model"))
	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffort("superdeep")),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Equal(t, responses.ReasoningEffort("superdeep"), params.Reasoning.Effort)
}

func TestBuildRequestParams_UnknownReasoningEffortPassesThroughKnownModel(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel(ModelGPT55))
	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffort("superdeep")),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Equal(t, responses.ReasoningEffort("superdeep"), params.Reasoning.Effort)
}
