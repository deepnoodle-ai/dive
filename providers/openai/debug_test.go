//go:build integration

package openai

// Debug test utilities for investigating OpenAI streaming behavior.
// Run with: go test -tags integration -run TestDebug -v -timeout 5m ./...

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

func setupDebugProvider(t *testing.T) *Provider {
	t.Helper()
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	return New(WithAPIKey(apiKey))
}

// TestDebugRawStreamEvents dumps all raw SDK streaming events and the
// corresponding Dive events for a given prompt. Useful for diagnosing
// how the OpenAI API maps to Dive's event model.
func TestDebugRawStreamEvents(t *testing.T) {
	provider := setupDebugProvider(t)

	model := envOr("DEBUG_MODEL", "o3")
	prompt := envOr("DEBUG_PROMPT", "What is the derivative of x^3 + 2x^2 - 5x + 1?")

	provider.model = model

	config := &llm.Config{}
	opts := []llm.Option{
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
		llm.WithMaxTokens(16000),
	}
	if effort, ok := os.LookupEnv("DEBUG_EFFORT"); ok {
		if effort != "" {
			opts = append(opts, llm.WithReasoningEffort(llm.ReasoningEffort(effort)))
		}
	} else {
		opts = append(opts, llm.WithReasoningEffort(llm.ReasoningEffort("high")))
	}
	if summary, ok := os.LookupEnv("DEBUG_SUMMARY"); ok {
		if summary != "" {
			opts = append(opts, llm.WithReasoningSummary(llm.ReasoningSummary(summary)))
		}
	} else {
		opts = append(opts, llm.WithReasoningSummary(llm.ReasoningSummary("detailed")))
	}
	config.Apply(opts...)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)

	t.Logf("Model: %s", model)
	t.Logf("Include: %v", params.Include)

	ctx := testContext(t)
	sdkStream := provider.client.Responses.NewStreaming(ctx, params)
	iter := newOpenAIStreamIterator(sdkStream, config)
	defer iter.Close()

	for sdkStream.Next() {
		rawEvent := sdkStream.Current()

		// Print raw SDK event type and key details
		switch data := rawEvent.AsAny().(type) {
		case responses.ResponseCreatedEvent:
			fmt.Printf("RAW %-45s ID=%s\n", "ResponseCreated", data.Response.ID)
		case responses.ResponseOutputItemAddedEvent:
			fmt.Printf("RAW %-45s Type=%s OutputIndex=%d\n", "OutputItemAdded", data.Item.Type, data.OutputIndex)
		case responses.ResponseOutputItemDoneEvent:
			fmt.Printf("RAW %-45s Type=%s OutputIndex=%d\n", "OutputItemDone", data.Item.Type, data.OutputIndex)
			if data.Item.Type == "reasoning" {
				r := data.Item.AsReasoning()
				fmt.Printf("    EncryptedContent=%v SummaryCount=%d\n", r.EncryptedContent != "", len(r.Summary))
				for i, s := range r.Summary {
					text := s.Text
					if len(text) > 100 {
						text = text[:100] + "..."
					}
					fmt.Printf("    Summary[%d]: %q\n", i, text)
				}
			}
		case responses.ResponseContentPartAddedEvent:
			fmt.Printf("RAW %-45s Part.Type=%s\n", "ContentPartAdded", data.Part.Type)
		case responses.ResponseTextDeltaEvent:
			fmt.Printf("RAW %-45s len=%d\n", "TextDelta", len(data.Delta))
		case responses.ResponseTextDoneEvent:
			fmt.Printf("RAW %-45s len=%d\n", "TextDone", len(data.Text))
		case responses.ResponseFunctionCallArgumentsDeltaEvent:
			fmt.Printf("RAW %-45s delta=%q\n", "FnCallArgsDelta", data.Delta)
		case responses.ResponseFunctionCallArgumentsDoneEvent:
			fmt.Printf("RAW %-45s args=%q\n", "FnCallArgsDone", data.Arguments)
		case responses.ResponseReasoningSummaryPartAddedEvent:
			fmt.Printf("RAW %-45s Part.Type=%s\n", "ReasoningSummaryPartAdded", data.Part.Type)
		case responses.ResponseReasoningSummaryTextDeltaEvent:
			fmt.Printf("RAW %-45s delta_len=%d\n", "ReasoningSummaryTextDelta", len(data.Delta))
		case responses.ResponseReasoningSummaryPartDoneEvent:
			fmt.Printf("RAW %-45s Part.Text_len=%d\n", "ReasoningSummaryPartDone", len(data.Part.Text))
		case responses.ResponseCompletedEvent:
			fmt.Printf("RAW %-45s OutputCount=%d\n", "Completed", len(data.Response.Output))
		case responses.ResponseFailedEvent:
			fmt.Printf("RAW %-45s Code=%s Msg=%s\n", "Failed", data.Response.Error.Code, data.Response.Error.Message)
		case responses.ResponseIncompleteEvent:
			fmt.Printf("RAW %-45s Reason=%s\n", "Incomplete", data.Response.IncompleteDetails.Reason)
		default:
			fmt.Printf("RAW %-45s\n", fmt.Sprintf("%T", data))
		}

		// Process through Dive's event handler
		events, err := iter.processOpenAIEvent(rawEvent)
		assert.NoError(t, err, "processOpenAIEvent error")
		for _, ev := range events {
			fmt.Printf("    -> DIVE %-25s", ev.Type)
			if ev.ContentBlock != nil {
				fmt.Printf(" CBType=%s", ev.ContentBlock.Type)
			}
			if ev.Delta != nil && ev.Delta.Type != "" {
				fmt.Printf(" DeltaType=%s", ev.Delta.Type)
			}
			fmt.Println()
		}
	}
	assert.NoError(t, sdkStream.Err(), "stream error")
}

