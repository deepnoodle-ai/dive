package main

import "sort"

const partyTopic = "Saturday noodle party"

type PartyKnowledge struct {
	Knows      bool   `json:"knows"`
	SourceID   string `json:"source_id,omitempty"`
	SourceName string `json:"source_name,omitempty"`
	HeardAt    Clock  `json:"heard_at,omitzero"`
	Evidence   string `json:"evidence,omitempty"`
}

type Relationship struct {
	VillagerID      string `json:"villager_id"`
	VillagerName    string `json:"villager_name"`
	Familiarity     int    `json:"familiarity"`
	Trust           int    `json:"trust"`
	Conversations   int    `json:"conversations"`
	LastInteraction string `json:"last_interaction,omitempty"`
}

type KnowledgeTransfer struct {
	Topic    string `json:"topic"`
	FromID   string `json:"from_id"`
	FromName string `json:"from_name"`
	ToID     string `json:"to_id"`
	ToName   string `json:"to_name"`
}

type SocialSnapshot struct {
	Party         map[string]PartyKnowledge          `json:"party"`
	Relationships map[string]map[string]Relationship `json:"relationships"`
}

type VillagerInspector struct {
	Villager      VillagerState  `json:"villager"`
	Party         PartyKnowledge `json:"party"`
	Relationships []Relationship `json:"relationships"`
	Memories      []MemoryEntry  `json:"memories"`
	MemoryCount   int            `json:"memory_count"`
}

func sortedRelationships(rels map[string]Relationship) []Relationship {
	out := make([]Relationship, 0, len(rels))
	for _, rel := range rels {
		out = append(out, rel)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Conversations != out[j].Conversations {
			return out[i].Conversations > out[j].Conversations
		}
		if out[i].Familiarity != out[j].Familiarity {
			return out[i].Familiarity > out[j].Familiarity
		}
		return out[i].VillagerName < out[j].VillagerName
	})
	return out
}
