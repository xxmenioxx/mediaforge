## Context

See `proposal.md` for the motivation. The current #1 implementation has two
process-local registries: request entries keyed by a deterministic request ID
and artifact entries keyed by a canonical artifact ID. Request entries retain
normalized input and a prepared descriptor; artifact entries own the
single-flight FFmpeg context, waiters, cancellation grace, published
destination, and ready/failed/canceled state. The current request retention is
two hours of access-based inactivity with a 500-entry bound. Artifact
publication writes a same-directory partial file, verifies it is non-empty,
then renames it to the final `.mp4`; persistent cache cleanup currently uses a
24-hour age limit plus existing item/size protections.

The API process opens SQLite with GORM migrations before constructing routes.
The existing `AppSetting` model stores whole JSON values under one key, while
the #1 preview registries are not persisted. The preview cache path is derived
from the canonical artifact ID under the configured Compatible Preview cache
root. These facts permit durable recovery metadata without making the database
the owner of live FFmpeg execution.

## Goals / Non-Goals

**Goals:**

- Preserve persistent request IDs and their request-to-artifact relationship
  across a backend restart.
- Recover valid final artifacts without re-encoding and reconcile metadata/file
  mismatches conservatively.
- Keep #1's canonical identity, shared single-flight, non-joinable terminal
  generations, bounded cancellation handoff, and atomic publication semantics.
- Make interruption, stale artifacts, expiration, failure, cancellation, and
  unknown IDs distinguishable to API consumers.
- Keep startup work NAS-safe: bounded database reads and direct stat checks,
  with deeper validation deferred until a request is consumed.

**Non-Goals:**

- Persisting or resuming an FFmpeg process, context, goroutine, PID, waiter,
  timer, mutex, or cancellation handle.
- Replacing #1's request/artifact lifecycle engine or inventing a new encode
  signature algorithm.
- Persisting ephemeral request-scoped previews; they intentionally remain
  process-scoped and may disappear at restart.
- Running ffprobe, decoding, or scanning the media library for every restored
  preview at startup.
- Implementing #15 slow x265 timeout UX or #19 LAB orchestration.

## Decisions

### 1. Use dedicated SQLite/GORM recovery metadata tables

Persist two related rows rather than an AppSetting-backed map:

- `CompatiblePreviewRequestRecord`: request ID primary key, artifact ID/index,
  normalized request payload or canonical request identity, resolved source
  path and source-revision evidence needed to prevent stale reuse, request
  lifecycle state, bounded failure/recovery reason, created/updated/accessed
  timestamps, expiration timestamp, and a schema/version marker.
- `CompatiblePreviewArtifactRecord`: canonical artifact ID primary key,
  encode signature, controlled final cache path, source-revision evidence,
  retention policy, last observed artifact state, bounded failure/recovery
  reason, created/updated/last-used timestamps, and artifact expiration/cache
  metadata. Request rows reference this identity rather than copying the
  artifact lifecycle.

The exact Go field names remain an implementation detail, but the durable
contract is the minimum set above. Staging paths, encoder process details,
diagnostic streams, and live synchronization primitives are not durable
fields. Failure text is bounded using the same diagnostic limit already used
by #1.

A dedicated table is preferred over an AppSetting map because request and
artifact rows need independent updates, indexed expiration/identity lookups,
shared-artifact reference checks, and transactional state transitions. An
AppSetting map would require whole-value read/modify/write operations, make
concurrent cleanup vulnerable to lost updates, and make TTL cleanup scale with
the entire serialized map. The asset-override mutex from #4 is unrelated and
is not reused. Sidecars are also rejected because they create a second source
of metadata truth and require unsafe cache scans.

The tables are added to the existing migration list. Migration is additive:
old installations simply have no request rows, and no old cache file is
deleted or renamed solely because the tables are introduced.

### 2. Keep the runtime registry authoritative for live work

Durable rows are recovery metadata and the restart handoff boundary, not a
second lifecycle engine. The existing in-memory request registry remains the
runtime lookup/index; the existing artifact registry remains the owner of
single-flight, consumer leases, generation contexts, cancellation grace, and
terminal generation replacement. Runtime transitions persist the minimal
corresponding observation in a transaction, but consumers still acquire
artifacts through #1.

