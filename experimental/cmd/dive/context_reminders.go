package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

func parseReminderSpecs(contextual, operator []string) ([]dive.Reminder, []dive.Reminder, error) {
	parse := func(specs []string, constructor func(string, string) (dive.Reminder, error)) ([]dive.Reminder, error) {
		reminders := make([]dive.Reminder, 0, len(specs))
		for _, spec := range specs {
			name, content, ok := strings.Cut(spec, "=")
			if !ok {
				return nil, fmt.Errorf("invalid reminder %q: expected NAME=TEXT", spec)
			}
			reminder, err := constructor(strings.TrimSpace(name), content)
			if err != nil {
				return nil, err
			}
			reminders = append(reminders, reminder)
		}
		return reminders, nil
	}
	pinned, err := parse(contextual, dive.NewContextReminder)
	if err != nil {
		return nil, nil, err
	}
	appended, err := parse(operator, dive.NewOperatorReminder)
	if err != nil {
		return nil, nil, err
	}
	return pinned, appended, nil
}

func pinRemindersHook(reminders []dive.Reminder) dive.PreGenerationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		for _, reminder := range reminders {
			if err := hctx.PinReminder(reminder); err != nil {
				return err
			}
		}
		return nil
	}
}

func reminderInputMessages(input string, extra []llm.Content, operator []dive.Reminder) []*llm.Message {
	content := []llm.Content{&llm.TextContent{Text: input}}
	content = append(content, extra...)
	messages := []*llm.Message{llm.NewUserMessage(content...)}
	for _, reminder := range operator {
		messages = append(messages, dive.NewReminderMessage(reminder))
	}
	return messages
}
