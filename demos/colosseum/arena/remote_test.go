package arena

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{`{"a":1}`, `{"a":1}`, true},
		{"here you go:\n```json\n{\"a\":1}\n```", `{"a":1}`, true},
		{`prefix {"a":{"b":2}} suffix`, `{"a":{"b":2}}`, true},
		{`{"text":"has } brace and \" quote"}`, `{"text":"has } brace and \" quote"}`, true},
		{`no json here`, "", false},
	}
	for _, c := range cases {
		got, ok := extractJSONObject(c.in)
		assert.Equal(t, c.ok, ok, "ok for %q", c.in)
		if c.ok {
			assert.Equal(t, c.want, got, "extracted for %q", c.in)
		}
	}
}

func TestParseRemoteActionSpeak(t *testing.T) {
	a, err := parseRemoteAction(`{"action":"speak","message":"hello village","reasoning":"buying time"}`,
		actSpeak, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, actSpeak, a.kind)
	assert.Equal(t, "hello village", a.message)
	assert.Equal(t, "buying time", a.reasoning)
}

func TestParseRemoteActionVoteValidatesTarget(t *testing.T) {
	cands := []string{"alice", "bob"}
	set := setOf(cands)

	ok, err := parseRemoteAction(`{"action":"vote","target":"bob","reasoning":"suspicious"}`, actVote, set, cands)
	assert.NoError(t, err)
	assert.Equal(t, "bob", ok.target)

	_, err = parseRemoteAction(`{"action":"vote","target":"zoe","reasoning":"x"}`, actVote, set, cands)
	assert.Error(t, err, "target not in candidates must be rejected")
}

func TestParseRemoteActionRequiresReasoning(t *testing.T) {
	_, err := parseRemoteAction(`{"action":"speak","message":"hi"}`, actSpeak, nil, nil)
	assert.Error(t, err, "missing reasoning must be rejected")
}

func TestParseRemoteActionWrongActionName(t *testing.T) {
	cands := []string{"a"}
	_, err := parseRemoteAction(`{"action":"speak","target":"a","reasoning":"x"}`, actVote, setOf(cands), cands)
	assert.Error(t, err, "mismatched action name must be rejected")
}

func TestParseRemoteActionToleratesProse(t *testing.T) {
	// Night action embedded in surrounding prose + code fence.
	text := "Sure — here is my move:\n```json\n{\"action\":\"night_action\",\"target\":\"a\",\"reasoning\":\"best target\"}\n```"
	cands := []string{"a", "b"}
	got, err := parseRemoteAction(text, actNight, setOf(cands), cands)
	assert.NoError(t, err)
	assert.Equal(t, "a", got.target)
}

func TestParseRemoteActionNoJSON(t *testing.T) {
	_, err := parseRemoteAction("I refuse to answer.", actVote, nil, nil)
	assert.Error(t, err)
}
