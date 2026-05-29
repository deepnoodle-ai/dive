package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

const nearbyDistance = 1

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func (p Point) Distance(other Point) int {
	dx := p.X - other.X
	if dx < 0 {
		dx = -dx
	}
	dy := p.Y - other.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

type Clock struct {
	Day    int `json:"day"`
	Minute int `json:"minute"`
}

func (c Clock) Advance(minutes int) Clock {
	if minutes <= 0 {
		return c
	}
	c.Minute += minutes
	for c.Minute >= 24*60 {
		c.Day++
		c.Minute -= 24 * 60
	}
	return c
}

func (c Clock) String() string {
	return fmt.Sprintf("day %d %02d:%02d", c.Day, c.Minute/60, c.Minute%60)
}

type Place struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Position    Point  `json:"position"`
}

type VillagerProfile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	HomeID string `json:"home_id"`
	Goal   string `json:"goal"`
	Quirk  string `json:"quirk"`
	Mood   string `json:"mood"`
}

type VillagerState struct {
	Profile     VillagerProfile `json:"profile"`
	Position    Point           `json:"position"`
	CurrentPlan string          `json:"current_plan,omitempty"`
	LastAction  string          `json:"last_action,omitempty"`
}

type WorldSnapshot struct {
	Width     int             `json:"width"`
	Height    int             `json:"height"`
	Clock     Clock           `json:"clock"`
	Places    []Place         `json:"places"`
	Villagers []VillagerState `json:"villagers"`
	Social    SocialSnapshot  `json:"social"`
}

type Perception struct {
	Clock                Clock                     `json:"clock"`
	Self                 VillagerState             `json:"self"`
	NearbyPlaces         []Place                   `json:"nearby_places"`
	NearbyVillagers      []VillagerState           `json:"nearby_villagers"`
	Party                PartyKnowledge            `json:"party"`
	NearbyPartyKnowledge map[string]PartyKnowledge `json:"nearby_party_knowledge,omitempty"`
	RetrievedMemories    []MemoryEntry             `json:"retrieved_memories"`
}

func (p Perception) Prompt() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Current NoodleVille perception for %s:\n", p.Self.Profile.Name)
	fmt.Fprintf(&b, "- Time: %s\n", p.Clock.String())
	fmt.Fprintf(&b, "- Location: (%d,%d)\n", p.Self.Position.X, p.Self.Position.Y)
	if p.Self.CurrentPlan == "" {
		b.WriteString("- Current plan: none yet\n")
	} else {
		fmt.Fprintf(&b, "- Current plan: %s\n", p.Self.CurrentPlan)
	}
	if p.Party.Knows {
		fmt.Fprintf(&b, "- Party idea: known; source=%s; evidence=%s\n", p.Party.SourceName, p.Party.Evidence)
	} else {
		b.WriteString("- Party idea: not heard yet\n")
	}
	if len(p.NearbyPlaces) == 0 {
		b.WriteString("- Nearby places: none within one tile\n")
	} else {
		b.WriteString("- Nearby places:\n")
		for _, place := range p.NearbyPlaces {
			fmt.Fprintf(&b, "  - %s (%s) at (%d,%d): %s\n",
				place.Name, place.Kind, place.Position.X, place.Position.Y, place.Description)
		}
	}
	if len(p.NearbyVillagers) == 0 {
		b.WriteString("- Nearby villagers: none within one tile\n")
	} else {
		b.WriteString("- Nearby villagers:\n")
		for _, v := range p.NearbyVillagers {
			fmt.Fprintf(&b, "  - %s, %s, at (%d,%d), mood: %s\n",
				v.Profile.Name, v.Profile.Role, v.Position.X, v.Position.Y, v.Profile.Mood)
		}
	}
	if len(p.RetrievedMemories) == 0 {
		b.WriteString("- Retrieved memories: none yet\n")
	} else {
		b.WriteString("- Retrieved memories:\n")
		for _, memory := range p.RetrievedMemories {
			fmt.Fprintf(&b, "  - [%s importance=%d] %s\n", memory.Kind, memory.Importance, memory.Text)
		}
	}
	return b.String()
}

type ActionKind string

const (
	ActionMove    ActionKind = "move_to"
	ActionTalk    ActionKind = "talk"
	ActionUse     ActionKind = "use"
	ActionReflect ActionKind = "reflect"
	ActionPlan    ActionKind = "plan"
	ActionNoop    ActionKind = "noop"
)

