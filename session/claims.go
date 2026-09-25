package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
)

// Session implements dive.SessionClaimer.
var _ dive.SessionClaimer = (*Session)(nil)

// sessionClaim is a claim on a session: its owner holds it until Expires.
type sessionClaim struct {
	Owner   string    `json:"owner"`
	Expires time.Time `json:"expires"`
}

// claimStore is implemented by stores that keep claims where every process
// using the store sees them, as FileStore does next to the session file. A
// session in memory, or in a MemoryStore, keeps its claim itself.
type claimStore interface {
	// claimSession claims the session for owner for ttl, reporting whether
	// owner held the claim already.
	claimSession(ctx context.Context, id, owner string, ttl time.Duration) (renewed bool, err error)

	// releaseSession ends owner's claim, if owner holds it.
	releaseSession(ctx context.Context, id, owner string) error

	// sessionOwner returns the owner of the session's claim, expired or
	// not, or "" when it has none.
	sessionOwner(ctx context.Context, id string) (string, error)
}

// claimedError is the error of a claim another owner holds.
func claimedError(id string, claim *sessionClaim) error {
	return fmt.Errorf("%w: session %s is held by %s until %s", dive.ErrSessionClaimed, id, claim.Owner, claim.Expires.Format(time.RFC3339Nano))
}

// ClaimSession claims the session for owner for ttl, or renews owner's claim,
// and fails with an error that wraps dive.ErrSessionClaimed while another
// owner's claim has not expired. It satisfies dive.SessionClaimer.
//
// A FileStore keeps the claim in a file next to the session's, so that the
// processes sharing the directory see it, and a newly claimed session reads
// back what its file holds: the writes another process made since this one
// opened it. Once claimed, the session refuses its writes with
// dive.ErrSessionClaimed if the claim is no longer this owner's, as when it
// expired and another owner claimed the session; every process that writes
// to the session must claim it for that to hold. A session in memory or in a
// MemoryStore keeps its claim itself, which excludes other owners in this
// process.
func (s *Session) ClaimSession(ctx context.Context, owner string, ttl time.Duration) error {
	if owner == "" {
		return errors.New("session: a claim needs an owner")
	}
	if ttl <= 0 {
		return errors.New("session: a claim needs a positive time to live")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, ok := s.appender.(claimStore)
	if !ok {
		now := time.Now()
		if s.claim != nil && s.claim.Owner != owner && now.Before(s.claim.Expires) {
			return claimedError(s.data.ID, s.claim)
		}
		s.claim = &sessionClaim{Owner: owner, Expires: now.Add(ttl)}
		s.claimOwner = owner
		return nil
	}
	renewed, err := cs.claimSession(ctx, s.data.ID, owner, ttl)
	if err != nil {
		return err
	}
	if !renewed {
		// Another process may have written since this one last read the
		// session: take what the store holds.
		if r, ok := s.appender.(storeReloader); ok {
			stored, err := r.reloadSession(ctx, s.data.ID)
			if err != nil {
				_ = cs.releaseSession(ctx, s.data.ID, owner)
				return fmt.Errorf("session: reload claimed session: %w", err)
			}
			s.data.Events = stored.Events
			s.data.Suspended = stored.Suspended
			s.data.PendingToolCalls = stored.PendingToolCalls
			s.data.CompletedToolCalls = stored.CompletedToolCalls
			s.data.BatchHalted = stored.BatchHalted
			s.data.UpdatedAt = stored.UpdatedAt
			s.data.Revision = stored.Revision
		}
	}
	s.claimOwner = owner
	return nil
}

// ReleaseSession ends owner's claim on the session. Releasing a claim owner
// does not hold does nothing. It satisfies dive.SessionClaimer.
func (s *Session) ReleaseSession(ctx context.Context, owner string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimOwner == owner {
		s.claimOwner = ""
	}
	cs, ok := s.appender.(claimStore)
	if !ok {
		if s.claim != nil && s.claim.Owner == owner {
			s.claim = nil
		}
		return nil
	}
	return cs.releaseSession(ctx, s.data.ID, owner)
}

// checkClaimLocked refuses a write through a session this instance claimed
// when its store no longer records the claim as this instance's owner's.
// Caller must hold s.mu.
func (s *Session) checkClaimLocked(ctx context.Context) error {
	if s.claimOwner == "" {
		return nil
	}
	cs, ok := s.appender.(claimStore)
	if !ok {
		return nil
	}
	owner, err := cs.sessionOwner(ctx, s.data.ID)
	if err != nil {
		return err
	}
	if owner != s.claimOwner {
		return fmt.Errorf("%w: session %s is no longer claimed by %s", dive.ErrSessionClaimed, s.data.ID, s.claimOwner)
	}
	return nil
}

// claimPath returns the path of the session's claim file.
func (s *FileStore) claimPath(id string) (string, error) {
	p, err := s.path(id)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(p, ".jsonl") + ".claim", nil
}

// lockClaim takes the lock that serializes changes to a claim file across
// processes: an exclusive operating-system lock (flock on Unix, LockFileEx on
// Windows) on {id}.claim.lock, which the system releases when the process
// holding it exits, so a lock is never left behind. It waits until the lock
// is free or ctx ends, and returns the function that releases it.
func lockClaim(ctx context.Context, claimPath string) (func(), error) {
	f, err := os.OpenFile(claimPath+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	for {
		locked, err := tryLockFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if locked {
			return func() {
				_ = unlockFile(f)
				f.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// readClaim returns the claim in a claim file, or nil when there is none.
func readClaim(claimPath string) (*sessionClaim, error) {
	data, err := os.ReadFile(claimPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var claim sessionClaim
	if err := json.Unmarshal(data, &claim); err != nil {
		return nil, fmt.Errorf("session claim %s: %w", claimPath, err)
	}
	return &claim, nil
}

// claimSession implements claimStore for FileStore.
func (s *FileStore) claimSession(ctx context.Context, id, owner string, ttl time.Duration) (bool, error) {
	claimPath, err := s.claimPath(id)
	if err != nil {
		return false, err
	}
	unlock, err := lockClaim(ctx, claimPath)
	if err != nil {
		return false, err
	}
	defer unlock()
	current, err := readClaim(claimPath)
	if err != nil {
		return false, err
	}
	now := time.Now()
	if current != nil && current.Owner != owner && now.Before(current.Expires) {
		return false, claimedError(id, current)
	}
	data, err := json.Marshal(&sessionClaim{Owner: owner, Expires: now.Add(ttl)})
	if err != nil {
		return false, err
	}
	if err := writeFileAtomic(claimPath, data); err != nil {
		return false, err
	}
	return current != nil && current.Owner == owner, nil
}

// releaseSession implements claimStore for FileStore.
func (s *FileStore) releaseSession(ctx context.Context, id, owner string) error {
	claimPath, err := s.claimPath(id)
	if err != nil {
		return err
	}
	unlock, err := lockClaim(ctx, claimPath)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := readClaim(claimPath)
	if err != nil || current == nil || current.Owner != owner {
		return err
	}
	if err := os.Remove(claimPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// sessionOwner implements claimStore for FileStore.
func (s *FileStore) sessionOwner(ctx context.Context, id string) (string, error) {
	claimPath, err := s.claimPath(id)
	if err != nil {
		return "", err
	}
	claim, err := readClaim(claimPath)
	if err != nil || claim == nil {
		return "", err
	}
	return claim.Owner, nil
}

// writeFileAtomic replaces path with data through a temporary file and a
// rename, so a reader sees the old file or the new one.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
