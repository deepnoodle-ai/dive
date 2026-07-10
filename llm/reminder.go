package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// ReminderTier describes the authority of runtime-injected context.
type ReminderTier string

const (
	// ReminderTierContextual is user-adjacent context such as environment facts,
	// surfaced memory, or tool-produced notifications.
	ReminderTierContextual ReminderTier = "contextual"
	// ReminderTierOperator is context asserted by the application operator.
	ReminderTierOperator ReminderTier = "operator"
)

var reminderNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidReminderName reports whether name is safe for a reminder tag attribute.
func ValidReminderName(name string) bool { return reminderNamePattern.MatchString(name) }

// ReminderContent is a typed runtime-context block. Providers render it to a
// recognizable <system-reminder> block at the wire boundary.
type ReminderContent struct {
	Name    string       `json:"name"`
	Tier    ReminderTier `json:"tier"`
	Content string       `json:"content"`
}

func (c *ReminderContent) Type() ContentType { return ContentTypeReminder }

func (c *ReminderContent) MarshalJSON() ([]byte, error) {
	type Alias ReminderContent
	return json.Marshal(struct {
		Type ContentType `json:"type"`
		*Alias
	}{Type: ContentTypeReminder, Alias: (*Alias)(c)})
}

// FormatReminder renders the provider-wire representation of a reminder.
func FormatReminder(c *ReminderContent) string {
	return fmt.Sprintf("<system-reminder name=%q>\n%s\n</system-reminder>", c.Name, c.Content)
}

// ReminderAuthorityResolver reports the native role and whether it is legal
// for the reminder message at index. A nil resolver means native operator
// authority is unavailable.
type ReminderAuthorityResolver func(index int, messages []*Message) (Role, bool)

// RenderReminders converts typed reminder blocks to provider-wire text without
// mutating caller-owned messages. Only messages containing ReminderContent are
// normalized; raw system/developer messages retain their existing behavior.
//
// Operator reminders render with the strongest role the resolver reports as
// legal for their position, falling back to a tagged user message everywhere
// else. A nil resolver means native operator authority is unavailable.
func RenderReminders(messages []*Message, resolve ReminderAuthorityResolver) ([]*Message, error) {
	out := make([]*Message, len(messages))
	copy(out, messages)
	for i, message := range messages {
		if message == nil {
			continue
		}
		containsReminder := false
		operator := false
		content := make([]Content, len(message.Content))
		for j, block := range message.Content {
			reminder, ok := block.(*ReminderContent)
			if !ok {
				content[j] = block
				continue
			}
			if !ValidReminderName(reminder.Name) {
				return nil, fmt.Errorf("invalid reminder name %q", reminder.Name)
			}
			if reminder.Tier != ReminderTierContextual && reminder.Tier != ReminderTierOperator {
				return nil, fmt.Errorf("invalid reminder tier %q", reminder.Tier)
			}
			containsReminder = true
			if reminder.Tier == ReminderTierOperator {
				operator = true
			}
			content[j] = &TextContent{Text: FormatReminder(reminder)}
		}
		if !containsReminder {
			continue
		}
		role := User
		if operator && resolve != nil {
			if nativeRole, native := resolve(i, messages); native {
				role = nativeRole
			}
		}
		out[i] = &Message{ID: message.ID, Role: role, Content: content}
	}
	return out, nil
}
