package agent

import (
	"context"
	"encoding/json"
)

type DescribeTeamTool struct {
	team *Team
	self *Agent
}

func NewDescribeTeamTool(team *Team, self *Agent) *DescribeTeamTool {
	return &DescribeTeamTool{team: team, self: self}
}

func (t *DescribeTeamTool) Name() string {
	return "DescribeTeam"
}

func (t *DescribeTeamTool) Description() string {
	return "Returns a description of the team, including the roles of all team members."
}

func (t *DescribeTeamTool) Definition() *ToolDefinition {
	return &ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: Schema{
			Type:       "object",
			Required:   []string{},
			Properties: map[string]SchemaProperty{},
		},
	}
}

func (t *DescribeTeamTool) Invoke(ctx context.Context, input json.RawMessage) (string, error) {
	return interpolateTemplate("team", teamTemplate,
		map[string]interface{}{
			"Team": t.team,
			"Self": t.self,
		})
}
