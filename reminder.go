package dive

import (
	"fmt"
	"regexp"

	"github.com/deepnoodle-ai/dive/llm"
)

// ReminderTier is the authority assigned to runtime-injected context.
type ReminderTier = llm.ReminderTier

const (
	ReminderTierContextual = llm.ReminderTierContextual
	ReminderTierOperator   = llm.ReminderTierOperator
)

// Reminder is a validated, named block of runtime-injected context.
type Reminder struct {
	Name    string       `json:"name"`
	Tier    ReminderTier `json:"tier"`
	Content string       `json:"content"`
}

// NewContextReminder creates user-adjacent runtime context.
func NewContextReminder(name, content string) (Reminder, error) {
	return newReminder(name, content, ReminderTierContextual)
}

// NewOperatorReminder creates context asserted by the application operator.
func NewOperatorReminder(name, content string) (Reminder, error) {
	return newReminder(name, content, ReminderTierOperator)
}

func newReminder(name, content string, tier ReminderTier) (Reminder, error) {
	r := Reminder{Name: name, Tier: tier, Content: content}
	if err := validateReminder(r); err != nil {
		return Reminder{}, err
	}
	return r, nil
}

func validateReminder(r Reminder) error {
	if !llm.ValidReminderName(r.Name) {
		return fmt.Errorf("invalid reminder name %q: must match [a-z][a-z0-9-]*", r.Name)
	}
	if r.Tier != ReminderTierContextual && r.Tier != ReminderTierOperator {
		return fmt.Errorf("invalid reminder tier %q", r.Tier)
	}
	return nil
}

func reminderContent(r Reminder) *llm.ReminderContent {
	return &llm.ReminderContent{Name: r.Name, Tier: r.Tier, Content: r.Content}
}

func reminderFromContent(c *llm.ReminderContent) Reminder {
	return Reminder{Name: c.Name, Tier: c.Tier, Content: c.Content}
}

// NewReminderMessage builds a recorded input message for an appended reminder.
// Operator reminders use the system role internally; providers normalize that
// role according to their known capabilities, falling back to a tagged user
// message where native operator authority is unavailable.
func NewReminderMessage(r Reminder) *llm.Message {
	role := llm.User
	if r.Tier == ReminderTierOperator {
		role = llm.System
	}
	return &llm.Message{Role: role, Content: []llm.Content{reminderContent(r)}}
}

// FindReminder finds a typed reminder in one message. It never parses text.
func FindReminder(message *llm.Message, name string) (Reminder, bool) {
	if message == nil {
		return Reminder{}, false
	}
	for i := len(message.Content) - 1; i >= 0; i-- {
		if c, ok := message.Content[i].(*llm.ReminderContent); ok && c.Name == name {
			return reminderFromContent(c), true
		}
	}
	return Reminder{}, false
}

// FindLatestReminder searches typed reminders from newest to oldest.
func FindLatestReminder(messages []*llm.Message, name string) (Reminder, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if r, ok := FindReminder(messages[i], name); ok {
			return r, true
		}
	}
	return Reminder{}, false
}

// StripReminders removes all typed reminders using copy-on-write. User-authored
// text that resembles a reminder is preserved.
func StripReminders(messages []*llm.Message) []*llm.Message {
	return stripTypedReminders(messages, "")
}

// RemoveReminder removes typed reminders with name using copy-on-write.
func RemoveReminder(messages []*llm.Message, name string) []*llm.Message {
	return stripTypedReminders(messages, name)
}

func stripTypedReminders(messages []*llm.Message, name string) []*llm.Message {
	out := make([]*llm.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		content := make([]llm.Content, 0, len(message.Content))
		changed := false
		for _, block := range message.Content {
			if c, ok := block.(*llm.ReminderContent); ok && (name == "" || c.Name == name) {
				changed = true
				continue
			}
			content = append(content, block)
		}
		if len(content) == 0 && changed {
			continue
		}
		if changed {
			out = append(out, &llm.Message{ID: message.ID, Role: message.Role, Content: content})
		} else {
			out = append(out, message)
		}
	}
	return out
}

var legacyReminderPattern = regexp.MustCompile(`(?s)^<system-reminder name="([a-z][a-z0-9-]*)">\n(.*)\n</system-reminder>$`)

// ParseLegacyReminderText recognizes one complete legacy text reminder. This
// is heuristic and must not be used for provenance- or security-sensitive UI.
func ParseLegacyReminderText(text string) (Reminder, bool) {
	match := legacyReminderPattern.FindStringSubmatch(text)
	if match == nil {
		return Reminder{}, false
	}
	return Reminder{Name: match[1], Tier: ReminderTierContextual, Content: match[2]}, true
}

// ReminderRecording controls whether an appended reminder is returned in
// OutputMessages and therefore eligible for session persistence.
type ReminderRecording string

const (
	Recorded  ReminderRecording = "recorded"
	ModelOnly ReminderRecording = "model_only"
)

type reminderDelivery struct {
	reminder  Reminder
	recording ReminderRecording
}

type reminderState struct {
	pending   []reminderDelivery
	modelOnly []*llm.Message
}

func (s *reminderState) appendModelOnly(message *llm.Message) {
	s.modelOnly = append(s.modelOnly, message)
}

func (s *reminderState) drainPending() []reminderDelivery {
	pending := s.pending
	s.pending = nil
	return pending
}

func queueReminderDeliveries(state *reminderState, deliveries []reminderDelivery) {
	if state == nil {
		return
	}
	state.pending = append(state.pending, deliveries...)
}
