package dive

type AgentTemplateData struct {
	Name      string
	Role      string
	Team      *Team
	IsManager bool
	IsWorker  bool
}

type agentTemplateData struct {
	*DiveAgent
	DelegateTargets []Agent
}

func NewAgentTemplateData(agent *DiveAgent) *agentTemplateData {
	var delegateTargets []Agent
	if agent.role.IsSupervisor {
		if agent.role.Subordinates == nil {
			if agent.team != nil {
				// Unspecified means we can delegate to all non-supervisors
				for _, a := range agent.team.Agents() {
					if !a.Role().IsSupervisor {
						delegateTargets = append(delegateTargets, a)
					}
				}
			}
		} else if agent.team != nil {
			for _, name := range agent.role.Subordinates {
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
