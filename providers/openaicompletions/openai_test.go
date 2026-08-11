package openaicompletions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestApplyRequestConfig_NormalizesReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort llm.ReasoningEffort
		want   ReasoningEffort
	}{
		{
			name:   "openai gpt-5.6 max passes through",
			model:  ModelGPT56Sol,
			effort: llm.ReasoningEffortMax,
			want:   ReasoningEffortMax,
		},
		{
			name:   "openai gpt-5.6 minimal maps to low",
			model:  ModelGPT56Terra,
			effort: llm.ReasoningEffortMinimal,
			want:   ReasoningEffortLow,
		},
		{
			name:   "openai gpt-5.5 max maps to xhigh",
			model:  ModelGPT55,
			effort: llm.ReasoningEffortMax,
			want:   ReasoningEffortXHigh,
		},
		{
			name:   "openai gpt-5.1 xhigh maps to high",
			model:  ModelGPT51,
			effort: llm.ReasoningEffortXHigh,
			want:   ReasoningEffortHigh,
		},
		{
			// grok-4.5 accepts xhigh, so max clamps to xhigh rather than
			// dropping two levels to high.
			name:   "openrouter x-ai grok max maps to xhigh",
			model:  "x-ai/grok-4.5",
			effort: llm.ReasoningEffortMax,
			want:   ReasoningEffortXHigh,
		},
		{
			// grok-build rejects the reasoning parameter entirely
			// ("does not support parameter reasoningEffort"), so it is omitted.
			name:   "openrouter x-ai grok build omits effort",
			model:  "x-ai/grok-build-0.1",
			effort: llm.ReasoningEffortMinimal,
			want:   ReasoningEffort(""),
		},
		{
			name:   "unknown model effort passes through",
			model:  "custom/model",
			effort: llm.ReasoningEffort("superdeep"),
			want:   ReasoningEffort("superdeep"),
		},
		{
			name:   "unknown effort passes through known model",
			model:  ModelGPT55,
			effort: llm.ReasoningEffort("superdeep"),
			want:   ReasoningEffort("superdeep"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(WithModel(tt.model))
			var req Request
			err := provider.applyRequestConfig(&req, &llm.Config{ReasoningEffort: tt.effort})
			assert.NoError(t, err)
			assert.Equal(t, tt.want, req.ReasoningEffort)
		})
	}
}

func TestApplyRequestConfig_UnsupportedReasoningEffortClampsForKnownModel(t *testing.T) {
	// The gpt-5 family takes minimal but not none, so none clamps up to the
	// least eager level the model actually accepts instead of failing.
	provider := New(WithModel(ModelGPT5))
	var req Request
	err := provider.applyRequestConfig(&req, &llm.Config{ReasoningEffort: llm.ReasoningEffortNone})
	assert.NoError(t, err)
	assert.Equal(t, ReasoningEffortMinimal, req.ReasoningEffort)
}

func TestApplyRequestConfig_OpenAIPromptCacheKey(t *testing.T) {
	provider := New(WithModel(ModelGPT56Luna), WithEndpoint(DefaultEndpoint+"/"))
	var req Request
	err := provider.applyRequestConfig(&req, &llm.Config{PromptCacheKey: "stable-session-key"})
	assert.NoError(t, err)
	assert.Equal(t, "stable-session-key", req.PromptCacheKey)
}

func TestApplyReportedUsageCostRejectsNegativeCharge(t *testing.T) {
	reported := -0.01
	usage := llm.Usage{}
	applyReportedUsageCost(Usage{Cost: &reported}, &usage, "model", "USD")
	assert.Nil(t, usage.Cost)
	assert.True(t, usage.CostEstimateUnavailable)
}

func TestGenerateTreatsNullUsageAsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-null-usage",
			"model":"gpt-5.5",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":null
		}`))
	}))
	t.Cleanup(server.Close)
	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel(ModelGPT55),
		WithMaxRetries(0),
		WithReportedUsageCost("USD"),
	)

	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Equal(t, "done", response.Message().Text())
	assert.Nil(t, response.Usage.Cost)
	assert.True(t, response.Usage.CostEstimateUnavailable)
}

func TestSupportsExplicitChatPromptCaching_AcceptsTrailingSlash(t *testing.T) {
	assert.True(t, supportsExplicitChatPromptCaching("openai-completions", DefaultEndpoint+"/", ModelGPT56Luna))
}

func TestAddPromptCacheBreakpoints(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "answer one"},
		{Role: "user", Content: "two"},
		{Role: "assistant", Content: "answer two"},
		{Role: "developer", Content: "three"},
		{Role: "user", Content: "four"},
	}
	addPromptCacheBreakpoints(messages, 3)

	body, err := json.Marshal(messages)
	assert.NoError(t, err)
	assert.Equal(t, 3, strings.Count(string(body), `"prompt_cache_breakpoint"`))
	assert.True(t, strings.Contains(string(body), `"mode":"explicit"`))
}

func TestApplyRequestConfig_OmitsTemperatureForModelsThatRejectIt(t *testing.T) {
	// gpt-5 answers 400 "Unsupported parameter: 'temperature'".
	temperature := 0.5
	logger := &recordingLogger{}
	provider := New(WithModel(ModelGPT5))
	var req Request
	err := provider.applyRequestConfig(&req, &llm.Config{
		Temperature: &temperature,
		Logger:      logger,
	})
	assert.NoError(t, err)
	assert.Nil(t, req.Temperature)
	assert.Len(t, logger.warnings, 1)
}

func TestApplyRequestConfig_KeepsTemperatureForModelsThatAcceptIt(t *testing.T) {
	temperature := 0.5
	logger := &recordingLogger{}
	provider := New(WithModel(ModelGPT51))
	var req Request
	err := provider.applyRequestConfig(&req, &llm.Config{
		Temperature: &temperature,
		Logger:      logger,
	})
	assert.NoError(t, err)
	assert.NotNil(t, req.Temperature)
	assert.Equal(t, temperature, *req.Temperature)
	assert.Len(t, logger.warnings, 0)
}

func TestApplyRequestConfig_MistralOmitsReasoningEffort(t *testing.T) {
	provider := New(
		WithEndpoint("https://api.mistral.ai/v1/chat/completions"),
		WithModel("mistral-large-3"),
	)
	var req Request
	err := provider.applyRequestConfig(&req, &llm.Config{ReasoningEffort: llm.ReasoningEffortHigh})
	assert.NoError(t, err)
	assert.Equal(t, ReasoningEffort(""), req.ReasoningEffort)
}

func TestApplyRequestConfig_NormalizesReasoningEffortForTools(t *testing.T) {
	tool := llm.NewToolDefinition().
		WithName("lookup").
		WithDescription("Looks up a value").
		WithSchema(&schema.Schema{Type: "object"})

	tests := []struct {
		name         string
		model        string
		effort       llm.ReasoningEffort
		withTools    bool
		want         ReasoningEffort
		wantWarnings int
	}{
		{
			name:         "gpt-5.4-mini uses none with function tools",
			model:        ModelGPT54Mini,
			effort:       llm.ReasoningEffortHigh,
			withTools:    true,
			want:         ReasoningEffortNone,
			wantWarnings: 1,
		},
		{
			name:         "gpt-5.4-mini snapshot uses none with function tools",
			model:        "gpt-5.4-mini-2026-03-17",
			effort:       llm.ReasoningEffortXHigh,
			withTools:    true,
			want:         ReasoningEffortNone,
			wantWarnings: 1,
		},
		{
			name:         "openai-prefixed gpt-5.4-mini uses none with function tools",
			model:        "openai/gpt-5.4-mini",
			effort:       llm.ReasoningEffortMedium,
			withTools:    true,
			want:         ReasoningEffortNone,
			wantWarnings: 1,
		},
		{
			name:      "gpt-5.4-mini preserves reasoning without tools",
			model:     ModelGPT54Mini,
			effort:    llm.ReasoningEffortHigh,
			withTools: false,
			want:      ReasoningEffortHigh,
		},
		{
			name:      "gpt-5.4 preserves reasoning with tools",
			model:     ModelGPT54,
			effort:    llm.ReasoningEffortHigh,
			withTools: true,
			want:      ReasoningEffortHigh,
		},
		{
			name:      "gpt-5.4-mini preserves none with tools",
			model:     ModelGPT54Mini,
			effort:    llm.ReasoningEffortNone,
			withTools: true,
			want:      ReasoningEffortNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &recordingLogger{}
			config := &llm.Config{ReasoningEffort: tt.effort, Logger: logger}
			if tt.withTools {
				config.Tools = []llm.Tool{tool}
			}

			provider := New(WithModel(tt.model))
			var req Request
			err := provider.applyRequestConfig(&req, config)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, req.ReasoningEffort)
			assert.Len(t, logger.warnings, tt.wantWarnings)
		})
	}
}

type recordingLogger struct {
	warnings []string
}

func (l *recordingLogger) Debug(string, ...any) {}
func (l *recordingLogger) Info(string, ...any)  {}
func (l *recordingLogger) Warn(msg string, _ ...any) {
	l.warnings = append(l.warnings, msg)
}
func (l *recordingLogger) Error(string, ...any)   {}
func (l *recordingLogger) With(...any) llm.Logger { return l }

func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}
}

func TestHelloWorld(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\""),
	))
	assert.NoError(t, err)
	// The model might respond with "Hello!" or other variations, so we check case-insensitive
	assert.Contains(t, strings.ToLower(response.Message().Text()), "hello")
}

func TestHelloWorldStream(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("count to 10. respond with the integers only, separated by spaces."),
	))
	assert.NoError(t, err)

	accum := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accum.AddEvent(event); err != nil {
			assert.NoError(t, err)
		}
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accum.IsComplete())

	response := accum.Response()
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)
	assert.Equal(t, "1 2 3 4 5 6 7 8 9 10", response.Message().Text())
}

func TestToolUse(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	response, err := provider.Generate(ctx,
		llm.WithModel("gpt-4o-2024-08-06"),
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	assert.NoError(t, err)

	// Use ToolCalls() which filters for tool_use content (model may also return text)
	toolCalls := response.ToolCalls()
	assert.Equal(t, 1, len(toolCalls))

	toolUse := toolCalls[0]
	assert.Equal(t, "add", toolUse.Name)

	// The exact format of the arguments may vary, so we just check that it contains the numbers
	assert.Contains(t, string(toolUse.Input), "567")
	assert.Contains(t, string(toolUse.Input), "111")
}

func TestMultipleToolUse(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	response, err := provider.Generate(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Calculate two results for me: add 567 and 111, and add 233 and 444")),
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(response.Message().Content))

	c1 := response.Message().Content[0]
	assert.Equal(t, llm.ContentTypeToolUse, c1.Type())

	toolUse, ok := c1.(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "add", toolUse.Name)
	assert.Contains(t, string(toolUse.Input), "567")
	assert.Contains(t, string(toolUse.Input), "111")

	c2 := response.Message().Content[1]
	assert.Equal(t, llm.ContentTypeToolUse, c2.Type())

	toolUse, ok = c2.(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "add", toolUse.Name)
	assert.Contains(t, string(toolUse.Input), "233")
	assert.Contains(t, string(toolUse.Input), "444")
}

func TestMultipleToolUseStreaming(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	message := llm.NewUserTextMessage("Calculate two results for me: add 567 and 111, and add 233 and 444")

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(message),
		llm.WithTools(add),
		llm.WithToolChoice(&llm.ToolChoice{Type: llm.ToolChoiceTypeAny}),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accumulator.AddEvent(event); err != nil {
			assert.NoError(t, err)
		}
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	toolCalls := response.ToolCalls()
	assert.Equal(t, 2, len(toolCalls))

	// The two calls can be in any order, so we need to check both

	var c1 *llm.ToolUseContent
	var c2 *llm.ToolUseContent

	if strings.Contains(string(toolCalls[0].Input), "567") {
		c1 = toolCalls[0]
		c2 = toolCalls[1]
	} else {
		c1 = toolCalls[1]
		c2 = toolCalls[0]
	}

	assert.Equal(t, "add", c1.Name)
	assert.Contains(t, string(c1.Input), "567")
	assert.Contains(t, string(c1.Input), "111")

	assert.Equal(t, "add", c2.Name)
	assert.Contains(t, string(c2.Input), "233")
	assert.Contains(t, string(c2.Input), "444")
}

func TestToolUseStream(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	iterator, err := provider.Stream(ctx,
		llm.WithModel("gpt-4o-2024-08-06"),
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithToolChoice(llm.ToolChoiceAuto),
		llm.WithTools(add),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accumulator.AddEvent(event); err != nil {
			assert.NoError(t, err)
		}
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	toolCalls := response.ToolCalls()
	assert.Equal(t, 1, len(toolCalls))

	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)

	// Check that we have at least one tool call
	assert.GreaterOrEqual(t, len(response.ToolCalls()), 1)

	// Check that the tool call is for the add function
	toolCall := response.ToolCalls()[0]
	assert.Equal(t, "add", toolCall.Name)

	// Check that the arguments contain the numbers
	assert.Contains(t, string(toolCall.Input), "567")
	assert.Contains(t, string(toolCall.Input), "111")
}

func TestConvertMessages(t *testing.T) {
	// Create a message with two ContentTypeToolUse content blocks
	message := &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ToolUseContent{
				ID:    "call_123",
				Name:  "Calculator",
				Input: []byte(`{"expression":"2 + 2"}`),
			},
			&llm.ToolUseContent{
				ID:    "call_456",
				Name:  "GoogleSearch",
				Input: []byte(`{"query":"math formulas"}`),
			},
		},
	}

	// Convert the message
	converted, err := convertMessages([]*llm.Message{message})
	assert.NoError(t, err)

	// Verify the conversion - should be a single message with multiple tool calls
	assert.Len(t, converted, 1)

	// Check the message has both tool calls
	assert.Equal(t, "assistant", converted[0].Role)
	assert.Len(t, converted[0].ToolCalls, 2)

	// Check first tool call
	assert.Equal(t, "call_123", converted[0].ToolCalls[0].ID)
	assert.Equal(t, "function", converted[0].ToolCalls[0].Type)
	assert.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"expression":"2 + 2"}`, converted[0].ToolCalls[0].Function.Arguments)

	// Check second tool call
	assert.Equal(t, "call_456", converted[0].ToolCalls[1].ID)
	assert.Equal(t, "function", converted[0].ToolCalls[1].Type)
	assert.Equal(t, "GoogleSearch", converted[0].ToolCalls[1].Function.Name)
	assert.Equal(t, `{"query":"math formulas"}`, converted[0].ToolCalls[1].Function.Arguments)
}

