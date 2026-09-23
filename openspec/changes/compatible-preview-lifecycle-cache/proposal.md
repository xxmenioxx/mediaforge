## Why

Compatible Preview currently has two identities and two partially coupled
lifecycle scopes: a deterministic in-memory request ID and a later artifact
cache key derived from the rendered FFmpeg arguments. Operation-specific
diagnostics and ephemeral timestamps can therefore cause duplicate encodes,
while the first HTTP caller also owns the FFmpeg context for every waiter.
Issue #1 makes one registered request resolve to one canonical encoded artifact
with consumer-independent lifecycle and explicit cancellation semantics. This
is important on the resource-constrained NAS because `/video`, `/inspect`,
`/metrics`, and output-frame consumers must not multiply FFmpeg work.

## What Changes

- Define the registered Compatible Preview lifecycle and its bounded in-memory
  request-record behavior, without adding restart persistence owned by #14.
- Consume the merged #12/#128 backend effective video decision, including its
  current HEVC Level/Tier result and provenance, without introducing another
  video resolver or changing #12 behavior.
- Distinguish `requestId`, canonical encode signature, `artifactId`/
  `cacheIdentity`, and artifact retention policy in the API and backend model.
- Keep request-record lifecycle independent from artifact-generation lifecycle:
  request expiry removes only the request handle, while canceled/failed
  artifacts leave a valid request eligible to retry.
- Derive one canonical artifact identity from effective encode semantics and
  source revision evidence, independent of consumer operation, diagnostics
  verbosity, HTTP details, or random timestamps.
- Make concurrent `/video`, `/inspect`, `/metrics`, and output-frame consumers
  share one disk-backed single-flight generation and one ready artifact.
- Separate consumer cancellation from backend generation cancellation and real
  FFmpeg failure; publish only complete artifacts and make canceled/failed
  generations retryable.
- Cancel an in-flight generation once its attached-consumer count reaches zero
  (with only a short bounded handoff grace period permitted), while protecting
  ready artifacts and never canceling while another consumer remains attached.
- Keep registry locks limited to in-memory state transitions; preparation,
  filesystem work, probes, analysis, DB access, resolver work, and FFmpeg run
  outside global registry locks and unrelated requests can progress
  independently.
- Bound registration preparation, store its canonical descriptor on the
  request record, and reuse it for consumers unless source revision or another
  material input invalidates it; registration is not a Fidelity workflow.
- Define ephemeral artifacts as request-scoped shared artifacts with bounded
  idle/TTL and cache-limit cleanup, never as per-consumer temporary files or
  persistent cross-request cache entries.
- Preserve bounded encoder diagnostics for QSV inspection as artifact metadata
  without giving `/inspect` a different encode identity.
- Keep source-frame generation on the source asset, while output-frame,
  metrics, and inspection analysis consume the canonical encoded artifact.
- Route the legacy scalar endpoint through the same canonical artifact
  primitive, with its compatibility-only request boundary documented; keep
  structured profiles on registered requests.
- Require the frontend to validate a non-empty request ID before constructing
  or fetching any registered resource URL, reset stale IDs when inputs change,
  and preserve credentialless request behavior.

## Capabilities

### New Capabilities

- `compatible-preview`: Registered Compatible Preview request lifecycle,
  canonical encode/artifact identity, shared single-flight generation,
  cancellation and failure semantics, consumer reuse, and frontend request-ID
  safety.

### Modified Capabilities

None.

## Impact

- Backend request registration in
  `backend/internal/handlers/compatible_preview_requests.go` and canonical
  generation/serving in `backend/internal/handlers/assets.go`.
- Compatible Preview HTTP responses and resource error semantics, while
  preserving range/video serving behavior and existing structured-profile
  registration.
- Frontend transport helpers in `frontend/src/api/client.ts` and the narrow
  Profile Lab preview/frame state boundary in
  `frontend/src/pages/ProfileLabPage.tsx`.
- Disk-backed temporary artifact files, bounded cache cleanup, encoder
  diagnostic metadata, and focused backend/frontend regression tests.

Out of scope are restart persistence (#14), preview-first LAB orchestration and
sequential Fidelity (#19), slow x265 UX/timeout policy (#15), and HEVC policy
behavior owned by #12. No application code is implemented by this change
proposal.
