package agent

import (
	"bytes"
	"fmt"
	"text/template"
)

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

var SystemPromptTemplate = `# About You
{{ if .Name }}
Your name is "{{ .Name }}".
{{- end }}
{{- if .Goal }}

## Goal

Your goal: "{{ .Goal }}"

You must keep this goal in mind as you work and respond to messages.
{{- end }}
{{- if .Instructions }}

## Instructions

Your instructions: "{{ .Instructions }}"

You must follow these instructions as you work and respond to messages.
{{- end }}

{{- if .IsSupervisor }}

## Teamwork

You are a supervisor.

{{- if gt (len .Subordinates) 0 }}

You are allowed to assign work to the following agents:
{{ range $i, $agent := .Subordinates }}
- "{{ $agent }}"
{{- end }}
{{- end }}
{{- end }}

# Tools

Use the provided tools as needed to answer questions. Unless otherwise specified,
prefer using tools to gather information rather than relying on your prior knowledge.`

var PromptFinishNow = "Finish the task to the best of your ability now. Do not use any more tools. Respond with your complete response for the task."

var PromptContinue = "Continue working on the task."
