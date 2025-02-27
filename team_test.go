package dive

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTeam(t *testing.T) {
	agent1 := NewAgent(AgentOptions{
		Role: Role{
			Description:  "AI researcher",
			IsSupervisor: true,
		},
	})
	agent2 := NewAgent(AgentOptions{
		Name: "Chris",
		Role: Role{Description: "Content writer"},
	})
	team, err := NewTeam(TeamOptions{
		Name:        "Research Team",
		Description: "Researching the history of Go",
		Agents:      []Agent{agent1, agent2},
	})
	require.NoError(t, err)
	require.NotNil(t, team)
	require.Equal(t, "Research Team", team.Name())
	require.Equal(t, "Researching the history of Go", team.Description())
	require.False(t, team.IsRunning())
	require.Len(t, team.Agents(), 2)
	require.Equal(t, "AI researcher", team.Agents()[0].Role().Description)
	require.Equal(t, "Content writer", team.Agents()[1].Role().Description)

	overview, err := team.Overview()
	require.NoError(t, err)

	expectedOverview := `The team is described as: "Researching the history of Go"

The team consists of the following agents:

- Name: AI researcher, Role: "AI researcher"
- Name: Chris, Role: "Content writer"`

	require.Equal(t, expectedOverview, overview)
}

func TestEmptyTeam(t *testing.T) {
	team, err := NewTeam(TeamOptions{})
	require.Error(t, err)
	require.Nil(t, team)
	require.Contains(t, err.Error(), "at least one agent is required")
}

func TestTeamWithoutSupervisors(t *testing.T) {
	team, err := NewTeam(TeamOptions{
		Agents: []Agent{
			NewAgent(AgentOptions{
				Role: Role{Description: "Content writer"},
			}),
			NewAgent(AgentOptions{
				Role: Role{Description: "Content writer"},
			}),
		},
	})
	require.Error(t, err)
	require.Nil(t, team)
	require.Contains(t, err.Error(), "at least one supervisor is required")
}

func TestTeamStartStop(t *testing.T) {
	ctx := context.Background()

	team, err := NewTeam(TeamOptions{
		Agents: []Agent{
			NewAgent(AgentOptions{Role: Role{Description: "Content writer"}}),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, team)
	require.False(t, team.IsRunning())

	err = team.Start(ctx)
	require.NoError(t, err)
	require.True(t, team.IsRunning())

	// Second start should fail
	err = team.Start(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "team already running")

	err = team.Stop(ctx)
	require.NoError(t, err)
	require.False(t, team.IsRunning())

	// Second stop should fail
	err = team.Stop(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "team not running")
}
