package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/dive/todo"
	"github.com/deepnoodle-ai/wonton/assert"
)

// integrationLLM is a lightweight scripted LLM used only by this
// integration test. It returns pre-programmed assistant responses in order
// and records the messages it received so assertions can inspect what the
// agent actually passed to the model across turns.
type integrationLLM struct {
	mu       sync.Mutex
	script   []integrationTurn
	idx      int
	received [][]*llm.Message
}

type integrationTurn struct {
	text     string
	toolUses []*llm.ToolUseContent
}

func (s *integrationLLM) Name() string { return "integration-llm" }

func (s *integrationLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	snap := make([]*llm.Message, len(cfg.Messages))
	for i, m := range cfg.Messages {
		snap[i] = m.Copy()
	}
	s.received = append(s.received, snap)
	if s.idx >= len(s.script) {
		return nil, fmt.Errorf("integrationLLM: unexpected call %d", s.idx+1)
	}
	turn := s.script[s.idx]
	s.idx++
	var content []llm.Content
	stop := "stop"
	if len(turn.toolUses) > 0 {
		for _, tu := range turn.toolUses {
			content = append(content, tu)
		}
		stop = "tool_use"
	} else {
		content = append(content, &llm.TextContent{Text: turn.text})
	}
	return &llm.Response{
		ID:         fmt.Sprintf("resp_%d", s.idx),
		Model:      s.Name(),
		Role:       llm.Assistant,
		Content:    content,
		Type:       "message",
		StopReason: stop,
		Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
	}, nil
}

// summaryLLM is the LLM used by compaction.CompactMessages during the test;
// it always returns a canned <summary>-wrapped response.
type summaryLLM struct{}

func (s *summaryLLM) Name() string { return "summary-stub" }

func (s *summaryLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return &llm.Response{
		ID:    "sum_1",
		Model: s.Name(),
		Role:  llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{Text: "<summary>compacted prior work</summary>"},
		},
		Type:       "message",
		StopReason: "stop",
		Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
	}, nil
}

func todoWriteToolUse(id string, items []todo.TodoItem) *llm.ToolUseContent {
	input, _ := json.Marshal(todo.WriteInput{Todos: items})
	return &llm.ToolUseContent{ID: id, Name: todo.ToolName, Input: input}
}

func containsTodoStateBlock(messages []*llm.Message) bool {
	for _, msg := range messages {
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			if strings.Contains(tc.Text, "<todo-state>") {
				return true
			}
		}
	}
	return false
}

// TestIntegration_CompactionPreservesTodoStateEndToEnd drives the full
// agent → TodoWrite → session → compaction pipeline end-to-end with real
// Agent, real Session, real todo.Extension, and real compaction.
// CompactMessages. It pins three behaviors:
//
//  1. The hidden <todo-state> block written by the TodoWrite extension
//     hook survives a call to session.Compact that delegates to
//     compaction.CompactMessages.
//  2. findLatestState on the compacted session recovers the latest todos.
//  3. The post-compaction turns-since-write count is at least the
//     pre-compaction count — compaction must not reset staleness.
func TestIntegration_CompactionPreservesTodoStateEndToEnd(t *testing.T) {
	items := []todo.TodoItem{
		{Content: "Investigate incident", Status: todo.TodoStatusInProgress, ActiveForm: "Investigating incident"},
	}

	mock := &integrationLLM{
		script: []integrationTurn{
			{toolUses: []*llm.ToolUseContent{todoWriteToolUse("tu-1", items)}},
			{text: "wrote it"},
			{text: "still investigating"},
			{text: "more progress"},
		},
	}
	sess := session.New("integration-compact")
	ext := todo.New(todo.WithReminderTurns(10))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:      mock,
		Session:    sess,
		Extensions: []dive.Extension{ext},
	})
	assert.NoError(t, err)

	for _, input := range []string{"start", "turn2", "turn3"} {
		_, err := agent.CreateResponse(context.Background(), dive.WithInput(input))
		assert.NoError(t, err)
	}

	// Sanity: state block is there before compaction.
	pre, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.True(t, containsTodoStateBlock(pre), "state block should be in pre-compaction history")
	_, preTurns, found := todo.LatestState(pre)
	assert.True(t, found)

	summarize := func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		compacted, _, err := CompactMessages(ctx, &summaryLLM{}, msgs, "", "", 0)
		return compacted, err
	}
	assert.NoError(t, sess.Compact(context.Background(), summarize))

	post, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.True(t, containsTodoStateBlock(post), "state block should be preserved by compaction")

	latest, postTurns, found := todo.LatestState(post)
	assert.True(t, found)
	assert.Len(t, latest, 1)
	assert.Equal(t, "Investigate incident", latest[0].Content)
	assert.True(t, postTurns >= preTurns, "compaction must not regress turnsSinceWrite")

	// The post-compaction session must still support a fresh turn without
	// panicking or losing the state.
	mock.script = append(mock.script, integrationTurn{text: "after compact"})
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("after compact"))
	assert.NoError(t, err)

	after, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	latest2, _, found := todo.LatestState(after)
	assert.True(t, found)
	assert.Len(t, latest2, 1)
	assert.Equal(t, "Investigate incident", latest2[0].Content)
}
