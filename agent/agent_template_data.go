package agent

import "github.com/deepnoodle-ai/dive"

// agentTemplateData is the data used to render the agent prompt template.
// It carries some information that isn't available via the Agent struct.
type agentTemplateData struct {
	*Agent
	DelegateTargets    []dive.Agent
	ResponseGuidelines string
}

func newAgentTemplateData(agent *Agent, responseGuidelines string) *agentTemplateData {
	// With Environment removed, agents can only delegate to explicitly configured subordinates
	// In the future, this could be enhanced by passing team info directly to agents
	var delegateTargets []dive.Agent
	// Since we no longer have access to other agents through Environment,
	// delegation is simplified to only work with explicitly configured subordinates
	// This could be enhanced in the future by providing a registry or team context
	return &agentTemplateData{
		Agent:              agent,
		DelegateTargets:    delegateTargets,
		ResponseGuidelines: responseGuidelines,
	}
}
