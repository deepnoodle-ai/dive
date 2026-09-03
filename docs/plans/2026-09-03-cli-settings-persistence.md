# CLI persistent settings + status-line diet

**Status:** Implemented and validated
**Author:** Dive
**Date:** 2026-09-03
**Workflow:** This note records the implementation behavior and review decisions.

## Context

Dive's CLI resolved `model`, `thinking-effort`, and `show-thinking` from flags/env only, with a hardcoded `medium` effort default. `/model` switched per-session and forgot on exit. The status line rendered a 3–5 row token breakdown (`tokensPanelView`) every frame, and two render papercuts (inconsistent footer padding and pasted-whitespace highlighting in the user bubble) made it feel unfinished next to Codex/Claude Code.

## Goals

- `/model`, `/effort`, `/thinking` persist across sessions by default and report where the effective value came from (`flag > env > settings > default/autodetect`).
- Status line is one row by default: `model · effort in dir on branch · ctx% · $total`; full breakdown available via `/usage` and opt-in live via `/usage full`.
- Fix: keep exactly one explicit blank row under the footer instead of variable double padding.
- Fix: pasted multi-line content no longer shows lit trailing-whitespace runs in the user bubble.
- All covered by `go test -count=1` in `experimental/cmd/dive` and `experimental/settings`.

## Non-goals

- No `-c key=value` generic overrides, no `--approval-mode`/`/approvals`, no `/init`/`/diff`/`/review` (P1 backlog from the brainstorm).
- No per-project model defaults beyond what the tiered merge already allows; only the user tier is written by the new commands.
- No migration of existing `~/.dive/history.json` or sessions.

## Proposal

### Settings tiers (all `settings.json`, Claude-Code semantics)

`~/.dive/settings.json` (user) < `.dive/settings.json` (project base) < `.dive/settings.local.json` (local wins). Merge is per-key recursive; arrays replace wholesale (`experimental/settings/settings.go: mergeSettingsMaps`). New user-tier keys: `model`, `thinking_effort`, `show_thinking`, `show_detailed_usage`. Permissions/sandbox continue to come from project tiers only.

`LoadEffectiveSettings(dir)` merges all three; `LoadSettings(dir)` stays project-only and hermetic for tests. `SaveUserSettings(patch)` reads-modify-writes only the user file (0700 dir, atomic rename).

### Resolution order (`cli_settings.go`)

`resolveModelName` → flag, `DIVE_MODEL`, settings, `getDefaultModel()` autodetect. Returns `(name, source)`; source is shown at startup and in `/status`.
`resolveThinkingEffort` → flag, `DIVE_THINKING_EFFORT` (explicit empty omits the parameter for non-reasoning models), settings, `medium`.
`resolveShowThinking`, `resolveShowDetailedUsage` → flag, env (`DIVE_SHOW_THINKING`, `DIVE_SHOW_DETAILED_USAGE`), settings, `false`.

### Commands (`cmd_settings.go`, wired in `app.go:handleCommand`)

- `/model [name]` — existing dialog; on switch, `persistModelSwitch` writes user settings (source becomes `session`).
- `/effort [none|minimal|low|medium|high|xhigh|max|<provider>]` — bare shows current + source; switch applies to `a.modelSettings.ReasoningEffort` live (next turn) and persists. `omit`/`off`/`""` clears to parameter-omitted.
- `/thinking [on|off]` — bare toggles; persists; flips `Thinking`/`ThinkingDisplay` live.
- `/usage [full|brief]` — bare prints the full report (unchanged); `full`/`brief` toggles + persists `show_detailed_usage`.
- `/status` — one report: model, effort, thinking, and detailed-usage (all with sources), plus session id, context %, and session cost.
- Autocomplete and `/help` updated.

### Status line (`render.go:statusLineView`)

One `Group` row: `model · effort · thinking? in dir on branch · ctx% · $session-total · ⚡fast?` (+ flash right-aligned). `sessionTotalCostString` prefers session total, falls back to turn total. `tokensPanelView` renders only when `showDetailedUsage` is set; `/usage` report is unaffected.

### Render fixes

- Blank line: `inputAreaView` (`app.go`) forced `footerViews` to at least two lines unconditionally. The final layout keeps one explicit breathing row below the status or autocomplete footer, while autocomplete still pads its active list to 8 lines for stable selection.
- Paste whitespace: `textMessageView` `roleUser` now renders `trimTrailingWhitespacePerLine(msg.Content)`; message content itself is unmodified (copy/scrollback unaffected).

### Downstream consistency

- New subagents snapshot the currently selected parent model and model settings, including effort and thinking display. A later switch affects newly spawned work without mutating a subagent already in flight.
- `/model` updates the model used by manual and mid-turn compaction. When the compaction threshold was automatic, it is recalculated for the new model; an explicit flag or environment value remains fixed.
- The mid-turn hook reads the current model, threshold, and system prompt instead of retaining startup values.

## Alternatives considered

- **`~/.dive/config.json` (new name) vs `~/.dive/settings.json`.** Rejected: we already have a `settings.json` concept at project level and Claude Code uses `~/.claude/settings.json` globally. One vocabulary, one merge implementation.
- **Persist project-tier instead of user-tier on switch.** Rejected: switching models shouldn't dirty the repo's `.dive/settings.json`; user tier is personal and never committed.
- **Keep full token table live by default.** Rejected: costs 3–4 rows of chrome every frame and duplicates `/usage`; brief-by-default with `full` opt-in preserves the data one command away.

## Tradeoffs and consequences

- Writes on every `/model`/`/effort`/`/thinking`/`/usage full|brief` — a surprising write if the user expected session-only. Mitigated by the notice naming the file (`saved to ~/.dive/settings.json`).
- `SaveUserSettings` is read-modify-write without locking; concurrent CLI instances could clobber. Acceptable for a single-user local file for now; noted, not solved.
- User settings are filtered to the four model/presentation keys before merging. A global file cannot silently inject project permission or sandbox policy.
- Status line no longer shows per-scope input/output at a glance; users who liked it must run `/usage full` once. That's the point, but it's a behavior change worth a CHANGELOG line.

## Rollout

Implementation: `experimental/settings/settings.go`, `experimental/cmd/dive/{app,main,render,cli_settings,cmd_settings}.go` + tests, CHANGELOG entry. No migration (global file is new; absence = defaults). Docs: `/help` + `/status` cover it; `docs/guides/experimental/` update optional.

## Open question

- Should `/model`/`/effort` accept a `--session` modifier for a one-off switch? The default should stay persistent, but an explicit escape hatch would avoid editing the user file for experiments.

## Quality-check notes

- Remaining known weakness is concurrent-writer handling for `SaveUserSettings`; atomic replacement prevents partial files but does not prevent lost updates.
- Length matches the change: three small features sharing one settings seam, not three separate specs.
