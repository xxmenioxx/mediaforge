## Why

Issue #1 made Compatible Preview request and artifact lifecycles explicit, but
the request registry and artifact-generation registry are still process-local.
After a backend restart, a frontend-held request ID becomes indistinguishable
from an unknown request even when the published preview artifact is still
valid, causing avoidable re-encoding and losing useful lifecycle state.

## What Changes

- Persist the minimum identity and recovery metadata for non-ephemeral
  Compatible Preview requests and their shared artifacts, including a frozen
  prepared request-to-artifact snapshot and a durable generation-attempt token.
- Restore persisted request/artifact relationships during backend startup and
  reconcile metadata with expiration, bounded expired tombstones, and the final
  cache file on disk.
- Preserve a valid ready artifact under its existing request ID without a new
  encode; reject unsafe, missing, empty, partial, or identity-mismatched files.
- Normalize persisted `generating` work to an interrupted/recoverable terminal
  state; never restore a dead FFmpeg owner, context, waiter, timer, or goroutine.
- Preserve #1's failed/canceled terminal and non-joinable semantics per
  generation attempt while allowing an explicit retry to create a new attempt
  for the same canonical artifact identity.
- Keep a bounded expired tombstone so an expired deterministic request ID
  returns explicit `410 expired` before eventually becoming unknown.
- Restore the frozen prepared descriptor relationship without re-resolving
  mutable ProfileID/configuration state; explicit recreate/retry is the only
  path that can prepare a current relationship.
- Define explicit API/frontend behavior for ready, expired, stale, interrupted,
  failed, canceled, and unknown request IDs, while keeping the frontend
  independent of the storage mechanism.
- Coordinate startup restore, runtime cleanup, TTL expiration, and artifact
  publication so stale metadata cannot advertise an invalid path.
- Add focused restart, deduplication, tombstone, generation-race,
  frozen-relationship, crash-window, expiration, path-safety, and API recovery
  coverage without changing the #1 encode/cache contract.

Out of scope: resuming a live FFmpeg process, changing #1's canonical encode
identity or shared single-flight rules, implementing #15 slow-work timeout UX,
or implementing #19 LAB preview-first orchestration.

## Capabilities

### New Capabilities

- `compatible-preview-request-persistence`: Durable Compatible Preview request
  identity, artifact recovery metadata, restart reconciliation, and explicit
  recovery semantics for the existing lifecycle.

### Modified Capabilities

None. The existing #1 lifecycle/cache capability remains the behavioral owner;
this change adds restart persistence around it.

## Impact

- Backend SQLite/GORM models and startup recovery wiring.
- Compatible Preview request/artifact registry hydration, cleanup, and path
  validation in the existing handler/cache code.
- Existing request resource responses and frontend polling/retry handling,
  without exposing persistence implementation details.
- Focused backend/frontend tests and OpenSpec validation. No preview cache
  migration or destructive filesystem operation is required.
