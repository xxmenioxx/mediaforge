## 1. Canonical request preparation and identity

- [ ] 1.1 Define the request-record, canonical encode signature, artifact ID/cache identity, and retention-policy data contracts; verify that request IDs are never empty and the response no longer aliases request identity to artifact identity.
- [ ] 1.2 Refactor Compatible Preview preparation to reuse the existing effective profile/video resolver and record source revision evidence without running FFmpeg during identity calculation; verify the resolved execution description includes all encode-semantic inputs listed in the spec.
- [ ] 1.3 Implement deterministic normalization/identity coverage for equivalent non-ephemeral requests and isolated stable IDs for independent ephemeral registrations; verify focused backend tests cover defaults, ordering/normalization, and request TTL/size cleanup.
- [ ] 1.4 Add canonical artifact identity tests for source path/revision, start/duration, effective profile/encoder/codec args, filters/cadence/FPS, geometry/color/subtitles, rate control, and frame structure; verify consumer operation, diagnostic verbosity, HTTP details, and timestamps do not alter the identity.
- [ ] 1.5 Add source-revision invalidation tests; verify a changed source fingerprint cannot reuse the old artifact identity or bytes and that an unrelated ready artifact is unaffected.

## 2. Shared artifact lifecycle and publication

- [ ] 2.1 Introduce the bounded disk-backed artifact lifecycle and single-flight registry keyed by artifact identity, with explicit registered/generating/ready/canceled/failed/expired outcomes; verify concurrent callers join one generation and ready callers perform no encode.
- [ ] 2.2 Detach artifact generation from the first HTTP request context and implement independent waiter attach/detach behavior; verify one canceled waiter does not cancel a generation still needed by another waiter and does not become a generic encode failure.
- [ ] 2.3 Implement explicit backend-generation cancellation classification and retry cleanup; verify canceled generations are distinguishable from real FFmpeg failures and a later request can retry successfully.
- [ ] 2.4 Replace unbounded FFmpeg stdout capture with disk staging and bounded stderr/diagnostic capture; verify the artifact payload remains on disk and diagnostic/error output is bounded.
- [ ] 2.5 Implement owner-specific staging, atomic final publication, stale partial cleanup, and ready-file validation; verify canceled/failed generations never publish partial files and concurrent waiters observe one terminal result.
- [ ] 2.6 Apply bounded persistent and request-scoped ephemeral retention cleanup; verify ephemeral consumers of one request share one artifact, independent ephemeral requests do not receive unintended persistent reuse, and expiry does not delete an unrelated valid artifact.

## 3. Consumer routing and diagnostics

- [ ] 3.1 Route registered `/video`, `/inspect`, `/metrics`, and output-frame consumers through one canonical artifact acquisition path; verify `/video` then `/inspect`, `/inspect` then `/video`, concurrent metrics/video, and concurrent output-frame/inspect/video each perform exactly one encode.
- [ ] 3.2 Keep source-frame generation on the source asset and output-frame generation on the ready artifact; verify source-frame requests do not require an output encode while output-frame requests reuse the canonical artifact.
- [ ] 3.3 Preserve bounded encoder diagnostics as artifact metadata and reuse them for QSV inspection; verify QSV inspect/video identity parity and that inspection/metrics analysis never launches a second video encode.
- [ ] 3.4 Preserve existing video response headers and range serving after artifact publication; verify repeated ready consumers are cache hits and no in-memory artifact payload is required.
- [ ] 3.5 Move the legacy scalar Compatible Preview endpoint onto the same preparation, identity, single-flight, and publication primitive; verify scalar video/inspection do not fork artifact identity and structured profiles still require registered requests.

## 4. Request endpoint errors and compatibility behavior

- [ ] 4.1 Harden registered resource endpoints for empty, unknown, expired, and post-restart request IDs; verify they return clear not-found/expired errors without starting generation and retain the bounded in-memory persistence boundary owned by #14.
- [ ] 4.2 Preserve the public create/resource response fields and document the new `cacheIdentity` meaning; verify valid registration returns non-empty request and artifact identities, expiration information, and the normalized path.
- [ ] 4.3 Add focused backend tests for actual FFmpeg failure, bounded diagnostics, retry after failure, retry after cancellation, and corrupt/partial artifact exclusion; verify real failures remain failures rather than cancellation results.

## 5. Frontend request-ID lifecycle

- [ ] 5.1 Add validated request-ID guards to `frontend/src/api/client.ts` registered URL helpers and JSON resource fetches; verify no helper can construct `/requests//video`, `/requests//inspect`, `/requests//metrics`, or `/requests//frame`.
- [ ] 5.2 Update Profile Lab preview/frame state so resource components and fetches remain disabled until request creation succeeds with a non-empty ID; verify pending and failed creation states never render a request-scoped URL.
- [ ] 5.3 Clear stale request IDs and request-scoped fidelity windows when material preview/profile inputs change; verify a new configuration cannot reuse an ID created for the previous configuration while replacement registration is pending.
- [ ] 5.4 Preserve cancellation handling in preview/fidelity consumers; verify aborted consumers detach without a misleading generic error while real transport/encode errors remain visible.
- [ ] 5.5 Preserve credentialless behavior for request creation and resource/fidelity fetches; verify the existing `credentials: omit` contract remains intact for both the shared request helper and direct video preview fetches.

## 6. Verification and handoff

- [ ] 6.1 Run the focused backend and frontend test suites covering identity, lifecycle, shared consumers, cancellation, diagnostics, legacy compatibility, and request-ID guards; verify all new acceptance scenarios pass.
- [ ] 6.2 Run `openspec validate compatible-preview-lifecycle-cache --type change --strict` and verify the change reports valid with all planning artifacts present.
- [ ] 6.3 For implementation completion, run the repository-prescribed `./scripts/verify.sh --auto`, inspect only reported failures, and verify the final result is `ALL CHECKS PASSED` before updating task checkboxes.
