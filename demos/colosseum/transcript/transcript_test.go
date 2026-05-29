package transcript

import (
	"bytes"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestWriteReadRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	assert.NoError(t, w.Write(Event{Type: TypeSpeak, Actor: "claude", Message: "hi", Reasoning: "secret"}))
	assert.NoError(t, w.Write(Event{Type: TypeVote, Actor: "gpt", Target: "claude"}))

	events, err := Read(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(events))
	assert.Equal(t, "claude", events[0].Actor)
	assert.Equal(t, "secret", events[0].Reasoning)
	assert.NotEqual(t, "", events[0].Time, "writer stamps a time")
	assert.Equal(t, "claude", events[1].Target)
}

func TestReadSkipsBlankLines(t *testing.T) {
	in := "\n{\"type\":\"speak\",\"actor\":\"a\"}\n\n{\"type\":\"vote\",\"actor\":\"b\"}\n"
	events, err := Read(bytes.NewBufferString(in))
	assert.NoError(t, err)
	assert.Equal(t, 2, len(events))
}

// sampleMatch builds a minimal but complete transcript for parser tests.
func sampleMatch() []Event {
	return []Event{
		{Type: TypeMatchStart, Data: map[string]any{
			"seed": float64(42),
			"players": []any{
				map[string]any{"id": "claude", "provider": "claude", "model": "haiku"},
				map[string]any{"id": "gpt", "provider": "gpt", "model": "mini"},
				map[string]any{"id": "grok", "provider": "grok", "model": "fast"},
			},
			"roles": map[string]any{"claude": "seer", "gpt": "villager", "grok": "werewolf"},
		}},
		{Type: TypeSpeak, Actor: "grok", Message: "I'm innocent", Reasoning: "lying"},
		{Type: TypeElimination, Target: "grok", Role: "werewolf", Data: map[string]any{"cause": "vote"}},
		{Type: TypeMatchEnd, Data: map[string]any{
			"winner":    "village",
			"rounds":    float64(1),
			"survivors": []any{"claude", "gpt"},
			"usage_by_player": map[string]any{
				"claude": map[string]any{"input_tokens": float64(100), "output_tokens": float64(20)},
			},
		}},
	}
}

func TestParse(t *testing.T) {
	m, err := Parse(sampleMatch())
	assert.NoError(t, err)
	assert.True(t, m.Complete)
	assert.Equal(t, int64(42), m.Seed)
	assert.Equal(t, "village", m.Winner)
	assert.Equal(t, 1, m.Rounds)
	assert.Equal(t, 3, len(m.Players))
	assert.Equal(t, []string{"claude", "gpt"}, m.Survivors)

	grok := m.PlayerByID("grok")
	assert.NotNil(t, grok)
	assert.Equal(t, "werewolf", grok.Role)
	assert.Equal(t, "grok", grok.Provider)

	assert.Equal(t, 100, m.Usage["claude"].InputTokens)
	assert.Equal(t, 20, m.Usage["claude"].OutputTokens)
}

func TestParseIncomplete(t *testing.T) {
	events := sampleMatch()
	events = events[:len(events)-1] // drop match_end
	m, err := Parse(events)
	assert.NoError(t, err)
	assert.False(t, m.Complete)
	assert.Equal(t, "", m.Winner)
	assert.Equal(t, 3, len(m.Players))
}

func TestParseNoStartIsError(t *testing.T) {
	_, err := Parse([]Event{{Type: TypeSpeak, Actor: "a"}})
	assert.Error(t, err)
}
