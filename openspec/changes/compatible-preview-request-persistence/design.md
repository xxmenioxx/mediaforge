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

The prepared descriptor audit found that existing video, frame, inspect, and
metrics paths consume the resolved source path, source revision, start,
duration, preview mode, normalization, requested/effective profile data,
requested/effective encoders and QSV rate-control values, resolved video
decision, artifact/cache identity, final path, and the canonical FFmpeg
argument array. Inspection also exposes the argument array and uses the
requested/effective profile snapshots for QSV/frame-structure reporting. The
frozen snapshot therefore preserves these bounded, serializable values rather
than re-running mutable policy resolution after restart.

## Goals / Non-Goals

**Goals:**

- Preserve persistent request IDs, their frozen request-to-artifact
  relationships, and bounded expired-vs-unknown semantics across restart.
- Recover valid final artifacts without re-encoding and reconcile metadata/file
  mismatches conservatively.
- Protect durable state transitions with a generation-attempt identity while
  retaining #1 runtime entry-object protection.
- Keep #1's canonical identity, shared single-flight, non-joinable terminal
  generations, bounded cancellation handoff, and atomic publication semantics.
- Make interruption, stale artifacts, expiration, failure, cancellation, and
  unknown IDs distinguishable to API consumers.
- Keep startup work NAS-safe: bounded database reads and direct stat/path/
  identity checks, with deeper validation deferred until a request is consumed.

**Non-Goals:**

- Persisting or resuming an FFmpeg process, context, goroutine, PID, waiter,
  timer, mutex, or cancellation handle.
- Replacing #1's request/artifact lifecycle engine or inventing a new encode
  signature algorithm.
- Persisting ephemeral request-scoped previews; they intentionally remain
  process-scoped and may disappear at restart.
- Running ffprobe, decoding, or scanning the media library for every restored
  preview at startup.
- Treating expired tombstones as historical audit records.
- Implementing #15 slow x265 timeout UX or #19 LAB orchestration.

## Decisions

### 1. Use dedicated SQLite/GORM recovery metadata tables

Persist two related durable records rather than an AppSetting-backed map:

- `CompatiblePreviewRequestRecord`: request ID primary key; normalized request
  identity/payload needed for explicit deterministic recreate; request state
  (`active`, `stale`, or `expired_tombstone`); prepared snapshot version and
  bounded JSON; artifact ID; created/updated/accessed/expired timestamps;
  tombstone expiration; and bounded recovery reason. The active row owns the
  frozen request-to-artifact relationship. When it becomes a tombstone, retain
  only the request ID, normalized identity, expiration/tombstone timestamps,
  and bounded reason; clear the prepared snapshot and artifact reference so a
  tombstone does not keep cache artifacts alive.
- `CompatiblePreviewArtifactRecord`: canonical artifact ID primary key;
  canonical encode signature; controlled final cache path; source-revision
  evidence; retention policy; current generation ID/token and generation
  version; generation state (`generating`, `ready`, `failed`, `canceled`,
  `interrupted`, or `stale`); bounded recovery reason; and created/updated/
  last-used/cache timestamps. Request rows reference this identity rather than
  copying the artifact lifecycle.

The generation token is an opaque per-attempt identity, such as a UUID, and
the generation version is monotonically increased for each replacement. The
current artifact row stores the token/version for the generation that owns
the row now. A separate historical attempt table is not required for this
ephemeral cache; the token is the compare-and-swap guard, not audit history.

The frozen prepared snapshot is versioned and bounded. Its minimum contract is
the values required to reconstruct the existing `compatiblePreviewDescriptor`
for supported consumers: resolved source path and source revision; artifact ID,
canonical encode signature, and final cache path; start, seconds, preview mode,
and normalization decision; requested/effective preview profile snapshots
where used; resolved video decision; requested/effective video encoder;
requested/effective QSV rate control; and the canonical FFmpeg argument array
needed by inspection output and any fresh generation that explicitly adopts
that snapshot. It excludes contexts, waiters, timers, goroutines, process
state, and unbounded encoder logs.