type Action struct {
	ActorID    string     `json:"actor_id"`
	Kind       ActionKind `json:"kind"`
	PlaceID    string     `json:"place_id,omitempty"`
	TargetID   string     `json:"target_id,omitempty"`
	Message    string     `json:"message,omitempty"`
	Thought    string     `json:"thought,omitempty"`
	Importance int        `json:"importance,omitempty"`
}

type Event struct {
	At         Clock              `json:"at"`
	Kind       ActionKind         `json:"kind"`
	ActorID    string             `json:"actor_id"`
	ActorName  string             `json:"actor_name"`
	TargetID   string             `json:"target_id,omitempty"`
	TargetName string             `json:"target_name,omitempty"`
	Text       string             `json:"text"`
	Importance int                `json:"importance"`
	Knowledge  *KnowledgeTransfer `json:"knowledge,omitempty"`
}

const partyGoal = "quietly organize a Saturday noodle party and invite neighbors through natural conversation"

type World struct {
	mu        sync.RWMutex
	width     int
	height    int
	clock     Clock
	places    map[string]Place
	villagers map[string]VillagerState
	party     map[string]PartyKnowledge
	relations map[string]map[string]Relationship
	feed      []Event
}

func NewSeedWorld() *World {
	return NewSeedWorldWithCount(5)
}

func NewSeedWorldWithCount(count int) *World {
	places := []Place{
		{ID: "square", Name: "Town Square", Kind: "commons", Description: "a sunny plaza with a message board", Position: Point{X: 2, Y: 2}},
		{ID: "cafe", Name: "Noodle Cafe", Kind: "cafe", Description: "warm noodles, coffee, and a corner table", Position: Point{X: 1, Y: 2}},
		{ID: "park", Name: "Lantern Park", Kind: "park", Description: "benches, old trees, and paper lanterns", Position: Point{X: 3, Y: 2}},
		{ID: "workshop", Name: "Tinker Workshop", Kind: "workshop", Description: "tools, spare parts, and half-built gadgets", Position: Point{X: 2, Y: 1}},
		{ID: "library", Name: "Pocket Library", Kind: "library", Description: "quiet shelves and a community notebook", Position: Point{X: 2, Y: 3}},
		{ID: "market", Name: "Morning Market", Kind: "market", Description: "produce crates and rumor-friendly stalls", Position: Point{X: 1, Y: 1}},
		{ID: "stage", Name: "Lantern Stage", Kind: "stage", Description: "a tiny platform used for announcements and songs", Position: Point{X: 3, Y: 3}},
		{ID: "bakery", Name: "Moon Bun Bakery", Kind: "bakery", Description: "sweet rolls cooling near an open window", Position: Point{X: 0, Y: 3}},
		{ID: "studio", Name: "River Studio", Kind: "studio", Description: "paint, clay, and a window facing the stream", Position: Point{X: 4, Y: 1}},
		{ID: "clinic", Name: "Little Clinic", Kind: "clinic", Description: "a calm room with tea and bandages", Position: Point{X: 4, Y: 3}},
	}
	profiles := seedProfiles()
	if count <= 0 || count > len(profiles) {
		count = len(profiles)
	}
	positions := seedPositions()
	villagers := make([]VillagerState, 0, count)
	for i := 0; i < count; i++ {
		profile := profiles[i]
		profile.HomeID = profile.ID + "_home"
		home := Place{
			ID:          profile.HomeID,
			Name:        profile.Name + "'s House",
			Kind:        "house",
			Description: "a small home on the edge of town",
			Position:    homePosition(i),
		}
		places = append(places, home)
		villagers = append(villagers, VillagerState{
			Profile:  profile,
			Position: positions[i%len(positions)],
		})
	}
	return NewWorld(5, 5, Clock{Day: 1, Minute: 8 * 60}, places, villagers)
}

