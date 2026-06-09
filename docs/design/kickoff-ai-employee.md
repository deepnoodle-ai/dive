# Kickoff Prompt — Your AI Employee Texts You 📱

> Paste this into a fresh session to begin building. Full design: `docs/design/plan-ai-employee.md`.
> Overall series context: `docs/design/demo-ideas.md`.

---

You are building **"Your AI Employee Texts You"**: a demo where a Dive agent works
autonomously, hits a real decision (a money/irreversible action), **suspends mid-turn and
pings a phone** for approval, then resumes — even across a full process restart — once the
human taps approve. This is a marketing/demo project for **Dive**, a Go library for building
AI agents. The point is to reframe Dive's rare **suspend/resume** capability as a product
magic moment.

The shareable moment we're aiming at: *"I asked my AI to plan and book a team offsite, then
went to lunch. It hit a $1,200 deposit and texted me to approve. I tapped 'yes' from the
restaurant. When I got back, it was done."* — recorded as a split-screen (agent log | real phone).

## Context about Dive

- Dive lives at `/Users/curtis/git/deepnoodle/dive` (module `github.com/deepnoodle-ai/dive`). Go 1.25.
- Core API: `dive.NewAgent(dive.AgentOptions{...})` → `*Agent`. `agent.CreateResponse(ctx, dive.WithInput(...))`.
- **Suspend/resume is the core mechanic.** A tool pauses the agent mid-turn by returning
  `dive.NewSuspendResultWithReason(prompt, dive.SuspendReasonAuth, metadata)` (or
  `NewSuspendResult(...)` for the default input reason). `CreateResponse` then returns a
  `*Response` with `Status == ResponseStatusSuspended` and `Response.Suspension *SuspensionState`.
- Resume paths: `dive.WithToolResults([]*ToolResult)` (session-backed) or
  `dive.WithResume(state, results)` (stateless / cross-process). `SuspendableSession` supports
  auto-persistence and `CancelSuspension(ctx)` for the deny path.
- The `OnSuspendHook` fires *before* persistence — use it to send the SMS/Slack notification
  at the moment of suspension. Returning an error from it aborts the suspend.
- Persistence across restart: a `FileStore`-backed session saves the suspended turn so a
  *different* process can resume it.
- Custom tools: `dive.FuncTool[T](...)` → `*dive.TypedToolAdapter[T]`.
- Tests use `github.com/deepnoodle-ai/wonton/assert`.

**Verify exact signatures against the source before relying on them** — confirm the suspend/
resume types and functions in `dive.go`, `response.go`, `tool.go`, `hooks.go`, and `session/`
rather than trusting prose.

## Before you write code, read these

- `examples/suspend/human_approval/` — the simplest suspend → resume loop (start here).
- `examples/suspend/async_webhook/` — suspend across restart, FileStore persistence, webhook
  resume. **This is the backbone of the whole demo** — internalize it.
- `hooks.go` — `OnSuspendHook` to fire the notification.
- `response.go` / `dive.go` — `SuspensionState`, `WithResume`, `NewSuspendResultWithReason`.
- `examples/a2alib_example/` — only if you take the optional remote-agent (`input-required`) route.

## Phase 1 deliverable (this session)

Nail the **technical core** before any phone integration — get suspend/resume across a
process restart working end to end:

1. A small Go service with a believable autonomous task: "plan + book a team offsite under
   $1,500" using web search/fetch for research, with a `book_venue` / `pay_deposit` tool that
   **suspends** (`SuspendReasonAuth`) instead of acting.
2. Persist the suspended turn to a `FileStore` session.
3. Demonstrate the kill-and-restart beat: the process exits while suspended; a **fresh
   process** loads the session and resumes via `WithResume(state, results)` with an
   approve/deny `ToolResult`. The agent then completes (or cancels) the task.
4. Use a **mock/sandbox payment** — never book anything real, but keep amounts/venues
   realistic for the eventual video.
5. Guard against double-resume (idempotency) and wire the deny path through `CancelSuspension`.

**Where it lives:** build under `demos/ai-employee/` in the dive repo as its own Go module —
`demos/ai-employee/go.mod`, module path `github.com/deepnoodle-ai/dive/demos/ai-employee`, with
`replace github.com/deepnoodle-ai/dive => ../..` (the **anthropic** provider is in the core
module, so no extra provider replace). If you take the optional A2A route, also add
`replace github.com/deepnoodle-ai/dive/a2a => ../../a2a`. The repo uses local `replace`
directives, not a `go.work` (see `examples/go.mod`). Its own module keeps the Twilio/Slack/web
deps out of the core `go.mod`. This one is most useful as a permanent in-repo demo (it extends
`examples/suspend/async_webhook`), but the isolated-module setup means it can be extracted later
with no rewrite.

## Constraints & conventions

- Match surrounding Go style; keep the notification + approval transport pluggable (so SMS,
  Slack, or email all drop in).
- Streaming resume is rough in Dive (per the design notes) — that's fine here, since approval
  is a discrete event, not a stream. Don't fight it.
- Treat the approval link as a real security surface: sign tokens (HMAC), expire them, bind
  them to the task id. This is the "production safety gate" story, so do it right.

## Definition of done for this session

From a clean start: the agent researches the task, hits the gated tool, **suspends** with an
`Auth` reason, the turn is persisted, the **process is killed and restarted**, a supplied
approve/deny result resumes the agent in a fresh process, and it finishes (approve) or aborts
cleanly (deny). No phone yet — just prove the cross-restart suspend/resume loop is rock-solid.

## Next (not this session)

`OnSuspend` hook → real SMS (Twilio) / Slack notification with a one-tap signed approval link
→ webhook handler that resumes on reply → stream the agent's reasoning to a clean view →
record the split-screen video with the kill-and-restart beat. See `plan-ai-employee.md`
Phases 3–4.
