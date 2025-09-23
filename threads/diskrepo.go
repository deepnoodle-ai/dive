package threads

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

var _ dive.ThreadRepository = &DiskRepository{}

// DiskRepository is a disk-based implementation of ThreadRepository
// that persists each thread to a separate file in the given directory.
// It reads directly from disk files without maintaining an in-memory cache.
type DiskRepository struct {
	mu        sync.RWMutex
	directory string
}

// NewDiskRepository creates a new DiskRepository
func NewDiskRepository(directory string) *DiskRepository {
	return &DiskRepository{
		directory: directory,
	}
}

// ensureDirectory creates the directory if it doesn't exist
func (r *DiskRepository) ensureDirectory() error {
	return os.MkdirAll(r.directory, 0755)
}

// getThreadFilePath returns the file path for a specific thread
func (r *DiskRepository) getThreadFilePath(threadID string) string {
	return filepath.Join(r.directory, fmt.Sprintf("thread-%s.json", threadID))
}

// saveThread saves a single thread to its own file
func (r *DiskRepository) saveThread(thread *dive.Thread) error {
	data, err := json.MarshalIndent(thread, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal thread: %v", err)
	}

	filePath := r.getThreadFilePath(thread.ID)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write thread file %s: %v", filePath, err)
	}

	return nil
}

// PutThread creates or updates a thread
func (r *DiskRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure directory exists
	if err := r.ensureDirectory(); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", r.directory, err)
	}

	now := time.Now()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	thread.UpdatedAt = now

	// Save thread to its own file
	return r.saveThread(thread)
}

// GetThread retrieves a thread by ID by reading from disk
func (r *DiskRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filePath := r.getThreadFilePath(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, dive.ErrThreadNotFound
		}
		return nil, fmt.Errorf("failed to read thread file %s: %v", filePath, err)
	}

	if len(data) == 0 {
		return nil, dive.ErrThreadNotFound
	}

	var thread dive.Thread
	if err := json.Unmarshal(data, &thread); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread from %s: %v", filePath, err)
	}

	return &thread, nil
}

// DeleteThread deletes a thread by ID
func (r *DiskRepository) DeleteThread(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filePath := r.getThreadFilePath(id)

	// Check if file exists first
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return dive.ErrThreadNotFound
	}

	// Remove the thread file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove thread file %s: %v", filePath, err)
	}

	return nil
}

// ListThreads returns all threads by scanning the directory for thread files
func (r *DiskRepository) ListThreads(ctx context.Context, input *dive.ListThreadsInput) (*dive.ListThreadsOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find all thread files in the directory
	pattern := filepath.Join(r.directory, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find thread files in %s: %v", r.directory, err)
	}
	var threads []*dive.Thread
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read thread file %s: %v", file, err)
		}
		if len(data) == 0 {
			continue
		}
		var thread dive.Thread
		if err := json.Unmarshal(data, &thread); err != nil {
			continue
		}
		threads = append(threads, &thread)
	}
	return &dive.ListThreadsOutput{Items: threads}, nil
}

// GetDirectory returns the directory path used by this repository
func (r *DiskRepository) GetDirectory() string {
	return r.directory
}