func seedProfiles() []VillagerProfile {
	return []VillagerProfile{
		{ID: "maya", Name: "Maya", Role: "cafe owner", Goal: "make everyone feel welcome", Quirk: "notices who needs a snack", Mood: "bright"},
		{ID: "ben", Name: "Ben", Role: "mechanic", Goal: "fix the square's old clock", Quirk: "turns every chat into a prototype idea", Mood: "focused"},
		{ID: "lina", Name: "Lina", Role: "librarian", Goal: "collect town stories", Quirk: "remembers tiny details", Mood: "curious"},
		{ID: "ori", Name: "Ori", Role: "gardener", Goal: "keep Lantern Park blooming", Quirk: "names every plant", Mood: "gentle"},
		{ID: "sol", Name: "Sol", Role: "mail carrier", Goal: "connect people with the right news", Quirk: "always knows a shortcut", Mood: "energetic"},
		{ID: "niko", Name: "Niko", Role: "baker", Goal: "invent a new festival bun", Quirk: "speaks in recipe metaphors", Mood: "warm"},
		{ID: "tess", Name: "Tess", Role: "nurse", Goal: "make the town slow down before anyone burns out", Quirk: "carries spare tea bags", Mood: "steady"},
		{ID: "jun", Name: "Jun", Role: "painter", Goal: "paint the town at golden hour", Quirk: "describes colors precisely", Mood: "dreamy"},
		{ID: "paz", Name: "Paz", Role: "market keeper", Goal: "match neighbors with what they need", Quirk: "knows every trade", Mood: "practical"},
		{ID: "ren", Name: "Ren", Role: "musician", Goal: "write a song everyone can hum", Quirk: "taps rhythms on railings", Mood: "playful"},
		{ID: "ada", Name: "Ada", Role: "teacher", Goal: "turn ordinary errands into lessons", Quirk: "asks excellent questions", Mood: "patient"},
		{ID: "cass", Name: "Cass", Role: "carpenter", Goal: "repair the Lantern Stage", Quirk: "measures things by eye", Mood: "confident"},
		{ID: "mina", Name: "Mina", Role: "beekeeper", Goal: "keep the rooftop hives calm", Quirk: "listens before answering", Mood: "soft-spoken"},
		{ID: "otto", Name: "Otto", Role: "archivist", Goal: "preserve the town's best rumors", Quirk: "labels everything", Mood: "dry"},
		{ID: "ivy", Name: "Ivy", Role: "tailor", Goal: "mend the festival banners", Quirk: "notices loose threads", Mood: "sharp"},
		{ID: "ravi", Name: "Ravi", Role: "fisher", Goal: "bring fresh trout to the cafe", Quirk: "predicts weather from ripples", Mood: "easygoing"},
		{ID: "zara", Name: "Zara", Role: "potter", Goal: "fire a perfect noodle bowl", Quirk: "keeps clay under her nails", Mood: "absorbed"},
		{ID: "eli", Name: "Eli", Role: "watchmaker", Goal: "help Ben tune the square clock", Quirk: "hears tiny clicks", Mood: "precise"},
		{ID: "noa", Name: "Noa", Role: "runner", Goal: "learn every shortcut in town", Quirk: "arrives slightly breathless", Mood: "restless"},
		{ID: "bea", Name: "Bea", Role: "florist", Goal: "make every table smell like spring", Quirk: "pairs flowers with moods", Mood: "cheerful"},
		{ID: "cal", Name: "Cal", Role: "chef", Goal: "test a midnight broth recipe", Quirk: "trusts steam more than timers", Mood: "intense"},
		{ID: "dot", Name: "Dot", Role: "printer", Goal: "print invitations and signs", Quirk: "spots typos instantly", Mood: "brisk"},
		{ID: "mo", Name: "Mo", Role: "kid inventor", Goal: "build a lantern kite", Quirk: "asks what happens if", Mood: "excited"},
		{ID: "sana", Name: "Sana", Role: "poet", Goal: "write a toast for the town", Quirk: "collects overheard phrases", Mood: "wistful"},
		{ID: "yuri", Name: "Yuri", Role: "stagehand", Goal: "keep events running smoothly", Quirk: "appears exactly when needed", Mood: "calm"},
	}
}

func seedPositions() []Point {
	return []Point{
		{X: 1, Y: 2}, {X: 2, Y: 1}, {X: 2, Y: 3}, {X: 3, Y: 2}, {X: 1, Y: 2},
		{X: 0, Y: 3}, {X: 4, Y: 3}, {X: 4, Y: 1}, {X: 1, Y: 1}, {X: 3, Y: 3},
		{X: 2, Y: 2}, {X: 3, Y: 3}, {X: 3, Y: 2}, {X: 2, Y: 3}, {X: 2, Y: 2},
		{X: 1, Y: 2}, {X: 4, Y: 1}, {X: 2, Y: 1}, {X: 1, Y: 1}, {X: 3, Y: 2},
		{X: 1, Y: 2}, {X: 3, Y: 3}, {X: 2, Y: 2}, {X: 2, Y: 3}, {X: 3, Y: 3},
	}
}

