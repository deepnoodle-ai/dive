package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/deepnoodle-ai/wonton/cli"
)

func runModels(ctx *cli.Context) error {
	available := ctx.Bool("available")

	var providers []providerInfo
	for _, p := range providerCatalog {
		if available && !p.Available() {
			continue
		}
		providers = append(providers, p)
	}
	writeModelsTable(os.Stdout, providers, providerInfo.Available)
	return nil
}

// writeModelsTable prints each provider's models under a status header. Column
// widths come from every row printed, so the columns line up across sections.
func writeModelsTable(w io.Writer, providers []providerInfo, available func(providerInfo) bool) {
	var idWidth, labelWidth, ctxWidth int
	for _, p := range providers {
		for _, m := range p.Models {
			idWidth = max(idWidth, utf8.RuneCountInString(m.ModelID))
			labelWidth = max(labelWidth, utf8.RuneCountInString(m.Label))
			ctxWidth = max(ctxWidth, len(formatContextWindow(contextWindowForModel(m.ModelID))))
		}
	}

	for _, p := range providers {
		if available(p) {
			fmt.Fprintf(w, "✓ %s\n", p.Name)
		} else {
			fmt.Fprintf(w, "✗ %s  (set %s)\n", p.Name, strings.Join(p.EnvVars, " or "))
		}
		for _, m := range p.Models {
			ctxStr := formatContextWindow(contextWindowForModel(m.ModelID))
			line := fmt.Sprintf("    %-*s   %-*s   %*s   %s",
				idWidth, m.ModelID, labelWidth, m.Label, ctxWidth, ctxStr, m.Description)
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
		fmt.Fprintln(w)
	}
}

// formatContextWindow formats a context window size for display (e.g. 1000000 -> "1M").
func formatContextWindow(tokens int) string {
	if tokens >= 1_000_000 {
		m := float64(tokens) / 1_000_000
		if m == float64(int(m)) {
			return fmt.Sprintf("%dM", int(m))
		}
		return fmt.Sprintf("%.1fM", m)
	}
	if tokens >= 1_000 {
		k := float64(tokens) / 1_000
		if k == float64(int(k)) {
			return fmt.Sprintf("%dk", int(k))
		}
		return fmt.Sprintf("%.1fk", k)
	}
	if tokens == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", tokens)
}
