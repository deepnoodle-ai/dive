package agent

type AgentTemplateProxy struct {
	Agent *Agent
}

func (a *AgentTemplateProxy) Name() string {
	return a.Agent.Name()
}

func (a *AgentTemplateProxy) Role() string {
	return a.Agent.Role()
}

func (a *AgentTemplateProxy) Backstory() string {
	return a.Agent.Backstory()
}

func (a *AgentTemplateProxy) Goal() string {
	return a.Agent.Goal()
}

func (a *AgentTemplateProxy) Team() *Team {
	return a.Agent.Team()
}

func (a *AgentTemplateProxy) CanDelegate() bool {
	return a.Agent.canDelegate
}

func (a *AgentTemplateProxy) AcceptsDelegation() bool {
	return a.Agent.acceptsDelegation
}

func (a *AgentTemplateProxy) DelegateTargets() []*Agent {
	if !a.Agent.canDelegate {
		return nil
	}
	var targets []*Agent
	if len(a.Agent.subordinates) > 0 {
		for _, name := range a.Agent.subordinates {
			agent, ok := a.Agent.Team().GetAgent(name)
			if ok && agent.acceptsDelegation && agent.Name() != a.Agent.Name() {
				targets = append(targets, agent)
			}
		}
	} else {
		for _, agent := range a.Agent.Team().Agents() {
			if agent.acceptsDelegation && agent.Name() != a.Agent.Name() {
				targets = append(targets, agent)
			}
		}
	}
	return targets
}
