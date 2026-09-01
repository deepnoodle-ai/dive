package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// Gemini 2.5 Pro has no thinking-level parameter, so a requested effort is
// served as a thinking budget. The response is where that translation, and the
// clamp to the model's bounds, become visible to the caller.
func TestGenerateReportsEffectiveControls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(minimalGenerateResponse))
	}))
	defer server.Close()

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	assert.NoError(t, err)

	config := llm.Config{
		Model:           ModelGemini25Pro,
		ReasoningEffort: llm.ReasoningEffortMax,
	}
	plan, ok := modelcaps.Preview("google", config)
	assert.True(t, ok)

	p := New(WithModel(ModelGemini25Pro), WithMaxRetries(0))
	p.client = client

	response, err := p.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithReasoningEffort(llm.ReasoningEffortMax),
	)
	assert.NoError(t, err)

	controls := response.Usage.Controls
	assert.NotNil(t, controls)
	assert.True(t, controls.Equal(plan.Effective))
	assert.Equal(t, llm.ReasoningEffort(""), controls.ReasoningEffort)
	assert.NotNil(t, controls.ReasoningBudget)
}
