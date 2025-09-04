package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
)

var _ dive.ThreadRepository = &FileThreadRepository{}

// FileThreadRepository is a file-based implementation of ThreadRepository
// that persists conversation threads to disk as JSON files
type FileThreadRepository struct {
	mu       sync.RWMutex
	filePath string
	threads  map[string]*dive.Thread
	dirty    bool // tracks if threads need to be saved
}

// NewFileThreadRepository creates a new FileThreadRepository
func NewFileThreadRepository(filePath string) *FileThreadRepository {
	return &FileThreadRepository{
		filePath: filePath,
		threads:  make(map[string]*dive.Thread),
		dirty:    false,
	}
}

// Load reads threads from the file into memory
func (r *FileThreadRepository) Load(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(r.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// If file doesn't exist, start with empty threads
	if _, err := os.Stat(r.filePath); os.IsNotExist(err) {
		r.threads = make(map[string]*dive.Thread)
		return nil
	}

	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return fmt.Errorf("failed to read session file %s: %v", r.filePath, err)
	}

	if len(data) == 0 {
		r.threads = make(map[string]*dive.Thread)
		return nil
	}

	var persistedThreads []*dive.Thread
	if err := json.Unmarshal(data, &persistedThreads); err != nil {
		return fmt.Errorf("failed to parse session file %s: %v", r.filePath, err)
	}

	// Convert slice to map for efficient lookups
	r.threads = make(map[string]*dive.Thread)
	for _, thread := range persistedThreads {
		r.threads[thread.ID] = thread
	}

	r.dirty = false
	return nil
}

// Save writes threads from memory to the file
func (r *FileThreadRepository) Save(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.dirty {
		return nil // No changes to save
	}

	// Convert map to slice for serialization
	var threads []*dive.Thread
	for _, thread := range r.threads {
		threads = append(threads, thread)
	}

	data, err := json.MarshalIndent(threads, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal threads: %v", err)
	}

	if err := os.WriteFile(r.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file %s: %v", r.filePath, err)
	}

	return nil
}

// PutThread creates or updates a thread
func (r *FileThreadRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	thread.UpdatedAt = now

	r.threads[thread.ID] = thread
	r.dirty = true

	// Auto-save after each update
	r.mu.Unlock()
	err := r.Save(ctx)
	r.mu.Lock()

	return err
}

// GetThread retrieves a thread by ID
func (r *FileThreadRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, ok := r.threads[id]
	if !ok {
		return nil, dive.ErrThreadNotFound
	}
	return thread, nil
}

// DeleteThread deletes a thread by ID
func (r *FileThreadRepository) DeleteThread(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.threads[id]; !ok {
		return dive.ErrThreadNotFound
	}

	delete(r.threads, id)
	r.dirty = true

	// Auto-save after deletion
	r.mu.Unlock()
	err := r.Save(ctx)
	r.mu.Lock()

	return err
}

// ListThreads returns all threads (useful for session management)
func (r *FileThreadRepository) ListThreads(ctx context.Context) ([]*dive.Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var threads []*dive.Thread
	for _, thread := range r.threads {
		threads = append(threads, thread)
	}
	return threads, nil
}

// GetFilePath returns the file path used by this repository
func (r *FileThreadRepository) GetFilePath() string {
	return r.filePath
}