A dedicated table is preferred over an AppSetting map because request and
artifact rows need independent updates, indexed expiration/identity lookups,
shared-artifact reference checks, and conditional generation transitions. An
AppSetting map would require whole-value read/modify/write operations, make
concurrent cleanup vulnerable to lost updates, and make TTL cleanup scale with
the entire serialized map. The asset-override mutex from #4 is unrelated and
is not reused. Sidecars are rejected because they create a second source of
metadata truth and require unsafe cache scans.

The tables are added to the existing migration list. Migration is additive:
old installations simply have no request rows, and no old cache file is
deleted or renamed solely because the tables are introduced.

### 2. Keep the runtime registry authoritative for live work

Durable rows are recovery metadata and the restart handoff boundary, not a
second lifecycle engine. The existing in-memory request registry remains the
runtime lookup/index; the existing artifact registry remains the owner of
single-flight, consumer leases, generation contexts, cancellation grace,
runtime entry identity, and terminal generation replacement. Runtime
transitions persist the minimal corresponding observation in a transaction,
but consumers still acquire artifacts through #1.

The durable generation token supplements runtime object identity. Every
generation-owned durable transition carries the expected artifact ID and
generation token. A transition for generation N that arrives after the row has
been replaced by generation N+1 affects zero rows and cannot overwrite N+1.
The conditional update reports whether it matched the current attempt. Only a
matching durable transition from the still-current runtime artifact entry may
propagate failed/canceled recovery state to request entries; a zero-row stale
completion is ignored for request lifecycle purposes as well as artifact state.

Request and artifact identity remain separate. A request row can point to an
artifact row that has other request references, preserving the relationship:

```text
request A ─┐
request B ─┼─> artifact X / generation N
request C ─┘
```

The implementation reuses the existing normalized request identity,
`compatiblePreviewEncodeSignature`, and `compatiblePreviewArtifactID`. It
does not hash diagnostics, HTTP operation, waiter state, generation token, or
restart metadata into a new artifact identity.

### 3. Preserve the frozen relationship and distinguish explicit recreate

At initial persistent request creation, the prepared descriptor is serialized
into the request record after canonical preparation establishes the artifact
relationship. On restart, the runtime request entry is rebuilt directly from
that snapshot. It does not resolve ProfileID, load a mutable profile, or
re-run policy/capability resolution merely because memory was lost.

An active request POST that is equivalent to its existing deterministic ID is
idempotent/touch-only and preserves the frozen snapshot. A POST that explicitly
recreates a stale, interrupted, failed, or canceled request is allowed to
prepare current inputs and replace the snapshot/relationship before normal #1
acquisition. Recreate preparation is performed in a candidate entry outside
the runtime registry; durable persistence and runtime installation commit the
replacement only after preparation succeeds. If preparation or persistence
fails, the prior recovery entry remains addressable and a prior tombstone
remains `expired`, rather than becoming `unknown`.

A POST whose deterministic ID finds an expired tombstone replaces the tombstone
with a fresh active row, clears recovery state, resets the
created/access/expiration timestamps, and prepares the current relationship.
This is the only path that lets profile/configuration changes affect a
request with that deterministic ID.

If the source revision in a restored snapshot no longer matches, the request
is stale and the old artifact is never served. A resource GET does not
re-resolve or regenerate. Explicit recreate may resolve current source and
profile/configuration values and may map the same request ID to artifact Y.

### 4. Use generation tokens for durable attempt replacement

Before starting a new runtime generation, persist a new generation token and
version for the artifact row in a transaction. Marking ready, failed,
canceled, interrupted, or stale is conditional on the token that initiated
the transition. Final publication still uses #1's same-directory partial-file
rename and ready-file validation; publishing a final file does not permit an
old generation to update a newer durable row.

The lifecycle distinction is explicit: `artifactID` identifies the canonical
shared encoded result, while `generationID` identifies one attempt to produce
it. Failed/canceled/interrupted generation N is terminal and non-joinable, but
explicit retry may replace the current attempt with generation N+1 for the
same artifact ID. If explicit preparation changes the canonical effective
descriptor, the request relationship changes to a different artifact ID
instead.

### 5. Use bounded expired tombstones

The active request limit remains 500. When access expiration is reached, or an
active LRU entry must be evicted to enforce that limit, the request transitions
to `expired_tombstone` rather than being deleted immediately. The tombstone
retention is two hours from the transition and the tombstone bound is 500
rows. Tombstones are indexed by request ID and hydrated into a bounded lookup
index at startup; they retain only enough metadata to return `410 expired` and
to recognize the deterministic explicit-create replacement.

