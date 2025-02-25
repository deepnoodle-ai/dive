package dive

import (
	"context"
	"fmt"
)

type Promise struct {
	task *Task
	ch   chan *TaskResult
}

func (p *Promise) Get(ctx context.Context) (*TaskResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-p.ch:
		if res.Error != nil {
			return nil, res.Error
		}
		return res, nil
	}
}

func (p *Promise) Set(res *TaskResult) {
	p.ch <- res
}

func (p *Promise) Task() *Task {
	return p.task
}

func WaitAll(ctx context.Context, promises []*Promise) ([]*TaskResult, error) {
	results := make([]*TaskResult, len(promises))
	var firstError error

	// Wait for all promises to complete
	for i, promise := range promises {
		result, err := promise.Get(ctx)
		if err != nil {
			if firstError == nil {
				firstError = err
			}
			continue
		}
		results[i] = result
	}

	if firstError != nil {
		return results, fmt.Errorf("one or more tasks failed: %w", firstError)
	}

	return results, nil
}
