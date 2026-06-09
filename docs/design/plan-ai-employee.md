# Implementation Plan — Your AI Employee Texts You 📱

> Suspend/resume reframed as a product moment: an agent works autonomously, hits a real
> decision, **pauses and pings your phone**, you approve from anywhere, and it resumes
> mid-turn — even across a process restart.
> **Pillar proven:** suspend/resume + A2A `input-required` (a genuinely rare capability).
> **Shareable artifact:** a ~60-second screen+phone split video — the "magic moment."

## The shareable moment

> "I asked my AI to plan and book a team offsite, then went to lunch. It researched venues,
> hit a $1,200 deposit, and **texted me** to approve. I tapped 'yes' from the restaurant.
> When I got back, it was done." → split-screen video: terminal/agent log on one side,
> a real phone receiving an SMS/Slack approval on the other.

Most agent demos either run to completion or crash. Dive's suspend/resume lets a tool
*pause mid-turn*, persist the exact state, and resume hours later from an out-of-band
signal. Framed as "your AI employee asks permission," it's both **practically impressive**
(real adopters want exactly this) and a crisp, reproducible short-video format.

## What it proves about Dive

- **Suspend/resume**: a tool returns `NewSuspendResultWithReason(prompt, SuspendReasonAuth, metadata)`;
  the agent pauses and returns `Status == ResponseStatusSuspended` with a `SuspensionState`.
- **Persistence across restart**: the suspended turn is saved to a `FileStore` session;
  the process can exit and a *different* process resumes it via `WithResume(state, results)`.
- **Out-of-band resume**: the approval arrives via webhook/SMS callback hours later — no polling.
- **`OnSuspend` hook**: fires before persistence — used here to actually send the
  notification (SMS/Slack/email) at the moment of suspension.
- **A2A mapping** (optional): suspension maps to the A2A `input-required` state, so the same
  pattern works for a remote agent answered via `SendTextOnTask`.
- **Permission classification**: `SuspendReasonAuth` vs `SuspendReasonInput` distinguishes
  "needs your approval" from "needs more info."

## The flow

```
 user: "Plan and book the team offsite, budget $1500."
   │
   ▼
 agent runs autonomously: WebSearch venues, Fetch details, draft plan
   │
   ▼  reaches a real-money / irreversible action
 book_venue tool → NewSuspendResultWithReason(
        "Approve $1,200 deposit at <venue>?", SuspendReasonAuth, {amount, venue, link})
   │
   ├─ OnSuspend hook fires → send SMS/Slack: "Approve $1,200 deposit? Reply YES/NO: <link>"
   ├─ turn persisted to FileStore; CreateResponse returns Status=Suspended
   │  (process can now exit entirely)
   ▼
 [hours later] user taps "Approve" in SMS/Slack
   │
   ▼  webhook handler receives approval
 load session → WithResume(state, []*ToolResult{approved}) → agent resumes mid-turn
   │
   ▼
 agent completes the booking, summarizes: "Booked. Confirmation #..., calendar invite sent."
```

## Architecture

```
┌──────────────┐   suspend    ┌─────────────────┐  notify  ┌───────────┐
│  Dive Agent  │─────────────▶│  FileStore      │─────────▶│  SMS /    │
│ + book tool  │  (persist     │  session +      │ OnSuspend│  Slack    │
│ + OnSuspend  │   turn)       │  SuspensionState│  hook    │  (Twilio) │
└──────────────┘              └─────────────────┘          └─────┬─────┘
        ▲                                                        │ user taps Approve
        │ WithResume(state, results)                             ▼
┌───────┴────────────────────────────────────────────────┐  ┌──────────────┐
│  Resume handler (HTTP webhook)                           │◀─│  Approval URL │
│  - look up session by task id                            │  │  / SMS reply  │
│  - build ToolResult(approved|denied)                     │  └──────────────┘
│  - agent.CreateResponse(WithResume(...))                 │
└──────────────────────────────────────────────────────────┘
```