Tombstone cleanup deletes rows after the deadline and evicts the oldest
tombstones when the bound is exceeded. Either purge path makes the ID
`unknown_request` (`404`). Tombstone cleanup does not delete an artifact merely
because the request expired; artifact cleanup considers only active references,
leases/generations, and the existing persistent cache policy.

The request lookup paths are intentionally separate:

- resource GET reads active/recovery entries, tombstones, or unknown state and
  never reactivates anything;
- explicit POST creation computes the deterministic ID, replaces an expired
  tombstone when present, and invokes current preparation only for a new or
  explicitly recreated lifecycle.

### 6. Reconcile in a fixed startup order before routes serve

Startup runs the following sequence after database opening and migration and
before normal preview routes accept traffic:

1. Convert active rows whose absolute `expiresAt` has elapsed into expired
   tombstones, clearing their frozen snapshot/artifact reference; purge
   tombstones past their deadline and enforce the 500/500 active/tombstone
   bounds.
2. Load the bounded active request rows and tombstones, derive unique artifact
   IDs from the active/stale request rows, and load only those referenced
   artifact rows. Unreferenced historical artifacts remain available to normal
   acquisition/cleanup paths but are not eagerly restored. Do not enumerate
   the media library or run ffprobe.
3. For each active request, deserialize and validate the frozen snapshot,
   source revision evidence, artifact identity, and canonical final path. The
   cleaned absolute path must remain under the controlled Compatible Preview
   cache root, have the expected artifact filename, and be a regular non-empty
   final file before it can support ready recovery. Partial, staging, log-only,
   zero-byte, and unrelated files are never ready evidence.
4. Reconcile the current artifact generation using the durable token: valid
   final file plus `generating` promotes that same generation to `ready`; no
   valid final file changes it to `interrupted`; `ready` without a valid file
   becomes `stale`; failed/canceled remain terminal. Each write is conditional
   on the current generation token.
5. If source revision or frozen snapshot validation fails, mark the request
   stale/recoverable and do not restore a ready runtime entry or re-resolve
   mutable configuration. If the artifact is valid, hydrate one runtime ready
   entry from the frozen snapshot and artifact metadata.
6. Publish the restored runtime request/artifact registries and only then allow
   normal cleanup/route traffic under the reconciliation gate. Missing files
   between stat and use are revalidated by the serving path and become stale
   rather than being served.

The startup pass performs direct metadata/stat/path/identity checks only. Full
inspection, ffprobe, decoding, and source analysis remain lazy at consumption.

### 7. Define restart, retry, and API state mapping

The durable state describes recovery, while #1 continues to own live artifact
generation. Resource endpoints retain their existing paths and successful
media responses. Lifecycle responses use stable JSON with `code`, `state`, a
bounded `reason` where available, and `retryable`/`recreate` flags:

| Condition | HTTP behavior | Recovery meaning |
| --- | --- | --- |
| Valid ready request/artifact | Existing `200` media/inspection response | Reuse frozen relationship; no encode |
| Unknown request ID | `404` with `unknown_request` | Never known or tombstone already purged |
| Expired tombstone | `410` with `expired` and recreate true | Explicit creation can reactivate the deterministic ID |
| Missing/unsafe/identity-mismatched artifact or source revision | `409` with `stale_artifact` | Old relationship is not served; explicit recreate is required |
| Restarted generation without valid final file | `409` with `interrupted` | Generation owner is gone; explicit retry creates a new token |
| Failed or canceled current generation | `409` with `failed` or `canceled` | Current attempt is terminal; explicit retry is required |

An explicit retry/recreate repeats request creation with current normalized
options. It may reuse the same deterministic request ID, but it creates a new
generation token or a new artifact relationship before #1 acquisition. A
resource GET never starts that replacement. Cancellation after restart is
state-only: there is no dead process to cancel; a newly retried generation gets
a new runtime cancellation owner through #1.

### 8. Make database/filesystem transitions crash-tolerant

Persist request/artifact generation metadata before starting runtime work where
possible, publish the final file atomically, then conditionally persist ready.
These operations cannot be treated as one cross-system transaction, so
reconciliation is idempotent:

