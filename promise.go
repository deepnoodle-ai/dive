package dive

import "context"

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