// Add a test for tool results
func TestConvertToolResultMessages(t *testing.T) {
	// Create a message with two ContentTypeToolResult content blocks
	message := &llm.Message{
		Role: "tool",
		Content: []llm.Content{
			&llm.ToolResultContent{
				Content:   "4",
				ToolUseID: "call_123",
			},
			&llm.ToolResultContent{
				Content:   "Found math formulas",
				ToolUseID: "call_456",
			},
		},
	}

	// Convert the message
	converted, err := convertMessages([]*llm.Message{message})
	assert.NoError(t, err)

	// Verify the conversion - should be two separate messages
	assert.Len(t, converted, 2)

	// Check first tool result message
	assert.Equal(t, "tool", converted[0].Role)
	assert.Equal(t, "4", converted[0].Content)
	assert.Equal(t, "call_123", converted[0].ToolCallID)

	// Check second tool result message
	assert.Equal(t, "tool", converted[1].Role)
	assert.Equal(t, "Found math formulas", converted[1].Content)
	assert.Equal(t, "call_456", converted[1].ToolCallID)
}

// Test for messages containing both text and tool use content
func TestConvertTextAndToolUseMessage(t *testing.T) {
	// Create a message with both text and tool use content blocks
	message := &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{
				Text: "I'll help you calculate that",
			},
			&llm.ToolUseContent{
				ID:    "call_123",
				Name:  "Calculator",
				Input: []byte(`{"expression":"2 + 2"}`),
			},
		},
	}

	// Convert the message
	converted, err := convertMessages(llm.Messages{message})
	assert.NoError(t, err)

	// Verify the conversion - should be a single message with text and tool call
	assert.Len(t, converted, 1)
	assert.Equal(t, "assistant", converted[0].Role)
	assert.Equal(t, "I'll help you calculate that", converted[0].Content)
	assert.Len(t, converted[0].ToolCalls, 1)
	assert.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)
}

