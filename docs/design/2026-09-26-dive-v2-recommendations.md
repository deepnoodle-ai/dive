# Dive v2: top recommendations

**Date:** 2026-09-26  
**Status:** Recommended direction; prototypes authorized, public API provisional.  
**Workflow:** Code review → focused prototypes → final design → implementation.

The [architecture review](2026-09-26-dive-v2-architecture-review.md) and
[feedback register](2026-09-26-dive-v2-integrator-feedback.md) contain the evidence.
These are the decisions with the greatest potential to simplify Dive and remove
work from Noodle, Nvoken, and Mobius.

## 1. Make execution records the engine's primary contract

Give every caller complete, typed records of model attempts, results, effective
tool calls, accepted results, usage, and termination. Separate those records from
display events and explicit policy decisions. An empty answer, refusal, partial
response, or provider failure must still have a record.

Today consumers reconstruct facts from callbacks, and observers can mutate the
objects that become history. A uniform recording protocol removes that duplicate
work and gives sessions and application runtimes the same evidence. Define exactly
which records must be acknowledged before an effect can start or execution can
continue. Keep observed results separate from acknowledged persistence.

## 2. Keep sessions, but put them around the engine

Retain a convenient session runner for loading history, claiming ownership,
recording steps, projecting replay history, and saving turns. The engine itself
must run equally well inside an application-owned transaction/lease system.

Removing sessions would transfer complexity to small applications. Keeping
session-specific branches inside the engine transfers complexity to sophisticated
ones. Both should share one execution implementation. Reuse the existing session
store's recovery, revision, and claim behavior when production work begins.

## 3. Make commands, recovery, and outcomes explicit

Separate starting a turn, continuing one, and accepting external results.
Persist turn-wide limits and accepted input so they survive invocation boundaries.
Distinguish completion, refusal, truncation, waiting, cancellation, infrastructure
failure, and uncertain execution. Unknown usage must remain unknown.

An acknowledged intent is permission to begin an effect, not proof it ran.
After a crash or lost acknowledgement, reconcile by stable identity; never
automatically repeat a possibly executed tool. Idempotent recording does not make
an external side effect exactly once. Expose both the last acknowledged state and
any observed result that could not be saved, including a failed terminal write.

## 4. Have one tool preparation and authorization pipeline

Resolve the tool, transform and validate its input, authorize the final effective
call, record it, execute it, then accept its result. Use that same call for previews,
traces, accounting, and replay. Keep model-facing tool errors distinct from
infrastructure failures. Require an explicit policy for missing approval UI.

The current input rewrite and hook-order behavior can authorize one call and
execute another. Sequential and parallel execution must eventually use the same
pipeline; concurrency should be bounded and late results explicitly handled.

## 5. Make the provider contract honest and lossless

Prepare requests before admission, reservations, or network attempts. Report the
effective settings and rejected/ignored options. Unsupported settings should fail
strictly unless the caller explicitly chooses a lenient policy. Preserve schemas,
citations, multimodal content, provider-private replay data, and served-model
identity. Normalize attempt lifecycle and usage without erasing provenance.

Silent option loss and incomplete content normalization make a portable interface
misleading. Keep the independently usable `llm` package, but test the same contract
against each provider adapter instead of assuming compatible option names imply
compatible behavior.

## 6. Prove consumer simplification before committing to v2

Success is a small Noodle integration and less custom recording, resume, and
accounting code in Nvoken and Mobius. It is not a smaller `agent.go` by itself.
Use those three integration shapes as acceptance fixtures. Migrate incrementally
behind adapters, preserve existing recovery behavior, and document intentional
compatibility breaks before releasing a major version.

## First prototypes

The isolated [execution prototypes](../../experimental/v2/README.md) test one
recording protocol with two owners: a file-backed session runner and a simulated
application transaction that records usage and consumes queued input atomically.
They use deterministic fake models and tools; no provider calls or production
API changes are needed.

The first pass covers faults before and after writes, record deduplication,
uncertain effects, external-result resume, persistent call limits, and mutation
isolation across observers and authorization. A second experiment runs separate
worker processes against a local HTTP service: it commits a simulated side effect,
loses the response, transfers ownership, reconciles the result, and safely handles
repeated result delivery. It also tests stale-owner fencing and conflicting IDs. Its file store is a disposable
single-writer experiment, not a replacement for the existing session store.

Next, test request preparation against the actual OpenAI, Anthropic, and Google
adapters, and port one representative consumer flow through the new boundary.
Streaming, parallel tools, fencing, full content replay, and production database
transactions remain required design work. Do not freeze the public interfaces
until those experiments expose where this first protocol is insufficient.
