package autotitle_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/autotitle"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Name() string { return "mock" }
func (m *mockLLM) Generate(_ context.Context, _ ...llm.Option) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		ID:         "resp_1",
		Model:      "mock",
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: m.response}},
		Type:       "message",
		StopReason: "stop",
		Usage:      llm.Usage{},
	}, nil
}

func makeHookContext(sess *session.Session, userText, responseText string) *dive.HookContext {
	hctx := dive.NewHookContext()
	hctx.Session = sess
	hctx.Messages = []*llm.Message{
		llm.NewUserTextMessage(userText),
		llm.NewAssistantTextMessage("ignored"),
	}
	// Build a minimal Response with the expected output text
	resp := &dive.Response{}
	resp.OutputMessages = []*llm.Message{
		llm.NewAssistantTextMessage(responseText),
	}
	hctx.Response = resp
	return hctx
}

func TestAutoTitleHook(t *testing.T) {
	ctx := context.Background()

	t.Run("sets title on first turn", func(t *testing.T) {
		titleLLM := &mockLLM{response: "Capital of France"}
		sess := session.New("s1")

		hook := autotitle.AutoTitleHook(titleLLM)
		hctx := makeHookContext(sess, "What is the capital of France?", "Paris is the capital of France.")

		err := hook(ctx, hctx)
		assert.NoError(t, err)
		assert.Equal(t, "Capital of France", sess.Title())
	})

	t.Run("skips if title already set", func(t *testing.T) {
		called := false
		titleLLM := &mockLLM{response: "Should Not Be Called"}
		_ = called

		sess := session.New("s2")
		sess.SetTitle("Existing Title")

		hook := autotitle.AutoTitleHook(titleLLM)
		hctx := makeHookContext(sess, "hello", "world")

		err := hook(ctx, hctx)
		assert.NoError(t, err)
		assert.Equal(t, "Existing Title", sess.Title())
	})

	t.Run("title LLM error is non-fatal", func(t *testing.T) {
		titleLLM := &mockLLM{err: fmt.Errorf("title LLM unavailable")}
		sess := session.New("s3")

		hook := autotitle.AutoTitleHook(titleLLM)
		hctx := makeHookContext(sess, "hello", "world")

		err := hook(ctx, hctx)
		assert.NoError(t, err) // hook does not surface LLM errors
		assert.Equal(t, "", sess.Title())
	})

	t.Run("no-op when session is nil", func(t *testing.T) {
		titleLLM := &mockLLM{response: "Title"}
		hook := autotitle.AutoTitleHook(titleLLM)

		hctx := dive.NewHookContext()
		hctx.Session = nil

		err := hook(ctx, hctx)
		assert.NoError(t, err)
	})
}
