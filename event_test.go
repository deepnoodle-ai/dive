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
	stream, pub := NewEventStream()
	defer stream.Close()

	expectedPayload := "test-payload"
	go func() {
		pub.Send(context.Background(), &Event{Payload: expectedPayload})
		pub.Close()
	}()

	results, err := ReadEventPayloads[string](context.Background(), stream)
	require.NoError(t, err)
	require.Equal(t, expectedPayload, results[0])
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
	stream, pub := NewEventStream()
	defer stream.Close()

	testErr := errors.New("test error")
	go func() {
		pub.Send(context.Background(), &Event{Error: testErr})
		pub.Close()
	}()

	results, err := ReadEventPayloads[string](context.Background(), stream)
	require.Error(t, err)
	require.Nil(t, results)
}

func TestEventStream_ContextTimeout(t *testing.T) {
	stream, _ := NewEventStream()
	defer stream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	results, err := ReadEventPayloads[string](ctx, stream)
	require.Error(t, err)
	require.Nil(t, results)
}
