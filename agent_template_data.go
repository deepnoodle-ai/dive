package dive

// agentTemplateData is the data used to render the agent prompt template.
// It carries some information that isn't available via the Agent Go interface.
type agentTemplateData struct {
	*DiveAgent
	DelegateTargets []Agent
}

func newAgentTemplateData(agent *DiveAgent) *agentTemplateData {
	var delegateTargets []Agent
	if agent.isSupervisor {
		if agent.subordinates == nil {
			if agent.team != nil {
				// Unspecified means we can delegate to all non-supervisors
				for _, a := range agent.team.Agents() {
					if !a.(TeamAgent).IsSupervisor() {
						delegateTargets = append(delegateTargets, a)
					}
				}
			}
		} else if agent.team != nil {
			for _, name := range agent.subordinates {
				other, found := agent.team.GetAgent(name)
				if found {
					delegateTargets = append(delegateTargets, other)
				}
			}
		}
	}
	return &agentTemplateData{
		DiveAgent:       agent,
		DelegateTargets: delegateTargets,
	}
}
