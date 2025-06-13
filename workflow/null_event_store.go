package workflow

import (
	"context"
	"time"
)

var _ ExecutionEventStore = &NullEventStore{}

func NewNullEventStore() *NullEventStore {
	return &NullEventStore{}
}

type NullEventStore struct{}

func (s *NullEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	return nil
}

func (s *NullEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	return nil, nil
}

func (s *NullEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return nil, nil
}

func (s *NullEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	return nil
}

func (s *NullEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	return nil, nil
}

func (s *NullEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	return nil, nil
}

func (s *NullEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	return nil
}

func (s *NullEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	return nil
}
