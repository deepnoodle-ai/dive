# Dive experimental CLI parity roadmap

**Status:** Proposed
**Date:** 2026-09-03
**Scope:** Daily coding-agent CLI equivalence, not feature-for-feature imitation

## Outcome

Dive should feel complete for the normal loop: start in a repository, understand the active model and safety boundary, steer work without fighting the input, recover or branch a session, inspect changes, and automate the same agent from a pipe. The settings/status work in the current working tree closes the most visible paper cuts. The next priorities are session continuity, live interaction control, and explicit safety controls.

This baseline was checked against the current official [Codex developer commands](https://learn.chatgpt.com/docs/developer-commands?surface=cli), [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference), [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference), [Claude Code interactive mode](https://code.claude.com/docs/en/interactive-mode), and [Claude Code settings](https://code.claude.com/docs/en/settings). Options change quickly, so this comparison is dated rather than presented as timeless.

## Current baseline

| Workflow | Dive now | Parity baseline | Assessment |
|---|---|---|---|
| Model and effort | Flags/env plus persistent user/project settings; `/model`, `/effort`, `/thinking`, `/status` | Both peers expose model selection; Codex exposes reasoning effort with `/model`, while Claude exposes `/model` and `/effort` | Closed by current patch |
| Compact status | Model, effort, repository/branch, context %, and session total cost on one row; detailed usage opt-in | Both peers keep primary state visible and offer status/context inspection | Closed by current patch |
| Session recovery | `--resume` opens a picker | Continue latest, resume by id/name, fork, rename, and session management | Major gap |
| Input while working | New submission is ignored while processing | Queue a follow-up, steer the active turn, interrupt predictably | Major gap |
| Prompt history | Up/down history persisted locally | Persistent history plus reverse search and pasted-input recall | Partial |
| Shell escape | Shell exists as an agent tool | Direct `!` shell mode that does not spend a model turn | Gap |
| Permissions/workspace | Permission prompts and one dangerous bypass flag | Named permission modes, approval policy, sandbox choice, allowed tools, and additional directories | Major gap |
| Change inspection | Tool calls are visible and expandable | Built-in diff, review, initialization, and project-instruction workflows | Gap |
| Non-interactive use | `--print` with text or one JSON result | stdin plus prompt composition, JSONL streaming, schemas, stable exit codes, turn/budget limits | Major gap |
| Subagents/background work | Agent, monitor, and stop tools exist | Built-in agent/background-job visibility and control | Partial |
| Extensibility/health | Skills load at startup | MCP/plugin management, diagnostics, auth/login, hooks, feedback/update flows | Gap |
| Display control | Managed screen, `--inline`, mouse and scrollback commands | Status-line customization, themes/accessibility, alternate-screen controls | Partial |

## Idea inventory

The divergent pass produced twelve candidate improvements, grouped by the job they solve.

### Orientation and configuration

1. Add `/config` to inspect every effective value, source, and settings path, with `dive config get|set|unset` for scripts.
2. Add a model/effort capability check so unsupported combinations are marked before the request instead of failing at the provider.
3. Add session-only modifiers to `/model`, `/effort`, `/thinking`, and `/usage`; persistence remains the default.

### Session continuity

4. Replace the boolean resume shape with `dive resume [id|name]`, add `dive continue`, and keep the picker when no target is supplied.
5. Add `/rename`, `/fork`, `/new`, `/sessions`, and archive/delete actions with confirmation where destructive.

### Fast operator loop

6. Queue Enter submissions while the model works; use a separate shortcut to steer the active turn and make Escape/Ctrl+C interruption states explicit.
7. Add `!command` shell mode with working-directory continuity, streamed output, and a clear distinction from model-visible shell calls.
8. Add Ctrl+R reverse history search scoped to the repository by default, with an all-projects toggle.
9. Add `/diff`, `/review`, and `/init`; make changed-file context cheap to inspect without asking the model to rediscover it.

### Safety and workspace boundaries

10. Expose `--permission-mode`, `--sandbox`, `--allowed-tool`, `--disallowed-tool`, and repeatable `--add-dir`, plus `/permissions` for the live effective policy.

### Automation and extensibility

11. Define a stable non-interactive contract: an `exec` subcommand (or a fully specified `--print` alias), combined stdin + prompt input, JSONL streaming, structured-output schemas, max turns/budget, output-last-message, and documented exit codes.
12. Add `mcp`, plugin, doctor, and update commands, followed by background-job and subagent status views.

## Evaluation

| Candidate | User impact | Delivery effort | Risk reduced | Priority |
|---|---:|---:|---:|---:|
| Session recovery and branching (#4–5) | Very high | Medium | High | 1 |
| Queue, steer, interrupt, shell, history (#6–8) | Very high | Medium-high | Medium | 2 |
| Explicit permissions and workspace scope (#10) | High | Medium | Very high | 3 |
| Non-interactive contract (#11) | High | High | High | 4 |
| Diff/review/init (#9) | High | Medium | Medium | 5 |
| Config inspection and capability validation (#1–3) | Medium | Low-medium | Medium | 6 |
| MCP/plugins/doctor/background UI (#12) | Medium | High | Medium | 7 |

The ordering favors daily friction and recovery first. Automation is important, but its schema and streaming contract deserve a separate design pass rather than being added piecemeal to `--print`.

## Selected next bets

### 1. Session continuity contract

Make resume addressable and composable before adding more session features. The minimum coherent slice is `dive continue`, `dive resume [query]`, `/rename`, and `/fork`. Store the effective model/effort with the session so resuming explains whether the session value or today's default won. Acceptance: a user can recover the last repository session non-interactively, fork it without altering the source, and see the selected session before the first new turn.

### 2. Live interaction control

Treat typing during work as intentional input. Enter queues a follow-up; a documented shortcut steers the active turn; Escape interrupts once and Ctrl+C retains its exit confirmation. Add `!` shell mode and Ctrl+R in the same input-state refactor because all three depend on explicit composer modes. Acceptance: no submitted text disappears, queued versus steering text is visibly labeled, interruption never exits accidentally, and direct shell commands do not enter model history unless requested.

### 3. Explicit safety boundary

Surface the policy the runtime already applies. Add named permission modes and additional-directory grants, show the effective sandbox/approval state in `/status` or `/permissions`, and keep the dangerous bypass visually unmistakable. Acceptance: users can tell what can be read, written, or executed before the first tool call; flags override settings; project configuration cannot silently broaden a managed/user boundary.

## Sequencing

- Ship the current settings and TUI cleanup first; it is a contained foundation and its tests are already local.
- Design session identity and persistence before implementing `/fork`, so names, repository filtering, model inheritance, and deletion semantics share one contract.
- Refactor the composer into explicit idle, working, queued, steering, shell, and interrupting states; then layer shortcuts on it.
- Expose safety flags from the existing permission machinery, then add tests proving precedence and path boundaries end to end.
- Write a separate automation spec before changing `--print`; preserve compatibility while introducing streaming and structured output.

## Equivalence exit criteria

Dive is equivalent for the intended daily workflow when a user can:

1. See and persist model, effort, thinking, usage detail, repository, branch, context, cost, and permission state.
2. Continue, target, rename, fork, archive, and safely delete sessions.
3. Queue, steer, interrupt, search history, and run direct shell commands without losing input.
4. Inspect the diff and request a review without spending turns reconstructing basic repository state.
5. Run the same agent non-interactively with streamed machine-readable events, bounded execution, and stable failure semantics.

MCP marketplaces, themes, remote execution, cloud task management, and every peer-specific command are follow-on differentiators, not prerequisites for this milestone.
