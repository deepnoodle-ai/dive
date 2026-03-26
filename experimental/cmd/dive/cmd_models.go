package main

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/wonton/cli"
)

func runModels(ctx *cli.Context) error {
	available := ctx.Bool("available")

	for _, p := range providerCatalog {
		isAvail := p.Available()
		if available && !isAvail {
			continue
		}

		// Provider header
		status := "✓"
		if !isAvail {
			status = "✗"
			envHint := strings.Join(p.EnvVars, " or ")
			fmt.Printf("%s %s  (set %s)\n", status, p.Name, envHint)
		} else {
			fmt.Printf("%s %s\n", status, p.Name)
		}

		// Model rows
		for _, m := range p.Models {
			ctx := contextWindowForModel(m.ModelID)
			ctxStr := formatContextWindow(ctx)
			fmt.Printf("    %-30s %-18s %6s    %s\n", m.ModelID, m.Label, ctxStr, m.Description)
		}
		fmt.Println()
	}

	return nil
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
