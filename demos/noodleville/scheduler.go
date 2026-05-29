package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
)

type TownOptions struct {
	Model              llm.LLM
	SessionDir         string
	Parallelism        int
	TickMinutes        int
	VillagerCount      int
	SeedParty          bool
	ReflectionInterval int
}

type Town struct {
	world              *World
	villagers          map[string]*VillagerRuntime
	parallelism        int
	tickMinutes        int
	reflectionInterval int
	tick               int
}

type TurnResult struct {
	VillagerID string        `json:"villager_id"`
	Output     string        `json:"output"`
	Action     Action        `json:"action"`
	HasAction  bool          `json:"has_action"`
	Callbacks  []string      `json:"callbacks,omitempty"`
	Duration   time.Duration `json:"duration"`
	Err        error         `json:"-"`
}

type TickReport struct {
	Tick         int            `json:"tick"`
	StartedAt    Clock          `json:"started_at"`
	EndedAt      Clock          `json:"ended_at"`
	Snapshot     WorldSnapshot  `json:"snapshot"`
	Parallelism  int            `json:"parallelism"`
	Turns        []TurnResult   `json:"turns"`
	Events       []Event        `json:"events"`
	Reflections  []Event        `json:"reflections,omitempty"`
	MemoryCounts map[string]int `json:"memory_counts,omitempty"`
}

func NewTown(ctx context.Context, opts TownOptions) (*Town, error) {
	if opts.Model == nil {
		return nil, fmt.Errorf("model is required")
	}
	if opts.SessionDir == "" {
		opts.SessionDir = ".noodleville/sessions"
	}
	if opts.Parallelism <= 0 {
		opts.Parallelism = 2
	}
	if opts.TickMinutes <= 0 {
		opts.TickMinutes = 10
	}
	if opts.VillagerCount <= 0 {
		opts.VillagerCount = 12
	}
	world := NewSeedWorldWithCount(opts.VillagerCount)
	if opts.SeedParty {
		if err := world.SeedPartyGoal("maya"); err != nil {
			return nil, err
		}
	}
	store, err := session.NewFileStore(opts.SessionDir)
	if err != nil {
		return nil, err
	}
	town := &Town{
		world:              world,
		villagers:          make(map[string]*VillagerRuntime),
		parallelism:        opts.Parallelism,
		tickMinutes:        opts.TickMinutes,
		reflectionInterval: opts.ReflectionInterval,
	}
	for _, villager := range world.Snapshot().Villagers {
		rt, err := NewVillagerRuntime(ctx, world, store, opts.Model, villager.Profile)
		if err != nil {
			return nil, err
		}
		town.villagers[villager.Profile.ID] = rt
	}
	if opts.SeedParty {
		if err := town.seedPartyMemory(ctx); err != nil {
			return nil, err
		}
	}
	return town, nil
}

func (t *Town) Run(ctx context.Context, ticks int, out io.Writer, jsonOutput bool) error {
	return t.RunWithCallback(ctx, ticks, out, jsonOutput, nil)
}

func (t *Town) RunWithCallback(ctx context.Context, ticks int, out io.Writer, jsonOutput bool, callback func(*TickReport) error) error {
	if ticks <= 0 {
		return nil
	}
	for i := 0; i < ticks; i++ {
		report, err := t.RunTick(ctx)
		if err != nil {
			return err
		}
		if callback != nil {
			if err := callback(report); err != nil {
				return err
			}
		}
		if jsonOutput {
			if err := json.NewEncoder(out).Encode(report); err != nil {
				return err
			}
		} else {
			report.Print(out)
		}
	}
	return nil
}

func (t *Town) RunTick(ctx context.Context) (*TickReport, error) {
	snapshot := t.world.Snapshot()
	villagers := snapshot.Villagers
	sem := make(chan struct{}, t.parallelism)
	results := make(chan indexedTurn, len(villagers))

	var wg sync.WaitGroup
	for i, villager := range villagers {
		rt := t.villagers[villager.Profile.ID]
		wg.Add(1)
		go func(i int, rt *VillagerRuntime) {
			defer wg.Done()
			if rt == nil {
				results <- indexedTurn{index: i, result: TurnResult{VillagerID: villager.Profile.ID, Err: fmt.Errorf("missing runtime")}}
				return
			}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- indexedTurn{index: i, result: TurnResult{VillagerID: rt.profile.ID, Err: ctx.Err()}}
				return
			}
			result, err := rt.TakeTurn(ctx)
			if err != nil {
				result.Err = err
			}
			results <- indexedTurn{index: i, result: result}
		}(i, rt)
	}

	wg.Wait()
	close(results)

	turns := make([]TurnResult, len(villagers))
	for item := range results {
		turns[item.index] = item.result
	}

	report := &TickReport{
		Tick:        t.tick,
		StartedAt:   snapshot.Clock,
		Parallelism: t.parallelism,
		Turns:       turns,
	}
	if err := ctx.Err(); err != nil {
		report.EndedAt = snapshot.Clock
		return report, err
	}
	for _, turn := range turns {
		if turn.Err != nil || !turn.HasAction {
			continue
		}
		event := t.world.ApplyAction(turn.Action)
		report.Events = append(report.Events, event)
		if err := t.appendEventMemories(ctx, report.Tick, event); err != nil {
			return report, err
		}
	}
	if t.shouldReflect() {
		reflections, err := t.runReflections(ctx, report.Tick)
		if err != nil {
			return report, err
		}
		report.Reflections = reflections
		report.Events = append(report.Events, reflections...)
	}
	report.EndedAt = t.world.Advance(t.tickMinutes)
	report.MemoryCounts = t.MemoryCounts(ctx)
	report.Snapshot = t.world.Snapshot()
	t.tick++
	return report, nil
}

