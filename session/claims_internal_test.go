package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

// The claim lock excludes a second holder, even through another open file
// in the same process, until it is released.
func TestLockClaimExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.claim")
	unlock, err := lockClaim(context.Background(), path)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = lockClaim(ctx, path)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))

	unlock()
	unlock2, err := lockClaim(context.Background(), path)
	assert.NoError(t, err)
	unlock2()
}

// Delete waits for a claim change in progress, and leaves the lock file, so
// that claims stay serialized through the same locked file.
func TestDeleteSerializesWithClaims(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	assert.NoError(t, err)
	_, err = store.Open(ctx, "deleted")
	assert.NoError(t, err)
	other, err := NewFileStore(dir)
	assert.NoError(t, err)
	claimPath, err := other.claimPath("deleted")
	assert.NoError(t, err)

	unlock, err := lockClaim(ctx, claimPath)
	assert.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- store.Delete(ctx, "deleted") }()
	select {
	case err := <-done:
		t.Fatalf("Delete returned while a claim change was in progress: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	unlock()
	assert.NoError(t, <-done)

	_, err = os.Stat(filepath.Join(dir, "deleted.jsonl"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(claimPath + ".lock")
	assert.NoError(t, err, "the lock file stays")

	held, err := lockClaim(ctx, claimPath)
	assert.NoError(t, err)
	defer held()
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = lockClaim(short, claimPath)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}