Request and artifact identity remain separate. A request row can point to an
artifact row that has other request references, preserving the relationship:

```text
request A ─┐
request B ─┼─> artifact X
request C ─┘
```

The implementation reuses the existing normalized request identity,
`compatiblePreviewEncodeSignature`, and `compatiblePreviewArtifactID`. It
does not hash diagnostics, HTTP operation, waiter state, or restart metadata
into a new artifact identity. Persisted source path/revision evidence is used
to reject reuse if meaningful source inputs changed.

### 3. Persist only persistent request scopes

Non-ephemeral registered requests are persisted. The existing ephemeral
request scope remains memory-only and request-scoped, so restart naturally
invalidates it and does not create durable garbage. Persistent request expiry
continues to use #1's two-hour access-based retention and 500-record bound;
`expiresAt` is stored as an absolute timestamp so a stopped backend cannot
extend a request accidentally.

The persistent artifact row may outlive an individual request to preserve
shared cache reuse. It remains governed by #1's persistent cache cleanup (the
current 24-hour age limit and 500-item/2 GiB bounds), active generation/lease
protection, and reference checks. Artifact rows with no live request and no
valid final artifact are eligible for metadata cleanup; this is ephemeral
cache metadata, not product history.

### 4. Reconcile before accepting normal preview traffic

Startup ordering is:

1. Run the additive database migration.
2. Load and page through non-expired persistent request/artifact metadata in a
   bounded batch. Do not enumerate the media library or probe every preview.
3. For each relationship, validate expiration and the final path before
   publishing it into the in-memory registries. Validation checks that the
   cleaned absolute path is under the canonical preview root, its filename
   encodes the expected artifact ID, it is a regular non-empty final file, and
   persisted identity/provenance agrees with the record. `.partial`, staging,
   log-only, zero-byte, and unrelated files are never ready evidence.
4. If the durable artifact state is `ready` and the final file is valid,
   hydrate one runtime ready entry. If it is missing, invalid, or unsafe,
   persist `stale`/recovery metadata and do not advertise ready.
5. If durable state is `generating`, first promote a valid final artifact to
   ready (the safe crash window). Otherwise mark the generation interrupted;
   discard runtime ownership and do not create a joinable generating entry.
6. Preserve failed/canceled rows as terminal, non-joinable entries until their
   request TTL or normal cache cleanup removes them.
7. Expire/delete eligible request rows and orphaned artifact metadata only
   after the state decision is recorded. Startup cache cleanup and restored
   registry publication use one reconciliation gate so cleanup cannot delete a
   file after it has been advertised ready without the same reconciliation
   path marking it stale.

The startup hook runs after `database.Migrate` and before routes begin serving.
If a file is missing between stat and later use, the normal acquire/serve path
revalidates it and converts the runtime entry to stale rather than serving an
invalid path. Deeper source or media validation is lazy and uses #1's existing
descriptor/source-revision checks.

### 5. Define restart and API state mapping

The durable state describes recovery, while #1 continues to own live artifact
generation states. The resource endpoints retain their existing paths and
successful media responses. Normal lifecycle responses become structured JSON
with a stable `code`, `state`, bounded `reason` when available, and
`retryable` flag:

| Condition | HTTP behavior | Recovery meaning |
| --- | --- | --- |
| Valid ready request/artifact | Existing `200` media/inspection response | Reuse; no encode |
| Unknown request ID | Existing not-found class, machine-readable `unknown_request` | Never existed in this process/database |
| Expired request | `410` with `expired` | Handle is no longer valid; recreate |
| Missing/unsafe/identity-mismatched artifact | `409` with `stale_artifact` | Retryable; never serve path |
| Restarted generation without valid final file | `409` with `interrupted` | Old owner is gone; retry starts fresh #1 generation |
| Failed or canceled terminal generation | `409` with `failed` or `canceled` | Deterministic terminal result; never join old generation |

