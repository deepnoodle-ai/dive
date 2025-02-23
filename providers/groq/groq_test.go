package groq

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

	text := strings.ToLower(response.Message().Text())
	require.Contains(t, text, "hello")
}
