// Package memory provides a cross-session memory service for Dive agents.
//
// The Service interface is designed to accommodate both keyword-based and
// vector-backed implementations without API changes. NewMemoryService returns
// an in-memory keyword implementation suitable for local development.
//
// Usage:
//
//	svc := memory.NewMemoryService()
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model: anthropic.New(),
//	    Tools: memory.MemoryTools(svc),
//	    Hooks: dive.Hooks{
//	        PreGeneration: []dive.PreGenerationHook{
//	            memory.InjectMemoriesHook(svc, 5),
//	        },
//	    },
//	})
package memory

import (
	"context"
	"time"
)

// Entry is a single memory stored in the service.
type Entry struct {
	ID        string
	Content   string
	Tags      []string
	SessionID string
	CreatedAt time.Time
	Metadata  map[string]any
}

// Service stores and retrieves memory entries across sessions.
type Service interface {
	// Save persists an entry. If the entry has no ID, one is assigned.
	Save(ctx context.Context, entry Entry) error

	// Search returns up to limit entries relevant to the query.
	Search(ctx context.Context, query string, limit int) ([]Entry, error)

	// Delete removes the entry with the given ID.
	Delete(ctx context.Context, id string) error
}