The exact response envelope should be shared by video, frame, inspect, and
metrics resource handlers. A retry repeats request creation with the current
normalized options (which may yield the same deterministic request ID) and
then uses the normal #1 acquisition path; it does not mutate an old terminal
generation into a live one. The frontend retains no storage-specific logic:
it continues to hold request IDs, treats ready responses normally, stops
polling on explicit terminal/recovery responses, and offers retry/recreate for
interrupted or stale states. Expired IDs are cleared and unknown IDs remain a
distinct reset condition.

Cancellation after restart is state-only: ready cancellation releases the
consumer and follows existing retention; interrupted/failed/canceled requests
have no process to cancel and remain deterministic; a newly retried generation
gets a new runtime cancel owner through #1. No API claims to cancel a dead
FFmpeg process.

### 6. Make database/filesystem transitions crash-tolerant

Persist generation metadata before starting a new runtime generation where
possible, publish the final file atomically in the existing cache directory,
then persist ready. These operations cannot be treated as one cross-system
transaction, so reconciliation is deliberately idempotent:

- final valid + durable generating => promote ready;
- final absent/invalid + durable generating => interrupted;
- durable ready + final absent/invalid => stale;
- durable failed/canceled => preserve terminal state unless explicitly
  expired;
- cleanup state is recorded before deletion when possible, and missing files
  are repaired to stale on the next pass if deletion wins the race.

Only final paths are persisted. Existing partial-file cleanup remains bounded
and must skip active runtime paths; restart cleanup may remove stale partials
under the controlled preview root but never a path merely because it was in a
request row.

### 7. Test through real lifecycle boundaries

Tests should construct a persistent SQLite database and controlled temporary
preview root, create records/artifacts through the existing request/artifact
entry points, clear runtime registries to simulate a process restart, run the
startup reconciler, and exercise the actual resource endpoints. The suite
must cover ready reuse/no encode, shared artifact reuse, generating
interruption, complete-file promotion, missing artifact, failed/canceled
retention, expiration, unsafe paths, partial/zero-byte files, cleanup races,
and bounded startup behavior. Frontend tests cover explicit recovery payloads,
polling stop/retry presentation, and ID clearing without knowing the storage
model.

## Risks / Trade-offs

- [Risk] SQLite metadata and filesystem publication can disagree after a hard
  crash. → Make reconciliation idempotent, trust only final validated files,
  and normalize every mismatch to a safe state.
- [Risk] Persisting a resolved source path can become stale after a library
  move. → Keep source revision/path evidence tied to the canonical signature,
  revalidate on consumption, and mark the request stale instead of serving an
  old artifact.
- [Risk] Startup work grows with old preview records. → Bound the request
  retention/count, page metadata, perform only stat/path checks, and defer
  deeper validation.
- [Risk] A shared artifact is removed while another request is restored. →
  Reconcile references before cleanup and require the existing active/lease
  protections for deletion.
- [Risk] A persisted path becomes an arbitrary-file primitive if trusted
  blindly. → Derive/validate the canonical root and artifact filename and
  reject all out-of-root or mismatched paths.
- [Risk] Existing clients treat any non-200 response as a generic preview
  failure. → Preserve successful route behavior and add stable lifecycle codes
  with focused frontend retry/expired handling.

## Migration Plan

1. Add the request/artifact metadata models and additive database migration.
2. Add runtime persistence at the existing #1 request/artifact transitions,
   without changing identity, single-flight, publication, or cancellation
   ownership.
3. Add startup reconciliation before routes serve and bounded cleanup
   coordination.
4. Add explicit API lifecycle envelopes and the minimal frontend handling for
   retryable interruption/stale states and expired IDs.
5. Run focused restart and real-path tests, then the repository's prescribed
   `./scripts/verify.sh --auto` validation.

Rollback is code rollback only: the additive tables and rows may remain
unused, and old in-memory behavior will naturally ignore them. Rollback must
not delete cache artifacts or metadata as a cleanup side effect.

## Open Questions

- Confirm whether the final deployment mounts the current Compatible Preview
  cache root across container recreation; process-restart recovery is useful
  only while that controlled root survives. This does not change the safety
  rule that missing files are stale.
