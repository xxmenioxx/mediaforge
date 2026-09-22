## Context

See `proposal.md` for the motivation and scope. The current implementation
shows why request and artifact lifecycle must be separate:

- `backend/internal/handlers/compatible_preview_requests.go` normalizes a
  `compatiblePreviewRequest`, hashes its JSON in
  `compatiblePreviewRequestIdentity`, and stores the request in an in-memory
  mutex-protected map with two-hour access TTL and a 500-item bound.
  `CreateCompatiblePreviewRequest` returns the same hash as both `requestId`
  and `cacheIdentity`. Resource handlers load that record and pass an
  operation to `serveCompatiblePreview`.
- `serveCompatiblePreview` in `backend/internal/handlers/assets.go` builds
  FFmpeg arguments, calls `previewCacheKey` with the resolved path, source
  size/modification time, and argument list, and then calls
  `generateCachedPreview`. For QSV inspection it changes `-loglevel` to
  `verbose` before the key is calculated. For `Ephemeral` it appends a
  timestamp to the key. `generateCachedPreview` uses
  `previewCacheState.inFlight`, but starts FFmpeg with the first caller's HTTP
  context and captures stdout with `cmd.Output` before staging the artifact.
- Inspection reads the generated `.encoder.log` for QSV evidence. Metrics and
  output-frame work also consume the generated output, while source-frame work
  can use the source path directly.
- `frontend/src/api/client.ts` accepts an arbitrary string in
  `compatiblePreviewRequestPath`; the JSON helpers use registered routes,
  while `compatiblePreviewPath` remains a legacy scalar URL and rejects a
  structured profile. `ProfileLabPage.tsx` creates request IDs for inspection,
  metrics, and frame comparisons, but it also has a scalar reference fallback
  and passes state-held IDs into video/frame URL helpers.

The branch now includes `origin/main` and merged #12/#128. The current preview
path uses `resolvePreviewVideoProfile`, which composes the established
effective motion and encoding decisions and the merged `resolveHEVCLevel`
behavior, then reports `effectiveVideoDecision` including the effective
encoder and HEVC Level/Tier state. Compatible Preview must consume that current
decision and its evidence; it must not create a second video resolver or alter
#12 policy.

The design must keep request records non-persistent, keep large artifacts on
disk for the NAS, and avoid adding a scheduler/job engine. After a restart,
registered request IDs may be unavailable until #14 addresses persistence.

## Goals / Non-Goals

**Goals:**

- Give request records and encoded artifacts independent lifecycle domains.
- Give each registered request a stable handle and each encoded result a
  canonical, operation-independent artifact identity.
- Reuse one generation across concurrent and sequential video, inspection,
  metrics, and output-frame consumers.
- Cancel an in-flight generation once no consumers remain, with only a short
  bounded handoff grace period, while never canceling while another consumer is
  attached.
- Make consumer detachment, manager cancellation, and actual FFmpeg failure
  observably different and retryable outcomes.
- Preserve bounded QSV diagnostics and existing video/range serving behavior
  without using diagnostic verbosity as an encode key.
- Keep request URLs safe at the frontend state boundary and preserve
  credentialless transport.
- Route the legacy scalar endpoint through the same artifact primitive.

**Non-Goals:**

