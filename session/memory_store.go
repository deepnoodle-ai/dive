package session

import (
	"context"
	"maps"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store implementation.
//
// Suitable for development, testing, and single-instance deployments.
// Data is lost when the process exits.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionData
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*sessionData),
	}
}

func (s *MemoryStore) Open(ctx context.Context, id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.sessions[id]
	if !ok {
		now := time.Now()
		data = &sessionData{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.sessions[id] = data
	}
	return &Session{
		data:     data,
		appender: s,
	}, nil
}

func (s *MemoryStore) Put(ctx context.Context, sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := cloneSessionData(sess.data)
	s.sessions[cp.ID] = cp
	sess.data = cp
	sess.appender = s
	return nil
}

func (s *MemoryStore) List(ctx context.Context, opts *ListOptions) (*ListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	infos := make([]*SessionInfo, 0, len(s.sessions))
	for _, data := range s.sessions {
		infos = append(infos, &SessionInfo{
			ID:         data.ID,
			Title:      data.Title,
			CreatedAt:  data.CreatedAt,
			UpdatedAt:  data.UpdatedAt,
			EventCount: len(data.Events),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt.After(infos[j].UpdatedAt)
	})

	if opts != nil {
		if opts.Offset > 0 {
			if opts.Offset < len(infos) {
				infos = infos[opts.Offset:]
			} else {
				infos = nil
			}
		}
		if opts.Limit > 0 && opts.Limit < len(infos) {
			infos = infos[:opts.Limit]
		}
	}

	return &ListResult{Sessions: infos}, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

// appendEvent implements eventAppender for MemoryStore. The session already
// updated its own data slice, so the store just updates the timestamp.
func (s *MemoryStore) appendEvent(ctx context.Context, sessionID string, evt *event) error {
	// No-op: MemoryStore shares data directly with Session, so the event
	// is already appended in Session.SaveTurn.
	return nil
}

// putSession implements eventAppender for MemoryStore. Used by Compact.
func (s *MemoryStore) putSession(ctx context.Context, data *sessionData) error {
	// No-op: MemoryStore shares data directly with Session.
	return nil
}

func cloneSessionData(data *sessionData) *sessionData {
	cp := &sessionData{
		ID:         data.ID,
		Title:      data.Title,
		CreatedAt:  data.CreatedAt,
		UpdatedAt:  data.UpdatedAt,
		ForkedFrom: data.ForkedFrom,
	}
	if len(data.Events) > 0 {
		cp.Events = make([]*event, len(data.Events))
		for i, e := range data.Events {
			cp.Events[i] = e.copy()
		}
	}
	if data.Metadata != nil {
		cp.Metadata = make(map[string]any, len(data.Metadata))
		maps.Copy(cp.Metadata, data.Metadata)
	}
	return cp
}
