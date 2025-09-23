package dive

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

var defaultSystemPrompt = `
{{ if .Name }}
Your name is "{{ .Name }}".
{{- end }}
{{- if .Goal }}

Your goal: "{{ .Goal }}"

Keep this goal in mind as you work and respond to messages.
{{- end }}
{{- if .Instructions }}

Your instructions: "{{ .Instructions }}"

Follow these instructions as you work and respond to messages.
{{- end }}

{{- if .IsSupervisor }}

You are a supervisor of other agents.

{{- if gt (len .Subordinates) 0 }}

You are allowed to delegate to the following agents using the "assign_work" tool:
{{ range $i, $agent := .Subordinates }}
- "{{ $agent }}"
{{- end }}
{{- end }}
{{- end }}

{{ if .HasTools -}}
Use the provided tools as needed to answer questions.
{{- end }}`

// SetDefaultSystemPrompt sets the default system prompt template used by agents.
func SetDefaultSystemPrompt(template string) {
	defaultSystemPrompt = template
}
