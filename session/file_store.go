package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
)

// ErrInvalidSessionID is returned when a session ID contains path separators,
// relative path components, or other characters that could cause path traversal.
var ErrInvalidSessionID = errors.New("invalid session ID")

// FileStore persists sessions as JSONL files on disk.
//
// Each session is stored as {dir}/{session_id}.jsonl. The first line is a
// session metadata header; subsequent lines are events. AppendEvent opens
// the file in append mode and writes a single line, making it the efficient
// hot-path write.
//
// # Concurrency model
//
// FileStore assumes single-writer-per-session semantics: at any given
// moment, only one process should hold a *Session for a given session ID.
// Sequential handoff between processes — suspend in process A, exit, then
// resume in process B — is fully supported and crash-consistent (writes
// use tmp file + rename + parent directory fsync, so a reader always sees
// either the previous or the new complete file).
//
// Concurrent access from multiple processes to the same session is NOT
// supported and may cause silent state loss: two processes can both read,
// both rewrite the JSONL, and the later rename wins. There is no OS-level
// file lock. For multi-instance deployments where the same session might
// be touched concurrently, implement a database-backed Session backend
// instead of using FileStore.
//
// Within a single process, a FileStore is safe for concurrent use across
// distinct sessions and serializes writes to the same session via an
// internal mutex.
//
// # Durability
//
// Full session writes (initial Open, Put, Compact) go through a
// tmp-file + fsync + rename + parent-directory fsync sequence and are
// crash-consistent under power loss. The hot-path event append
// (SaveTurn → appendEvent) does not fsync by default: a successful Write
// only guarantees the bytes have reached the OS pagecache, so a power
// loss between commit and the kernel flush can lose the most recent
// completed turn. For most workloads this trade-off is correct — fsync
// on every append costs a disk round-trip per message.
//
// Callers who need power-loss durability for every turn can opt in by
// constructing the store with NewFileStoreWithSync(dir, true). When
// enabled, appendEvent calls f.Sync() before closing the file. This
// still does not fsync the parent directory on every append; we rely on
// the up-front existence of the file (established by Open) staying
// stable.
type FileStore struct {
	mu   sync.RWMutex
	dir  string
	sync bool
}

// NewFileStore creates a FileStore rooted at dir. The directory is created
// if it does not exist. The returned store uses pagecache durability for
// the hot-path event append — see FileStore's documentation for the
// trade-off and NewFileStoreWithSync for power-loss durability.
func NewFileStore(dir string) (*FileStore, error) {
	return NewFileStoreWithSync(dir, false)
}

// NewFileStoreWithSync creates a FileStore rooted at dir with explicit
// control over hot-path durability. When sync is true, appendEvent
// fsyncs the JSONL file before closing it, guaranteeing that a
// successfully returned SaveTurn call survives an immediate power loss
// at the cost of a disk round-trip per append. When sync is false the
// store behaves like NewFileStore.
func NewFileStoreWithSync(dir string, sync bool) (*FileStore, error) {
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, dir[2:])
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &FileStore{dir: dir, sync: sync}, nil
}

// validateID rejects session IDs that could escape the store directory.
func validateID(id string) error {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, "/\\") ||
		strings.Contains(id, "..") {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, id)
	}
	return nil
}

// path returns the file path for the given session ID after validating that
// the result is confined to the store directory.
func (s *FileStore) path(id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}
	p := filepath.Join(s.dir, id+".jsonl")
	p = filepath.Clean(p)
	// Verify the cleaned path is still within the store directory.
	if !strings.HasPrefix(p, s.dir+string(filepath.Separator)) && p != s.dir {
		return "", fmt.Errorf("%w: %q resolves outside store directory", ErrInvalidSessionID, id)
	}
	return p, nil
}

// jsonlLine is the on-disk format for each line in a session JSONL file.
type jsonlLine struct {
	LineType string          `json:"line_type"` // "header" or "event"
	Data     json.RawMessage `json:"data"`
}

