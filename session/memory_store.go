package session

import (
	"context"
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
	sessions map[string]*Session
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session),
	}
}

func (s *MemoryStore) Open(ctx context.Context, id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		now := time.Now()
		sess = &Session{
			data: &sessionData{
				ID:        id,
				CreatedAt: now,
				UpdatedAt: now,
			},
			appender: s,
		}
		s.sessions[id] = sess
	}
	return sess, nil
}

func (s *MemoryStore) Put(ctx context.Context, sess *Session) error {
	sess.mu.Lock()
	sess.appender = s
	id := sess.data.ID
	sess.mu.Unlock()

	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) List(ctx context.Context, opts *ListOptions) (*ListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	infos := make([]*SessionInfo, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sess.mu.RLock()
		info := &SessionInfo{
			ID:         sess.data.ID,
			Title:      sess.data.Title,
			CreatedAt:  sess.data.CreatedAt,
			UpdatedAt:  sess.data.UpdatedAt,
			EventCount: len(sess.data.Events),
			Metadata:   sess.data.Metadata,
		}
		sess.mu.RUnlock()
		infos = append(infos, info)
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