// Test for tool use followed by tool result
func TestConvertToolUseAndResultMessages(t *testing.T) {
	// Create sequence of messages: first the assistant's tool use, then the tool result
	messages := []*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{
					ID:    "call_111",
					Name:  "Calculator",
					Input: []byte(`{"expression":"1 + 1"}`),
				},
				&llm.ToolUseContent{
					ID:    "call_999",
					Name:  "Calculator",
					Input: []byte(`{"expression":"2 + 2"}`),
				},
			},
		},
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.ToolResultContent{
					Content:   "1",
					ToolUseID: "call_111",
				},
				&llm.ToolResultContent{
					Content:   "2",
					ToolUseID: "call_999",
				},
			},
		},
	}

	// Convert the messages. The tool result content blocks are split across
	// two messages (how OpenAI does it).
	converted, err := convertMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, converted, 3)

	assert.Equal(t, "assistant", converted[0].Role)
	assert.Len(t, converted[0].ToolCalls, 2)
	assert.Equal(t, "call_111", converted[0].ToolCalls[0].ID)
	assert.Equal(t, "call_999", converted[0].ToolCalls[1].ID)

	assert.Equal(t, "tool", converted[1].Role)
	assert.Equal(t, "1", converted[1].Content)
	assert.Equal(t, "call_111", converted[1].ToolCallID)

	assert.Equal(t, "tool", converted[2].Role)
	assert.Equal(t, "2", converted[2].Content)
	assert.Equal(t, "call_999", converted[2].ToolCallID)

}

