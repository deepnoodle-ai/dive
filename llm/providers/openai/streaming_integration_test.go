//go:build integration

package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/stretchr/testify/require"
)

// Simple tool for testing
type TestCalculatorTool struct{}

func (t *TestCalculatorTool) Name() string {
	return "calculator"
}

func (t *TestCalculatorTool) Description() string {
	return "Perform basic arithmetic calculations"
}

func (t *TestCalculatorTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"operation": {
				Type:        "string",
				Description: "The operation to perform",
				Enum:        []string{"add", "subtract", "multiply", "divide"},
			},
			"a": {
				Type:        "number",
				Description: "First number",
			},
			"b": {
				Type:        "number",
				Description: "Second number",
			},
		},
		Required: []string{"operation", "a", "b"},
	}
}

func (t *TestCalculatorTool) Execute(ctx context.Context, params string) (string, error) {
	var args struct {
		Operation string  `json:"operation"`
		A         float64 `json:"a"`
		B         float64 `json:"b"`
	}

	if err := json.Unmarshal([]byte(params), &args); err != nil {
		return "", err
	}

	var result float64
	switch args.Operation {
	case "add":
		result = args.A + args.B
	case "subtract":
		result = args.A - args.B
	case "multiply":
		result = args.A * args.B
	case "divide":
		if args.B == 0 {
			return "Error: Division by zero", nil
		}
		result = args.A / args.B
	}

	resultBytes, _ := json.Marshal(map[string]float64{"result": result})
	return string(resultBytes), nil
}

func TestStreamingBasicText(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	provider := New(
		WithAPIKey(apiKey),
		WithModel("gpt-4o"), // Use the standard model for integration tests
	)

	ctx := context.Background()

	// Now test streaming
	t.Logf("Creating stream...")
	stream, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Say 'Hello, world!' and nothing else.")),
		llm.WithMaxTokens(50),
	)
	require.NoError(t, err)
	defer stream.Close()

	t.Logf("Stream created successfully, starting iteration...")

	var events []*llm.Event
	var hasText bool
	eventCount := 0
	var textAccumulator string

	for stream.Next() {
		eventCount++
		event := stream.Event()
		events = append(events, event)
		t.Logf("Event %d: Type=%s", eventCount, event.Type)

		// Check if we received text content
		if event.Type == llm.EventTypeContentBlockDelta &&
			event.Delta != nil &&
			event.Delta.Type == llm.EventDeltaTypeText {
			hasText = true
			textAccumulator += event.Delta.Text
			t.Logf("Received text delta: %s", event.Delta.Text)
		}

		// Safety break to avoid infinite loops
		if eventCount > 100 {
			t.Logf("Breaking after 100 events to avoid infinite loop")
			break
		}
	}

	t.Logf("Stream ended with error: %v", stream.Err())
	t.Logf("Total events received: %d", len(events))

	require.NoError(t, stream.Err())
	require.NotEmpty(t, events)
	require.True(t, hasText, "Expected to receive text content in stream")

	// Verify we got start and stop events
	var hasStart, hasStop bool
	for _, event := range events {
		if event.Type == llm.EventTypeMessageStart {
			hasStart = true
		}
		if event.Type == llm.EventTypeMessageStop {
			hasStop = true
		}
	}
	require.True(t, hasStart, "Expected message start event")
	require.True(t, hasStop, "Expected message stop event")
	require.Equal(t, "Hello, world!", textAccumulator)
}

func TestStreamingWithToolCall(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	provider := New(
		WithAPIKey(apiKey),
		WithModel("gpt-4o"), // Use the standard model for integration tests
	)

	tool := &TestCalculatorTool{}

	ctx := context.Background()
	stream, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Calculate 15 + 27 using the calculator tool.")),
		llm.WithTools(tool),
		llm.WithMaxTokens(100),
	)
	require.NoError(t, err)
	defer stream.Close()

	var events []*llm.Event
	var hasToolUse bool
	var toolUseID, toolName, toolInput string

	for stream.Next() {
		event := stream.Event()
		events = append(events, event)

		// Check if we received a tool use
		if event.ContentBlock != nil && event.ContentBlock.Type == llm.ContentTypeToolUse {
			hasToolUse = true
			if event.ContentBlock.ID != "" {
				toolUseID = event.ContentBlock.ID
			}
			if event.ContentBlock.Name != "" {
				toolName = event.ContentBlock.Name
			}
			fmt.Printf("Content block: %+v\n", event.ContentBlock)
			if event.ContentBlock.Input != nil {
				toolInput += string(event.ContentBlock.Input)
			}
		} else if event.Delta != nil && event.Delta.Type == llm.EventDeltaTypeInputJSON {
			fmt.Printf("Delta: %+v\n", event.Delta)
			toolInput += event.Delta.PartialJSON
		}
	}

	require.NoError(t, stream.Err())
	require.NotEmpty(t, events)
	require.True(t, hasToolUse, "Expected to receive tool use in stream")

	// Verify we got start and stop events
	var hasStart, hasStop bool
	for _, event := range events {
		if event.Type == llm.EventTypeMessageStart {
			hasStart = true
		}
		if event.Type == llm.EventTypeMessageStop {
			hasStop = true
		}
	}
	require.True(t, hasStart, "Expected message start event")
	require.True(t, hasStop, "Expected message stop event")

	t.Logf("Tool use ID: %s", toolUseID)
	t.Logf("Tool name: %s", toolName)
	t.Logf("Tool input: %s", toolInput)

	require.True(t, strings.HasPrefix(toolUseID, "call_"))
	require.Equal(t, "calculator", toolName)
	require.Equal(t, `{"a":15,"b":27,"operation":"add"}`, toolInput)
}

func TestStreamingReasoning(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	provider := New(
		WithAPIKey(apiKey),
		WithModel("o3"),
	)

	stream, err := provider.Stream(
		context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("What is the derivative of x^3 + 2x^2 - 5x + 1?")),
		llm.WithReasoningEffort(llm.ReasoningEffortHigh),
		llm.WithReasoningSummary(llm.ReasoningSummaryDetailed),
		llm.WithMaxTokens(16000),
	)
	require.NoError(t, err)
	defer stream.Close()

	var thinkingAccum, responseAccum string
	for stream.Next() {
		event := stream.Event()
		if event.Delta != nil {
			if event.Delta.Type == llm.EventDeltaTypeThinking {
				thinkingAccum += event.Delta.Thinking
			} else if event.Delta.Type == llm.EventDeltaTypeText {
				responseAccum += event.Delta.Text
			}
		}
	}

	require.NoError(t, stream.Err())
	require.NotEmpty(t, thinkingAccum, "Expected to receive thinking content")
	// It should be "3x² + 4x − 5" but leave off a chunk of that because
	// the formatting can vary
	require.True(t, strings.Contains(thinkingAccum, " + 4x"),
		"Expected to find derivative result, got: %s", thinkingAccum)
	require.Contains(t, strings.ToLower(thinkingAccum), "derivative")

	require.True(t, strings.Contains(responseAccum, " + 4x"),
		"Expected to find derivative result, got: %s", responseAccum)
	require.Contains(t, strings.ToLower(responseAccum), "derivative")

	t.Logf("Received %d characters of thinking content", len(thinkingAccum))
	t.Logf("Final response: %s", responseAccum)
}