func (t *Town) shouldReflect() bool {
	return t.reflectionInterval > 0 && (t.tick+1)%t.reflectionInterval == 0
}

func (t *Town) runReflections(ctx context.Context, tick int) ([]Event, error) {
	ids := make([]string, 0, len(t.villagers))
	for id := range t.villagers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	reflections := make([]Event, 0, len(ids))
	for _, id := range ids {
		rt := t.villagers[id]
		action, err := rt.ReflectionAction(ctx)
		if err != nil {
			return nil, err
		}
		event := t.world.ApplyAction(action)
		reflections = append(reflections, event)
		if err := t.appendEventMemories(ctx, tick, event); err != nil {
			return nil, err
		}
		if err := rt.CompactMemory(ctx); err != nil {
			return nil, err
		}
	}
	return reflections, nil
}

func (t *Town) seedPartyMemory(ctx context.Context) error {
	rt := t.villagers["maya"]
	if rt == nil {
		return nil
	}
	return rt.memory.Append(ctx, MemoryEntry{
		Kind:       "goal",
		Importance: 10,
		Text:       "Maya wants to throw a Saturday noodle party and spread the idea through ordinary neighbor conversations.",
	})
}

func (t *Town) appendEventMemories(ctx context.Context, tick int, event Event) error {
	if rt := t.villagers[event.ActorID]; rt != nil {
		if err := rt.AppendMemory(ctx, tick, event.At, event); err != nil {
			return err
		}
	}
	if event.TargetID != "" {
		if rt := t.villagers[event.TargetID]; rt != nil {
			if err := rt.AppendMemory(ctx, tick, event.At, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Town) Snapshot() WorldSnapshot {
	return t.world.Snapshot()
}

func (t *Town) InspectVillager(ctx context.Context, id string) (VillagerInspector, error) {
	villager, ok := t.world.Villager(id)
	if !ok {
		return VillagerInspector{}, fmt.Errorf("unknown villager %q", id)
	}
	rt := t.villagers[id]
	if rt == nil {
		return VillagerInspector{}, fmt.Errorf("missing runtime for %q", id)
	}
	memories, err := rt.memory.Retrieve(ctx, 8)
	if err != nil {
		return VillagerInspector{}, err
	}
	count, err := rt.MemoryCount(ctx)
	if err != nil {
		return VillagerInspector{}, err
	}
	return VillagerInspector{
		Villager:      villager,
		Party:         t.world.PartyKnowledge(id),
		Relationships: t.world.RelationshipSnapshot(id),
		Memories:      memories,
		MemoryCount:   count,
	}, nil
}

func (t *Town) MemoryCounts(ctx context.Context) map[string]int {
	ids := make([]string, 0, len(t.villagers))
	for id := range t.villagers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		n, err := t.villagers[id].MemoryCount(ctx)
		if err == nil {
			counts[id] = n
		}
	}
	return counts
}

type indexedTurn struct {
	index  int
	result TurnResult
}

func (r *TickReport) Print(out io.Writer) {
	fmt.Fprintf(out, "tick=%d start=%s end=%s parallelism=%d\n", r.Tick, r.StartedAt.String(), r.EndedAt.String(), r.Parallelism)
	for _, turn := range r.Turns {
		if turn.Err != nil {
			fmt.Fprintf(out, "  villager=%s error=%q\n", turn.VillagerID, turn.Err.Error())
			continue
		}
		fmt.Fprintf(out, "  villager=%s action=%s duration=%s output=%q\n",
			turn.VillagerID, turn.Action.Kind, turn.Duration.Round(time.Millisecond), turn.Output)
		if len(turn.Callbacks) > 0 {
			fmt.Fprintf(out, "    callbacks=%v\n", turn.Callbacks)
		}
	}
	for _, event := range r.Events {
		fmt.Fprintf(out, "  event kind=%s actor=%s text=%q\n", event.Kind, event.ActorName, event.Text)
	}
	fmt.Fprintln(out)
}
