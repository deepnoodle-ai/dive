package agent

import (
	"bytes"
	"fmt"
	"text/template"
)

func interpolateTemplate(name, text string, input any) (string, error) {
	tmpl, err := template.New("name").Parse(text)
	if err != nil {
		return "", fmt.Errorf("parsing %s template: %w", name, err)
	}
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, input); err != nil {
		return "", fmt.Errorf("executing %s template: %w", name, err)
	}
	return buffer.String(), nil
}

func ExecuteTemplate(tmpl *template.Template, input any) (string, error) {
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, input); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buffer.String(), nil
}
