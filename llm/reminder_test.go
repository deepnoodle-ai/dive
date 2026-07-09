package llm

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestReminderContentRoundTrip(t *testing.T) {
	message := &Message{
		Role: User,
		Content: []Content{&ReminderContent{
			Name: "environment", Tier: ReminderTierContextual, Content: "OS: linux",
		}},
	}
	data, err := json.Marshal(message)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"type":"reminder"`)

	var decoded Message
	assert.NoError(t, json.Unmarshal(data, &decoded))
	assert.Len(t, decoded.Content, 1)
	reminder, ok := decoded.Content[0].(*ReminderContent)
	assert.True(t, ok)
	assert.Equal(t, "environment", reminder.Name)
	assert.Equal(t, ReminderTierContextual, reminder.Tier)
	assert.Equal(t, "OS: linux", reminder.Content)

	copy := message.Copy()
	if copy == message {
		t.Fatal("Copy returned the original message pointer")
	}
	_, ok = copy.Content[0].(*ReminderContent)
	assert.True(t, ok)
}

func TestRenderReminders(t *testing.T) {
	operator := &Message{Role: System, Content: []Content{&ReminderContent{
		Name: "mode", Tier: ReminderTierOperator, Content: "Read only",
	}}}
	messages := []*Message{NewUserTextMessage("continue"), operator}

	rendered, err := RenderReminders(messages, OperatorAuthorityBestEffort,
		func(_ int, _ []*Message) (Role, bool) { return Developer, true })
	assert.NoError(t, err)
	assert.Equal(t, Developer, rendered[1].Role)
	assert.Equal(t, "<system-reminder name=\"mode\">\nRead only\n</system-reminder>", rendered[1].Content[0].(*TextContent).Text)
	_, stillTyped := messages[1].Content[0].(*ReminderContent)
	assert.True(t, stillTyped, "rendering must not mutate stored messages")

	fallback, err := RenderReminders(messages, OperatorAuthorityBestEffort, nil)
	assert.NoError(t, err)
	assert.Equal(t, User, fallback[1].Role)

	_, err = RenderReminders(messages, OperatorAuthorityStrict, nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrOperatorAuthorityUnavailable))
}
