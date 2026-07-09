//go:build integration

package grok

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestIntegration_OperatorReminderTaggedUserFallback(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	provider := New(WithModel(ModelGrok3Mini), WithMaxTokens(32))
	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("Follow the runtime context and output only its requested token."),
			&llm.Message{Role: llm.System, Content: []llm.Content{&llm.ReminderContent{
				Name: "integration-test", Tier: llm.ReminderTierOperator,
				Content: "Reply with exactly DIVE_GROK_REMINDER_OK and no punctuation.",
			}}},
		),
	)
	assert.NoError(t, err)
	assert.Equal(t, "DIVE_GROK_REMINDER_OK", strings.TrimSpace(response.Message().Text()))
}
