## Context

See proposal.md for motivation and the capability spec for observable behavior. Publisher currently loads all Queue jobs and filters with the legacy validation summary. The manual handoff already calls the canonical gate; the durable publication attempt rechecks that gate and Result identity, then preflights required sources and frozen destinations.

## Goals / Non-Goals

**Goals:** Expose a bounded read-only projection of manual eligibility, reuse canonical validation and delivery checks, and make override-backed publication an explicit operator action.

**Non-Goals:** Change Validation policy or override scopes, change automatic advancement, alter Queue lifecycle state, or add persistence/migration fields.

## Decisions

- Add a Publisher-specific eligibility endpoint with a server-capped limit over recent completed unpublished jobs. Return a small job projection, one of `ALLOWED`, `ALLOWED_BY_OVERRIDE`, or `BLOCKED`, and the backend's reason.
- Determine authorization with `canonicalRunAllowsPublish(..., false)`. Bind the candidate with the existing Publisher identity resolver, validate required sidecars, and reuse the delivery preflight's read-only checks. Keep the preview separate from publication persistence and discard any in-memory manifest annotations.
- At the manual publish boundary, continue to re-read the persisted Queue job and canonical ValidationRun/Result, then run the existing publication attempt checks before promotion. AutoPublisher continues to pass automatic authority and cannot consume a manual-only override.
- The Publisher page queries eligibility rather than filtering queue summaries. Override-backed candidates open a confirmation dialog and require an operator reason. Successful publication invalidates Publisher eligibility and publication summaries; it does not rewrite or invalidate Validation history.

Alternatives considered: extending `/queue/jobs` would couple a bounded Publisher decision to the general Queue inventory API; using `validationStatus` in the UI would repeat the original authority bug. Persisting eligibility would create stale parallel state, so the endpoint computes it on demand.

## Risks / Trade-offs

- [Eligibility reads inspect a bounded number of Result and sidecar paths] → Cap the candidate count and perform only read operations; no media probing or directory scan.
- [A candidate can become stale after the GET response] → Re-run all authoritative checks on manual publication and keep filesystem preflight at the attempt boundary.
- [Frontend and backend status names can drift] → Share a typed response contract and cover the canonical override and blocked cases in focused tests.

## Migration Plan

No schema migration is required. Deploy the backend endpoint and frontend together; older clients can continue using the existing publish endpoint, which still enforces canonical manual eligibility.
