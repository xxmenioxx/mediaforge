## Context

See `proposal.md` for the motivation and scope. The current implementation
shows why request and artifact lifecycle must be separated:

- `backend/internal/handlers/compatible_preview_requests.go` normalizes a
  `compatiblePreviewRequest`, hashes its JSON in
  `compatiblePreviewRequestIdentity`, and stores the request in an in-memory
  mutex-protected map with two-hour access TTL and a 500-item bound.
  `CreateCompatiblePreviewRequest` returns the same hash as both `requestId`
  and `cacheIdentity`. Resource handlers load that record and pass the
  operation to `serveCompatiblePreview`.
- `serveCompatiblePreview` in `backend/internal/handlers/assets.go` builds
  FFmpeg arguments, calls `previewCacheKey` with the resolved path, source
  size/modification time, and argument list, and then calls
  `generateCachedPreview`. For QSV inspection it changes `-loglevel` to
  `verbose` before the key is calculated. For `Ephemeral` it appends a
  timestamp to the key. The same function then uses the generated artifact for
  video, inspection, metrics, and output-frame work; source-frame work can use
  the source path directly.
- `generateCachedPreview` uses `previewCacheState.inFlight` and waits on a
  shared `done` channel, but starts `ffmpeg` with the first caller's HTTP
  context. It captures stdout with `cmd.Output`, writes a `.partial` file,
  renames it, and separately publishes `<artifact>.encoder.log`. Inspection
  reads that log for QSV evidence.
- `frontend/src/api/client.ts` accepts an arbitrary string in
  `compatiblePreviewRequestPath`; the JSON helpers use the registered routes,
  while `compatiblePreviewPath` remains a legacy scalar URL and rejects a
  structured profile. `ProfileLabPage.tsx` creates request IDs for inspection,
  metrics, and frame comparisons, but it also has a scalar reference fallback
  and passes state-held IDs into video/frame URL helpers.

The design must keep the existing effective video/profile resolver as the
authority, keep large artifacts on disk for the NAS, and avoid adding request
record persistence. After a restart, registered request IDs may be unavailable
until issue #14 addresses persistence.

## Goals / Non-Goals

**Goals:**

- Give each registered request a stable handle and each encoded result a
  canonical, operation-independent artifact identity.
- Reuse one generation across concurrent and sequential video, inspection,
  metrics, and output-frame consumers.
- Make consumer detachment, backend cancellation, and actual FFmpeg failure
  observably different, retryable states.
- Preserve bounded QSV diagnostics and existing video/range serving behavior
  without using diagnostic verbosity as an encode key.
- Keep request URLs safe at the frontend state boundary and preserve
  credentialless transport.
- Route legacy scalar previews through the same artifact primitive.

**Non-Goals:**

