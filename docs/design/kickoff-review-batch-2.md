# Kickoff: API Review Remediation — Batch 2

Copy everything below the line into a fresh Claude Code session in `~/git/deepnoodle/dive`.

---

We're continuing remediation of the findings in `docs/reviews/2026-06-09-api-review.md` (read it first — it is the source of truth, with per-finding `file:line` references and ✅ annotations for what's already fixed). Batch 1 merged as PRs #181–#186 and fixed the low-hanging fruit: session FileStore durability, toolkit fail-closed workspace validation, agent-loop races and background-results persistence, provider retry/logging/ID fixes, MCP client lifecycle, and streaming usage/indices.

**Your task: implement Batch 2.** Same workflow as Batch 1: work in git worktrees off `main`, one branch + PR per group below (parallel agents are fine), `github.com/deepnoodle-ai/wonton/assert` for all tests, regression test per fix, `gofmt` + `go build ./...` + `go test ./...` (plus `-race` for concurrency fixes) before committing. Commit messages end with `Co-Authored-By:` trailer per CLAUDE.md; PR bodies end with the Claude Code attribution line. **Re-verify every finding against current `main` before fixing** — Batch 1 changed some of these files, and line numbers in the review doc may have drifted. If a finding no longer holds, skip it and say why in the PR body.

**Still excluded:** the `permission/` package (§1) — deferred by explicit decision, do not touch it. Also out of scope: everything in §8 (breaking changes for the next major).

## Group A — `fix/agent-resume-session-semantics` (needs the most care)

1. **§4.3 `WithResume` drops session history** (`agent.go`, explicit-suspension branch around the `fullHistory` construction): when a session is attached and the caller passed `WithResume` without `WithMessages`, fall back to the loaded session messages as pre-turn history instead of generating against only the suspended turn. Found independently by two reviewers; the documented cross-process-handoff flow is broken without it.
2. **§3.5 suspend-phase usage dropped** (`session/session.go`, `SaveResumedTurn` + replace-last branch of `SaveSuspendedTurn`): sum the replaced event's usage into the new event so `TotalUsage()` doesn't lose the tokens paid before suspension.
3. **§3.4 remaining item — `SaveTurn` aliases caller-owned message pointers** (`session/session.go:~357`): deep-copy messages (and usage) on ingestion so post-call mutation of `response.OutputMessages` can't rewrite stored history. Watch the cost: `llm.Message.Copy` is a JSON round-trip; reuse it for correctness now, note the structural-copy optimization as follow-up.
4. **Up-front precondition check** (review §3 / core C3): `WithResume` against a suspendable session that is NOT suspended currently fails only after generation (tokens spent, turn discarded). Detect the mismatch before calling the LLM.

## Group B — `fix/session-open-caching`

**§3.3 FileStore double-`Open` divergence** (`session/file_store.go`): two `Open` calls for one ID return divergent in-memory copies; a full rewrite from one silently deletes the other's turns. Recommended fix (decision pre-made — implement this): cache live `*Session` per ID in `FileStore` mirroring `MemoryStore`'s behavior, with entries evicted on `Delete`. Document the new aliasing semantics on `Open`. This was deliberately deferred from #181 — read that PR's diff first so you build on the healed-read path, not around it.

## Group C — `fix/toolkit-remaining`

1. **§2.5 default exclude patterns miss top-level dirs** (`toolkit/glob.go`, `toolkit/grep.go` pure-Go path): `**/node_modules/**` under `gobwas/glob` doesn't match at the search root. Fix the matching (e.g. also test `"./"+relPath` or add bare-prefix pattern variants) so the ripgrep and pure-Go paths agree. Verify empirically against `gobwas/glob` like the review did.
2. **§2.6 Grep `offset` and `show_lines` accepted but ignored** (`toolkit/grep.go`): implement both (apply offset after grouping, honor `-n` in output) or remove them from the input schema — do not leave documented no-ops. Implementing is preferred; `head_limit`+`offset` should compose into working pagination.
3. **§2.4 Fetch SSRF DNS-rebinding gap** (`toolkit/fetch.go`): pin validation to connection — custom `DialContext` that validates the resolved IP at connect time (covering redirects), replacing the lookup-then-forget check. Keep the existing policy semantics; only close the re-resolution window.
4. **Bash/ReadFile scanner issues** (review §2 trailing notes): Bash ignores `scanner.Err()` and reports truncated output as success; ReadFile's offset/limit path uses the default 64 KB scanner and diverges from whole-file reads. Surface scan errors and raise the ReadFile buffer (mirror bash.go's 1 MB).

## Group D — `fix/skill-loader-hardening` (skills, NOT the permission package)

1. **§6.1 shell-expansion injection**: in `skill/expand.go`, run `!{...}` expansion on the raw template BEFORE substituting `$ARGUMENTS`/`$N`, and never execute `!{` sequences introduced by args. Model-controlled args must not gain shell execution. Add an injection regression test (template + hostile args).
2. **§6.2 `allowed-tools` parsed but unenforced** (`skill/skill.go`): minimal honest fix — either enforce or re-document. For this batch: re-document as informational ("not currently enforced") and file the enforcement as follow-up; do not ship a half-enforcement.
3. **§6.3 Skill tool executes catalog-hidden commands** (`skill/tool.go`): resolve through a skills-only lookup in `Call` so user-invocable-only commands can't be triggered by the model.
4. **§6 trailing — `catalogHook` `lastHash` race** (`skill/agent.go`): guard the closure-captured hash (atomic or mutex); add a `-race` test with concurrent `CreateResponse`.

## Group E — `fix/a2a-and-settings`

1. **§7.5 RemoteAgent returns intermediate text** (`a2a/executor.go` + `remoteagent.go`): emit a single final artifact (or make `extractText` prefer the last artifact) so tool-using turns return the final answer, matching `Response.OutputText()` semantics. This changes observable `TaskResult.Text` for multi-message turns — call that out in the PR body.
2. **§7.7 settings.local.json shadows instead of merging** (`experimental/settings/settings.go`): implement base+override merge (Claude Code semantics, which the doc already claims). Define merge rules explicitly in a doc comment (maps merge per-key, local wins; slices replace wholesale).
3. **§7 trailing — `Manager.RefreshTools` stale-key cleanup never matches** (`experimental/mcp/manager.go`): the cleanup looks for `server.`-prefixed keys that are never written. Fix the cleanup to match actual keys and add the missing duplicate-name guard on refresh. (Tool namespacing itself is a §8 breaking change — don't do it here.)

## Deferred from this batch (don't start, but mention in the tracking PR bodies if adjacent)

- §5.4 Anthropic web-search error-variant decode, §5.5 OpenAI double-retry, §5.6 registry `/`-routing — provider batch 3.
- §4.5/§4.7/§4.8 (SessionStart Values, partial-result-on-error, hook Messages freshness) — agent batch 3.
- §7.4 sandbox allowlist enforcement, §7.6 A2A incremental streaming — need design discussion.
- All of §1 (permission) and §8 (breaking changes).

## When done

Update `docs/reviews/2026-06-09-api-review.md`: mark each fixed finding ✅ with its PR number (follow the existing annotation style), update the remediation-status paragraph, and adjust the fix-order table. Report all PR URLs, anything skipped with reasons, and test results.
