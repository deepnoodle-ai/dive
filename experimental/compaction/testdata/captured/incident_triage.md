Your conversation history was compacted to free up context. A previous instance of you was working on this task and left the handoff notes below. Treat them as an accurate record of what happened and continue the work seamlessly.

## Task Overview
**Request:** Investigate production PagerDuty alert — `api` service's 500 error rate jumped from ~0.1% to ~8% over 20 minutes. Root cause analysis only; read-only investigation, no restarts/scaling/writes without explicit approval.

**Stack:** Go service on Google Cloud Run, Postgres on Neon, pgx connection pool.

**Success criteria:** Deliver root-cause hypothesis with evidence + proposed fix for user approval.

## Current State
**Investigation completed.** Root cause identified with full evidence chain.

### Files Analyzed
- `internal/db/pool.go` — pool configuration (MaxConns=10, MinConns=2)
- `internal/orders/repo.go` — connection leak in `ListByCustomer()` function (lines 66–79)
- `internal/users/repo.go` — verified no leak (proper defer Release)
- Neon DB `pg_stat_activity` query — confirmed 9 leaked connections

### Key Findings
**ROOT CAUSE:** Connection leak in `internal/orders/repo.go:ListByCustomer` (lines 71–79)
```
conn, err := r.pool.Acquire(ctx)  // line 71
if err != nil { return nil, err }  // MISSING Release() on error path
rows, err := conn.Query(...)       // line 76
if err != nil { return nil, err }  // <-- MISSING Release() on error path
defer rows.Close()
orders, err := scanOrders(rows)
conn.Release()                     // Only runs on success path
return orders, err
```

On query error, connection is acquired but never released. Pool size = 10; each failed query leaks one slot. After ~10 failures, pool exhausted → all `/v1/orders` requests fail with 500.

### Evidence
1. **Application logs:** Repeated `failed to acquire connection: timeout... (pool exhausted, 10/10 in use)` on GET /v1/orders
2. **DB logs:** `pgx: ERROR: canceling statement due to user request (SQLSTATE 57014)` on the exact query — indicates query timeouts/cancellations triggering the leak
3. **Database state:** `pg_stat_activity` shows **9 connections in `idle in transaction`** state — leaked connections holding pool slots but idle
4. **Endpoint correlation:** Only `/v1/orders` endpoint failing, which is the only one calling `ListByCustomer()`

## Important Discoveries
- pgx `Acquire()` must be paired with `Release()` via `defer` immediately after successful acquire, regardless of subsequent error paths
- The query failure itself (what started 20 min ago) is secondary; the leak is the amplifier that converts transient errors into complete service degradation
- Neon's modest connection limits (plan-dependent) make pool exhaustion high-impact
- Proper pattern: `defer conn.Release()` immediately after `Acquire()`, before any business logic

## Next Steps
**Pending user approval for:**

1. **Code fix (required to prevent recurrence):**
   - Add `defer conn.Release()` immediately after line 71 in `internal/orders/repo.go`
   - Remove the manual `conn.Release()` on line 78
   - Redeploy the revision

2. **Recovery (one of two options, user chooses):**
   - **Option A (preferred):** Deploy code fix; new Cloud Run revision resets pool automatically
   - **Option B (immediate but temporary):** Restart current Cloud Run revision to free leaked connections (user has veto power per constraints)

3. **Post-fix validation:**
   - Recheck `pg_stat_activity` for leaked connections
   - Monitor `/v1/orders` error rate for return to <0.1%

**Blockers:** None technical. Awaiting user approval to deploy code fix and/or restart revision.

## Context to Preserve
- User enforced strict read-only constraint — all actions logged, no silent mutations
- User will approve any proposed action before execution
- This is production; any changes must be minimal and justified
- The 20-minute coincidence timing suggests something external started triggering query failures (e.g., data anomaly, timeout threshold, downstream service). Secondary investigation may be warranted post-fix, but leak is the immediate root cause.
- User prefers evidence-backed analysis (logs + code + DB state) over speculation