- final valid + current durable generating token => promote that token to
  ready;
- final absent/invalid + current durable generating token => interrupted;
- durable ready + final absent/invalid => stale;
- durable failed/canceled => preserve the terminal current token unless
  explicitly replaced or expired;
- a late transition from an old token updates zero rows after replacement;
- cleanup records a safe state before deletion when possible, and missing files
  are repaired to stale on the next pass if deletion wins the race.

Only final paths are persisted. Existing partial-file cleanup remains bounded
and skips active runtime paths; restart cleanup may remove stale partials under
the controlled preview root but never a path merely because it appeared in a
request row.

### 9. Test through real lifecycle boundaries

Tests should construct a persistent SQLite database and controlled temporary
preview root, create prepared records/artifacts through the existing request/
artifact entry points, clear runtime registries to simulate a process restart,
run the startup reconciler, and exercise actual resource endpoints. Deterministic
coordination primitives (channels, barriers, or injected hooks) must prove
generation N's late transition cannot overwrite N+1; arbitrary sleeps are not
acceptable.

The suite must cover ready reuse/no encode, shared artifact reuse, tombstone
`410` then purge to `404`, explicit recreate after expiration, frozen
ProfileID/configuration relationship preservation, recreate using new profile
configuration, source-revision staleness, generation interruption and token
replacement, complete-file promotion, failed/canceled retention, expiration,
unsafe paths, partial/zero-byte files, cleanup races, and bounded startup.
Frontend tests cover explicit recovery payloads, polling stop/retry
presentation, ID clearing, and cancellation without knowing the storage model.

## Risks / Trade-offs

- [Risk] SQLite metadata and filesystem publication can disagree after a hard
  crash. → Make reconciliation idempotent, trust only final validated files,
  and normalize every mismatch to a safe state using the current generation
  token.
- [Risk] A late generation callback could overwrite a retry. → Require the
  artifact ID plus generation token in every durable update and retain #1
  runtime object-identity checks.
- [Risk] A frozen snapshot can become incompatible after a code/schema change.
  → Version the snapshot, reject unsupported versions as stale/recovery, and
  let explicit recreate prepare the current descriptor.
- [Risk] Persisting a resolved source path can become stale after a library
  move. → Keep source revision/path evidence tied to the frozen relationship,
  revalidate on consumption, and mark the request stale instead of serving old
  bytes.
- [Risk] Startup work grows with old preview records. → Bound active requests
  and tombstones, page metadata, perform only stat/path checks, and defer
  deeper validation.
- [Risk] A shared artifact is removed while another request is restored. →
  Reconcile active references before cleanup and require existing active/lease
  protections for deletion.
- [Risk] A persisted path becomes an arbitrary-file primitive if trusted
  blindly. → Derive/validate the canonical root and artifact filename and
  reject all out-of-root or mismatched paths.
- [Risk] Existing clients treat any non-200 response as a generic preview
  failure. → Preserve successful route behavior and add stable lifecycle codes
  with focused frontend retry/expired handling.

## Migration Plan

1. Add the request/artifact metadata models, frozen snapshot version, generation
   token/version, and additive database migration.
2. Add runtime persistence at the existing #1 request/artifact transitions,
   including conditional generation-token updates, without changing identity,
   single-flight, publication, or cancellation ownership.
3. Add tombstone transition/purge and startup reconciliation before routes
   serve, with bounded cleanup coordination.
4. Restore runtime entries from frozen snapshots; add explicit API lifecycle
   envelopes and the minimal frontend handling for retryable interruption/stale
   states, expired tombstones, and unknown IDs.
5. Run focused restart, generation-race, frozen-relationship, and real-path
   tests, then the repository's prescribed `./scripts/verify.sh --auto`
   validation.

Rollback is code rollback only: the additive tables and rows may remain
unused, and old in-memory behavior will naturally ignore them. Rollback must
not delete cache artifacts, frozen snapshots, or tombstones as a cleanup side
effect.

## Open Questions

- Confirm whether the final deployment mounts the current Compatible Preview
  cache root across container recreation; process-restart recovery is useful
  only while that controlled root survives. This does not change the safety
  rule that missing files are stale.