- Persisting request records or request identity across restart (#14).
- Preview-first LAB orchestration, selected-primary-window policy, sequential
  Fidelity windows, or background Fidelity work (#19).
- Slow x265 policy, timeout UX, or messaging (#15).
- Changing HEVC policy or inventing encoder capabilities (#12).
- Creating a distributed job engine, a general media cache, or a second video
  resolver.

## Decisions

### 1. Use a two-level identity model

The implementation will keep four explicitly different concepts:

1. `requestId` identifies a normalized registered request record and is the
   only identifier accepted by registered resource URLs. For non-ephemeral
   requests it remains deterministic for equivalent normalized intent while
   the in-memory record exists. An ephemeral registration receives a stable
   handle for its own scope and may receive a fresh handle for each independent
   registration.
2. `canonicalEncodeSignature` is an internal canonical hash of the effective
   execution description. It is not a request URL and is not formed from
   operation-specific presentation arguments.
3. `artifactId` is the opaque identity of the disk artifact. The existing
   response field `cacheIdentity` will describe this artifact identity/scope,
   not alias `requestId`. A persistent artifact ID is derived from the
   canonical signature; an ephemeral artifact ID also includes the registered
   request scope.
4. `retentionPolicy` describes whether the artifact participates in the
   bounded persistent preview cache or is request-scoped and removed after its
   request/consumer retention window.

Registration will perform the bounded canonical preparation needed to expose
the current `cacheIdentity` without running FFmpeg. That preparation uses the
existing effective resolver and a source fingerprint, not a new video policy
resolver. The request record stores the prepared descriptor. Every acquisition
revalidates source revision evidence under the request lock; if the source has
changed, it replaces the descriptor with a new signature/artifact ID rather
than reusing old bytes. The response field is informational and is never used
as the request URL handle or as permission to bypass current preparation.

This preserves the useful deterministic request behavior while making source
revision changes safe. It also prevents an API consumer from confusing a
request record that has expired with an artifact file that may still exist on
disk.

**Alternative considered:** Continue using the request hash as the artifact
key. Rejected because source revision, effective resolver output, and artifact
retention scope can change independently of normalized request intent.

### 2. Extract one canonical preparation boundary from the existing path

The existing `serveCompatiblePreview` path will be organized around a shared
preparation result rather than duplicated policy. Conceptually it will be:

```text
registered request
    -> normalize and resolve source/profile/effective video decision
    -> capture source revision and canonical encode description
    -> canonicalEncodeSignature + artifactId + retentionPolicy
    -> acquire or generate artifact
    -> consumer-specific serve/inspect/metrics/frame analysis
```

The effective profile, encoder, filters, cadence, normalization, subtitle
selection, rate-control, and frame-structure decisions continue to come from
the existing backend resolver and capability evidence. The preparation result
must describe the actual execution semantics, not merely copy the request
fields or stringify mutable command presentation.

The canonical description includes, when applicable:

- resolved source path and source revision/fingerprint;
- effective start, duration, input seek/preroll behavior, and output format;
- effective profile and encoder, including codec arguments and rate control;
- video filters, cadence/output FPS, geometry/upscale, pixel/color/output
  normalization, and subtitle burn-in selection;
- frame-structure settings and any other effective option that can change
  encoded bytes.

It excludes operation (`video`, `inspect`, `metrics`, `output_frame`), log
  verbosity used only for diagnostics, response headers, HTTP timing, and
  random values. If a runtime capability probe changes the effective command,
  its resolved result is represented in the canonical description through the
  effective decision; no caller-specific probe result is appended as an
  unrelated cache salt.

**Alternative considered:** Hash the final argument array blindly. Rejected
because it currently makes `-loglevel verbose` and ephemeral timestamps create
different artifacts even though neither is an encoded-byte semantic input.

### 3. Make the artifact manager own generation, not an HTTP caller

The shared generation record will be keyed by `artifactId` and contain the
artifact path, state, bounded terminal error, diagnostics metadata, a single
completion signal, and the number of attached consumers. A ready file on disk
is checked before starting work; an in-flight record is then joined by all
concurrent consumers for that identity.

Generation receives its own background-derived context and cancellation
controller. A consumer waits using its own HTTP context and can detach without
canceling the generation. Detaching decrements the waiter count. While another
consumer remains attached, the generation cannot be canceled by the detached
consumer. If no consumers remain, a bounded manager policy MAY cancel an
unneeded generation; that transition is explicitly `canceled`, not `failed`.

The completion signal closes once and carries one terminal result. Waiters
observe that result or their own context cancellation. The implementation must
not use polling or make a waiter hold the first HTTP request's context alive.
Failed and canceled in-flight records are removed after completion, and their
destination is not considered ready, so a later acquisition can retry.

**Alternative considered:** Keep `exec.CommandContext` attached to the first
HTTP request. Rejected because an aborted `/inspect` or browser video fetch can
kill a generation still needed by other consumers.

### 4. Publish artifacts atomically and keep payloads on disk

Each generation writes FFmpeg stdout to an owner-specific staging file under
the preview cache directory. It does not use an unbounded in-memory stdout
buffer. On successful process completion, the staging file is flushed and
atomically renamed to the canonical artifact path; only then does the manager
transition to `ready` and release waiters. The final file must be non-empty and
usable according to the existing serving/probe checks.

Encoder stderr is captured through a bounded sink. The successful bounded log
is published as artifact metadata (retaining the existing `.encoder.log`
compatibility where consumers need it), and failed diagnostics are truncated
to a safe response/log size. A failed or canceled generation removes only its
own staging/log files. Stale staging files are cleaned by bounded cache
housekeeping and are never treated as ready artifacts.

Persistent artifacts remain subject to bounded age/size/item cleanup, excluding
active generations. Ephemeral artifacts use their request-scoped artifact ID
and are removed when the request/consumer retention window ends; they are not
eligible to become a cross-request persistent cache. Both policies keep files
on disk and avoid retaining encoded payloads in Go or browser memory.

### 5. Keep diagnostics independent from encode identity

The registered generation path will decide before FFmpeg starts whether the
bounded encoder evidence needed by inspection consumers must be captured. Since
all registered preview artifacts can be inspected, the simplest repository-
consistent policy is to capture a fixed bounded diagnostic level for the
canonical generation, regardless of which consumer arrived first. The chosen
logging level is execution metadata only and is excluded from
`canonicalEncodeSignature`; it cannot fork `/video` and `/inspect`.

Inspection continues to use `probePreviewCharacteristics`, frame-structure
analysis, and the QSV parser against the ready artifact and its associated
encoder log. Metrics may perform its existing frame analysis against the same
artifact. These operations may fail independently after the artifact is ready,
but they must not invalidate or regenerate the video artifact.

**Alternative considered:** Run a second verbose QSV encode only when
`/inspect` arrives. Rejected because it violates the one-artifact contract and
is especially expensive on the NAS.

### 6. Define consumer routing explicitly

Registered resource handlers will all load and validate the request record,
prepare the current artifact descriptor, and acquire the same artifact before
their operation-specific work:

- `/video` serves the canonical file with existing headers and range behavior.
- `/inspect` probes source and ready output and reads bounded diagnostics.
- `/metrics` performs lightweight metrics against the ready output and may
  retain structured evidence with the artifact.
- `/frame?frame=output` decodes a frame from the ready output artifact.
- `/frame?frame=source` uses the source path and source-side normalization
  directly and does not require a video artifact.

This makes the source/output distinction explicit while guaranteeing the four
encoded-output consumers share one generation. Request expiry is independent
of an unrelated ready persistent artifact: expiring a request removes its
handle, not an artifact still owned by another valid request.

### 7. Preserve one legacy endpoint boundary

The legacy scalar handler will keep its public query shape and continue to
reject structured profiles in the frontend transport helper. Internally it
will call the same canonical preparation and artifact acquisition primitive,
including the same operation-independent identity and atomic publication.
Because it has no registered request handle, it cannot promise cross-resource
sharing across separate legacy HTTP calls; identical persistent signatures may
still hit the shared disk artifact. Its behavior and this limitation will be
documented as compatibility-only. It must not grow a separate cache or FFmpeg
command path.

### 8. Enforce the frontend request-ID boundary

`compatiblePreviewRequestPath` and all registered URL helpers will validate a
trimmed, non-empty ID before interpolation. The public helper contract will
make it difficult to pass an optional state value directly; an invalid ID
fails locally and does not issue a fetch. Components will render request-scoped
video/frame resources only when their ID state is valid, otherwise they show
the existing pending/empty state or a local validation state.

`ProfileLabPage` will clear processed request IDs and request-scoped Fidelity
window data when the material draft/request signature changes. New IDs become
usable only from the successful `createCompatiblePreviewRequest` result. The
legacy scalar reference fallback may remain only where it is intentionally a
non-request-scoped compatibility URL; structured profile conversion,
inspection, metrics, and source/output frame resources require registered IDs.
The cancellation path will preserve the existing `AbortError` suppression and
must not convert an intentional detach into a generic preview error. The
shared `request` helper and direct preview fetches retain `credentials: omit`.

**Alternative considered:** Redesign Profile Lab around a new preview
orchestrator in this change. Rejected because #19 owns preview-first and
sequential Fidelity orchestration; this issue only hardens the ID/resource
boundary around existing flows.

## Risks / Trade-offs

- [Risk] Canonical preparation may make request creation perform more bounded
  resolver and source-stat work than today. → Keep it free of FFmpeg encoding,
  reuse the existing resolver, and defer only work that cannot be resolved
  without execution while still refusing to claim a false artifact identity.
- [Risk] A source can change between registration and generation. → Recheck
  revision evidence under the request/artifact lock and replace the descriptor
  rather than serving an old artifact.
- [Risk] Richer diagnostic stderr can consume NAS memory or disk. → Use a
  bounded sink, publish only bounded metadata, and include log cleanup in the
  same cache policy.
- [Risk] Separating waiter cancellation from generation cancellation can leave
  an encode running after the browser leaves. → Track attached consumers and
  apply a bounded zero-consumer cancellation policy; never let one waiter
  cancel a generation with remaining consumers.
- [Risk] Legacy callers may expect operation-specific ephemeral behavior. →
  Preserve the scalar route and query shape, document its compatibility-only
  sharing boundary, and apply the same canonical artifact policy.
- [Risk] Existing frontend tests may rely on passing IDs as plain strings. →
  Keep valid URL output unchanged, add local invalid-ID coverage, and preserve
  existing credentialless and abort behavior.

## Migration Plan

1. Add canonical identity/preparation and lifecycle result types beside the
   current Compatible Preview helpers, reusing the existing effective profile
   and FFmpeg planning logic.
2. Move registered and legacy consumers to the common artifact acquisition
   primitive; preserve route shapes, JSON fields, range serving, and the
   scalar structured-profile boundary.
3. Replace caller-owned generation context and in-memory stdout capture with
   manager-owned disk staging, bounded diagnostics, atomic publication, and
   bounded cleanup.
4. Add backend concurrency, cancellation, identity, source-revision, and
   operation-reuse regressions, then add frontend request-ID, stale-state,
   cancellation, and credentialless transport regressions.
5. Run the strict OpenSpec validation required for this planning change. The
   application verifier belongs to implementation review and is not required
   for this spec/design-only pass.

Rollback is a code revert only. No database migration, request-record
backfill, source-media rewrite, publication change, or restart persistence
migration is introduced. Existing in-memory request records may disappear on
restart until #14 is implemented.

## Open Questions

None. The contract intentionally leaves internal state type names, exact HTTP
status codes for cancellation, and the concrete bounded diagnostic level to
implementation, but those choices cannot alter the specified identity,
single-flight, atomic publication, or cancellation semantics.
