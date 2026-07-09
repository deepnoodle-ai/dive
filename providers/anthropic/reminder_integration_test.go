//go:build integration

package anthropic

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestIntegration_OperatorReminderNativeSystemRole(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	provider := New(WithModel(ModelClaudeOpus48), WithMaxTokens(32))
	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("Follow the runtime instruction and output only its requested token."),
			&llm.Message{Role: llm.System, Content: []llm.Content{&llm.ReminderContent{
				Name: "integration-test", Tier: llm.ReminderTierOperator,
				Content: "Reply with exactly DIVE_ANTHROPIC_REMINDER_OK and no punctuation.",
			}}},
		),
		llm.WithOperatorAuthority(llm.OperatorAuthorityStrict),
	)
	assert.NoError(t, err)
	assert.Equal(t, "DIVE_ANTHROPIC_REMINDER_OK", strings.TrimSpace(response.Message().Text()))
}
