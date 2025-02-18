package anthropic

import (
	"context"
	"strings"
	"testing"

	"github.com/getstingrai/agents/llm"
	"github.com/stretchr/testify/require"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, []*llm.Message{
		llm.NewUserMessage("respond with \"hello\""),
	})
	require.NoError(t, err)
	require.Equal(t, "hello", response.Message().Text())
}

func TestHelloWorldStream(t *testing.T) {
	ctx := context.Background()
	provider := New()
	stream, err := provider.Stream(ctx, []*llm.Message{
		llm.NewUserMessage("count to 10"),
	})
	require.NoError(t, err)

	var events []*llm.StreamEvent
	for {
		event, ok := stream.Next(ctx)
		if !ok {
			break
		}
		events = append(events, event)
	}

	var finalText string
	var texts []string
	for _, event := range events {
		switch event.Type {
		case llm.EventContentBlockDelta:
			numbers := strings.Fields(event.Delta.Text)
			texts = append(texts, numbers...)
			finalText = event.AccumulatedText
		}
	}
	require.Equal(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10", finalText)
	require.Equal(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10", strings.Join(texts, "\n"))
}

func TestToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := []*llm.Message{
		llm.NewUserMessage("add 567 and 111"),
	}

	add := llm.Tool{
		Name:        "add",
		Description: "Returns the sum of two numbers, \"a\" and \"b\"",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*llm.SchemaProperty{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		},
	}

	response, err := provider.Generate(ctx, messages,
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoice{
			Type: "tool",
			Name: "add",
		}),
	)
	require.NoError(t, err)

	require.Equal(t, 1, len(response.Message().Content))
	content := response.Message().Content[0]
	require.Equal(t, llm.ContentTypeToolUse, content.Type)
	require.Equal(t, "add", content.Name)
	require.Equal(t, `{"a":567,"b":111}`, string(content.Input))
}