A small HTTP service (Go) hosts: (a) the agent runner, (b) the approval webhook/landing
page, (c) the Twilio/Slack integration. Sessions persist to disk so suspend survives a
full process restart — the demo should explicitly **kill and restart the process** between
suspend and resume to make the persistence point undeniable.

## Where it lives

`demos/ai-employee/` as its own Go module (`github.com/deepnoodle-ai/dive/demos/ai-employee`),
following the repo's local-`replace` multi-module pattern (see `examples/go.mod`). Keeps the
Twilio/Slack deps out of the core `go.mod`. Best kept as a permanent in-repo demo (extends
`examples/suspend/async_webhook`), still extractable later. Exact `replace` recipe (core, plus
`a2a` if you take the remote route) is in `kickoff-ai-employee.md`.

## Build phases

**Phase 1 — Local terminal version (no phone yet)**
- Start from `examples/suspend/human_approval/`: a tool that suspends for terminal approval.
- Confirm the suspend → `WithToolResults` resume loop works inline.

**Phase 2 — Across-restart persistence**
- Start from `examples/suspend/async_webhook/`: persist the suspended turn to FileStore,
  exit the process, resume from a fresh process via `WithResume(state, results)`.
- This is the technical core. Get it rock-solid.

**Phase 3 — Real phone notification (the shareable layer)**
- `OnSuspend` hook → send an SMS (Twilio) or Slack message with the approval prompt + a
  one-tap approve/deny link.
- Webhook handler receives the reply → loads session → resumes.
- A believable task: "plan + book a team offsite under $1,500" using `WebSearch`/`Fetch`,
  with `book_venue`/`pay_deposit` as the gated (suspending) tools.

**Phase 4 — Polish for video**
- Stream the agent's reasoning (`WithEventCallback`) to a clean terminal/web view.
- Record split-screen: agent log + real phone. The kill-and-restart beat sells the
  persistence claim.

## Starting points in the repo

- `examples/suspend/human_approval/` — simplest suspend/resume (Phase 1).
- `examples/suspend/async_webhook/` — suspend across restart + FileStore + webhook (Phase 2/3 backbone).
- `examples/a2alib_example/` — `input-required` mapping if going the remote-agent route.
- `hooks.go` — `OnSuspend` hook to fire the notification at suspension time.
- `response.go` / `dive.go` — `SuspensionState`, `WithResume`, `NewSuspendResultWithReason`.

## Tech choices

- Go HTTP service (single binary), FileStore for sessions.
- Notifications: Twilio for SMS (most universally relatable for video) and/or Slack
  (more "AI employee in the workplace" framing). Make it pluggable.
- Approval landing page: dead-simple signed one-tap link (HMAC token tied to task id) so
  the demo isn't bogged down in auth.

## Risks / open questions

- **Streaming resume is rough** (capability map note): resume is request/response, not
  real-time. Fine here — approval is inherently a discrete event.
- **Security of the approval link**: sign tokens (HMAC), expire them, bind to task id.
  Worth doing right since this is the "production safety gate" story.
- **No real money in the demo**: use a sandbox/mock payment so we don't actually book
  anything — but keep the amounts and venues realistic for the video.
- **Idempotency**: guard against double-resume (user taps twice). Use `CancelSuspension`
  for the deny path.

## Effort estimate

- Phase 1: **~0.5 day** (mostly reading the existing example).
- Phase 2: **~1 day**.
- Phase 3: **~1.5 days** (Twilio/Slack + webhook + landing page).
- Phase 4 polish/recording: **~1 day**.

Total: **~4 days** — the quick win of the three.

## Definition of "done & shareable"

A recorded split-screen video where the agent suspends, the **process is killed and
restarted**, a real phone receives the approval request, a tap resumes the agent, and it
finishes the task — plus a forkable repo with Twilio/Slack and a mock-payment tool.
