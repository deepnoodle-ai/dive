package openai

import (
	"encoding/json"
	"strings"
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
			InputTokensDetails:  responses.ResponseUsageInputTokensDetails{CachedTokens: 10, CacheWriteTokens: 20},
			OutputTokensDetails: responses.ResponseUsageOutputTokensDetails{ReasoningTokens: 20},
		},
	}
	out, err := decodeAssistantResponse(resp)
	assert.NoError(t, err)
	assert.Equal(t, 70, out.Usage.InputTokens)
	assert.Equal(t, 50, out.Usage.OutputTokens)
	assert.Equal(t, 10, out.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, out.Usage.CacheCreationInputTokens)
	assert.Equal(t, 100, out.Usage.TotalInputTokens())
	assert.Equal(t, 20, out.Usage.ReasoningTokens)
}

func TestDecodeAssistantResponse_ClampsInvalidCacheCounts(t *testing.T) {
	tests := []struct {
		name       string
		prompt     int64
		cached     int64
		written    int64
		wantInput  int
		wantCached int
		wantWrite  int
	}{
		{name: "valid write", prompt: 10, cached: 2, written: 7, wantInput: 1, wantCached: 2, wantWrite: 7},
		{name: "cached above prompt", prompt: 10, cached: 20, written: 5, wantInput: 0, wantCached: 10},
		{name: "write above remainder", prompt: 10, cached: 7, written: 8, wantInput: 0, wantCached: 7, wantWrite: 3},
		{name: "negative cached and write", prompt: 10, cached: -20, written: -4, wantInput: 10, wantCached: 0},
		{name: "negative prompt", prompt: -10, cached: 5, written: 5, wantInput: 0, wantCached: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := decodeAssistantResponse(&responses.Response{
				Usage: responses.ResponseUsage{
					InputTokens: tt.prompt,
					InputTokensDetails: responses.ResponseUsageInputTokensDetails{
						CachedTokens:     tt.cached,
						CacheWriteTokens: tt.written,
					},
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, tt.wantInput, out.Usage.InputTokens)
			assert.Equal(t, tt.wantCached, out.Usage.CacheReadInputTokens)
			assert.Equal(t, tt.wantWrite, out.Usage.CacheCreationInputTokens)
			assert.Equal(t, max(0, int(tt.prompt)), out.Usage.TotalInputTokens())
		})
	}
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

func TestBuildRequestParams_GPT56PromptCaching(t *testing.T) {
	provider := New(WithAPIKey("test"))
	config := &llm.Config{}
	config.Apply(
		llm.WithModel("gpt-5.6-luna"),
		llm.WithPromptCacheKey("stable-session-key"),
		llm.WithMessages(
			llm.NewUserTextMessage("one"),
			llm.NewAssistantTextMessage("answer one"),
			llm.NewUserTextMessage("two"),
			llm.NewAssistantTextMessage("answer two"),
			llm.NewUserTextMessage("three"),
			llm.NewAssistantTextMessage("answer three"),
			llm.NewUserTextMessage("four"),
		),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	body, err := json.Marshal(params)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(body), `"prompt_cache_key":"stable-session-key"`))
	assert.Equal(t, 3, strings.Count(string(body), `"prompt_cache_breakpoint"`))
}

func TestBuildRequestParams_AstraPromptCaching(t *testing.T) {
	provider := New(WithAPIKey("test"))
	config := &llm.Config{}
	config.Apply(
		llm.WithModel("gpt-6-astra"),
		llm.WithPromptCacheKey("stable-session-key"),
		llm.WithMessages(
			llm.NewUserTextMessage("one"),
			llm.NewAssistantTextMessage("answer one"),
			llm.NewUserTextMessage("two"),
			llm.NewAssistantTextMessage("answer two"),
			llm.NewUserTextMessage("three"),
			llm.NewAssistantTextMessage("answer three"),
			llm.NewUserTextMessage("four"),
		),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	body, err := json.Marshal(params)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(body), `"prompt_cache_key":"stable-session-key"`))
	assert.Equal(t, 3, strings.Count(string(body), `"prompt_cache_breakpoint"`))
}

func TestBuildRequestParams_OlderModelOmitsExplicitPromptCaching(t *testing.T) {
	provider := New(WithAPIKey("test"))
	config := &llm.Config{}
	config.Apply(
		llm.WithModel("gpt-5.5"),
		llm.WithPromptCacheKey("stable-session-key"),
		llm.WithMessages(llm.NewUserTextMessage("one")),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	body, err := json.Marshal(params)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(body), `"prompt_cache_key":"stable-session-key"`))
	assert.False(t, strings.Contains(string(body), `"prompt_cache_breakpoint"`))
}

