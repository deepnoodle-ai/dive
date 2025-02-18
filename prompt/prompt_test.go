package prompt

import (
	"testing"

	"github.com/getstingrai/agents/llm"
	"github.com/stretchr/testify/require"
)

func TestBasicPrompt(t *testing.T) {
	p := New(
		WithSystem("You are a helpful assistant named {{.Name}}."),
		WithMessage(llm.User, "Hello, how are you?"),
	)
	msgs, err := p.Build(map[string]any{"Name": "John"})
	require.NoError(t, err)
	require.Equal(t, 2, len(msgs))

	require.Equal(t, llm.Message{
		Role: llm.System,
		Content: []llm.Content{{
			Type: llm.ContentTypeText,
			Text: "You are a helpful assistant named John.",
		}},
	}, msgs[0])

	require.Equal(t, llm.Message{
		Role: llm.User,
		Content: []llm.Content{{
			Type: llm.ContentTypeText,
			Text: "Hello, how are you?",
		}},
	}, msgs[1])
}
