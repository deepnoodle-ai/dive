package session

import (
	"context"
	"errors"
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