func TestBuildRequestParams_CompatibleProviderOptsIntoPromptCacheKey(t *testing.T) {
	provider := New(
		WithAPIKey("test"),
		WithName("compatible"),
		WithEndpoint("https://example.com/v1"),
		WithPromptCacheKeySupport(),
	)
	config := &llm.Config{}
	config.Apply(
		llm.WithPromptCacheKey("stable-session-key"),
		llm.WithMessages(llm.NewUserTextMessage("one")),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.True(t, params.PromptCacheKey.Valid())
	assert.Equal(t, "stable-session-key", params.PromptCacheKey.Value)
}

func TestBuildRequestParams_CompatibleProviderOmitsUnsupportedPromptCacheKey(t *testing.T) {
	provider := New(
		WithAPIKey("test"),
		WithName("compatible"),
		WithEndpoint("https://example.com/v1"),
	)
	config := &llm.Config{}
	config.Apply(
		llm.WithPromptCacheKey("stable-session-key"),
		llm.WithMessages(llm.NewUserTextMessage("one")),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.False(t, params.PromptCacheKey.Valid())
}

// Pinned to a non-reasoning model so the assertion isolates what the test is
// about: a tool that opts out contributes no includes. A reasoning model adds
// reasoning.encrypted_content on its own account, which would make an empty
// Include say nothing about the tool.
func TestBuildRequestParams_NoIncludesWhenToolOptsOut(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel(ModelGPT4o))

	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithTools(&fakeIncludeTool{includes: nil}),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Empty(t, params.Include)
}

func hasEncryptedReasoningInclude(params responses.ResponseNewParams) bool {
	for _, inc := range params.Include {
		if string(inc) == string(IncludeReasoningEncryptedContent) {
			return true
		}
	}
	return false
}

// The include used to be gated on the caller having named a reasoning effort,
// which tied reasoning continuity to a setting that has nothing to do with it.
// A model that reasons at a model-chosen depth still produces a chain of
// thought, and without the include it is dropped at every tool-result boundary,
// so each turn of an agent loop re-reasons from scratch.
func TestEncryptedReasoningIncludeRequestedWithoutAnEffort(t *testing.T) {
	provider := New(WithAPIKey("test"))

	config := &llm.Config{}
	config.Apply(llm.WithMessages(llm.NewUserTextMessage("hi")))

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.True(t, hasEncryptedReasoningInclude(params),
		"a reasoning model must have its encrypted reasoning requested even "+
			"when the caller names no effort")
	assert.Equal(t, string(params.Reasoning.Effort), "",
		"no effort was requested, so none should be sent")
}

func TestEncryptedReasoningIncludeRequestedWithAnEffort(t *testing.T) {
	provider := New(WithAPIKey("test"))

	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffortLow),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.True(t, hasEncryptedReasoningInclude(params))
	assert.Equal(t, string(params.Reasoning.Effort), "low")
}

// gpt-4o has no reasoning at all, and asking for reasoning it cannot produce is
// how this fix would have turned one silent bug into a louder one.
func TestEncryptedReasoningIncludeSkippedForNonReasoningModel(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel(ModelGPT4o))

	config := &llm.Config{}
	config.Apply(llm.WithMessages(llm.NewUserTextMessage("hi")))

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.False(t, hasEncryptedReasoningInclude(params))
}

// An id the tables do not recognize keeps the behavior it had before the
// capability tables existed: nothing is inferred about what it accepts.
func TestEncryptedReasoningIncludeSkippedForUnknownModel(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel("some-finetune-of-unknown-shape"))

	config := &llm.Config{}
	config.Apply(llm.WithMessages(llm.NewUserTextMessage("hi")))

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.False(t, hasEncryptedReasoningInclude(params))
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
	// The gpt-5 family takes minimal but not none, so none clamps up to the
	// least eager supported level rather than failing the request.
	provider := New(WithAPIKey("test"), WithModel(ModelGPT5))
	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffortNone),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Equal(t, responses.ReasoningEffort("minimal"), params.Reasoning.Effort)
}

func TestBuildRequestParams_ModelWithoutReasoningOmitsEffort(t *testing.T) {
	// gpt-4o rejects the field outright ("Unsupported parameter:
	// 'reasoning.effort'"), and the CLI now defaults effort to medium, so this
	// would otherwise 400 on every request.
	provider := New(WithAPIKey("test"), WithModel("gpt-4o"))
	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithReasoningEffort(llm.ReasoningEffortMedium),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.Equal(t, responses.ReasoningEffort(""), params.Reasoning.Effort)
}

func TestBuildRequestParams_ModelWithoutTemperatureOmitsIt(t *testing.T) {
	provider := New(WithAPIKey("test"), WithModel(ModelGPT5))
	config := &llm.Config{}
	config.Apply(
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithTemperature(0.5),
	)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)
	assert.False(t, params.Temperature.Valid())
}

func TestBuildRequestParams_NormalizesGrokReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort llm.ReasoningEffort
		want   responses.ReasoningEffort
	}{
		{
			// grok-4.5 accepts xhigh, so max clamps one level rather than two.
			name:   "grok 4.5 max maps to xhigh",
			model:  "grok-4.5",
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("xhigh"),
		},
		{
			// grok-4.5 is the one Grok model that rejects none.
			name:   "grok 4.5 none clamps to minimal",
			model:  "grok-4.5",
			effort: llm.ReasoningEffortNone,
			want:   responses.ReasoningEffort("minimal"),
		},
		{
			// "does not support parameter reasoningEffort": omit the field.
			name:   "grok build omits effort",
			model:  "grok-build-0.1",
			effort: llm.ReasoningEffortMinimal,
			want:   responses.ReasoningEffort(""),
		},
		{
			// grok-code-fast-1 likewise has no reasoning parameter.
			name:   "grok code fast omits effort",
			model:  "grok-code-fast-1",
			effort: llm.ReasoningEffortHigh,
			want:   responses.ReasoningEffort(""),
		},
		{
			// Despite the name, this model rejects the reasoning parameter.
			name:   "grok 4.20 reasoning omits effort",
			model:  "grok-4.20-0309-reasoning",
			effort: llm.ReasoningEffortHigh,
			want:   responses.ReasoningEffort(""),
		},
		{
			// The multi-agent model is the only Grok model that accepts max.
			name:   "grok multi-agent keeps max",
			model:  "grok-4.20-multi-agent-0309",
			effort: llm.ReasoningEffortMax,
			want:   responses.ReasoningEffort("max"),
		},
		{
			name:   "grok multi-agent accepts none",
			model:  "grok-4.20-multi-agent-0309",
			effort: llm.ReasoningEffortNone,
			want:   responses.ReasoningEffort("none"),
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
