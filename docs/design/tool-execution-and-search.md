# Tool execution and filesystem search contract

**Status:** Accepted for implementation

**Author:** Codex

**Date:** 2026-09-22
**Workflow:** Spec and build in parallel; the contract is clear enough to implement while edge cases are verified.

## Context

PR #301 made cancelled walks and tool batches return promptly. The surrounding tools still give an agent inconsistent answers: some search options do nothing, an incomplete search can say "no matches," limits are hidden in UI-only display text, and a cancelled parallel batch can leave a tool running. These are contract problems, not just isolated bugs.

## Goals

- A search reports no matches only after searching its stated scope without errors.
- A limit or skipped file is visible in model-facing content, with enough detail to refine the next call.
- Glob and Grep give the same path, filter, and error semantics across supported backends.
- All advertised Grep options work or fail clearly; none silently become no-ops.
- Cancellation stops cooperative work promptly and prevents events from an ended run. The API describes what cannot be stopped for an arbitrary in-process tool.
- Search memory is bounded independently of the size of one file or result stream.

## Proposal

Glob searches regular files below the requested directory, without following directory symlinks. `**/` matches zero or more directories. Results are the newest matching files, with a stable path tie-breaker. The implementation scans the complete tree and retains only the configured number of newest files. It marks truncation in content.

Grep treats no matches as a successful result. A missing or unreadable search root is an error. Unreadable entries below a readable root produce an explicitly incomplete result. Searches include regular text files within the size limit; binary, oversized, symlink, and special files are outside that scope. A content result limits matching entries; file and count results limit file entries. Counts are complete for each displayed file. Pagination and truncation are visible to the model.

Searches consider text files up to 64 MiB. Each call returns at most 10,000 entries and 1 MiB of output text; any output cap is stated in model-facing content. These caps bound memory and give the agent a clear way to narrow a broad search.

The optional ripgrep path streams records, checks decoder errors, and bounds individual records. It uses explicit flags for hidden and ignored files to match the Go path. Both backends honor context lines, multiline search, filters, and cancellation. If a backend cannot support a requested option, it returns an error instead of a plausible but wrong result.

The agent checks cancellation before dispatch and after tool completion. Parallel tool streaming and progress callbacks are closed when the batch ends, so background stragglers cannot emit into a completed response. In-process tools remain responsible for honoring their context; the agent cannot forcibly terminate a goroutine. Built-in file tools check context around blocking or mutating operations.

## Alternatives considered

- **Remove ripgrep and maintain one Go implementation.** This simplifies parity but gives up a useful fast path for large source trees.
- **Use ripgrep as a required dependency.** This improves speed but removes the pure-Go portability and predictable behavior when `rg` is unavailable.
- **Keep current best-effort results.** This minimizes code changes but allows false negatives that an agent cannot distinguish from a complete search.

## Tradeoffs and rollout

Complete scans for newest Glob results and accurate Grep counts may take longer than stopping at the first configured number of matches. The work stays cancellable and memory bounded. Existing tool names and input fields remain; clearer errors and model-visible truncation can change agent responses. Focused parity, limit, cancellation, and error tests accompany the implementation. The guide will state the behavior and the in-process cancellation limit.
