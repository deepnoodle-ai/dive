package a2a_test

import (
	"context"
	"strings"
	"testing"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// An OpenAI commentary message followed by the final answer is two passages,
// even with nothing between them. The server gives each its own text part,
// and the remote client, blocking or streaming, gets OutputText.
func TestRemoteAgentSeparatesPhases(t *testing.T) {
	phase := func(text, p string) *llm.TextContent {
		return &llm.TextContent{Text: text, Metadata: llm.ProviderMetadata{llm.TextPhaseMetadataKey: p}}
	}
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		resp := textResponse("")
		resp.Content = []llm.Content{
			phase("Checking the numbers.", "commentary"),
			phase("Revenue grew ", "final_answer"),
			phase("12%.", "final_answer"),
		}
		return resp, nil
	}}
	agent := buildAgent(t, model)
	ts, client := startServer(t, agent)
	const want = "Checking the numbers.\n\nRevenue grew 12%."

	local, err := agent.CreateResponse(context.Background(), dive.WithInput("Revenue?"))
	assert.NoError(t, err)
	assert.Equal(t, want, local.OutputText())

	task := sendAndExpectTask(t, client, a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart("Revenue?")))
	assert.Len(t, task.Artifacts, 1)
	assert.Len(t, task.Artifacts[0].Parts, 2)

	remote, err := a2a.NewRemoteAgentFromURL(context.Background(), ts.URL)
	assert.NoError(t, err)
	result, err := remote.SendText(context.Background(), "Revenue?")
	assert.NoError(t, err)
	assert.Equal(t, want, result.Text)

	// The test client streams (its card declares streaming); a client built
	// from a bare URL falls back to a blocking send.
	var chunks []string
	streamed, err := a2a.NewRemoteAgent(client).StreamText(context.Background(), "Revenue?", func(chunk string) {
		chunks = append(chunks, chunk)
	})
	assert.NoError(t, err)
	assert.Equal(t, want, streamed.Text)
	assert.Equal(t, want, strings.Join(chunks, ""))
}
