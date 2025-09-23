package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecoverableError(t *testing.T) {
	err := NewRecoverableError(errors.New("test error"))
	assert.True(t, IsRecoverable(err))
	assert.False(t, IsRecoverable(errors.New("test error")))
	assert.False(t, IsRecoverable(nil))
}

func TestRetry(t *testing.T) {
	ctx := context.Background()
	count := 0
	err := Do(ctx, func() error {
		count++
		return NewRecoverableError(errors.New("test error"))
	}, WithMaxRetries(3), WithBaseWait(time.Millisecond*20))
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())
	assert.Equal(t, 4, count)
}

func TestRetryZeroMaxRetries(t *testing.T) {
	ctx := context.Background()
	count := 0
	err := Do(ctx, func() error {
		count++
		return NewRecoverableError(errors.New("test error"))
	}, WithMaxRetries(0), WithBaseWait(time.Millisecond*20))
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())
	assert.Equal(t, 1, count) // Should still try once even with 0 retries
}