func TestConvertMixedToolResultsAndAuxTextKeepsToolBatchContiguous(t *testing.T) {
	assistant := &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_K4TI", Name: "memory_recall", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_LQW", Name: "read_file", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_nSK", Name: "grep", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_xlO", Name: "list_dir", Input: []byte(`{}`)},
		},
	}
	toolResults := (&llm.Message{
		Role: llm.User,
		Content: []llm.Content{
			toolResult("call_K4TI", "memory result"),
			&llm.TextContent{Text: "No memories matched this recall."},
			toolResult("call_LQW", "read result"),
			toolResult("call_nSK", "grep result"),
			toolResult("call_xlO", "list result"),
		},
	}).Copy()

	converted, err := convertMessages([]*llm.Message{assistant, toolResults})
	assert.NoError(t, err)
	assert.Len(t, converted, 6)

	assert.Equal(t, converted[0].Role, "assistant")
	assert.Len(t, converted[0].ToolCalls, 4)

	for i, want := range []struct {
		id      string
		content string
	}{
		{id: "call_K4TI", content: "memory result"},
		{id: "call_LQW", content: "read result"},
		{id: "call_nSK", content: "grep result"},
		{id: "call_xlO", content: "list result"},
	} {
		msg := converted[i+1]
		assert.Equal(t, msg.Role, "tool")
		assert.Equal(t, msg.ToolCallID, want.id)
		assert.Equal(t, msg.Content, want.content)
	}

	assert.Equal(t, converted[5].Role, "user")
	assert.Equal(t, converted[5].Content, "No memories matched this recall.")
	assert.Equal(t, converted[5].ToolCallID, "")
}

func toolResult(id, text string) *llm.ToolResultContent {
	return &llm.ToolResultContent{
		ToolUseID: id,
		Content: []*dive.ToolResultContent{{
			Type: dive.ToolResultContentTypeText,
			Text: text,
		}},
	}
}

// TestConvertMessagesSkipsThinkingContent verifies that ThinkingContent (which
// this provider's stream iterator can produce from "reasoning" deltas)
// round-trips through convertMessages without error: it is skipped on encode
// since the Chat Completions API has no field for replaying reasoning.
func TestConvertMessagesSkipsThinkingContent(t *testing.T) {
	messages := []*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ThinkingContent{Thinking: "Let me think about this..."},
				&llm.TextContent{Text: "The answer is 4."},
			},
		},
	}
	result, err := convertMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "assistant", result[0].Role)
	assert.Equal(t, "The answer is 4.", result[0].Content)
}

