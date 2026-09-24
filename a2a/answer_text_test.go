package a2a_test

import (
	"context"
	"iter"
	"net/http/httptest"
	"strings"
	"testing"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// An OpenAI commentary message followed by the final answer is two passages.
// The server sends the answer as one text part, and the remote client,
// blocking or streaming, gets OutputText.
func TestRemoteAgentSeparatesPhases(t *testing.T) {
	phase := func(text, p string) *llm.TextContent {
		return &llm.TextContent{Text: text, Metadata: llm.ProviderMetadata{"openai.phase": p}}
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
	assert.Len(t, task.Artifacts[0].Parts, 1)
	assert.Equal(t, want, task.Artifacts[0].Parts[0].Text())

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

// A server that streams its answer token by token sends artifact updates with
// Append set. The SDK stores each appended chunk as its own part, so the
// client must concatenate the parts exactly, adding nothing between them.
func TestRemoteAgentConcatenatesAppendedChunks(t *testing.T) {
	tokens := []string{"Hel", "lo, ", "world", "."}
	executor := a2asrv.AgentExecutorFunc(func(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
		return func(yield func(a2asdk.Event, error) bool) {
			if execCtx.StoredTask == nil {
				if !yield(a2asdk.NewSubmittedTask(execCtx, execCtx.Message), nil) {
					return
				}
			}
			if !yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateWorking, nil), nil) {
				return
			}
			first := a2asdk.NewArtifactEvent(execCtx, a2asdk.NewTextPart(tokens[0]))
			if !yield(first, nil) {
				return
			}
			for i, tok := range tokens[1:] {
				ev := a2asdk.NewArtifactUpdateEvent(execCtx, first.Artifact.ID, a2asdk.NewTextPart(tok))
				ev.LastChunk = i == len(tokens)-2
				if !yield(ev, nil) {
					return
				}
			}
			yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateCompleted, nil), nil)
		}
	})
	caps := &a2asdk.AgentCapabilities{Streaming: true}
	handler := a2asrv.NewHandler(executor, a2asrv.WithCapabilityChecks(caps))
	ts := httptest.NewServer(a2asrv.NewJSONRPCHandler(handler))
	t.Cleanup(ts.Close)
	card := &a2asdk.AgentCard{
		Capabilities: *caps,
		SupportedInterfaces: []*a2asdk.AgentInterface{{
			URL:             ts.URL,
			ProtocolBinding: a2asdk.TransportProtocolJSONRPC,
			ProtocolVersion: a2asdk.Version,
		}},
	}
	client, err := a2aclient.NewFromCard(context.Background(), card)
	assert.NoError(t, err)
	const want = "Hello, world."

	var chunks []string
	streamed, err := a2a.NewRemoteAgent(client).StreamText(context.Background(), "Hi", func(chunk string) {
		chunks = append(chunks, chunk)
	})
	assert.NoError(t, err)
	assert.Equal(t, tokens, chunks)
	assert.Equal(t, want, streamed.Text)

	remote, err := a2a.NewRemoteAgentFromURL(context.Background(), ts.URL)
	assert.NoError(t, err)
	result, err := remote.SendText(context.Background(), "Hi")
	assert.NoError(t, err)
	assert.Equal(t, want, result.Text)
}
