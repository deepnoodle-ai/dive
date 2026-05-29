package main

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestWorldPerceptionAndApplyAction(t *testing.T) {
	world := NewSeedWorld()

	perception, err := world.Perception("maya", nil)
	assert.NoError(t, err)
	assert.Equal(t, perception.Self.Profile.Name, "Maya")
	assert.True(t, len(perception.NearbyPlaces) > 0, "expected nearby places")
	assert.True(t, len(perception.NearbyVillagers) > 0, "expected nearby villagers")

	event := world.ApplyAction(Action{
		ActorID:    "maya",
		Kind:       ActionMove,
		PlaceID:    "park",
		Importance: 4,
	})
	assert.Equal(t, event.Kind, ActionMove)
	assert.Equal(t, event.Text, "Maya walked to Lantern Park.")

	snapshot := world.Snapshot()
	var maya VillagerState
	for _, villager := range snapshot.Villagers {
		if villager.Profile.ID == "maya" {
			maya = villager
		}
	}
	assert.Equal(t, maya.Position, (Point{X: 3, Y: 2}))
}

func TestTalkRequiresCoLocation(t *testing.T) {
	world := NewSeedWorld()
	event := world.ApplyAction(Action{
		ActorID:  "maya",
		Kind:     ActionTalk,
		TargetID: "ben",
		Message:  "Clock parts and coffee?",
	})
	assert.Equal(t, event.Kind, ActionNoop)
	assert.Contains(t, event.Text, "not co-located")

	world.ApplyAction(Action{ActorID: "maya", Kind: ActionMove, PlaceID: "workshop"})
	event = world.ApplyAction(Action{
		ActorID:  "maya",
		Kind:     ActionTalk,
		TargetID: "ben",
		Message:  "Clock parts and coffee?",
	})
	assert.Equal(t, event.Kind, ActionTalk)
	assert.Contains(t, event.Text, "Clock parts and coffee?")
}

func TestUseActionAvoidsDoubleToPurpose(t *testing.T) {
	world := NewSeedWorld()
	event := world.ApplyAction(Action{
		ActorID: "maya",
		Kind:    ActionUse,
		PlaceID: "cafe",
		Thought: "to prepare a welcoming bowl",
	})
	assert.Equal(t, event.Kind, ActionUse)
	assert.Equal(t, event.Text, "Maya used Noodle Cafe: prepare a welcoming bowl.")
}

func TestPartyKnowledgePropagatesThroughTalk(t *testing.T) {
	world := NewSeedWorld()
	assert.NoError(t, world.SeedPartyGoal("maya"))

	perception, err := world.Perception("sol", nil)
	assert.NoError(t, err)
	assert.False(t, perception.Party.Knows)
	assert.Contains(t, perception.Prompt(), "- Party idea: not heard yet")
	assert.NotContains(t, perception.Prompt(), "party: knows")

	event := world.ApplyAction(Action{
		ActorID:  "maya",
		Kind:     ActionTalk,
		TargetID: "sol",
		Message:  "The Saturday noodle party is taking shape at the cafe.",
	})
	assert.Equal(t, event.Kind, ActionTalk)
	assert.True(t, event.Knowledge != nil, "expected party knowledge transfer")

	solParty := world.PartyKnowledge("sol")
	assert.True(t, solParty.Knows, "expected Sol to know the party idea")
	assert.Equal(t, solParty.SourceID, "maya")
	assert.Contains(t, solParty.Evidence, "Saturday noodle party")

	rels := world.RelationshipSnapshot("maya")
	assert.Equal(t, len(rels), 1)
	assert.Equal(t, rels[0].VillagerID, "sol")
	assert.Equal(t, rels[0].Conversations, 1)
	assert.Equal(t, rels[0].Trust, 1)
}

func TestSessionMemoryRetrievalScoresRecencyAndImportance(t *testing.T) {
	ctx := context.Background()
	store, err := session.NewFileStore(t.TempDir())
	assert.NoError(t, err)
	sess, err := store.Open(ctx, "test-memory")
	assert.NoError(t, err)
	memory := NewSessionMemory(sess)

	assert.NoError(t, memory.Append(ctx, MemoryEntry{Kind: "observation", Importance: 2, Text: "Saw the park lanterns."}))
	assert.NoError(t, memory.Append(ctx, MemoryEntry{Kind: "dialogue", Importance: 8, Text: "Maya invited Ben to noodles."}))
	assert.NoError(t, memory.Append(ctx, MemoryEntry{Kind: "observation", Importance: 3, Text: "Clock chimed late."}))

	got, err := memory.Retrieve(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].Text, "Maya invited Ben to noodles.")
	assert.Equal(t, got[1].Text, "Clock chimed late.")
}