func homePosition(i int) Point {
	perimeter := []Point{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 3, Y: 0}, {X: 4, Y: 0},
		{X: 0, Y: 1}, {X: 4, Y: 2}, {X: 0, Y: 4}, {X: 1, Y: 4}, {X: 2, Y: 4},
		{X: 3, Y: 4}, {X: 4, Y: 4}, {X: 0, Y: 2}, {X: 4, Y: 3}, {X: 0, Y: 3},
	}
	return perimeter[i%len(perimeter)]
}

func NewWorld(width, height int, clock Clock, places []Place, villagers []VillagerState) *World {
	w := &World{
		width:     width,
		height:    height,
		clock:     clock,
		places:    make(map[string]Place, len(places)),
		villagers: make(map[string]VillagerState, len(villagers)),
		party:     make(map[string]PartyKnowledge, len(villagers)),
		relations: make(map[string]map[string]Relationship, len(villagers)),
	}
	for _, place := range places {
		w.places[place.ID] = place
	}
	for _, villager := range villagers {
		w.villagers[villager.Profile.ID] = villager
		w.party[villager.Profile.ID] = PartyKnowledge{}
		w.relations[villager.Profile.ID] = make(map[string]Relationship)
	}
	return w
}

func (w *World) SeedPartyGoal(villagerID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	villager, ok := w.villagers[villagerID]
	if !ok {
		return fmt.Errorf("unknown villager %q", villagerID)
	}
	villager.Profile.Goal = partyGoal
	w.villagers[villagerID] = villager
	w.party[villagerID] = PartyKnowledge{
		Knows:      true,
		SourceID:   villagerID,
		SourceName: villager.Profile.Name,
		HeardAt:    w.clock,
		Evidence:   "seed goal",
	}
	return nil
}

func (w *World) Snapshot() WorldSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()

	places := make([]Place, 0, len(w.places))
	for _, place := range w.places {
		places = append(places, place)
	}
	sort.Slice(places, func(i, j int) bool { return places[i].ID < places[j].ID })

	villagers := make([]VillagerState, 0, len(w.villagers))
	for _, villager := range w.villagers {
		villagers = append(villagers, villager)
	}
	sort.Slice(villagers, func(i, j int) bool { return villagers[i].Profile.ID < villagers[j].Profile.ID })

	return WorldSnapshot{
		Width:     w.width,
		Height:    w.height,
		Clock:     w.clock,
		Places:    places,
		Villagers: villagers,
		Social:    w.socialSnapshotLocked(),
	}
}

func (w *World) Perception(villagerID string, memories []MemoryEntry) (Perception, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	self, ok := w.villagers[villagerID]
	if !ok {
		return Perception{}, fmt.Errorf("unknown villager %q", villagerID)
	}
	p := Perception{
		Clock:                w.clock,
		Self:                 self,
		Party:                w.party[self.Profile.ID],
		NearbyPartyKnowledge: make(map[string]PartyKnowledge),
		RetrievedMemories:    memories,
	}
	for _, place := range w.places {
		if self.Position.Distance(place.Position) <= nearbyDistance {
			p.NearbyPlaces = append(p.NearbyPlaces, place)
		}
	}
	sort.Slice(p.NearbyPlaces, func(i, j int) bool {
		di := self.Position.Distance(p.NearbyPlaces[i].Position)
		dj := self.Position.Distance(p.NearbyPlaces[j].Position)
		if di != dj {
			return di < dj
		}
		return p.NearbyPlaces[i].Name < p.NearbyPlaces[j].Name
	})
	for _, villager := range w.villagers {
		if villager.Profile.ID == villagerID {
			continue
		}
		if self.Position.Distance(villager.Position) <= nearbyDistance {
			p.NearbyVillagers = append(p.NearbyVillagers, villager)
			p.NearbyPartyKnowledge[villager.Profile.ID] = w.party[villager.Profile.ID]
		}
	}
	sort.Slice(p.NearbyVillagers, func(i, j int) bool {
		di := self.Position.Distance(p.NearbyVillagers[i].Position)
		dj := self.Position.Distance(p.NearbyVillagers[j].Position)
		if di != dj {
			return di < dj
		}
		return p.NearbyVillagers[i].Profile.Name < p.NearbyVillagers[j].Profile.Name
	})
	return p, nil
}

