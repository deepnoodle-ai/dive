package orchestration

import (
	"context"
	"sync"
)

// run is a single cancellable background run tracked by Runs.
type run struct {
	description string
	cancel      context.CancelFunc
}

// Runs tracks cancellable background runs — background subagent spawns and
// monitors — keyed by task_id, so the TaskStop tool can cancel them. It is the
// entire shared substrate for the control axis: construct one with NewRuns and
// hand it to the Agent, Monitor, and TaskStop tools. Safe for concurrent use.
type Runs struct {
	mu sync.Mutex
	m  map[string]run
}

// NewRuns creates an empty run tracker.
func NewRuns() *Runs {
	return &Runs{m: make(map[string]run)}
}

// add registers a cancellable run under id. Called by the tools that start
// background work (the Agent spawner and Monitor).
func (r *Runs) add(id, description string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[id] = run{description: description, cancel: cancel}
}

// remove drops a run that has finished on its own.
func (r *Runs) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}

// stop cancels the run with the given id and drops it, returning the run's
// description and whether a matching run was found.
func (r *Runs) stop(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.m[id]
	if !ok {
		return "", false
	}
	delete(r.m, id)
	h.cancel()
	return h.description, true
}