- Persisting request records or request identity across restart (#14).
- Preview-first LAB orchestration, selected-primary-window policy, sequential
  Fidelity windows, or background Fidelity work (#19).
- Slow x265 policy, timeout UX, or messaging (#15).
- Changing HEVC policy or inventing encoder capabilities beyond consuming the
  merged #12 effective video decision.
- Creating a distributed job engine, a general media cache, or a second video
  resolver.

## Decisions

### 1. Use two independent lifecycle domains

The implementation will keep request records and artifact generations as
separate state machines:

| Domain | States | Meaning |
| --- | --- | --- |
| Request record | `registered`, `expired`/`unknown` | Whether the request handle and its prepared descriptor are available. |
| Artifact generation | `absent`, `generating`, `ready`, `canceled`, `failed` | Whether the canonical encoded artifact exists or a generation attempt has a terminal result. |

These are not a single combined lifecycle. A registered request may point to an
absent artifact, may retry a canceled or failed generation, and may continue to
point to a ready persistent artifact. Expiring a request removes only its
handle and request-record data; it does not delete a persistent artifact that
another valid request can use. Conversely, artifact cancellation or failure
does not expire a registered request.

`ready` is reserved for a complete, atomically published, usable encoded file.
An artifact failure or cancellation removes its in-flight attempt and owned
staging files, but leaves the request record registered until its own TTL/size
policy expires. A new acquisition can create a new generation for that
request. Unknown/expired resource IDs fail before artifact lookup or encode.

**Alternative considered:** Keep one state enum containing registered,
generating, ready, canceled, failed, and expired. Rejected because request
expiry and artifact terminal results have different owners, retention rules,
and retry semantics.

### 2. Prepare one bounded canonical descriptor at registration

Registration will perform the bounded normalization, source revision capture,
and effective profile/video preparation needed to return `cacheIdentity`, but it
will never run FFmpeg. The resulting descriptor is stored on the request
record, including the canonical execution description, source revision, the
effective video decision, artifact identity, and retention policy.

The preparation path will extract or reuse the existing preparation used by
`serveCompatiblePreview`; it will not copy video policy into a Compatible
Preview-specific resolver. In particular, it consumes the current
`resolvePreviewVideoProfile` result and `effectiveVideoDecision`, including
merged #12/#128 HEVC Level/Tier fields, warnings, and provenance that affect
the actual command or its interpretation.

Preparation will reuse persisted analysis and effective-resolver evidence when
available. It must not turn registration into a full Fidelity workflow, a
background operation, or an encode. Acquisitions normally use the descriptor
without repeating the expensive preparation for each `/video`, `/inspect`,
`/metrics`, or output-frame request. A source revision change or another
material invalidation is the boundary that permits recomputation. If a
recomputation is needed, the request record is updated with a new descriptor
and artifact identity; the request ID itself need not change.

**Alternative considered:** Keep registration as a hash-only operation and
recompute all resolver/probe state for every consumer. Rejected because it
duplicates expensive preparation on the NAS and makes `cacheIdentity` unable
to describe the prepared artifact scope.

### 3. Keep four identities explicit

The design keeps four distinct concepts:

1. `requestId` identifies a normalized registered request record and is the
   only identifier accepted by registered resource URLs. For non-ephemeral
   requests it remains deterministic for equivalent normalized intent while
   the in-memory record exists. An ephemeral registration receives a stable
   handle for its own scope and may receive a fresh handle for each independent
   registration.
2. `canonicalEncodeSignature` is an internal hash of the effective execution
   description. It is not a request URL and is not formed from
   operation-specific presentation arguments.
3. `artifactId` is the opaque identity of the disk artifact. The existing
   response field `cacheIdentity` describes this artifact identity or scope;
   it does not alias `requestId`. Persistent artifact IDs derive from the
   canonical signature. Ephemeral artifact IDs also include the registered
   request scope.
4. `retentionPolicy` determines whether the artifact participates in the
   bounded persistent cache or is request-scoped and eligible for cleanup
   after its idle/TTL rules.

The canonical description includes, when applicable, resolved source path and
revision, effective start/duration and seek behavior, profile and encoder,
codec/rate-control/frame-structure arguments, filters/cadence/FPS,
geometry/upscale, color/output normalization, subtitle burn-in selection, and
other inputs that can change encoded bytes. It excludes operation, diagnostic
verbosity, response headers, HTTP timing, and random timestamps.

**Alternative considered:** Hash the final argument array blindly. Rejected
because it currently makes `-loglevel verbose` and ephemeral timestamps create
different artifacts even though neither is an encoded-byte semantic input.

### 4. Separate global lock scope from external work

Global request and artifact registry locks protect only short in-memory state
changes:

- the request registry lock protects map lookup/insert/delete and bounded
  request-record membership;
- a request-entry lock protects its descriptor pointer/version and request-local
  refresh state;
- the artifact registry lock protects artifact-entry lookup/install/removal;
- an artifact-entry lock protects state, waiter accounting, terminal result,
  and publication metadata.

No global registry mutex, and no lock held solely to protect the registry map,
may remain held during filesystem I/O, database queries, ffprobe, media
analysis, effective profile/video resolution, or FFmpeg. Preparation follows a
copy/prepare/update pattern: snapshot the current descriptor/version under a
short lock, perform external work without the global lock, then commit only if
the request version is still current; otherwise discard/retry the descriptor.
Artifact acquisition uses the same pattern for file checks and generation.

Per-entry synchronization prevents unrelated requests from being serialized by
one slow source, probe, resolver, analysis step, or encode. The design does
not add a scheduler or distributed job engine.

**Alternative considered:** Hold the existing shared request mutex from lookup
through preparation and FFmpeg. Rejected because one slow preview would block
unrelated request creation, refresh, and artifact acquisition.

### 5. Make the artifact manager own generation and zero-consumer cancellation

The artifact manager is keyed by `artifactId` and owns the artifact path,
state, bounded terminal error, diagnostics metadata, completion signal, and
attached-consumer count. A valid final file is checked before starting work; an
in-flight entry is joined by all consumers for that identity.

Generation receives its own background-derived context and cancellation
controller. A consumer waits with its own HTTP context and can detach without
canceling generation while another consumer remains attached. When the count
reaches zero, the manager schedules cancellation after an optional short,
bounded handoff grace period. If a new consumer attaches during that grace,
the cancellation is withdrawn. If the count is still zero when grace ends,
the manager cancels the generation. It must not allow a zero-consumer encode to
run indefinitely.

The zero-consumer transition is always `canceled`, never `failed`. The
completion signal closes once and carries one terminal result. Waiters observe
that result or their own context cancellation. If generation finishes
successfully before a pending zero-consumer cancellation takes effect, it
becomes `ready`; detaching afterward cannot cancel or delete that ready file.
Failed and canceled in-flight entries are removed after completion, and their
destination is not considered ready, so a later acquisition can retry.

**Alternative considered:** Keep `exec.CommandContext` attached to the first
HTTP request, or let a zero-consumer generation run to completion. Rejected
because the first allows one browser abort to kill other consumers, while the
second permits orphan FFmpeg work on the NAS.

### 6. Publish artifacts atomically and keep payloads on disk

Each generation writes FFmpeg stdout to an owner-specific staging file under
the preview cache directory. It does not use an unbounded in-memory stdout
buffer. On successful process completion, the staging file is flushed and
atomically renamed to the canonical artifact path; only then does the manager
transition to `ready` and release waiters. The final file must be non-empty and
usable according to existing serving/probe checks.

Encoder stderr is captured through a bounded sink. The successful bounded log
is published as artifact metadata, retaining the existing `.encoder.log`
compatibility where consumers need it. Failed diagnostics are truncated to a
safe response/log size. A canceled or failed generation removes only its own
staging/log files. Stale staging files are cleaned by bounded housekeeping and
are never treated as ready artifacts.

Persistent artifacts remain subject to bounded age/size/item cleanup, excluding
active generations. No cleanup operation may remove an artifact while it is
being served or analyzed.

### 7. Give ephemeral artifacts request-scoped bounded retention

An ephemeral descriptor derives an artifact identity from the canonical
signature plus the registered request scope. This makes all consumers of one
ephemeral request share one artifact while preventing the file from becoming a
persistent cross-request cache entry.

The artifact manager tracks active generation, serving, and analysis references
for ephemeral entries. Cleanup cannot remove an entry while any of those
activities is active. It also cannot delete the file at the end of an
individual HTTP consumer, because sequential consumers of the same request
must reuse it.

Once the request scope has expired and no consumer is active, the ephemeral
artifact becomes eligible for bounded idle/TTL cleanup. It remains subject to
bounded cache item/size limits and may be removed by housekeeping according to
those limits. Exact durations and sizes remain implementation constants.

**Alternative considered:** Append a fresh timestamp for every HTTP consumer
or defer cleanup with no item/size bound. Rejected because the former defeats
same-request sharing and the latter is unsafe on the NAS.

### 8. Keep diagnostics independent from encode identity

The registered generation path decides before FFmpeg starts whether the
bounded encoder evidence needed by inspection consumers must be captured.
Because all registered artifacts can be inspected, a fixed bounded diagnostic
level for canonical generation is acceptable regardless of which consumer
arrived first. The logging level is execution metadata only and is excluded
from `canonicalEncodeSignature`.

Inspection continues to use source/output probes, frame-structure analysis,
the QSV parser, and artifact-associated encoder logs against the ready artifact.
Metrics may perform its existing frame analysis against that artifact. These
operations may fail independently after the artifact is ready, but they must
not invalidate or regenerate the video artifact.

**Alternative considered:** Run a second verbose QSV encode only when
`/inspect` arrives. Rejected because it violates the one-artifact contract and
is especially expensive on the NAS.

### 9. Route consumers and preserve the legacy boundary

Registered resource handlers will load and validate the request record, use its
current prepared descriptor, acquire the artifact, and then perform
operation-specific work:

- `/video` serves the canonical file with existing headers and range behavior;
- `/inspect` probes source and ready output and reads bounded diagnostics;
- `/metrics` performs lightweight metrics against the ready output and may
  retain structured evidence with the artifact;
- `/frame?frame=output` decodes from the ready output artifact;
- `/frame?frame=source` uses the source path and source-side normalization
  directly and does not require an output artifact.

The legacy scalar handler keeps its public query shape and continues to reject
structured profiles in the frontend helper. Internally it calls the same
preparation, artifact acquisition, and atomic publication primitive. It has no
registered request handle for sharing across separate legacy calls, although
identical persistent signatures may hit a ready persistent artifact. It must
not grow a competing cache or FFmpeg command path.

### 10. Enforce the frontend request-ID boundary

`compatiblePreviewRequestPath` and all registered URL helpers validate a
trimmed, non-empty ID before interpolation. The public helper contract makes
it difficult to pass an optional state value directly; an invalid ID fails
locally and does not issue a fetch. Components render registered video/frame
resources only when their ID state is valid, otherwise they show the existing
pending/empty state or local validation state.

`ProfileLabPage` clears processed request IDs and request-scoped Fidelity
window data when the material draft/request signature changes. New IDs become
usable only from successful `createCompatiblePreviewRequest` results. The
legacy scalar reference fallback may remain only where it is intentionally a
non-request-scoped compatibility URL; structured profile conversion,
inspection, metrics, and source/output frame resources require registered IDs.
Cancellation preserves existing `AbortError` suppression and does not convert
an intentional detach into a generic preview error. The shared `request`
helper and direct preview fetches retain `credentials: omit`.

## Risks / Trade-offs

- [Risk] Bounded registration preparation may do more resolver/stat work than
  today's hash-only registration. → Keep it free of FFmpeg and Fidelity work,
  reuse persisted evidence, and reuse the stored descriptor for all consumers.
- [Risk] A source changes between registration and acquisition. → Recheck
  revision evidence with per-entry synchronization and replace the descriptor
  rather than serving old bytes.
- [Risk] A short cancellation grace can briefly retain an unused FFmpeg
  process. → Bound the grace period and cancel deterministically when the count
  remains zero.
- [Risk] Richer diagnostic stderr can consume NAS memory or disk. → Use a
  bounded sink, publish only bounded metadata, and include logs in cleanup.
- [Risk] Per-entry refresh/update races can produce stale descriptors. → Use
  versioned copy/prepare/update commits and discard work whose source version
  is no longer current.
- [Risk] Legacy callers may expect operation-specific ephemeral behavior. →
  Preserve the scalar route and query shape, document its compatibility-only
  sharing boundary, and apply the same artifact policy.
- [Risk] Existing frontend tests may rely on passing IDs as plain strings. →
  Keep valid URL output unchanged, add local invalid-ID coverage, and preserve
  existing credentialless and abort behavior.

## Migration Plan

1. Add separate request-record and artifact-generation state/result types and
   canonical descriptor preparation, reusing the current effective video
   resolver and merged #12 decision.
2. Move registered and legacy consumers to the common artifact acquisition
   primitive; preserve route shapes, JSON fields, range serving, and the
   scalar structured-profile boundary.
3. Replace caller-owned generation context and in-memory stdout capture with
   manager-owned disk staging, bounded diagnostics, atomic publication, and
   deterministic zero-consumer cancellation.
4. Add lock-boundary and unrelated-request concurrency regressions, then add
   identity, source-revision, ephemeral retention, shared-consumer,
   cancellation, diagnostics, legacy, and frontend request-ID regressions.
5. Run strict OpenSpec validation for this planning change. The application
   verifier belongs to implementation review and is not required for this
   spec/design-only pass.

Rollback is a code revert only. No database migration, request-record
backfill, source-media rewrite, publication change, or restart-persistence
migration is introduced. Existing in-memory request records may disappear on
restart until #14 is implemented.

## Open Questions

None. Exact internal state names, lock types, grace duration, HTTP status code
for cancellation, and cache duration/size constants remain implementation
choices, but they cannot alter the two lifecycle domains, bounded preparation,
lock boundaries, zero-consumer cancellation, identity, or retention contract.
