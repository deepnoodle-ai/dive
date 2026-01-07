package dive

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

// FileSessionRepository stores sessions as individual JSON files on disk.
//
// Each session is stored as a separate file named {session_id}.json in the
// configured base directory (typically ~/.dive/sessions/).
//
// This implementation is suitable for CLI applications and single-user
// deployments where sessions need to persist across process restarts.
//
// All operations are thread-safe using a read-write mutex.
//
// Example:
//
//	repo, err := dive.NewFileSessionRepository("~/.dive/sessions")
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Name:              "Assistant",
//	    Model:             model,
//	    SessionRepository: repo,
//	})
type FileSessionRepository struct {
	mu      sync.RWMutex
	baseDir string
}

// NewFileSessionRepository creates a new file-based session repository.
//
// The baseDir specifies where session JSON files will be stored. The directory
// is created if it does not exist.
//
// Returns an error if the directory cannot be created.
func NewFileSessionRepository(baseDir string) (*FileSessionRepository, error) {
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

	return &FileSessionRepository{
		baseDir: baseDir,
	}, nil
}

// sessionPath returns the file path for a session ID.
func (r *FileSessionRepository) sessionPath(sessionID string) string {
	return filepath.Join(r.baseDir, sessionID+".json")
}

// PutSession stores a session, creating it if new or replacing if it exists.
func (r *FileSessionRepository) PutSession(ctx context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.sessionPath(session.ID), data, 0644)
}

// GetSession retrieves a session by ID.
// Returns ErrSessionNotFound if the session does not exist.
func (r *FileSessionRepository) GetSession(ctx context.Context, id string) (*Session, error) {
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
// This operation is idempotent; deleting a non-existent session returns nil.
func (r *FileSessionRepository) DeleteSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	err := os.Remove(r.sessionPath(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListSessions returns sessions sorted by UpdatedAt descending (most recent first).
//
// Supports pagination via Offset and Limit in ListSessionsInput.
func (r *FileSessionRepository) ListSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error) {
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
			continue // Skip files we can't read
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue // Skip malformed files
		}

		sessions = append(sessions, &session)
	}

	// Sort by UpdatedAt descending (most recent first)
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
//
// The forked session receives a new auto-generated ID and contains deep copies
// of all messages from the original session. The original session is not modified.
//
// All session metadata (UserID, AgentID, AgentName, Title, Metadata) is copied
// to the forked session. Timestamps are set to the current time.
//
// The forked session is automatically stored before being returned.
//
// Returns ErrSessionNotFound if the source session does not exist.
func (r *FileSessionRepository) ForkSession(ctx context.Context, sessionID string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Read original session
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

	// Deep copy the messages using Message.Copy() to ensure independence
	messagesCopy := make([]*llm.Message, len(original.Messages))
	for i, msg := range original.Messages {
		messagesCopy[i] = msg.Copy()
	}

	// Create forked session with new auto-generated ID
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

	// Shallow copy metadata (sufficient for typical use cases)
	if original.Metadata != nil {
		forked.Metadata = make(map[string]interface{}, len(original.Metadata))
		for k, v := range original.Metadata {
			forked.Metadata[k] = v
		}
	}

	// Store the forked session
	forkedData, err := json.MarshalIndent(forked, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(r.sessionPath(forked.ID), forkedData, 0644); err != nil {
		return nil, err
	}

	return forked, nil
}
