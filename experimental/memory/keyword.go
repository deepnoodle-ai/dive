package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type memoryService struct {
	mu      sync.RWMutex
	entries []Entry
	nextID  int
}

// NewMemoryService returns an in-memory keyword-search implementation of Service.
// It is thread-safe and suitable for local development and testing.
func NewMemoryService() Service {
	return &memoryService{}
}

func (s *memoryService) Save(_ context.Context, entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		s.nextID++
		entry.ID = fmt.Sprintf("mem_%d", s.nextID)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	for i, e := range s.entries {
		if e.ID == entry.ID {
			s.entries[i] = entry
			return nil
		}
	}
	s.entries = append(s.entries, entry)
	return nil
}

func (s *memoryService) Search(_ context.Context, query string, limit int) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		return nil, nil
	}
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	type scored struct {
		entry Entry
		score int
	}
	var results []scored
	for _, e := range s.entries {
		score := scoreEntry(e, queryTokens)
		if score > 0 {
			results = append(results, scored{e, score})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].entry.CreatedAt.After(results[j].entry.CreatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	out := make([]Entry, len(results))
	for i, r := range results {
		out[i] = r.entry
	}
	return out, nil
}

func (s *memoryService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func tokenize(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	})
	seen := make(map[string]bool, len(words))
	unique := words[:0]
	for _, w := range words {
		if len(w) > 2 && !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}
	return unique
}

func scoreEntry(e Entry, queryTokens []string) int {
	entryTokens := tokenize(e.Content)
	tokenSet := make(map[string]bool, len(entryTokens))
	for _, t := range entryTokens {
		tokenSet[t] = true
	}
	for _, tag := range e.Tags {
		for _, t := range tokenize(tag) {
			tokenSet[t] = true
		}
	}
	score := 0
	for _, qt := range queryTokens {
		if tokenSet[qt] {
			score++
		}
	}
	return score
}
