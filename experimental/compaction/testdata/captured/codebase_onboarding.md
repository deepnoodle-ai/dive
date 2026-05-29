Your conversation history was compacted to free up context. A previous instance of you was working on this task and left the handoff notes below. Treat them as an accurate record of what happened and continue the work seamlessly.

## Task Overview
**Request:** Add API-key authentication alongside existing JWT auth in the `api` service (chi router). The user explicitly requested a read-and-explain task only—no code changes yet. They want to understand the full end-to-end auth flow before implementation.

**Success Criteria:**
- Explain where auth is wired into the router
- Explain how caller identity gets attached to requests
- Identify the cleanest place to hook in a second auth scheme
- No code modifications (read-only task)

## Current State
**Completed:** Full analysis of the existing JWT authentication flow.

**Files Analyzed:**
- `internal/httpapi/router.go` — Router setup; auth middleware applied to protected chi Group
- `internal/auth/middleware.go` — Core middleware; validates `Authorization: Bearer <token>` header and calls `Verifier.Verify()`
- `internal/auth/jwt.go` — `Verifier` interface definition and `JWTVerifier` implementation
- `internal/auth/identity.go` — `Identity` struct (Subject, Scopes, Method) and context helpers (`withIdentity`, `IdentityFromContext`)
- `internal/orders/handler.go` and `internal/users/handler.go` — Sample handlers showing how they consume identity via `IdentityFromContext`

## Important Discoveries

**Auth Architecture:**
1. Protected routes live in a chi `Group` with `auth.Middleware(deps.Auth.Verifier)` applied
2. Public routes (`/healthz`, `/v1/auth/login`) are in a separate group with no auth
3. The middleware is credential-agnostic: it delegates verification to a `Verifier` interface
4. All handlers consume authentication the same way: `id := auth.IdentityFromContext(r.Context())` — completely decoupled from the scheme

**Key Design:** The `Verifier` interface is the seam for adding new auth schemes:
```go
type Verifier interface {
	Verify(ctx context.Context, credential string) (*Identity, error)
}
```

**Identity Context Flow:**
- Middleware calls `v.Verify()` and receives an `*Identity`
- Middleware calls `withIdentity(r.Context(), id)` to attach it
- Handlers retrieve it with `IdentityFromContext(ctx)` and check `Scopes`
- `Identity.Method` field ("jwt", etc.) records which scheme was used

## Recommendation Made (Not Yet Implemented)

**Option A (Recommended):**
- Create an `APIKeyVerifier` implementing the `Verifier` interface
- Wrap both `JWTVerifier` and `APIKeyVerifier` in a `MultiVerifier` that tries API key first, then falls back to JWT
- Zero changes needed to middleware or handlers — they already work polymorphically off `Identity`
- Caveat: Current middleware assumes `Bearer` prefix; API key would need same or middleware's header parsing must be generalized

**Option B (Alternative):**
- Add a separate `APIKeyMiddleware` reading `X-API-Key` header
- Chain it before JWT middleware with fallthrough when header absent
- More flexible but requires exporting `withIdentity` and defining precedence

User agreed this analysis was complete but has **not yet approved implementation**. Task stopped here pending their go-ahead.

## Next Steps (Pending User Approval)

1. **User confirms chosen approach** (Option A or B, or revises)
2. **Implement chosen verifier:**
   - If Option A: Create `APIKeyVerifier` struct, implement `Verify()` method, create `MultiVerifier` wrapper
   - If Option B: Create `APIKeyMiddleware` function, export identity setter, chain in router
3. **Handle header parsing:** Decide if API keys use `Bearer <key>` or a custom header
4. **Test:** Verify both JWT and API key paths work; confirm handlers remain scheme-agnostic
5. **Docs/scopes:** Ensure API key credentials can represent the same scopes as JWT claims

## Open Questions
- Should API keys use `Authorization: Bearer <key>` or a custom header like `X-API-Key`?
- Where/how are API keys stored and validated (database lookup, config, etc.)? Not yet defined.
- Should API keys support the same scope model as JWT claims?

## Context to Preserve
- User is new to the `api` service; explicit preference for step-by-step understanding before code
- Codebase uses chi v5, golang-jwt v5, context patterns for auth attachment
- Handlers check scopes via `id.HasScope()` — any new verifier must populate this correctly
- No changes have been made to any files yet; codebase is in original state
