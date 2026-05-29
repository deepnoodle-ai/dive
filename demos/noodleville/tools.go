package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

type actionRecorder struct {
	mu      sync.Mutex
	actions []Action
}

func (r *actionRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = nil
}

func (r *actionRecorder) Record(action Action) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, action)
}

func (r *actionRecorder) Last() (Action, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.actions) == 0 {
		return Action{}, false
	}
	return r.actions[len(r.actions)-1], true
}

type moveToInput struct {
	Place string `json:"place" description:"The destination place name or id, such as Noodle Cafe, Town Square, Lantern Park, Pocket Library, or Tinker Workshop."`
}

type talkInput struct {
	Target  string `json:"target" description:"The villager name or id to speak with. They should be at the same tile."`
	Message string `json:"message" description:"A short, friendly line of dialogue under 18 words."`
}

type useInput struct {
	Object  string `json:"object" description:"The nearby place or object to use, such as Noodle Cafe, Lantern Park, Tinker Workshop, or Pocket Library."`
	Purpose string `json:"purpose" description:"What the villager is trying to accomplish with this object or place."`
}

type reflectInput struct {
	Thought    string `json:"thought" description:"A concise reflection about what the villager noticed or feels."`
	Importance int    `json:"importance,omitempty" description:"Importance from 1 to 10."`
}

type planInput struct {
	Plan       string `json:"plan" description:"A concise next-step plan for the villager."`
	Importance int    `json:"importance,omitempty" description:"Importance from 1 to 10."`
}

func newActionTools(world *World, recorder *actionRecorder, actorID string) []dive.Tool {
	return []dive.Tool{
		dive.FuncTool("move_to",
			"Move to a known NoodleVille place.",
			func(ctx context.Context, input *moveToInput) (*dive.ToolResult, error) {
				place, ok := world.PlaceByNameOrID(input.Place)
				if !ok {
					return dive.NewToolResultError(fmt.Sprintf("unknown place %q", input.Place)), nil
				}
				recorder.Record(Action{
					ActorID:    actorID,
					Kind:       ActionMove,
					PlaceID:    place.ID,
					Importance: 3,
				})
				return dive.NewToolResultText(fmt.Sprintf("Recorded move to %s at (%d,%d).", place.Name, place.Position.X, place.Position.Y)), nil
			}),
		dive.FuncTool("talk",
			"Say a short line to a co-located villager.",
			func(ctx context.Context, input *talkInput) (*dive.ToolResult, error) {
				target, ok := world.VillagerByNameOrID(input.Target)
				if !ok {
					return dive.NewToolResultError(fmt.Sprintf("unknown villager %q", input.Target)), nil
				}
				message := cleanSentence(input.Message, 140)
				recorder.Record(Action{
					ActorID:    actorID,
					Kind:       ActionTalk,
					TargetID:   target.Profile.ID,
					Message:    message,
					Importance: 6,
				})
				return dive.NewToolResultText(fmt.Sprintf("Recorded line to %s: %q.", target.Profile.Name, message)), nil
			}),
		dive.FuncTool("use_object",
			"Use a nearby place or object in the town.",
			func(ctx context.Context, input *useInput) (*dive.ToolResult, error) {
				place, ok := world.PlaceByNameOrID(input.Object)
				if !ok {
					return dive.NewToolResultError(fmt.Sprintf("unknown object %q", input.Object)), nil
				}
				purpose := cleanSentence(input.Purpose, 140)
				recorder.Record(Action{
					ActorID:    actorID,
					Kind:       ActionUse,
					PlaceID:    place.ID,
					Thought:    purpose,
					Importance: 4,
				})
				return dive.NewToolResultText(fmt.Sprintf("Recorded use of %s to %s.", place.Name, purpose)), nil
			}),
		dive.FuncTool("reflect",
			"Record an internal reflection for this villager.",
			func(ctx context.Context, input *reflectInput) (*dive.ToolResult, error) {
				thought := cleanSentence(input.Thought, 180)
				recorder.Record(Action{
					ActorID:    actorID,
					Kind:       ActionReflect,
					Thought:    thought,
					Importance: clampImportance(input.Importance),
				})
				return dive.NewToolResultText("Recorded reflection: " + thought), nil
			}),
		dive.FuncTool("plan_day",
			"Set or update the villager's short-term plan.",
			func(ctx context.Context, input *planInput) (*dive.ToolResult, error) {
				plan := cleanSentence(input.Plan, 180)
				recorder.Record(Action{
					ActorID:    actorID,
					Kind:       ActionPlan,
					Thought:    plan,
					Importance: clampImportance(input.Importance),
				})
				return dive.NewToolResultText("Recorded plan: " + plan), nil
			}),
	}
}
