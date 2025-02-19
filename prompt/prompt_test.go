package prompt

import (
	"testing"

	"github.com/getstingrai/agents/llm"
	"github.com/stretchr/testify/require"
)

func TestSimplePrompt(t *testing.T) {
	p, err := New(
		WithSystemMessage("Talk about {{.Topic}}"),
		WithUserMessage("Hello {{.Name}}, how are you?"),
	).Build(map[string]any{
		"Name":  "John",
		"Topic": "cats",
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(p.Messages))
	require.Equal(t, "Talk about cats", p.System)

	require.Equal(t, &llm.Message{
		Role: llm.User,
		Content: []llm.Content{{
			Type: llm.ContentTypeText,
			Text: "Hello John, how are you?",
		}},
	}, p.Messages[0])
}

func TestPromptWithDirectives(t *testing.T) {
	p, err := New(
		WithSystemMessage("Talk with a pirate accent"),
		WithUserMessage("Hello, how are you?"),
		WithDirective("You are a helpful pirate"),
	).Build()
	require.NoError(t, err)
	require.Equal(t, 1, len(p.Messages))

	expectedSystem := `Talk with a pirate accent

Directives:
- You are a helpful pirate
`
	require.Equal(t, expectedSystem, p.System)

	require.Equal(t, &llm.Message{
		Role: llm.User,
		Content: []llm.Content{{
			Type: llm.ContentTypeText,
			Text: "Hello, how are you?",
		}},
	}, p.Messages[0])
}
