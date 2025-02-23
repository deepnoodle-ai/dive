package openai

import (
	"context"
	"strings"
	"testing"

	"github.com/getstingrai/agents/llm"
	"github.com/getstingrai/agents/tools"
	"github.com/stretchr/testify/require"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, []*llm.Message{
		llm.NewUserMessage("respond with \"hello\""),
	})
	require.NoError(t, err)

	// Oddly, uppercase "Hello" is often returned :-)
	require.Equal(t, "hello", strings.ToLower(response.Message().Text()))
}

func TestToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

	calculator := &tools.MockCalculatorTool{Result: "4"}

	response, err := provider.Generate(ctx, []*llm.Message{
		llm.NewUserMessage("What is 2 + 2?"),
	}, llm.WithTools(calculator))
	require.NoError(t, err)

	require.Len(t, response.ToolCalls(), 1)
	call := response.ToolCalls()[0]
	require.Equal(t, "Calculator", call.Name)
	require.Equal(t, `{"expression":"2 + 2"}`, call.Input)
}
