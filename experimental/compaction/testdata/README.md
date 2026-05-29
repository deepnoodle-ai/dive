# Compaction fixtures

Larger, realistic agent transcripts used to exercise compaction on real-world
content (not the toy messages in the unit tests). Each `*.json` file is a full
multi-turn conversation — `[]*llm.Message` with proper `tool_use`/`tool_result`
pairing — and is the **source of truth**: edit the JSON directly.

| Fixture | What it is |
|---|---|
| `swe_debugging.json` | The largest fixture (~48 messages). A multi-phase coding session: fix a failing discount test → add a multi-item regression test → investigate and fix a separate per-line tax-rounding (off-by-a-cent) bug with its own reproduction test → verify the fix is localized across the repo → run the full suite. Debug→fix→test→verify across several files, with a hard "don't weaken the tests" constraint. |
| `codebase_onboarding.json` | An agent answers "how does request auth work and where do I add API-key auth?" by reading the router, middleware, JWT verifier, and identity helpers. Read-only; dominated by source-file tool results. |
| `incident_triage.json` | A read-only production incident investigation: Cloud Run error logs → pool exhaustion → a connection leak in a repo → corroboration via a read-only `pg_stat_activity` query. Carries explicit production constraints (no restarts/writes without approval) a good summary must preserve. |

## How they're used

- **`fixtures_test.go`** (`TestFixturesAreWellFormed`, `TestNonDestructiveCompactionCycle`)
  loads every fixture and runs it through the real `CompactMessages` +
  `session.Compact` path with a deterministic stub summarizer. Asserts the
  non-destructive contract — active window collapses to the summary,
  `AllMessages` still returns the full transcript, `CompactionHistory` records
  one checkpoint. Runs in CI; no network.

- **`capture_test.go`** (`TestCaptureCompactionOutputs`) runs the same fixtures
  against a live Anthropic model and writes the **raw compaction output** to
  `captured/` — each `<scenario>.md` is the compacted message verbatim (the
  handoff-framed summary that becomes the new active window). Opt-in (paid
  network calls):

  ```sh
  CAPTURE_COMPACTION=1 \
    go test ./experimental/compaction -run TestCaptureCompactionOutputs -v
  ```

  Override the summarizer with `DIVE_COMPACTION_MODEL` (default
  `claude-haiku-4-5`). The committed files under `captured/` are the outputs of
  one such run — read them to see what compaction actually produces.
