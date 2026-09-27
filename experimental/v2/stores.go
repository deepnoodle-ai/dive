package v2

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
)

type Store interface {
	Recorder
	Load(context.Context, string) ([]Step, error)
}

func reject(err error) error { return &CommitError{Disposition: Rejected, Cause: err} }

// appendStep checks both idempotent retries and valid, contiguous transitions.
func appendStep(records []Step, step Step) ([]Step, bool, error) {
	if step.Sequence > 0 && step.Sequence <= len(records) {
		if reflect.DeepEqual(records[step.Sequence-1], step) {
			return records, false, nil
		}
		return nil, false, reject(errors.New("step identity reused with different content"))
	}
	next := append(copyValue(records), copyValue(step))
	if _, err := replay(next); err != nil {
		return nil, false, reject(err)
	}
	return next, true, nil
}

// FileJournal uses atomically replaced snapshots so the spike can test reopening
// without building a second append-log recovery implementation. It is O(n) per
// commit and supports one writer/runner only, with no cross-process fencing.
// Production should adapt the existing session store instead.
type FileJournal struct {
	mu  sync.Mutex
	dir string
}

func NewFileJournal(dir string) (*FileJournal, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &FileJournal{dir: dir}, nil
}

func (f *FileJournal) path(id string) string {
	return filepath.Join(f.dir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(id))))
}

func (f *FileJournal) load(id string) ([]Step, error) {
	data, err := os.ReadFile(f.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Step
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 || records[0].TurnID != id {
		return nil, errors.New("snapshot has no matching turn")
	}
	_, err = replay(records)
	return records, err
}

func (f *FileJournal) Load(ctx context.Context, id string) ([]Step, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.load(id)
}

func (f *FileJournal) Commit(ctx context.Context, step Step) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return reject(err)
	}
	old, err := f.load(step.TurnID)
	if err != nil {
		return reject(err)
	}
	next, changed, err := appendStep(old, step)
	if err != nil || !changed {
		return err
	}
	data, err := json.Marshal(next)
	if err != nil {
		return reject(err)
	}
	tmp, err := os.CreateTemp(f.dir, ".step-*")
	if err != nil {
		return reject(err)
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return reject(err)
	}
	if err := os.Rename(tmp.Name(), f.path(step.TurnID)); err != nil {
		return reject(err)
	}
	// After replacement, failures are ambiguous. A reload may see the new state.
	dir, err := os.Open(f.dir)
	if err != nil {
		return &CommitError{Disposition: Unknown, Cause: err}
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return &CommitError{Disposition: Unknown, Cause: err}
	}
	return nil
}

// SessionRunner is deliberately turn-scoped. Multi-turn history projection,
// claims, compaction, and forks belong in the later real session adapter.
type SessionRunner struct {
	mu     sync.Mutex
	Store  Store
	Engine Engine
}

func (r *SessionRunner) Run(ctx context.Context, turnID string, command Command) (Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if command.Start != nil && command.Start.TurnID != turnID {
		return Outcome{}, errors.New("start turn differs from loaded turn")
	}
	records, err := r.Store.Load(ctx, turnID)
	if err != nil {
		return Outcome{}, err
	}
	return r.Engine.Execute(ctx, records, command, r.Store)
}

// TransactionalHost simulates an application database transaction. It proves
// the shape of the recording seam, not SQL isolation, real billing, or leases.
// Only one engine may own a turn; the mutex serializes commits, not execution.
type TransactionalHost struct {
	mu      sync.Mutex
	records map[string][]Step
	inbox   map[string]string
	usage   Usage
	unknown int
	fail    bool
	epochs  map[string]uint64
}

func NewTransactionalHost() *TransactionalHost {
	return &TransactionalHost{records: make(map[string][]Step), inbox: make(map[string]string), epochs: make(map[string]uint64)}
}

func (h *TransactionalHost) Queue(turnID, input string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inbox[turnID] = input
}

func (h *TransactionalHost) FailNextCommit() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fail = true
}

func (h *TransactionalHost) Snapshot() (Usage, int, map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.usage, h.unknown, copyValue(h.inbox)
}

func (h *TransactionalHost) Load(ctx context.Context, id string) ([]Step, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return copyValue(h.records[id]), nil
}

func (h *TransactionalHost) Commit(ctx context.Context, step Step) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epochs[step.TurnID] != 0 {
		return reject(errors.New("claimed turn requires its fenced recorder"))
	}
	return h.commit(ctx, step)
}

// commit requires h.mu, keeping the fencing check and state mutation atomic.
func (h *TransactionalHost) commit(ctx context.Context, step Step) error {
	if err := ctx.Err(); err != nil {
		return reject(err)
	}
	next, changed, err := appendStep(h.records[step.TurnID], step)
	if err != nil || !changed {
		return err
	}
	if step.Kind == Started {
		input, exists := h.inbox[step.TurnID]
		if !exists || input != step.Start.Input {
			return reject(errors.New("accepted input differs from queued input"))
		}
	}
	if h.fail {
		h.fail = false
		return reject(errors.New("simulated transaction rollback"))
	}
	// All validation completes before any application state is changed.
	h.records[step.TurnID] = next
	if step.Kind == Started {
		delete(h.inbox, step.TurnID)
	}
	if step.Kind == ModelFinished {
		if step.Model.Usage == nil {
			h.unknown++
		} else {
			h.usage.Input += step.Model.Usage.Input
			h.usage.Output += step.Model.Usage.Output
		}
	}
	return nil
}

// OwnerStore simulates a recorder bound to an application-owned fencing token.
// Tokens fence writes; the downstream effect service must also enforce them.
// One invocation at a time may use each owner. There is no lease timer here.
type OwnerStore struct {
	host   *TransactionalHost
	turnID string
	epoch  uint64
}

// Takeover is a trusted coordinator operation, not a worker self-service API.
// Production needs a transactional lease acquisition/expiry policy.
func (h *TransactionalHost) Takeover(turnID string) *OwnerStore {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.epochs[turnID]++
	return &OwnerStore{host: h, turnID: turnID, epoch: h.epochs[turnID]}
}

func (o *OwnerStore) Epoch() uint64 { return o.epoch }

func (o *OwnerStore) check(turnID string) error {
	if turnID != o.turnID || o.host.epochs[turnID] != o.epoch {
		return reject(errors.New("stale owner or wrong turn"))
	}
	return nil
}

func (o *OwnerStore) Commit(ctx context.Context, step Step) error {
	o.host.mu.Lock()
	defer o.host.mu.Unlock()
	if err := o.check(step.TurnID); err != nil {
		return err
	}
	return o.host.commit(ctx, step)
}

func (o *OwnerStore) Load(ctx context.Context, turnID string) ([]Step, error) {
	o.host.mu.Lock()
	defer o.host.mu.Unlock()
	if err := o.check(turnID); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return copyValue(o.host.records[turnID]), nil
}
