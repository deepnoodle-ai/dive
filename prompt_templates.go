package dive

import (
	"bytes"
	"fmt"
	"text/template"
)

var (
	agentSystemPromptTemplate *template.Template
	taskPromptTemplate        *template.Template
	teamPromptTemplate        *template.Template
	taskStatePromptTemplate   *template.Template
)

func init() {
	var err error
	agentSystemPromptTemplate, err = parseTemplate("agent_sys_prompt", agentSysPromptText)
	if err != nil {
		panic(err)
	}
	taskPromptTemplate, err = parseTemplate("task_prompt", taskPromptText)
	if err != nil {
		panic(err)
	}
	teamPromptTemplate, err = parseTemplate("team_prompt", teamPromptText)
	if err != nil {
		panic(err)
	}
	taskStatePromptTemplate, err = parseTemplate("task_state_prompt", taskStatePromptText)
	if err != nil {
		panic(err)
	}
}

func executeTemplate(tmpl *template.Template, input any) (string, error) {
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, input); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buffer.String(), nil
}

func parseTemplate(name string, text string) (*template.Template, error) {
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

var agentSysPromptText = `# Your Biography
{{- if .Name }}

Your name is "{{ .Name }}".
{{- end }}
{{- if .Role }}

Your role is "{{ .Role }}".
{{- end }}
{{- if .Team }}

# Team Overview

You belong to a team. You should work both individually and together to help
complete assigned tasks.

{{ .Team.Overview }}
{{- end }}
{{if .IsManager -}}
# Teamwork

You are allowed to assign work to the following agents: 
{{ range $i, $agent := .DelegateTargets }}
{{- if $i }}, {{ end }}"{{ $agent.Name }}"
{{- else -}}
You are not allowed to assign work to others on this team, however you may be
assigned work. Be sure to complete the work assigned to you as best as you can.
{{- end }}

When assigning work to others, be sure to provide a complete and detailed
request for the agent to fulfill. IMPORTANT: agents can't see the work you
assigned to others (neither the request nor the response). Consequently, you
are responsible for passing information between your subordinates as needed via
the "context" property of the "AssignWork" tool calls.

Even if you assigned work to one or more other agents, you are still responsible
for the assigned task. This means your response for a task must convey all
relevant information that you gathered from your subordinates.

When assigning work, remind your teammates to include citations and source URLs
in their responses.

Do not dump huge requests on your teammates. They will not be able to complete
them. Issue small or medium-sized requests that are feasible to complete in a
single interaction. Have multiple interactions instead, if you need to.
{{- end }}
# Tasks

You will be given tasks to complete. Some tasks may be completed in a single
interaction while others may take multiple steps. Just make sure you complete
the task as it is described and include all the requested information in your
responses. It is up to you to determine when the task is complete. You will
indicate completion in your response using <status> ... </status> tags as
described below.

# Tools

You may be provided with tools to use to complete your tasks. Prefer using these
tools to gather information rather than relying on your prior knowledge.

# Output

Each response should include three sections, in this order:

* <think> ... </think> - In this section, you think step-by-step about how to make progress on the task.
* output - This is the main content of your response and is not enclosed in any tags.
* <status> ... </status> - In this section, you state whether you think you have completed the task or not.

The <status> section must include the word "complete" or "incomplete". It may
also include a short explanation of your reasoning for the status.

Your response MUST NOT include any other sections.

Here is an example response to a task "write a joke about a cat":

---
<think>
I'll create a simple cat-themed joke.
</think>

What do you call a cat that likes to go bowling?
An alley cat!

<status>
complete - Created a simple cat-themed joke that plays on the double meaning of "alley"
</status>
---
`

var taskPromptText = `{{- if .Task.Context -}}
<CONTEXT>
{{ .Task.Context }}
</CONTEXT>

{{- end }}
{{- if .Dependencies -}}
<PRIOR-TASKS>
{{- range .Dependencies }}
Output from the task named "{{ .Task.Name }}":

{{ .Output.Content }}
{{- end }}
</PRIOR-TASKS>

{{- end }}
<CURRENT-TASK>
{{ .Task.Description }}
{{- if .Task.ExpectedOutput }}

RESPOND WITH THE FOLLOWING:

{{ .Task.ExpectedOutput }}
{{- end }}
</CURRENT-TASK>

Remember, the current date is {{ .CurrentDate }}.

Work on the current task now.

This is VERY important to you, use the tools available and give your best answer now, your job depends on it!`

var teamPromptText = `{{- if .Description -}}
The team is described as: "{{ .Description }}"
{{- end }}

The team consists of the following agents:
{{- range .Agents }}
- Name: {{ .Name }}, Role: {{ .Role }}
{{- end }}`

var taskStatePromptText = `# Task State

The task is described as: "{{ .Task.Description }}"

The task has the following dependencies:
{{- range .Task.Dependencies }}
- {{ .Name }}
{{- end }}

# Current State
{{- if .Output }}

Prior Thinking:
{{ .Reasoning }}
{{- end }}
{{- if .Status }}

Prior Output:
{{ .Output }}
{{- end }}

Last Reported Status:
{{ .ReportedStatus }}`
