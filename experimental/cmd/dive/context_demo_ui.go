package main

import (
	"github.com/deepnoodle-ai/wonton/tui"
)

type contextDemoNoticeEvent struct {
	baseEvent
	notice contextDemoNotice
}

func (a *App) notifyContextDemoNotice(notice contextDemoNotice) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.SendEvent(contextDemoNoticeEvent{baseEvent: newBaseEvent(), notice: notice})
}

func (a *App) resetContextDemoTrace() {
	a.contextDemoNotices = make(map[string]contextDemoNotice)
	a.contextDemoOrder = nil
}

func (a *App) handleContextDemoNotice(notice contextDemoNotice) {
	name := notice.Reminder.Name
	if a.contextDemoNotices == nil {
		a.contextDemoNotices = make(map[string]contextDemoNotice)
	}
	if _, exists := a.contextDemoNotices[name]; !exists {
		a.contextDemoOrder = append(a.contextDemoOrder, name)
	}
	a.contextDemoNotices[name] = notice

	a.appendMarkedNotice("◇ ", "%s %s · %s · %s",
		name, notice.Action, notice.Reminder.Tier, notice.Delivery)
}

func (a *App) printContextDemoReport() {
	if a.contextDemos.empty() {
		a.appendNotice("Context demos are off. Restart with --context-demo NAME or run 'dive context-demos' to list presets.")
		return
	}

	views := []tui.View{
		tui.Text("Context demo reminders").Bold(),
		tui.Text("  enabled: %s", a.contextDemos.displaySummary()),
		tui.Text("  shows --context-demo reminders only; skill and application reminders are not included").Style(hintStyle()),
		tui.Text("  model-only reminders are appended at the request tail and are not saved to conversation history").Style(hintStyle()),
	}
	if len(a.contextDemoOrder) == 0 {
		views = append(views,
			tui.Text(""),
			tui.Text("No context-demo reminder payloads were observed during the latest turn.").Style(hintStyle()),
			tui.Text("Send a message or use a tool, then run /context again.").Style(hintStyle()),
		)
		a.appendReport(tui.Stack(views...))
		return
	}

	views = append(views, tui.Text(""), tui.Text("Latest payloads").Bold())
	for _, name := range a.contextDemoOrder {
		notice := a.contextDemoNotices[name]
		views = append(views,
			tui.Text("  %s · %s · %s · %s", name, notice.Reminder.Tier, notice.Delivery, notice.Action).Bold(),
			tui.PaddingLTRB(4, 0, 0, 0, tui.Text("%s", notice.Reminder.Content).Wrap().Style(hintStyle())),
		)
	}
	views = append(views, tui.Text(""))
	a.appendReport(tui.Stack(views...).Gap(0))
}
