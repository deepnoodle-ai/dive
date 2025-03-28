package dive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventStream_BasicFlow(t *testing.T) {
	assert := require.New(t)
	stream, pub := NewEventStream()
	defer stream.Close()

	testEvent := &Event{
		Type: "test",
		Origin: EventOrigin{
			AgentName: "test-agent",
		},
		Payload: "test-payload",
	}

	go func() {
		err := pub.Send(context.Background(), testEvent)
		assert.NoError(err)
		pub.Close()
	}()

	// Read and verify event
	assert.True(stream.Next(context.Background()))

	receivedEvent := stream.Event()
	assert.Equal(testEvent.Type, receivedEvent.Type)
	assert.Equal(testEvent.Origin.AgentName, receivedEvent.Origin.AgentName)
	assert.Equal(testEvent.Payload, receivedEvent.Payload)
}

func TestEventStream_ContextCancellation(t *testing.T) {
	assert := require.New(t)
	stream, _ := NewEventStream()
	defer stream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	assert.False(stream.Next(ctx))
	assert.ErrorIs(stream.Err(), context.Canceled)
}

func TestEventStream_WaitForEvent(t *testing.T) {
	assert := require.New(t)
	stream, pub := NewEventStream()
	defer stream.Close()

	expectedPayload := "test-payload"
	go func() {
		pub.Send(context.Background(), &Event{Payload: expectedPayload})
		pub.Close()
	}()

	result, err := WaitForEvent[string](context.Background(), stream)
	assert.NoError(err)
	assert.Equal(expectedPayload, result)
}

func TestEventStream_SendAfterClose(t *testing.T) {
	assert := require.New(t)
	stream, pub := NewEventStream()
	defer stream.Close()

	pub.Close()

	err := pub.Send(context.Background(), &Event{Type: "test"})
	assert.ErrorIs(err, ErrStreamClosed)
}

func TestEventStream_MultipleClose(t *testing.T) {
	assert := require.New(t)
	stream, pub := NewEventStream()

	// Multiple closes should not panic
	assert.NotPanics(func() {
		stream.Close()
		stream.Close()
		pub.Close()
	})
}

func TestEventStream_ErrorEvent(t *testing.T) {
	assert := require.New(t)
	stream, pub := NewEventStream()
	defer stream.Close()

	testErr := errors.New("test error")
	go func() {
		pub.Send(context.Background(), &Event{Error: testErr})
		pub.Close()
	}()

	_, err := WaitForEvent[string](context.Background(), stream)
	assert.Error(err)
	assert.Contains(err.Error(), "test error")
}

func TestEventStream_ContextTimeout(t *testing.T) {
	assert := require.New(t)
	stream, _ := NewEventStream()
	defer stream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := WaitForEvent[string](ctx, stream)
	assert.ErrorIs(err, context.DeadlineExceeded)
}
