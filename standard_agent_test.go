package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStandardAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agent := NewStandardAgent(StandardAgentSpec{
		Name: "test",
		Role: &Role{
			Name: "test",
		},
	})

	err := agent.Start(ctx)
	require.NoError(t, err)

	err = agent.Event(ctx, &Event{Name: "test"})
	require.NoError(t, err)

	err = agent.Stop(ctx)
	require.NoError(t, err)
}
