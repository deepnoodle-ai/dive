package todo

import (
	"encoding/json"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

const (
	stateBlockStart = "<todo-state>"
	stateBlockEnd   = "</todo-state>"
)

type stateSnapshot struct {
	Todos           []TodoItem `json:"todos"`
	TurnsSinceWrite int        `json:"turnsSinceWrite"`
}

func formatStateBlock(items []TodoItem, turnsSinceWrite int) string {
	snap := stateSnapshot{
		Todos:           cloneTodos(items),
		TurnsSinceWrite: turnsSinceWrite,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return ""
	}
	return stateBlockStart + string(data) + stateBlockEnd
}

func parseStateBlock(text string) (stateSnapshot, bool) {
	var snap stateSnapshot
	start := strings.Index(text, stateBlockStart)
	if start < 0 {
		return snap, false
	}
	start += len(stateBlockStart)
	end := strings.Index(text[start:], stateBlockEnd)
	if end < 0 {
		return snap, false
	}
	if err := json.Unmarshal([]byte(text[start:start+end]), &snap); err != nil {
		return stateSnapshot{}, false
	}
	return snap, true
}

// findLatestState walks messages from newest to oldest looking for the most
// recent persisted todo-state block. The returned turnsSince includes both the
// base value captured in the block and assistant turns that happened after it.
func findLatestState(messages []*llm.Message) (todos []TodoItem, turnsSince int, found bool) {
	turnsAfterState := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			snap, ok := parseStateBlock(tc.Text)
			if !ok {
				continue
			}
			return cloneTodos(snap.Todos), snap.TurnsSinceWrite + turnsAfterState, true
		}
		if msg.Role == llm.Assistant {
			turnsAfterState++
		}
	}
	return nil, 0, false
}

// LatestState returns the most recent persisted todo state block found in the
// message history. It is useful for session or compaction integrators that need
// to carry todo state forward when rewriting conversation history.
func LatestState(messages []*llm.Message) (todos []TodoItem, turnsSince int, found bool) {
	return findLatestState(messages)
}

// StateBlock returns the hidden persistence block used to carry successful todo
// state through saved message history.
func StateBlock(items []TodoItem, turnsSinceWrite int) string {
	return formatStateBlock(items, turnsSinceWrite)
}

func cloneTodos(items []TodoItem) []TodoItem {
	if items == nil {
		return nil
	}
	out := make([]TodoItem, len(items))
	copy(out, items)
	return out
}