func TestMistralThinkingChunksDecodeAndReplay(t *testing.T) {
	var message Message
	assert.NoError(t, json.Unmarshal([]byte(`{
		"role":"assistant",
		"content":[
			{"type":"thinking","thinking":[{"type":"text","text":"First reason. "},{"type":"text","text":"Second reason."}]},
			{"type":"text","text":"Final answer."}
		]
	}`), &message))

	content := responseMessageContent(message, "mistral-mistral-small-latest")
	assert.Len(t, content, 2)
	thinking, ok := content[0].(*llm.ThinkingContent)
	assert.True(t, ok)
	assert.Equal(t, "First reason. Second reason.", thinking.Thinking)
	assert.Equal(t, "true", thinking.Metadata[mistralThinkingMetadataKey])
	answer, ok := content[1].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "Final answer.", answer.Text)

	replayed, err := convertMessagesForProvider([]*llm.Message{{
		Role:    llm.Assistant,
		Content: content,
	}}, "mistral-mistral-small-latest")
	assert.NoError(t, err)
	assert.Len(t, replayed, 1)
	body, err := json.Marshal(replayed[0])
	assert.NoError(t, err)
	assert.Equal(t,
		`{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"First reason. Second reason."}]},{"type":"text","text":"Final answer."}]}`,
		string(body))
}

func TestMistralDoesNotReplayAnotherProvidersThinking(t *testing.T) {
	messages, err := convertMessagesForProvider([]*llm.Message{{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{Thinking: "anthropic reasoning", Signature: "anthropic-signature"},
			&llm.TextContent{Text: "answer"},
		},
	}}, "mistral-mistral-small-latest")
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "answer", messages[0].Content)
	assert.Empty(t, messages[0].ContentParts)
}

func TestOpenRouterReasoningDetailsDecodeAndReplay(t *testing.T) {
	var message Message
	assert.NoError(t, json.Unmarshal([]byte(`{
		"role":"assistant",
		"content":"answer",
		"reasoning":"summary",
		"reasoning_details":[{"type":"reasoning.text","text":"summary","signature":"sig","format":"anthropic-claude-v1","index":0}]
	}`), &message))

	content := responseMessageContent(message, "openrouter")
	assert.Len(t, content, 2)
	thinking := content[0].(*llm.ThinkingContent)
	assert.Equal(t, "summary", thinking.Thinking)
	assert.True(t, json.Valid([]byte(thinking.Metadata[openRouterReasoningDetailsMetadataKey])))

	replayed, err := convertMessagesForProvider([]*llm.Message{{
		Role:    llm.Assistant,
		Content: content,
	}}, "openrouter")
	assert.NoError(t, err)
	assert.Len(t, replayed, 1)
	assert.Equal(t, "answer", replayed[0].Content)
	assert.Empty(t, replayed[0].Reasoning)
	assert.Equal(t, message.ReasoningDetails, replayed[0].ReasoningDetails)
}

func TestOpenRouterReplaysMultipleReasoningDetailBlocks(t *testing.T) {
	messages, err := convertMessagesForProvider([]*llm.Message{{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{Metadata: llm.ProviderMetadata{
				openRouterReasoningDetailsMetadataKey: `[{"type":"reasoning.text","text":"first"}]`,
			}},
			&llm.ThinkingContent{Metadata: llm.ProviderMetadata{
				openRouterReasoningDetailsMetadataKey: `[{"type":"reasoning.text","text":"second"}]`,
			}},
			&llm.TextContent{Text: "answer"},
		},
	}}, "openrouter")
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
	var details []map[string]any
	assert.NoError(t, json.Unmarshal(messages[0].ReasoningDetails, &details))
	assert.Len(t, details, 2)
	assert.Equal(t, "first", details[0]["text"])
	assert.Equal(t, "second", details[1]["text"])
}

