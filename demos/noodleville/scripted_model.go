package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

type ScriptedPlanner struct {
	delay          time.Duration
	counter        atomic.Uint64
	inFlight       atomic.Int64
	maxConcurrency atomic.Int64
	mu             sync.Mutex
	calls          int
}

func NewScriptedPlanner(delay time.Duration) *ScriptedPlanner {
	return &ScriptedPlanner{delay: delay}
}

func (s *ScriptedPlanner) Name() string {
	return "scripted-noodleville"
}

func (s *ScriptedPlanner) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)

	inFlight := s.inFlight.Add(1)
	for {
		max := s.maxConcurrency.Load()
		if inFlight <= max || s.maxConcurrency.CompareAndSwap(max, inFlight) {
			break
		}
	}
	defer s.inFlight.Add(-1)

	s.mu.Lock()
	s.calls++
	s.mu.Unlock()

	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	id := s.counter.Add(1)
	if lastMessageIsToolResult(cfg.Messages) {
		text := "Done. I made a small choice that fits the moment."
		return &llm.Response{
			ID:         fmt.Sprintf("scripted_%d", id),
			Model:      s.Name(),
			Role:       llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: text}},
			Type:       "message",
			StopReason: "stop",
			Usage:      llm.Usage{InputTokens: 12, OutputTokens: 10},
		}, nil
	}

	actor := actorName(cfg.SystemPrompt)
	toolName, input := scriptedAction(actor, cfg.SystemPrompt)
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return &llm.Response{
		ID:    fmt.Sprintf("scripted_%d", id),
		Model: s.Name(),
		Role:  llm.Assistant,
		Content: []llm.Content{&llm.ToolUseContent{
			ID:    fmt.Sprintf("toolu_scripted_%d", id),
			Name:  toolName,
			Input: raw,
		}},
		Type:       "message",
		StopReason: "tool_use",
		Usage:      llm.Usage{InputTokens: 20, OutputTokens: 8},
	}, nil
}

func (s *ScriptedPlanner) MaxConcurrency() int64 {
	return s.maxConcurrency.Load()
}

func (s *ScriptedPlanner) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func lastMessageIsToolResult(messages []*llm.Message) bool {
	if len(messages) == 0 {
		return false
	}
	last := messages[len(messages)-1]
	for _, content := range last.Content {
		if _, ok := content.(*llm.ToolResultContent); ok {
			return true
		}
	}
	return false
}

func actorName(systemPrompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	systemPrompt = strings.TrimPrefix(systemPrompt, "You are ")
	if i := strings.Index(systemPrompt, ","); i >= 0 {
		return strings.TrimSpace(systemPrompt[:i])
	}
	fields := strings.Fields(systemPrompt)
	if len(fields) > 0 {
		return fields[0]
	}
	return "villager"
}

func scriptedAction(actor, prompt string) (string, any) {
	if ownPartyKnown(prompt) {
		if !currentPlanMentions(prompt, "party") {
			return "plan_day", &planInput{
				Plan:       "Invite neighbors to a Saturday noodle party through ordinary conversations.",
				Importance: 9,
			}
		}
		if target := firstColocatedVillager(prompt); target != "" {
			return "talk", &talkInput{
				Target:  target,
				Message: "I am gathering neighbors for a Saturday noodle party.",
			}
		}
		return "move_to", &moveToInput{Place: "Town Square"}
	}
	if target := firstColocatedVillager(prompt); target != "" {
		return "talk", &talkInput{
			Target:  target,
			Message: fmt.Sprintf("%s here. What should we notice next?", actor),
		}
	}
	switch strings.ToLower(actor) {
	case "maya":
		return "use_object", &useInput{Object: "Noodle Cafe", Purpose: "prepare a welcoming bowl of noodles"}
	case "ben":
		return "use_object", &useInput{Object: "Tinker Workshop", Purpose: "check the gears for the town clock"}
	case "lina":
		return "use_object", &useInput{Object: "Pocket Library", Purpose: "write down a fresh town observation"}
	case "ori":
		return "use_object", &useInput{Object: "Lantern Park", Purpose: "water the path flowers"}
	case "sol":
		return "move_to", &moveToInput{Place: "Town Square"}
	case "niko":
		return "use_object", &useInput{Object: "Moon Bun Bakery", Purpose: "test a festival bun"}
	case "tess":
		return "use_object", &useInput{Object: "Little Clinic", Purpose: "prepare calming tea"}
	case "jun":
		return "use_object", &useInput{Object: "River Studio", Purpose: "mix a golden-hour color"}
	case "paz":
		return "use_object", &useInput{Object: "Morning Market", Purpose: "match neighbors with useful supplies"}
	case "ren":
		return "use_object", &useInput{Object: "Lantern Stage", Purpose: "try a new chorus"}
	default:
		return "reflect", &reflectInput{Thought: "The town is waking up and I am finding my place.", Importance: 3}
	}
}

func currentPlanMentions(prompt, word string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- Current plan:") {
			continue
		}
		return strings.Contains(strings.ToLower(line), strings.ToLower(word))
	}
	return false
}

func ownPartyKnown(prompt string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- Party idea:") {
			continue
		}
		return strings.Contains(strings.ToLower(line), "known")
	}
	return false
}

func firstColocatedVillager(prompt string) string {
	selfLocation := perceptionLocation(prompt)
	if selfLocation == "" {
		return ""
	}
	lines := strings.Split(prompt, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "- Nearby villagers:" {
			continue
		}
		for _, candidate := range lines[i+1:] {
			candidate = strings.TrimSpace(candidate)
			if !strings.HasPrefix(candidate, "- ") {
				break
			}
			if !strings.Contains(candidate, selfLocation) {
				continue
			}
			name := strings.TrimPrefix(candidate, "- ")
			if comma := strings.Index(name, ","); comma >= 0 {
				name = name[:comma]
			}
			return strings.TrimSpace(name)
		}
	}
	return ""
}

func perceptionLocation(prompt string) string {
	re := regexp.MustCompile(`- Location: \((\d+),(\d+)\)`)
	matches := re.FindStringSubmatch(prompt)
	if len(matches) != 3 {
		return ""
	}
	return fmt.Sprintf("(%s,%s)", matches[1], matches[2])
}