// TestDebugNonStreamingResponse dumps the non-streaming response for comparison.
func TestDebugNonStreamingResponse(t *testing.T) {
	provider := setupDebugProvider(t)

	model := envOr("DEBUG_MODEL", "o3")
	prompt := envOr("DEBUG_PROMPT", "What is the derivative of x^3 + 2x^2 - 5x + 1?")

	provider.model = model

	config := &llm.Config{}
	opts := []llm.Option{
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
		llm.WithMaxTokens(16000),
	}
	if effort, ok := os.LookupEnv("DEBUG_EFFORT"); ok {
		if effort != "" {
			opts = append(opts, llm.WithReasoningEffort(llm.ReasoningEffort(effort)))
		}
	} else {
		opts = append(opts, llm.WithReasoningEffort(llm.ReasoningEffort("high")))
	}
	if summary, ok := os.LookupEnv("DEBUG_SUMMARY"); ok {
		if summary != "" {
			opts = append(opts, llm.WithReasoningSummary(llm.ReasoningSummary(summary)))
		}
	} else {
		opts = append(opts, llm.WithReasoningSummary(llm.ReasoningSummary("detailed")))
	}
	config.Apply(opts...)

	params, err := provider.buildRequestParams(config)
	assert.NoError(t, err)

	ctx := testContext(t)
	resp, err := provider.client.Responses.New(ctx, params)
	assert.NoError(t, err)

	t.Logf("Response ID: %s", resp.ID)
	t.Logf("Model: %s", resp.Model)
	t.Logf("Status: %s", resp.Status)
	t.Logf("Usage: input=%d output=%d cached=%d",
		resp.Usage.InputTokens, resp.Usage.OutputTokens,
		resp.Usage.InputTokensDetails.CachedTokens)

	for i, item := range resp.Output {
		fmt.Printf("Output[%d]: Type=%s ID=%s\n", i, item.Type, item.ID)
		switch item.Type {
		case "reasoning":
			r := item.AsReasoning()
			fmt.Printf("  EncryptedContent present: %v\n", r.EncryptedContent != "")
			fmt.Printf("  Summary count: %d\n", len(r.Summary))
			for j, s := range r.Summary {
				fmt.Printf("  Summary[%d] (Type=%s): %s\n", j, s.Type, s.Text)
			}
		case "message":
			msg := item.AsMessage()
			for j, c := range msg.Content {
				fmt.Printf("  Content[%d]: Type=%s\n", j, c.Type)
				if c.Type == "output_text" {
					text := c.AsOutputText().Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					fmt.Printf("    Text: %s\n", text)
				}
			}
		}
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	timeout := 30 * time.Second
	if deadline, ok := t.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
