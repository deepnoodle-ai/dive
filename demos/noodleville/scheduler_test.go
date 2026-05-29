package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestTownRunTickUsesBoundedParallelismAndPersistsMemories(t *testing.T) {
	ctx := context.Background()
	planner := NewScriptedPlanner(20 * time.Millisecond)
	town, err := NewTown(ctx, TownOptions{
		Model:         planner,
		SessionDir:    t.TempDir(),
		Parallelism:   2,
		TickMinutes:   10,
		VillagerCount: 5,
	})
	assert.NoError(t, err)

	report, err := town.RunTick(ctx)
	assert.NoError(t, err)
	assert.Equal(t, report.Tick, 0)
	assert.Equal(t, report.StartedAt.String(), "day 1 08:00")
	assert.Equal(t, report.EndedAt.String(), "day 1 08:10")
	assert.Equal(t, len(report.Turns), 5)
	assert.Equal(t, len(report.Events), 5)
	assert.True(t, planner.MaxConcurrency() <= 2, "expected LLM calls to be throttled")

	counts := town.MemoryCounts(ctx)
	assert.Equal(t, len(counts), 5)
	for id, count := range counts {
		assert.True(t, count > 0, "expected memory for "+id)
	}
}

func TestRunTickHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	town, err := NewTown(context.Background(), TownOptions{
		Model:         NewScriptedPlanner(10 * time.Millisecond),
		SessionDir:    t.TempDir(),
		Parallelism:   1,
		VillagerCount: 5,
	})
	assert.NoError(t, err)

	_, err = town.RunTick(ctx)
	assert.Error(t, err)
}

func TestPartySeedPropagatesThroughPlanningAndDialogue(t *testing.T) {
	ctx := context.Background()
	town, err := NewTown(ctx, TownOptions{
		Model:         NewScriptedPlanner(1 * time.Millisecond),
		SessionDir:    t.TempDir(),
		Parallelism:   3,
		TickMinutes:   10,
		VillagerCount: 12,
		SeedParty:     true,
	})
	assert.NoError(t, err)
	assert.Equal(t, len(town.Snapshot().Villagers), 12)

	var solPartyEvent bool
	for i := 0; i < 3; i++ {
		report, err := town.RunTick(ctx)
		assert.NoError(t, err)
		for _, event := range report.Events {
			if event.ActorID == "sol" && event.Kind == ActionPlan && contains(event.Text, "Saturday noodle party") {
				solPartyEvent = true
			}
		}
	}
	assert.True(t, solPartyEvent, "expected Sol to pick up and plan around the party idea")

	snapshot := town.Snapshot()
	assert.True(t, snapshot.Social.Party["maya"].Knows, "expected Maya to know the party idea")
	assert.True(t, snapshot.Social.Party["sol"].Knows, "expected Sol to learn the party idea")
	assert.Equal(t, snapshot.Social.Party["sol"].SourceID, "maya")
	assert.True(t, snapshot.Social.Relationships["maya"]["sol"].Conversations > 0, "expected Maya and Sol to have a recorded tie")

	inspector, err := town.InspectVillager(ctx, "sol")
	assert.NoError(t, err)
	assert.True(t, inspector.Party.Knows, "expected inspector to expose Sol's party knowledge")
	assert.True(t, inspector.MemoryCount > 0, "expected inspector memory count")
	assert.True(t, len(inspector.Relationships) > 0, "expected inspector relationships")
}

func TestReflectionCompactsVillagerSessions(t *testing.T) {
	ctx := context.Background()
	town, err := NewTown(ctx, TownOptions{
		Model:              NewScriptedPlanner(1 * time.Millisecond),
		SessionDir:         t.TempDir(),
		Parallelism:        2,
		VillagerCount:      5,
		ReflectionInterval: 1,
	})
	assert.NoError(t, err)

	report, err := town.RunTick(ctx)
	assert.NoError(t, err)
	assert.Equal(t, len(report.Reflections), 5)

	records, err := town.villagers["maya"].session.CompactionHistory(ctx)
	assert.NoError(t, err)
	assert.True(t, len(records) > 0, "expected reflection to compact Maya's session")
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
