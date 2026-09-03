package main

import (
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/tui"
)

// effortLevels lists the known reasoning-effort values for /effort help and
// validation. Provider-specific values are still accepted as-is.
var effortLevels = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

// handleEffortCommand shows or switches the reasoning effort. The switch
// applies to the next turn and is persisted to ~/.dive/settings.json.
func (a *App) handleEffortCommand(args string) {
	arg := strings.ToLower(strings.TrimSpace(args))
	if arg == "" {
		current := strings.TrimSpace(string(a.reasoningEffort))
		if current == "" {
			current = "(unset — parameter omitted)"
		}
		a.appendNotice("Thinking effort: %s (from %s). Levels: %s. Usage: /effort <level>",
			current, a.effortSourceOrDefault(), strings.Join(effortLevels, ", "))
		return
	}
	// Accept an explicit empty (quoted) or "omit" to drop the parameter for
	// non-reasoning models; otherwise parse known levels case-insensitively
	// and pass anything else through as a provider-specific value.
	var effort llm.ReasoningEffort
	if arg == `""` || arg == "omit" || arg == "off" {
		effort = ""
	} else {
		effort = parseThinkingEffort(arg)
	}
	a.setEffort(effort, "session")
	if err := saveUserEffort(effort); err != nil {
		a.appendNotice("Effort set to %s for this session, but saving to settings failed: %v", effortDisplay(effort), err)
		return
	}
	a.appendNotice("Thinking effort set to %s (saved to ~/.dive/settings.json)", effortDisplay(effort))
}

// setEffort applies the effort to app state and the live model settings.
func (a *App) setEffort(effort llm.ReasoningEffort, source string) {
	a.reasoningEffort = effort
	a.effortSource = source
	if a.modelSettings != nil {
		a.modelSettings.ReasoningEffort = effort
	}
}

func effortDisplay(effort llm.ReasoningEffort) string {
	if strings.TrimSpace(string(effort)) == "" {
		return "unset (parameter omitted)"
	}
	return string(effort)
}

func (a *App) effortSourceOrDefault() string {
	if a.effortSource != "" {
		return a.effortSource
	}
	return "default"
}

// handleThinkingCommand shows or toggles summarized model thinking. No args
// toggles; on/off sets explicitly. Persisted to ~/.dive/settings.json.
func (a *App) handleThinkingCommand(args string) {
	arg := strings.ToLower(strings.TrimSpace(args))
	switch arg {
	case "":
		a.setThinking(!a.showThinking, "session")
	case "on", "true", "1", "show", "yes":
		a.setThinking(true, "session")
	case "off", "false", "0", "hide", "no":
		a.setThinking(false, "session")
	default:
		a.appendNotice("Usage: /thinking [on|off]")
		return
	}
	if err := saveUserThinking(a.showThinking); err != nil {
		a.appendNotice("Thinking %s for this session, but saving to settings failed: %v", thinkingStateWord(a.showThinking), err)
		return
	}
	a.appendNotice("Thinking display %s (saved to ~/.dive/settings.json)", thinkingStateWord(a.showThinking))
}

// setThinking applies the thinking display to app state and live model settings.
func (a *App) setThinking(show bool, source string) {
	a.showThinking = show
	a.thinkingSource = source
	if a.modelSettings != nil {
		if show {
			a.modelSettings.Thinking = llm.ThinkingTypeAdaptive
			a.modelSettings.ThinkingDisplay = llm.ThinkingDisplaySummarized
		} else {
			a.modelSettings.Thinking = ""
			a.modelSettings.ThinkingDisplay = ""
		}
	}
}

func thinkingStateWord(show bool) string {
	if show {
		return "on"
	}
	return "off"
}

// handleUsageCommand extends /usage and /cost: bare shows the full report;
// "full" enables the live breakdown panel (persisted); "brief" disables it.
func (a *App) handleUsageCommand(args string) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "full", "detail", "detailed", "on":
		a.setDetailedUsage(true, "session")
		if err := saveUserDetailedUsage(true); err != nil {
			a.appendNotice("Detailed usage shown for this session, but saving to settings failed: %v", err)
			return
		}
		a.appendNotice("Detailed usage will show under the status line (saved to ~/.dive/settings.json)")
	case "brief", "compact", "off":
		a.setDetailedUsage(false, "session")
		if err := saveUserDetailedUsage(false); err != nil {
			a.appendNotice("Detailed usage hidden for this session, but saving to settings failed: %v", err)
			return
		}
		a.appendNotice("Status line shows only the session total cost (saved to ~/.dive/settings.json)")
	case "":
		a.printUsageReport()
	default:
		a.appendNotice("Usage: /usage [full|brief]")
	}
}

func (a *App) setDetailedUsage(show bool, source string) {
	a.showDetailedUsage = show
	a.detailedUsageSource = source
}

// handleStatusCommand prints the effective configuration: model, effort,
// thinking, usage mode, session, and context — the one place all of it is
// visible together.
func (a *App) handleStatusCommand() {
	modelSource := a.modelSource
	if modelSource == "" {
		modelSource = "default"
	}
	views := []tui.View{
		tui.Text("Status").Bold(),
		tui.Text("  model:           %s (from %s)", a.modelDisplayName(), modelSource),
		tui.Text("  thinking effort: %s (from %s)", effortDisplay(a.reasoningEffort), a.effortSourceOrDefault()),
		tui.Text("  thinking display: %s (from %s)", thinkingStateWord(a.showThinking), settingSourceOrDefault(a.thinkingSource)),
		tui.Text("  detailed usage:   %s (from %s)", thinkingStateWord(a.showDetailedUsage), settingSourceOrDefault(a.detailedUsageSource)),
	}
	if a.currentSession != nil && a.currentSession.ID() != "" {
		views = append(views, tui.Text("  session:         %s", a.currentSession.ID()))
	}
	if a.lastUsage != nil {
		views = append(views, tui.Text("  context:         %d%%", a.contextPercent()))
	}
	if cost := a.sessionTotalCostString(); cost != "" {
		views = append(views, tui.Text("  session cost:    %s", cost))
	}
	a.appendReport(tui.Stack(views...))
}

func settingSourceOrDefault(source string) string {
	if source == "" {
		return "default"
	}
	return source
}

// persistModelSwitch saves the model default after an in-session switch.
func (a *App) persistModelSwitch(modelID string) {
	a.modelSource = "session"
	if err := saveUserModel(modelID); err != nil {
		a.appendNotice("Switched to %s for this session, but saving to settings failed: %v", a.modelDisplayName(), err)
		return
	}
	a.appendNotice("Switched to %s (saved to ~/.dive/settings.json)", a.modelDisplayName())
}