// sessionHeader is the first line of a session JSONL file.
type sessionHeader struct {
	ID                 string                    `json:"id"`
	Title              string                    `json:"title,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
	Metadata           map[string]any            `json:"metadata,omitempty"`
	ForkedFrom         string                    `json:"forked_from,omitempty"`
	Suspended          bool                      `json:"suspended,omitempty"`
	PendingToolCalls   []*dive.PendingToolCall   `json:"pending_tool_calls,omitempty"`
	CompletedToolCalls []*dive.CompletedToolCall `json:"completed_tool_calls,omitempty"`
}

func (s *FileStore) Open(ctx context.Context, id string) (*Session, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readSession(id)
	if err != nil {
		if err != ErrNotFound {
			return nil, err
		}
		// Create new session
		now := time.Now()
		data = &sessionData{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.writeSession(data); err != nil {
			return nil, err
		}
	}
	return &Session{
		data:     data,
		appender: s,
	}, nil
}

func (s *FileStore) Put(ctx context.Context, sess *Session) error {
	if err := validateID(sess.data.ID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeSession(sess.data); err != nil {
		return err
	}
	sess.appender = s
	return nil
}

func (s *FileStore) List(ctx context.Context, opts *ListOptions) (*ListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &ListResult{}, nil
		}
		return nil, err
	}

	var infos []*SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		data, err := s.readSession(id)
		if err != nil {
			continue
		}
		if opts != nil && opts.Suspended != nil && *opts.Suspended != data.Suspended {
			continue
		}
		infos = append(infos, &SessionInfo{
			ID:         data.ID,
			Title:      data.Title,
			CreatedAt:  data.CreatedAt,
			UpdatedAt:  data.UpdatedAt,
			EventCount: len(data.Events),
			Metadata:   data.Metadata,
			Suspended:  data.Suspended,
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

func (s *FileStore) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(id)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// appendEvent implements eventAppender for FileStore.
func (s *FileStore) appendEvent(ctx context.Context, sessionID string, evt *event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.path(sessionID)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	eventData, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	line := jsonlLine{LineType: "event", Data: eventData}
	encoded, err := json.Marshal(line)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := f.Write(encoded); err != nil {
		return err
	}
	if s.sync {
		if err := f.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// putSession implements eventAppender for FileStore. Used by Compact.
func (s *FileStore) putSession(ctx context.Context, data *sessionData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeSession(data)
}

// readSession parses a JSONL file into sessionData. Must be called with at
// least a read lock held.
func (s *FileStore) readSession(id string) (*sessionData, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var header sessionHeader
	var events []*event
	first := true

	for scanner.Scan() {
		var line jsonlLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return nil, err
		}
		switch line.LineType {
		case "header":
			if err := json.Unmarshal(line.Data, &header); err != nil {
				return nil, err
			}
			first = false
		case "event":
			var evt event
			if err := json.Unmarshal(line.Data, &evt); err != nil {
				return nil, err
			}
			events = append(events, &evt)
		default:
			if first {
				return nil, ErrNotFound
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	data := &sessionData{
		ID:                 header.ID,
		Title:              header.Title,
		CreatedAt:          header.CreatedAt,
		UpdatedAt:          header.UpdatedAt,
		Events:             events,
		ForkedFrom:         header.ForkedFrom,
		Suspended:          header.Suspended,
		PendingToolCalls:   header.PendingToolCalls,
		CompletedToolCalls: header.CompletedToolCalls,
	}
	if header.Metadata != nil {
		data.Metadata = make(map[string]any, len(header.Metadata))
		maps.Copy(data.Metadata, header.Metadata)
	}

	// Derive UpdatedAt from the last event if events exist.
	if len(events) > 0 {
		last := events[len(events)-1]
		if last.Timestamp.After(data.UpdatedAt) {
			data.UpdatedAt = last.Timestamp
		}
	}

	return data, nil
}

// writeSession writes a complete session as a JSONL file (header + events).
// Must be called with the write lock held.
//
// Writes go to a temp file in the same directory and are promoted via
// os.Rename so a crash mid-write cannot truncate the live JSONL and lose
// pending-call state — readers always see either the previous or the new
// complete file.
func (s *FileStore) writeSession(data *sessionData) error {
	p, err := s.path(data.ID)
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	base := filepath.Base(p)
	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Ensure we clean up the temp file on any error path.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	w := bufio.NewWriter(tmp)

	hdr := sessionHeader{
		ID:                 data.ID,
		Title:              data.Title,
		CreatedAt:          data.CreatedAt,
		UpdatedAt:          data.UpdatedAt,
		Metadata:           data.Metadata,
		ForkedFrom:         data.ForkedFrom,
		Suspended:          data.Suspended,
		PendingToolCalls:   data.PendingToolCalls,
		CompletedToolCalls: data.CompletedToolCalls,
	}
	hdrData, err := json.Marshal(hdr)
	if err != nil {
		tmp.Close()
		return err
	}
	line := jsonlLine{LineType: "header", Data: hdrData}
	encoded, err := json.Marshal(line)
	if err != nil {
		tmp.Close()
		return err
	}
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		tmp.Close()
		return err
	}

	for _, evt := range data.Events {
		eventData, err := json.Marshal(evt)
		if err != nil {
			tmp.Close()
			return err
		}
		line := jsonlLine{LineType: "event", Data: eventData}
		encoded, err := json.Marshal(line)
		if err != nil {
			tmp.Close()
			return err
		}
		if _, err := w.Write(append(encoded, '\n')); err != nil {
			tmp.Close()
			return err
		}
	}

	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, p); err != nil {
		return err
	}
	// Sync the parent directory so the rename itself is durable. Without
	// this, a crash after rename can lose the rename on power loss even
	// though the file contents are on disk.
	dirf, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent dir for sync: %w", err)
	}
	if err := dirf.Sync(); err != nil {
		dirf.Close()
		return fmt.Errorf("sync parent dir: %w", err)
	}
	if err := dirf.Close(); err != nil {
		return fmt.Errorf("close parent dir: %w", err)
	}
	committed = true
	return nil
}
