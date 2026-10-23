## Context

See `proposal.md` and `specs/publisher-manual-request-lifecycle/spec.md`. Manual `PublishJob` and the Publisher retry action currently call the full delivery path in the request handler. `PublisherPublication` already owns immutable delivery identity, attempt leases/fencing, recovery state, and the bounded status summary. `AutoPublisher` runs recovery at startup and on its existing periodic cadence.

## Goals / Non-Goals

**Goals:** Persist accepted manual intent beside the publication it owns; route it through existing Publisher claims and recovery; distinguish request acceptance from final delivery; and keep the acceptance path bounded enough to meet the sub-second local ACK target where the environment supports it.

**Non-Goals:** Change canonical Validation authorization, pipeline stage transitions, publication materialization, archive ownership, or automatic-publishing policy. Do not add another Queue, generic job table, in-memory-only task, NAS publish fixture, or external dependency.

## Decisions

### Store the intent on the Publisher identity

Add bounded manual-request fields to `PublisherPublication`: request ID, request state, operator reason, and accepted/completed timestamps. Persist the request fields and append-only audit event in one SQLite transaction. A repeated request while accepted or processing returns the stored request ID. A terminal failure is retried only through the audited retry action, which creates a new request ID; existing audit evidence keeps prior request history.

This reuses the single identity already unique to a Queue execution and Result. A separate request table or generic outbox would create another lifecycle owner and require its own claim/recovery protocol. Existing model migration handles nullable/defaulted columns; old rows remain readable with an empty request state.

### Bound HTTP work to acceptance checks

Before acceptance, reload the persisted Queue job, verify its completed/dismissed state, canonical manual Validation and Result binding, frozen destination identity, and required subtitle artifact declarations. Require a nonempty audited reason and persist the identity/request/audit before returning `202 Accepted`. Keep staged subtitle file verification, expensive media verification, copy/promotion, original archival, and reconciliation in the background worker. Execution repeats the canonical identity and full delivery checks before any mutation.

The successful response includes `status=accepted`, `requestId`, and `publicationId`; it contains no success claim. Existing conflict and blocked responses remain synchronous when a cheap authoritative gate fails or the durable write fails. Measure the route ACK in focused tests/benchmarks using temporary SQLite and file fixtures; the target is p95 <= 1 second where supported.

### Reuse the existing Publisher worker and attempt fence

Have `AutoPublisher.tick` consume accepted manual requests before consulting the automatic-publishing setting. It invokes the existing manual-authority Publisher path, which rechecks eligibility, claims the persisted publication, and performs delivery under the current lease/fencing protocol. The manual request status remains accepted/processing while the publication is active, then records completion or failure from persisted publication state. A hold prevents the worker from advancing the `HELD` row; audited release makes its unchanged pending request eligible again.

After restart, pending request rows are discovered from SQLite. If delivery had already reached `PUBLISHING`, existing lease recovery owns the attempt and the request status is reconciled from the resulting persisted state. Duplicate worker scans are harmless because the publication claim remains the only authority to begin filesystem work. Automatic selection must leave publications with a pending manual request to the manual worker so manual-only overrides cannot be consumed under automatic authority.

### Keep the current read model as progress authority

Include manual request state and request ID in the existing bounded publication summary. Publisher continues polling that summary and renders `Accepted`/`In progress` from persisted request/publication state, and `Published` only from `Publication.state=PUBLISHED`. It shows stored `FAILED`/`BLOCKED` reasons and keeps the explicit retry/resolution actions. No per-job status API or filesystem polling is added.

## Risks / Trade-offs

- [A request can be accepted just before Validation, Result, or destination evidence changes] → Recheck canonical authorization and complete delivery preflight again in the worker before mutation; retain fail-closed behavior.
- [A worker can stop between accepting intent and claiming an attempt] → Keep intent on the existing publication row and scan it at startup and periodically.
- [Legacy clients may interpret the changed HTTP status as completion] → Return an explicit accepted status/body and update the first-party client/UI with separate accepted and published wording; document the endpoint contract change.
- [A long publication delays other manual requests on the existing worker cadence] → Preserve a bounded durable backlog in publication rows and expose each request's accepted state while waiting; do not add parallel NAS writers.

## Migration Plan

Deploy the SQLite model migration and API/UI together. Existing publication rows receive empty request fields and retain all current state/evidence. New manual Publish requests are durably accepted and processed by the existing worker; restart recovery requires no backfill. Rollback leaves the added nullable/defaulted fields unused and existing publication identity intact. No NAS or production data mutation is part of validation.