func TestMessageMarshalContentPartsPreservesOtherFields(t *testing.T) {
	message := Message{
		Role:         "assistant",
		ContentParts: []ContentPart{{Type: "text", Text: "answer"}},
		Name:         "named-assistant",
		Reasoning:    "reasoning",
		ReasoningDetails: json.RawMessage(
			`[{"type":"reasoning.text","text":"reasoning"}]`,
		),
	}
	data, err := json.Marshal(message)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"content":[{"type":"text","text":"answer"}]`)
	assert.Contains(t, string(data), `"name":"named-assistant"`)
	assert.Contains(t, string(data), `"reasoning":"reasoning"`)
	assert.Contains(t, string(data), `"reasoning_details":[{"type":"reasoning.text","text":"reasoning"}]`)
}

func TestResponseMessageContentPreservesPlainReasoning(t *testing.T) {
	content := responseMessageContent(Message{
		Content:   "answer",
		Reasoning: "visible reasoning",
	}, "openrouter")
	assert.Len(t, content, 2)
	thinking := content[0].(*llm.ThinkingContent)
	assert.Equal(t, "visible reasoning", thinking.Thinking)
	assert.Equal(t, "true", thinking.Metadata[openRouterReasoningMetadataKey])
}

func TestMessageIgnoresNullReasoningDetails(t *testing.T) {
	var message Message
	assert.NoError(t, json.Unmarshal([]byte(`{
		"role":"assistant",
		"content":"answer",
		"reasoning_details":null
	}`), &message))
	assert.Nil(t, message.ReasoningDetails)
	content := responseMessageContent(message, "openrouter")
	assert.Len(t, content, 1)
	assert.Equal(t, "answer", content[0].(*llm.TextContent).Text)
}

// TestGenerateUsageDetails verifies that cached prompt tokens and reasoning
// tokens from a non-streaming response are carried into llm.Usage.
func TestGenerateUsageDetails(t *testing.T) {
	var result Response
	err := json.Unmarshal([]byte(`{
		"id": "chatcmpl-9",
		"object": "chat.completion",
		"model": "gpt-5",
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello"}, "finish_reason": "stop"}],
		"usage": {
			"prompt_tokens": 200,
			"completion_tokens": 25,
			"total_tokens": 225,
			"prompt_tokens_details": {"cached_tokens": 100, "cache_write_tokens": 50},
			"completion_tokens_details": {"reasoning_tokens": 10}
		}
	}`), &result)
	assert.NoError(t, err)
	usage := result.Usage.toLLMUsage()
	assert.Equal(t, 50, usage.InputTokens)
	assert.Equal(t, 25, usage.OutputTokens)
	assert.Equal(t, 100, usage.CacheReadInputTokens)
	assert.Equal(t, 50, usage.CacheCreationInputTokens)
	assert.Equal(t, 200, usage.TotalInputTokens())
	assert.Equal(t, 10, usage.ReasoningTokens)
}

func TestToLLMUsageClampsInvalidCacheCounts(t *testing.T) {
	tests := []struct {
		name       string
		prompt     int
		cached     int
		written    int
		wantInput  int
		wantCached int
		wantWrite  int
	}{
		{name: "valid", prompt: 100, cached: 20, written: 70, wantInput: 10, wantCached: 20, wantWrite: 70},
		{name: "cached above prompt", prompt: 10, cached: 20, wantInput: 0, wantCached: 10},
		{name: "write above remainder", prompt: 10, cached: 7, written: 8, wantInput: 0, wantCached: 7, wantWrite: 3},
		{name: "negative cached", prompt: 10, cached: -20, written: -5, wantInput: 10, wantCached: 0},
		{name: "negative prompt", prompt: -10, cached: 5, written: 5, wantInput: 0, wantCached: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := Usage{
				PromptTokens: tt.prompt,
				PromptTokensDetails: &PromptTokensDetails{
					CachedTokens:     tt.cached,
					CacheWriteTokens: tt.written,
				},
			}.toLLMUsage()
			assert.Equal(t, tt.wantInput, usage.InputTokens)
			assert.Equal(t, tt.wantCached, usage.CacheReadInputTokens)
			assert.Equal(t, tt.wantWrite, usage.CacheCreationInputTokens)
			assert.Equal(t, max(0, tt.prompt), usage.TotalInputTokens())
		})
	}
}