func (w *World) RelationshipSnapshot(villagerID string) []Relationship {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return sortedRelationships(w.relations[villagerID])
}

func (w *World) PartyKnowledge(villagerID string) PartyKnowledge {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.party[villagerID]
}

func (w *World) Villager(id string) (VillagerState, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	villager, ok := w.villagers[id]
	return villager, ok
}

func (w *World) PlaceByNameOrID(query string) (Place, bool) {
	key := normalize(query)
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, place := range w.places {
		if normalize(place.ID) == key || normalize(place.Name) == key {
			return place, true
		}
	}
	return Place{}, false
}

func (w *World) VillagerByNameOrID(query string) (VillagerState, bool) {
	key := normalize(query)
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, villager := range w.villagers {
		if normalize(villager.Profile.ID) == key || normalize(villager.Profile.Name) == key {
			return villager, true
		}
	}
	return VillagerState{}, false
}

func (w *World) ApplyAction(action Action) Event {
	w.mu.Lock()
	defer w.mu.Unlock()

	actor, ok := w.villagers[action.ActorID]
	if !ok {
		return Event{
			At:         w.clock,
			Kind:       ActionNoop,
			ActorID:    action.ActorID,
			Text:       fmt.Sprintf("Unknown villager %q could not act.", action.ActorID),
			Importance: 1,
		}
	}

	event := Event{
		At:         w.clock,
		Kind:       action.Kind,
		ActorID:    actor.Profile.ID,
		ActorName:  actor.Profile.Name,
		Importance: clampImportance(action.Importance),
	}
	switch action.Kind {
	case ActionMove:
		place, ok := w.places[action.PlaceID]
		if !ok {
			event.Kind = ActionNoop
			event.Text = fmt.Sprintf("%s wanted to move, but the destination was missing.", actor.Profile.Name)
			break
		}
		actor.Position = place.Position
		actor.LastAction = fmt.Sprintf("walked to %s", place.Name)
		w.villagers[actor.Profile.ID] = actor
		event.Text = fmt.Sprintf("%s walked to %s.", actor.Profile.Name, place.Name)
	case ActionTalk:
		target, ok := w.villagers[action.TargetID]
		if !ok {
			event.Kind = ActionNoop
			event.Text = fmt.Sprintf("%s tried to talk, but no one was there.", actor.Profile.Name)
			break
		}
		event.TargetID = target.Profile.ID
		event.TargetName = target.Profile.Name
		if actor.Position != target.Position {
			event.Kind = ActionNoop
			event.Text = fmt.Sprintf("%s looked for %s, but they were not co-located.", actor.Profile.Name, target.Profile.Name)
			break
		}
		message := cleanSentence(action.Message, 140)
		actor.LastAction = fmt.Sprintf("told %s: %s", target.Profile.Name, message)
		target.LastAction = fmt.Sprintf("heard from %s: %s", actor.Profile.Name, message)
		w.villagers[actor.Profile.ID] = actor
		w.villagers[target.Profile.ID] = target
		w.recordConversationLocked(actor, target, message)
		event.Text = fmt.Sprintf("%s told %s: %q", actor.Profile.Name, target.Profile.Name, message)
		if transfer := w.propagatePartyLocked(actor, target, message); transfer != nil {
			event.Knowledge = transfer
			event.Text += fmt.Sprintf(" %s now knows about the %s.", target.Profile.Name, partyTopic)
		}
		if event.Importance < 5 {
			event.Importance = 5
		}
	case ActionUse:
		place, ok := w.places[action.PlaceID]
		if !ok {
			event.Kind = ActionNoop
			event.Text = fmt.Sprintf("%s tried to use something, but it was missing.", actor.Profile.Name)
			break
		}
		if actor.Position.Distance(place.Position) > nearbyDistance {
			event.Kind = ActionNoop
			event.Text = fmt.Sprintf("%s wanted to use %s, but it was too far away.", actor.Profile.Name, place.Name)
			break
		}
		thought := cleanPurpose(action.Thought, 140)
		actor.LastAction = fmt.Sprintf("used %s: %s", place.Name, thought)
		w.villagers[actor.Profile.ID] = actor
		event.Text = fmt.Sprintf("%s used %s: %s.", actor.Profile.Name, place.Name, thought)
	case ActionReflect:
		thought := cleanSentence(action.Thought, 180)
		actor.LastAction = "reflected"
		w.villagers[actor.Profile.ID] = actor
		event.Text = fmt.Sprintf("%s reflected: %s", actor.Profile.Name, thought)
	case ActionPlan:
		plan := cleanSentence(action.Thought, 180)
		actor.CurrentPlan = plan
		actor.LastAction = "made a plan"
		w.villagers[actor.Profile.ID] = actor
		if mentionsParty(plan) && !w.party[actor.Profile.ID].Knows {
			w.party[actor.Profile.ID] = PartyKnowledge{
				Knows:      true,
				SourceID:   actor.Profile.ID,
				SourceName: actor.Profile.Name,
				HeardAt:    w.clock,
				Evidence:   "made a party plan",
			}
		}
		event.Text = fmt.Sprintf("%s made a plan: %s", actor.Profile.Name, plan)
	default:
		event.Kind = ActionNoop
		actor.LastAction = "paused"
		w.villagers[actor.Profile.ID] = actor
		event.Text = fmt.Sprintf("%s paused and watched the town.", actor.Profile.Name)
	}
	if event.Importance == 0 {
		event.Importance = 3
	}
	w.feed = append(w.feed, event)
	return event
}

