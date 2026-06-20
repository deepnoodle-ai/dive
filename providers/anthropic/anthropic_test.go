package anthropic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\""),
	))
	assert.NoError(t, err)
	assert.Equal(t, "hello", response.Message().Text())
}

func TestStreamCountTo10(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("count to 10. respond with the integers only, separated by spaces."),
	))
	assert.NoError(t, err)
	defer iterator.Close()

	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
	}
	assert.NoError(t, iterator.Err())

	var accumulatedText string
	for _, event := range events {
		switch event.Type {
		case llm.EventTypeContentBlockDelta:
			accumulatedText += event.Delta.Text
		}
	}

	expectedOutput := "1 2 3 4 5 6 7 8 9 10"
	normalizedText := strings.Join(strings.Fields(accumulatedText), " ")
	assert.Equal(t, expectedOutput, normalizedText)
}

func TestToolUse(t *testing.T) {
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
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithTools(add),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "add",
		}),
	)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(response.Message().Content))
	content := response.Message().Content[0]
	assert.Equal(t, llm.ContentTypeToolUse, content.Type())

	toolUse, ok := content.(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "add", toolUse.Name)
	assert.Equal(t, `{"a":567,"b":111}`, string(toolUse.Input))
}

func TestToolCallStream(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Define a simple calculator tool
	calculatorTool := llm.NewToolDefinition().
		WithName("calculator").
		WithDescription("Perform a calculation").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"operation", "a", "b"},
			Properties: map[string]*schema.Property{
				"operation": {
					Type:        "string",
					Description: "The operation to perform",
					Enum:        []any{"add", "subtract", "multiply", "divide"},
				},
				"a": {
					Type:        "number",
					Description: "The first operand",
				},
				"b": {
					Type:        "number",
					Description: "The second operand",
				},
			},
		})

	// Force the calculator tool so the test deterministically exercises the
	// tool-call streaming path. Without this, the model may answer "2+2"
	// directly and emit no tool call (flaky).
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("What is 2+2?")),
		llm.WithTools(calculatorTool),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "calculator",
		}),
	)

	assert.NoError(t, err)
	defer iterator.Close()

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
	assert.NotNil(t, response, "Should have received a final response")

	// Check if tool calls were properly processed
	toolCalls := response.ToolCalls()
	assert.Equal(t, 1, len(toolCalls))

	toolCall := toolCalls[0]
	assert.NotEmpty(t, toolCall.ID, "Tool call ID should not be empty")
	assert.NotEmpty(t, toolCall.Name, "Tool call name should not be empty")
	assert.NotEmpty(t, toolCall.Input, "Tool call input should not be empty")

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Input), &params); err != nil {
		t.Fatalf("Failed to unmarshal tool call input: %v", err)
	}

	assert.Equal(t, "add", params["operation"])
	assert.Equal(t, 2.0, params["a"])
	assert.Equal(t, 2.0, params["b"])
}

func TestDocumentContentHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    *llm.Message
		expected func(*testing.T, *llm.Message)
	}{
		{
			name: "DocumentContent with base64 data",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.DocumentContent{
						Title: "test.pdf",
						Source: &llm.ContentSource{
							Type:      llm.ContentSourceTypeBase64,
							MediaType: "application/pdf",
							Data:      "JVBERi0xLjQK...",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				assert.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				assert.True(t, ok, "Expected DocumentContent, got %T", msg.Content[0])
				assert.Equal(t, "test.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeBase64, docContent.Source.Type)
				assert.Equal(t, "application/pdf", docContent.Source.MediaType)
				assert.Equal(t, "JVBERi0xLjQK...", docContent.Source.Data)
			},
		},
		{
			name: "DocumentContent with URL",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.DocumentContent{
						Title: "remote.pdf",
						Source: &llm.ContentSource{
							Type: llm.ContentSourceTypeURL,
							URL:  "https://example.com/document.pdf",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				assert.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				assert.True(t, ok)
				assert.Equal(t, "remote.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeURL, docContent.Source.Type)
				assert.Equal(t, "https://example.com/document.pdf", docContent.Source.URL)
			},
		},
		{
			name: "DocumentContent with file ID",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.DocumentContent{
						Title: "api-file.pdf",
						Source: &llm.ContentSource{
							Type:   llm.ContentSourceTypeFile,
							FileID: "file-abc123",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				assert.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				assert.True(t, ok)
				assert.Equal(t, "api-file.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeFile, docContent.Source.Type)
				assert.Equal(t, "file-abc123", docContent.Source.FileID)
			},
		},
		{
			name: "Mixed content with DocumentContent and TextContent",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Please analyze this document:"},
					&llm.DocumentContent{
						Title: "report.pdf",
						Source: &llm.ContentSource{
							Type:      llm.ContentSourceTypeBase64,
							MediaType: "application/pdf",
							Data:      "JVBERi0xLjQK...",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				assert.Len(t, msg.Content, 2)

				// First content should remain as TextContent
				textContent, ok := msg.Content[0].(*llm.TextContent)
				assert.True(t, ok)
				assert.Equal(t, "Please analyze this document:", textContent.Text)

				// Second content should remain as DocumentContent
				docContent, ok := msg.Content[1].(*llm.DocumentContent)
				assert.True(t, ok)
				assert.Equal(t, "report.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeBase64, docContent.Source.Type)
				assert.Equal(t, "application/pdf", docContent.Source.MediaType)
				assert.Equal(t, "JVBERi0xLjQK...", docContent.Source.Data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []*llm.Message{tt.input}
			converted, err := convertMessages(messages)
			assert.NoError(t, err)
			assert.Len(t, converted, 1)
			tt.expected(t, converted[0])
		})
	}
}

// applyTestCaching builds a Request from system/messages and runs the hybrid
// cache placement against a provider configured for the default endpoint
// (automatic caching supported) unless overridden.
func applyTestCaching(t *testing.T, p *Provider, config *llm.Config, system string, messages []*llm.Message) *Request {
	t.Helper()
	converted, err := convertMessages(messages)
	assert.NoError(t, err)
	req := &Request{Messages: converted}
	if system != "" {
		req.System = []*SystemBlock{{Type: "text", Text: system}}
	}
	p.applyCaching(req, config)
	return req
}

func lastBlockCacheControl(msg *llm.Message) *llm.CacheControl {
	contents := msg.Content
	switch c := contents[len(contents)-1].(type) {
	case *llm.TextContent:
		return c.CacheControl
	case *llm.ToolResultContent:
		return c.CacheControl
	case *llm.ToolUseContent:
		return c.CacheControl
	}
	return nil
}

func countMessageBreakpoints(messages []*llm.Message) int {
	n := 0
	for _, m := range messages {
		for _, content := range m.Content {
			switch c := content.(type) {
			case *llm.TextContent:
				if c.CacheControl != nil {
					n++
				}
			case *llm.ToolResultContent:
				if c.CacheControl != nil {
					n++
				}
			case *llm.ToolUseContent:
				if c.CacheControl != nil {
					n++
				}
			}
		}
	}
	return n
}

func TestApplyCachingDoesNotMutateOriginal(t *testing.T) {
	// convertMessages clones content, so applyCaching must not touch the original.
	original := &llm.TextContent{Text: "hello"}
	messages := []*llm.Message{
		{Role: llm.User, Content: []llm.Content{original}},
	}
	applyTestCaching(t, New(), &llm.Config{}, "you are helpful", messages)
	assert.Nil(t, original.CacheControl)
}

func TestApplyCachingSystemBreakpoint(t *testing.T) {
	// The system prefix gets its own breakpoint, independent of the message tail.
	messages := []*llm.Message{llm.NewUserTextMessage("hi")}
	req := applyTestCaching(t, New(), &llm.Config{}, "you are helpful", messages)

	assert.Len(t, req.System, 1)
	assert.NotNil(t, req.System[0].CacheControl)
	assert.Equal(t, llm.CacheControlTypeEphemeral, req.System[0].CacheControl.Type)
	assert.Equal(t, "", req.System[0].CacheControl.TTL) // default 5m without extended cache
}

func TestApplyCachingAutomaticOwnsTail(t *testing.T) {
	// On the default endpoint, automatic caching owns the tail: top-level
	// cache_control is set and the last message block carries no explicit marker.
	messages := []*llm.Message{llm.NewUserTextMessage("hi")}
	req := applyTestCaching(t, New(), &llm.Config{}, "sys", messages)

	assert.NotNil(t, req.CacheControl)
	assert.Equal(t, llm.CacheControlTypeEphemeral, req.CacheControl.Type)
	assert.Nil(t, lastBlockCacheControl(req.Messages[len(req.Messages)-1]))
}

func TestApplyCachingPortabilityFallback(t *testing.T) {
	// On a non-default endpoint (e.g. Bedrock/Vertex/custom) automatic caching
	// is unavailable, so the tail gets an explicit breakpoint and no top-level
	// cache_control is set.
	p := New(WithEndpoint("https://bedrock.example.com/v1/messages"))
	messages := []*llm.Message{llm.NewUserTextMessage("hi")}
	req := applyTestCaching(t, p, &llm.Config{}, "sys", messages)

	assert.Nil(t, req.CacheControl)
	assert.NotNil(t, lastBlockCacheControl(req.Messages[len(req.Messages)-1]))
	assert.NotNil(t, req.System[0].CacheControl)
}

func TestApplyCachingOptOut(t *testing.T) {
	off := false
	messages := []*llm.Message{llm.NewUserTextMessage("hi")}
	req := applyTestCaching(t, New(), &llm.Config{Caching: &off}, "sys", messages)

	assert.Nil(t, req.CacheControl)
	assert.Nil(t, req.System[0].CacheControl)
	assert.Equal(t, 0, countMessageBreakpoints(req.Messages))
}

func TestApplyCachingOptOutClearsCallerMarkers(t *testing.T) {
	// Opt-out must strip caller-provided cache markers so none leak into the
	// request — including the top-level cache_control, system, and messages.
	off := false
	msgs, err := convertMessages([]*llm.Message{llm.NewUserTextMessage("hi")})
	assert.NoError(t, err)
	for _, m := range msgs {
		for _, c := range m.Content {
			if s, ok := c.(llm.CacheControlSetter); ok {
				s.SetCacheControl(&llm.CacheControl{Type: llm.CacheControlTypeEphemeral})
			}
		}
	}
	req := &Request{
		CacheControl: &llm.CacheControl{Type: llm.CacheControlTypeEphemeral},
		System:       []*SystemBlock{{Type: "text", Text: "sys", CacheControl: &llm.CacheControl{Type: llm.CacheControlTypeEphemeral}}},
		Messages:     msgs,
	}

	New().applyCaching(req, &llm.Config{Caching: &off})

	assert.Nil(t, req.CacheControl, "top-level cache_control should be cleared on opt-out")
	assert.Nil(t, req.System[0].CacheControl, "system marker should be cleared on opt-out")
	assert.Equal(t, 0, countMessageBreakpoints(req.Messages), "message markers should be cleared on opt-out")
}

func TestApplyCachingExtendedTTL(t *testing.T) {
	// With the extended-cache feature enabled, the stable prefix uses the
	// 1-hour TTL while the tail (automatic) stays at the default 5m.
	config := &llm.Config{}
	config.Apply(llm.WithFeatures(FeatureExtendedCache))
	messages := []*llm.Message{llm.NewUserTextMessage("hi")}
	req := applyTestCaching(t, New(), config, "sys", messages)

	assert.Equal(t, llm.CacheTTL1h, req.System[0].CacheControl.TTL)
	assert.Equal(t, "", req.CacheControl.TTL)
}

func TestApplyCachingNeverExceedsBudget(t *testing.T) {
	// A turn with a large tool-call fan-out must keep total breakpoints within
	// the API's 4-slot budget (automatic consumes one; explicit blocks <= 3).
	var assistant []llm.Content
	var results []llm.Content
	for range 13 {
		assistant = append(assistant, &llm.ToolUseContent{ID: "t", Name: "upsert", Input: json.RawMessage(`{}`)})
		results = append(results, &llm.ToolResultContent{ToolUseID: "t", Content: "ok"})
	}
	messages := []*llm.Message{
		llm.NewUserTextMessage("start"),
		{Role: llm.Assistant, Content: assistant},
		{Role: llm.User, Content: results},
	}
	req := applyTestCaching(t, New(), &llm.Config{}, "sys", messages)

	explicit := countMessageBreakpoints(req.Messages)
	if req.System[0].CacheControl != nil {
		explicit++
	}
	// Automatic caching consumes one slot, so explicit must stay <= 3.
	assert.True(t, explicit <= 3, "explicit breakpoints %d exceed budget", explicit)
}

func TestApplyCachingFanoutAnchorWithinWindow(t *testing.T) {
	// Regression for the incident: a turn appending >20 content blocks must
	// leave an anchor within the 20-block lookback window of the tail so the
	// message history is not fully rewritten.
	var assistant []llm.Content
	var results []llm.Content
	for range 13 {
		assistant = append(assistant, &llm.ToolUseContent{ID: "t", Name: "upsert", Input: json.RawMessage(`{}`)})
		results = append(results, &llm.ToolResultContent{ToolUseID: "t", Content: "ok"})
	}
	messages := []*llm.Message{
		llm.NewUserTextMessage("start"),
		{Role: llm.Assistant, Content: []llm.Content{llm.NewTextContent("older turn")}},
		{Role: llm.User, Content: []llm.Content{llm.NewTextContent("older user")}},
		{Role: llm.Assistant, Content: assistant},
		{Role: llm.User, Content: results},
	}
	req := applyTestCaching(t, New(), &llm.Config{}, "sys", messages)

	// Find the distance (in content blocks from the tail) of the nearest anchor.
	nearest := -1
	dist := 0
	for mi := len(req.Messages) - 1; mi >= 0; mi-- {
		contents := req.Messages[mi].Content
		for bi := len(contents) - 1; bi >= 0; bi-- {
			if cc := blockCacheControl(contents[bi]); cc != nil {
				if nearest == -1 {
					nearest = dist
				}
			}
			dist++
		}
	}
	assert.True(t, nearest >= 0, "expected at least one message anchor for the fan-out turn")
	assert.True(t, nearest <= cacheLookbackWindow, "nearest anchor %d blocks from tail exceeds lookback window", nearest)
}

func TestApplyCachingWireFormat(t *testing.T) {
	// The serialized request must render system as an array of text blocks (so a
	// breakpoint can attach) and carry a top-level cache_control for automatic
	// caching on the default endpoint.
	messages := []*llm.Message{llm.NewUserTextMessage("hi")}
	req := applyTestCaching(t, New(), &llm.Config{}, "you are helpful", messages)
	req.Model = "claude-opus-4-8"

	body, err := json.Marshal(req)
	assert.NoError(t, err)
	var decoded map[string]any
	assert.NoError(t, json.Unmarshal(body, &decoded))

	system, ok := decoded["system"].([]any)
	assert.True(t, ok, "system should marshal as an array")
	assert.Len(t, system, 1)
	block := system[0].(map[string]any)
	assert.Equal(t, "text", block["type"])
	assert.NotNil(t, block["cache_control"])

	cc, ok := decoded["cache_control"].(map[string]any)
	assert.True(t, ok, "top-level cache_control should be present")
	assert.Equal(t, "ephemeral", cc["type"])
}

func blockCacheControl(c llm.Content) *llm.CacheControl {
	switch v := c.(type) {
	case *llm.TextContent:
		return v.CacheControl
	case *llm.ToolResultContent:
		return v.CacheControl
	case *llm.ToolUseContent:
		return v.CacheControl
	}
	return nil
}
