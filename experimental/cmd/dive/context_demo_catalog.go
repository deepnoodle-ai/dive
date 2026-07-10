package main

import (
	"fmt"
	"io"
	"strings"
)

type contextDemoID uint16

const (
	contextDemoWorkspace contextDemoID = 1 << iota
	contextDemoSources
	contextDemoVerification
	contextDemoRecovery
	contextDemoPipeline
	contextDemoGo
	contextDemoQuality
	contextDemoSecurity
)

type contextDemoDefinition struct {
	ID          contextDemoID
	Name        string
	Description string
}

var contextDemoCatalog = []contextDemoDefinition{
	{contextDemoWorkspace, "workspace", "Refresh the Git branch and dirty paths before every model call."},
	{contextDemoSources, "sources", "Track successful reads, searches, directory listings, and web fetches."},
	{contextDemoVerification, "verification", "Carry edit debt until a later direct test or analysis command passes."},
	{contextDemoRecovery, "recovery", "Coach a changed approach after a failed or denied tool call."},
	{contextDemoPipeline, "pipeline", "Map recognized build, task-runner, CI, container, and dependency surfaces."},
	{contextDemoGo, "go", "Surface Go module scope and an advisory format, build, test, vet, and race-check loop."},
	{contextDemoQuality, "quality", "Track observed build, test, analysis, and security command outcomes."},
	{contextDemoSecurity, "security", "Request review after sensitive edits or high-impact SDLC commands."},
}

type contextDemoSelection uint16

func (s contextDemoSelection) empty() bool { return s == 0 }

func (s contextDemoSelection) enabled(id contextDemoID) bool {
	return uint16(s)&uint16(id) != 0
}

func (s contextDemoSelection) names() []string {
	var names []string
	for _, demo := range contextDemoCatalog {
		if s.enabled(demo.ID) {
			names = append(names, demo.Name)
		}
	}
	return names
}

func (s contextDemoSelection) displaySummary() string {
	names := s.names()
	switch len(names) {
	case 0:
		return "off"
	case 1, 2, 3:
		return strings.Join(names, ", ")
	case len(contextDemoCatalog):
		return fmt.Sprintf("all %d demos", len(names))
	default:
		return fmt.Sprintf("%d demos", len(names))
	}
}

func allContextDemos() contextDemoSelection {
	var selection contextDemoSelection
	for _, demo := range contextDemoCatalog {
		selection |= contextDemoSelection(demo.ID)
	}
	return selection
}

// parseContextDemoNames accepts repeatable values and comma-separated groups so
// both --context-demo workspace --context-demo sources and
// --context-demo workspace,sources are convenient at the shell.
func parseContextDemoNames(specs []string) (contextDemoSelection, error) {
	var selection contextDemoSelection
	for _, spec := range specs {
		for _, rawName := range strings.Split(spec, ",") {
			name := strings.ToLower(strings.TrimSpace(rawName))
			if name == "" {
				return 0, fmt.Errorf("context demo name cannot be empty; run 'dive context-demos' to list presets")
			}
			if name == "all" {
				selection |= allContextDemos()
				continue
			}
			matched := false
			for _, demo := range contextDemoCatalog {
				if name == demo.Name {
					selection |= contextDemoSelection(demo.ID)
					matched = true
					break
				}
			}
			if !matched {
				return 0, fmt.Errorf("unknown context demo %q; run 'dive context-demos' to list presets", name)
			}
		}
	}
	return selection, nil
}

func writeContextDemoCatalog(w io.Writer) error {
	if _, err := fmt.Fprintln(w, "Runtime context demos"); err != nil {
		return err
	}
	for _, demo := range contextDemoCatalog {
		if _, err := fmt.Fprintf(w, "  %-14s %s\n", demo.Name, demo.Description); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, `
Usage:
  dive --context-demo all
  dive --context-demo pipeline,quality --context-demo security

In interactive mode, /context shows the exact context-demo reminder payloads
observed during the latest turn. Model-only reminders are not saved to
conversation history.
`)
	return err
}
