package dive

import (
	"context"
	"fmt"
	"sync/atomic"
)

// errSoftCanceled ends a run whose context carries a soft cancel request. It
// wraps context.Canceled, so the turn is classified exactly as a hard
// cancellation.
var errSoftCanceled = fmt.Errorf("dive: soft cancel requested: %w", context.Canceled)

// softCancelKey is the context key of a soft cancel request.
type softCancelKey struct{}

// softCancel is one soft cancel request. parent is the request of an
// enclosing WithSoftCancel, so a request made on an outer context reaches a
// run started under an inner one.
type softCancel struct {
	requested atomic.Bool
	parent    *softCancel
}

// WithSoftCancel returns a copy of parent that carries a cancel request.
// Calling cancel asks an agent run using ctx to end at its next step
// boundary: before its next model call, and before each tool call it has not
// started. Tool calls already running finish; calls not started are answered
// with ToolCallNotRunText. The turn ends incomplete with TurnReasonCanceled
// and an error for which errors.Is(err, context.Canceled) holds, exactly as
// a hard cancellation does. A second, ordinary context cancellation still
// stops the run at once.
//
// The request travels with the context, so a subagent started by a tool sees
// it and stops at its own boundary. Calling cancel more than once has no
// further effect.
func WithSoftCancel(parent context.Context) (ctx context.Context, cancel func()) {
	sc := &softCancel{parent: softCancelFrom(parent)}
	return context.WithValue(parent, softCancelKey{}, sc), func() { sc.requested.Store(true) }
}

// SoftCanceled reports whether ctx carries a soft cancel request that has
// been made. A long-running tool may check it to wind down early.
func SoftCanceled(ctx context.Context) bool {
	for sc := softCancelFrom(ctx); sc != nil; sc = sc.parent {
		if sc.requested.Load() {
			return true
		}
	}
	return false
}

func softCancelFrom(ctx context.Context) *softCancel {
	sc, _ := ctx.Value(softCancelKey{}).(*softCancel)
	return sc
}

// stepStopErr returns the error that stops a run at a step boundary: the
// context's error, or errSoftCanceled when a soft cancel was requested.
func stepStopErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if SoftCanceled(ctx) {
		return errSoftCanceled
	}
	return nil
}