func (w *World) socialSnapshotLocked() SocialSnapshot {
	party := make(map[string]PartyKnowledge, len(w.party))
	for id, knowledge := range w.party {
		party[id] = knowledge
	}
	relationships := make(map[string]map[string]Relationship, len(w.relations))
	for id, rels := range w.relations {
		relationships[id] = make(map[string]Relationship, len(rels))
		for otherID, rel := range rels {
			relationships[id][otherID] = rel
		}
	}
	return SocialSnapshot{Party: party, Relationships: relationships}
}

func (w *World) recordConversationLocked(actor, target VillagerState, message string) {
	w.bumpRelationshipLocked(actor, target, message)
	w.bumpRelationshipLocked(target, actor, message)
}

func (w *World) bumpRelationshipLocked(owner, other VillagerState, message string) {
	if w.relations[owner.Profile.ID] == nil {
		w.relations[owner.Profile.ID] = make(map[string]Relationship)
	}
	rel := w.relations[owner.Profile.ID][other.Profile.ID]
	rel.VillagerID = other.Profile.ID
	rel.VillagerName = other.Profile.Name
	rel.Conversations++
	rel.Familiarity = clampScore(rel.Familiarity + 2)
	rel.Trust = clampScore(rel.Trust + 1)
	rel.LastInteraction = cleanSentence(message, 90)
	w.relations[owner.Profile.ID][other.Profile.ID] = rel
}

func (w *World) propagatePartyLocked(actor, target VillagerState, message string) *KnowledgeTransfer {
	if !mentionsParty(message) || !w.party[actor.Profile.ID].Knows || w.party[target.Profile.ID].Knows {
		return nil
	}
	knowledge := PartyKnowledge{
		Knows:      true,
		SourceID:   actor.Profile.ID,
		SourceName: actor.Profile.Name,
		HeardAt:    w.clock,
		Evidence:   cleanSentence(message, 120),
	}
	w.party[target.Profile.ID] = knowledge
	return &KnowledgeTransfer{
		Topic:    partyTopic,
		FromID:   actor.Profile.ID,
		FromName: actor.Profile.Name,
		ToID:     target.Profile.ID,
		ToName:   target.Profile.Name,
	}
}

func mentionsParty(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "party") || strings.Contains(s, "saturday noodle")
}

func clampScore(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

func (w *World) Advance(minutes int) Clock {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clock = w.clock.Advance(minutes)
	return w.clock
}

func (w *World) Feed() []Event {
	w.mu.RLock()
	defer w.mu.RUnlock()
	feed := make([]Event, len(w.feed))
	copy(feed, w.feed)
	return feed
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func clampImportance(n int) int {
	if n < 1 {
		return 3
	}
	if n > 10 {
		return 10
	}
	return n
}

func cleanSentence(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "take in the moment"
	}
	if len(s) > max {
		return strings.TrimSpace(s[:max]) + "..."
	}
	return s
}

func cleanPurpose(s string, max int) string {
	s = cleanSentence(s, max)
	return strings.TrimPrefix(s, "to ")
}
