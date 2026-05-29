package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
)

const memoryPrefix = "NOODLEVILLE_MEMORY "

type MemoryEntry struct {
	At         time.Time `json:"at"`
	Clock      string    `json:"clock"`
	Tick       int       `json:"tick"`
	Kind       string    `json:"kind"`
	Importance int       `json:"importance"`
	Text       string    `json:"text"`
}

type SessionMemory struct {
	session *session.Session
}

func NewSessionMemory(sess *session.Session) *SessionMemory {
	return &SessionMemory{session: sess}
}

func (m *SessionMemory) Append(ctx context.Context, entry MemoryEntry) error {
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	entry.Kind = strings.TrimSpace(entry.Kind)
	if entry.Kind == "" {
		entry.Kind = "observation"
	}
	entry.Importance = clampImportance(entry.Importance)
	entry.Text = cleanSentence(entry.Text, 500)

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return m.session.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage(memoryPrefix + string(data)),
	}, nil)
}

func (m *SessionMemory) Retrieve(ctx context.Context, limit int) ([]MemoryEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	msgs, err := m.session.AllMessages(ctx)
	if err != nil {
		return nil, err
	}
	type scored struct {
		entry MemoryEntry
		score int
		index int
	}
	var scoredEntries []scored
	for i, msg := range msgs {
		for _, entry := range parseMemoryEntries(msg.Text()) {
			recency := i + 1
			scoredEntries = append(scoredEntries, scored{
				entry: entry,
				score: entry.Importance*100 + recency,
				index: i,
			})
		}
	}
	sort.SliceStable(scoredEntries, func(i, j int) bool {
		if scoredEntries[i].score != scoredEntries[j].score {
			return scoredEntries[i].score > scoredEntries[j].score
		}
		return scoredEntries[i].index > scoredEntries[j].index
	})
	if len(scoredEntries) > limit {
		scoredEntries = scoredEntries[:limit]
	}
	out := make([]MemoryEntry, len(scoredEntries))
	for i, entry := range scoredEntries {
		out[i] = entry.entry
	}
	return out, nil
}

func (m *SessionMemory) Count(ctx context.Context) (int, error) {
	msgs, err := m.session.AllMessages(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	for _, msg := range msgs {
		count += len(parseMemoryEntries(msg.Text()))
	}
	return count, nil
}

func (m *SessionMemory) ReflectionText(ctx context.Context, villagerName string, limit int) (string, error) {
	memories, err := m.Retrieve(ctx, limit)
	if err != nil {
		return "", err
	}
	if len(memories) == 0 {
		return fmt.Sprintf("%s has little to go on yet, so the next useful step is to notice who is nearby.", villagerName), nil
	}
	var parts []string
	for _, memory := range memories {
		if memory.Kind == string(ActionReflect) {
			continue
		}
		if memory.Text != "" {
			parts = append(parts, memory.Text)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s should stay attentive and make one concrete connection.", villagerName), nil
	}
	return fmt.Sprintf("%s is connecting these moments: %s", villagerName, cleanSentence(strings.Join(parts, " "), 260)), nil
}

func (m *SessionMemory) CompactToSummary(ctx context.Context, villagerName string) error {
	return m.session.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		entries := entriesFromMessages(msgs)
		summary := summarizeEntries(villagerName, entries)
		return []*llm.Message{llm.NewUserTextMessage(summary)}, nil
	})
}

func entriesFromMessages(msgs []*llm.Message) []MemoryEntry {
	var entries []MemoryEntry
	for _, msg := range msgs {
		entries = append(entries, parseMemoryEntries(msg.Text())...)
	}
	return entries
}

func summarizeEntries(villagerName string, entries []MemoryEntry) string {
	if len(entries) == 0 {
		return fmt.Sprintf("NoodleVille memory summary for %s: no durable memories yet.", villagerName)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Importance != entries[j].Importance {
			return entries[i].Importance > entries[j].Importance
		}
		return entries[i].At.After(entries[j].At)
	})
	if len(entries) > 6 {
		entries = entries[:6]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "NoodleVille memory summary for %s:\n", villagerName)
	for _, entry := range entries {
		fmt.Fprintf(&b, "- [%s importance=%d] %s\n", entry.Kind, entry.Importance, entry.Text)
	}
	return strings.TrimSpace(b.String())
}

func parseMemoryEntries(text string) []MemoryEntry {
	var entries []MemoryEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, memoryPrefix) {
			continue
		}
		var entry MemoryEntry
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, memoryPrefix)), &entry); err == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}
