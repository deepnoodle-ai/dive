package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
)

type VillagerRuntime struct {
	profile  VillagerProfile
	agent    *dive.Agent
	session  *session.Session
	memory   *SessionMemory
	recorder *actionRecorder
}

func NewVillagerRuntime(ctx context.Context, world *World, store *session.FileStore, model llm.LLM, profile VillagerProfile) (*VillagerRuntime, error) {
	sess, err := store.Open(ctx, "noodleville-"+profile.ID)
	if err != nil {
		return nil, err
	}
	recorder := &actionRecorder{}
	memory := NewSessionMemory(sess)
	rt := &VillagerRuntime{
		profile:  profile,
		session:  sess,
		memory:   memory,
		recorder: recorder,
	}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:            profile.Name,
		ID:              profile.ID,
		Description:     profile.Role,
		Model:           model,
		Session:         sess,
		Tools:           newActionTools(world, recorder, profile.ID),
		SystemPrompt:    systemPrompt(profile),
		ResponseTimeout: 2 * time.Minute,
		Hooks: dive.Hooks{
			PreGeneration: []dive.PreGenerationHook{
				perceptionHook(world, memory, profile.ID),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	rt.agent = agent
	return rt, nil
}

func systemPrompt(profile VillagerProfile) string {
	return fmt.Sprintf(`You are %s, a NoodleVille villager.

Role: %s.
Personal goal: %s.
Quirk: %s.
Mood: %s.

Each town tick, choose one concrete action by calling exactly one tool:
- move_to when a place would help.
- talk when a villager is co-located and a short line would matter.
- use_object when a nearby place or object supports your goal.
- reflect when the best action is internal.
- plan_day when you need to update your short-term plan.

After the tool result, respond with one short sentence in your own voice. Keep dialogue warm, local, and under 18 words.`,
		profile.Name, profile.Role, profile.Goal, profile.Quirk, profile.Mood)
}

func perceptionHook(world *World, memory *SessionMemory, villagerID string) dive.PreGenerationHook {
	return func(ctx context.Context, hctx *dive.HookContext) error {
		memories, err := memory.Retrieve(ctx, 5)
		if err != nil {
			return err
		}
		perception, err := world.Perception(villagerID, memories)
		if err != nil {
			return err
		}
		hctx.SystemPrompt = strings.TrimSpace(hctx.SystemPrompt) + "\n\n" + perception.Prompt()
		hctx.Values["noodleville_perception"] = perception
		return nil
	}
}

func (v *VillagerRuntime) TakeTurn(ctx context.Context) (TurnResult, error) {
	v.recorder.Reset()
	start := time.Now()
	var callbacks []string
	var callbacksMu sync.Mutex
	resp, err := v.agent.CreateResponse(ctx,
		dive.WithInput("Take your next small town action now. Use exactly one action tool, then summarize it in one short sentence."),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			appendCallback := func(value string) {
				callbacksMu.Lock()
				defer callbacksMu.Unlock()
				callbacks = append(callbacks, value)
			}
			switch item.Type {
			case dive.ResponseItemTypeToolCall:
				if item.ToolCall != nil {
					appendCallback(fmt.Sprintf("tool_call:%s", item.ToolCall.Name))
				}
			case dive.ResponseItemTypeToolCallResult:
				if item.ToolCallResult != nil {
					appendCallback(fmt.Sprintf("tool_result:%s", item.ToolCallResult.Name))
				}
			}
			return nil
		}),
	)
	if err != nil {
		return TurnResult{VillagerID: v.profile.ID, Duration: time.Since(start)}, err
	}
	action, ok := v.recorder.Last()
	if !ok {
		action = Action{
			ActorID:    v.profile.ID,
			Kind:       ActionReflect,
			Thought:    fallbackThought(resp.OutputText()),
			Importance: 2,
		}
	}
	callbacksMu.Lock()
	callbackCopy := append([]string(nil), callbacks...)
	callbacksMu.Unlock()
	return TurnResult{
		VillagerID: v.profile.ID,
		Output:     cleanSentence(resp.OutputText(), 220),
		Action:     action,
		HasAction:  true,
		Callbacks:  callbackCopy,
		Duration:   time.Since(start),
	}, nil
}

func fallbackThought(output string) string {
	output = cleanSentence(output, 180)
	if output == "" {
		return "I need a moment to decide what matters next."
	}
	return "I paused instead of using a tool: " + output
}

func (v *VillagerRuntime) AppendMemory(ctx context.Context, tick int, clock Clock, event Event) error {
	if event.Text == "" {
		return nil
	}
	return v.memory.Append(ctx, MemoryEntry{
		Clock:      clock.String(),
		Tick:       tick,
		Kind:       string(event.Kind),
		Importance: event.Importance,
		Text:       event.Text,
	})
}

func (v *VillagerRuntime) ReflectionAction(ctx context.Context) (Action, error) {
	text, err := v.memory.ReflectionText(ctx, v.profile.Name, 6)
	if err != nil {
		return Action{}, err
	}
	return Action{
		ActorID:    v.profile.ID,
		Kind:       ActionReflect,
		Thought:    text,
		Importance: 7,
	}, nil
}

func (v *VillagerRuntime) CompactMemory(ctx context.Context) error {
	return v.memory.CompactToSummary(ctx, v.profile.Name)
}

func (v *VillagerRuntime) MemoryCount(ctx context.Context) (int, error) {
	return v.memory.Count(ctx)
}
