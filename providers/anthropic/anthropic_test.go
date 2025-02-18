package anthropic

import (
	"context"
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
