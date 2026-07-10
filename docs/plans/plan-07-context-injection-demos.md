---
Title: Context injection demos for the experimental CLI
Author: Curtis Myzie
Status: Draft
Last Updated: 2026-07-10
---

# Context injection demos for the experimental CLI

**Workflow:** Lightweight spec followed immediately by implementation.

## Context

Dive's experimental CLI currently demonstrates static pinned context and one-time
operator reminders through `--context` and `--operator-reminder`. Those flags
prove the wire format, but they do not show why typed reminders are useful in an
agent loop: context can be replaced as reality changes, derived from tool use,
scoped to one response, and assigned contextual or operator authority. We want a
small set of opt-in demos that make those properties visible without turning
advisory reminders into permission or policy enforcement.

## Brainstorm

The divergent pass produced twelve candidates before evaluation:

1. a live Git/workspace pulse;
2. an evidence and provenance ledger;
3. verification debt after mutations;
4. failure-specific retry coaching;
5. a timebox that escalates near a deadline;
6. a token or tool-call budget tripwire;
7. a compact user-constraint ledger to resist instruction drift;
8. dependency-file blast-radius guidance;
9. nearest-owner or local-guidance discovery for touched files;
10. background-task completion and staleness notices;
11. freshness labels for cached external data;
12. a resumable handoff summary for another agent.

These clustered into live state (1, 5, 6, 10, 11), evidence and continuity
(2, 7, 12), and tool-loop guidance (3, 4, 8, 9). The selected four have high
day-to-day value, need no external service or additional user configuration,
and collectively demonstrate pinned replacement, accumulated context,
late-arriving operator events, model-only recording, and failure hooks. The
remaining ideas are useful follow-ups but either need a richer contract or
overlap with the selected patterns.

## Proposal

Add a repeatable `--context-demo NAME` flag to the experimental CLI. It accepts
four demos, plus `all` as a convenience:

- `workspace`: pin a live workspace snapshot before generation and refresh it
  after successful tools, so branch and dirty-state changes are visible without
  persisting stale state.
- `sources`: build a contextual evidence ledger from successful read, search,
  and fetch tools. Replace the pinned ledger as sources accumulate during the
  current response.
- `verification`: append model-only operator reminders after `Write` or `Edit`,
  and append a verification checkpoint after a successful recognized test or
  lint command. This demonstrates late events without polluting saved sessions.
- `recovery`: append a model-only operator reminder after a failed tool call,
  naming the failed call and coaching the model to change one variable before
  retrying.

The implementation lives entirely in `experimental/cmd/dive`, with one flat Go
file per demo and shared option/state wiring in `context_demos.go`. A small
turn-local tracker is installed through `HookContext.Values`; it is protected by
a mutex because parallel tool batches can run hooks concurrently. Model-facing
source and path sets are deterministically ordered, capped at 12 entries, and
report omission counts. Verification recognizes direct toolchain invocations
only when the verifier is the final shell segment. Both print and interactive
paths use the same option-wiring helper. Documentation shows a single
`--context-demo all` command and individual examples.

## Alternatives considered

- Add static persona, user-profile, and project-guideline presets. Rejected
  because `--context NAME=TEXT` already covers that shape and would not exercise
  dynamic delivery.
- Make verification or recovery reminders enforce behavior. Rejected because
  reminders are advisory; permissions and hooks that return errors are the real
  enforcement boundary.
- Add a standalone demo binary. Rejected because the user asked for CLI
  integration, and a flag lets the demos run against the same tools and models
  people already use.

## Tradeoffs and consequences

The workspace snapshot shells out to `git`, and verification-command detection
is intentionally conservative: indirect wrapper scripts are not recognized.
The demos are opt-in and experimental, their reminders say what was observed
rather than claiming complete coverage, and failures to inspect Git degrade to a
plain working-directory snapshot. The evidence ledger records bounded tool
inputs, not truth or citation correctness.

## Open questions

None for the demo scope. If these patterns graduate into reusable library
extensions, their state and configuration should move out of the CLI package.
