package agent

import (
	"context"

	"github.com/deepnoodle-ai/dive"
)

type callbackPublisher struct {
	callback dive.EventCallback
}

func (p *callbackPublisher) Send(ctx context.Context, event *dive.ResponseEvent) error {
	return p.callback(ctx, event)
}

func (p *callbackPublisher) Close() {}

type nullEventPublisher struct{}

func (p *nullEventPublisher) Send(ctx context.Context, event *dive.ResponseEvent) error {
	return nil
}

func (p *nullEventPublisher) Close() {}
