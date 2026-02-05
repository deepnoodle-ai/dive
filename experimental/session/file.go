package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// FileRepository stores sessions as individual JSON files on disk.
//
// Each session is stored as a separate file named {session_id}.json in the
// configured base directory (typically ~/.dive/sessions/).
//
// This implementation is suitable for CLI applications and single-user
// deployments where sessions need to persist across process restarts.
type FileRepository struct {
	mu      sync.RWMutex
	baseDir string
}

// NewFileRepository creates a new file-based session repository.
//
// The baseDir specifies where session JSON files will be stored. The directory
// is created if it does not exist.
func NewFileRepository(baseDir string) (*FileRepository, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(baseDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		baseDir = filepath.Join(home, baseDir[2:])
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}

	return &FileRepository{
		baseDir: baseDir,
	}, nil
}

func (r *FileRepository) sessionPath(sessionID string) string {
	return filepath.Join(r.baseDir, sessionID+".json")
}

// PutSession stores a session.
func (r *FileRepository) PutSession(ctx context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.sessionPath(session.ID), data, 0644)
}

// GetSession retrieves a session by ID.
func (r *FileRepository) GetSession(ctx context.Context, id string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, err := os.ReadFile(r.sessionPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// DeleteSession removes a session by ID.
func (r *FileRepository) DeleteSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	err := os.Remove(r.sessionPath(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListSessions returns sessions sorted by UpdatedAt descending.
func (r *FileRepository) ListSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, err := os.ReadDir(r.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &ListSessionsOutput{Items: []*Session{}}, nil
		}
		return nil, err
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.baseDir, entry.Name()))
		if err != nil {
			continue
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		sessions = append(sessions, &session)
	}

	// Sort by UpdatedAt descending
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	// Apply pagination
	if input != nil {
		if input.Offset > 0 {
			if input.Offset < len(sessions) {
				sessions = sessions[input.Offset:]
			} else {
				sessions = nil
			}
		}
		if input.Limit > 0 && input.Limit < len(sessions) {
			sessions = sessions[:input.Limit]
		}
	}

	return &ListSessionsOutput{Items: sessions}, nil
}

// ForkSession creates a new session as a copy of an existing session.
func (r *FileRepository) ForkSession(ctx context.Context, sessionID string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.sessionPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	var original Session
	if err := json.Unmarshal(data, &original); err != nil {
		return nil, err
	}

	// Deep copy the messages
	messagesCopy := make([]*llm.Message, len(original.Messages))
	for i, msg := range original.Messages {
		messagesCopy[i] = msg.Copy()
	}

	forked := &Session{
		ID:        newSessionID(),
		UserID:    original.UserID,
		AgentID:   original.AgentID,
		AgentName: original.AgentName,
		Title:     original.Title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  messagesCopy,
	}

	if original.Metadata != nil {
		forked.Metadata = make(map[string]any, len(original.Metadata))
		for k, v := range original.Metadata {
			forked.Metadata[k] = v
		}
	}

	forkedData, err := json.MarshalIndent(forked, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(r.sessionPath(forked.ID), forkedData, 0644); err != nil {
		return nil, err
	}

	return forked, nil
}
