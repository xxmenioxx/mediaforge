## 1. Canonical request preparation and identity

- [ ] 1.1 Define separate request-record and artifact-generation state/result contracts; verify request records use only `registered` and `expired/unknown`, artifact generations use `absent/generating/ready/canceled/failed`, and artifact terminal results do not expire registered requests.
- [ ] 1.2 Refactor bounded registration preparation to store a reusable canonical descriptor on the request record without encoding media; verify registration does not run FFmpeg, does not start a Fidelity/background workflow, and later consumers reuse the descriptor instead of repeating expensive preparation.
- [ ] 1.3 Consume the merged #12/#128 effective video decision from the existing authoritative resolver, including current HEVC Level/Tier state and provenance; verify Compatible Preview has no second video resolver and focused tests preserve #12 behavior.
- [ ] 1.4 Implement deterministic normalization/identity coverage for equivalent non-ephemeral requests and isolated stable IDs for independent ephemeral registrations; verify defaults, normalization, request TTL/size cleanup, and non-empty request IDs.
- [ ] 1.5 Add canonical artifact identity tests for source path/revision, start/duration, effective profile/decision/encoder/codec args, filters/cadence/FPS, geometry/color/subtitles, rate control, and frame structure; verify consumer operation, diagnostic verbosity, HTTP details, and timestamps do not alter identity.
- [ ] 1.6 Add descriptor invalidation tests; verify source revision or another material invalidation recomputes the descriptor and artifact identity while an unchanged request reuses its stored descriptor.

## 2. Independent artifact lifecycle and synchronization

- [ ] 2.1 Introduce the bounded disk-backed artifact registry and single-flight generation keyed by artifact identity; verify concurrent callers join one generation, ready means a complete published artifact only, and failed/canceled attempts remain retryable.
- [ ] 2.2 Establish lock boundaries for request and artifact registries; verify global locks cover only in-memory map/state/waiter transitions and are never held during filesystem I/O, DB queries, ffprobe, media analysis, effective resolution, or FFmpeg.
- [ ] 2.3 Add unrelated-request concurrency regression coverage; verify a slow preparation/acquisition for one Compatible Preview request does not block another request's descriptor preparation or artifact acquisition through a global registry lock.
- [ ] 2.4 Detach generation from the first HTTP request context and implement independent waiter attach/detach accounting; verify one consumer detaching cannot cancel a generation while another consumer remains attached and the detached waiter receives cancellation semantics.
- [ ] 2.5 Make zero-consumer behavior deterministic; verify an in-flight generation is canceled when attached-consumer count reaches zero after only a short bounded handoff grace period, the result is `canceled` rather than `failed`, and a later consumer starts a fresh generation.
- [ ] 2.6 Verify ready-artifact detachment behavior; after an artifact reaches `ready`, verify consumer detachment cannot cancel or delete it outside its retention policy.
- [ ] 2.7 Replace unbounded FFmpeg stdout capture with disk staging and bounded stderr/diagnostic capture; verify encoded payloads remain on disk and failure/cancellation diagnostics are bounded.
- [ ] 2.8 Implement owner-specific staging, atomic final publication, stale partial cleanup, and ready-file validation; verify canceled/failed generations never publish partial files and concurrent waiters observe one terminal result.

## 3. Retention, consumers, and diagnostics

- [ ] 3.1 Implement persistent artifact cleanup with bounded age/item/size policy while excluding active generation, serving, and analysis; verify request expiry removes only the request handle and does not delete a valid unrelated persistent artifact.
- [ ] 3.2 Implement request-scoped ephemeral retention; verify all consumers of one ephemeral request share one artifact, independent requests do not reuse it as a persistent cache, active generation/serving/analysis protects it, and cleanup is bounded by idle/TTL plus item/size limits after scope expiry.
- [ ] 3.3 Route registered `/video`, `/inspect`, `/metrics`, and output-frame consumers through one artifact acquisition path; verify video/inspect in both orders, concurrent metrics/video, concurrent output-frame/inspect/video, and repeated ready consumers each perform at most one encode.
- [ ] 3.4 Keep source-frame generation on the source asset and output-frame generation on the ready artifact; verify source-frame requests do not require output encoding while output-frame requests reuse the canonical artifact.
- [ ] 3.5 Preserve bounded encoder diagnostics as artifact metadata and reuse them for QSV inspection; verify QSV inspect/video identity parity and that inspection/metrics analysis never launches a second video encode.
- [ ] 3.6 Preserve existing video response headers and range serving after artifact publication; verify no unbounded artifact payload is required for serving or coordination.
- [ ] 3.7 Move the legacy scalar Compatible Preview endpoint onto the same preparation, identity, single-flight, and publication primitive; verify scalar video/inspection do not fork artifact identity and structured profiles still require registered requests.

## 4. Request endpoint errors and compatibility behavior

- [ ] 4.1 Harden registered resource endpoints for empty, unknown, expired, and post-restart request IDs; verify they return clear not-found/expired errors without generation and retain the non-persistent request boundary owned by #14.
- [ ] 4.2 Preserve the public create/resource response fields and document the new `cacheIdentity` meaning; verify valid registration returns non-empty request and artifact identities, expiration information, and the normalized path.
- [ ] 4.3 Add focused backend tests for actual FFmpeg failure, bounded diagnostics, retry after failure, retry after cancellation, and corrupt/partial artifact exclusion; verify real failures remain failures rather than cancellation results.

## 5. Frontend request-ID lifecycle

- [ ] 5.1 Add validated request-ID guards to `frontend/src/api/client.ts` registered URL helpers and JSON resource fetches; verify no helper can construct `/requests//video`, `/requests//inspect`, `/requests//metrics`, or `/requests//frame`.
- [ ] 5.2 Update Profile Lab preview/frame state so resource components and fetches remain disabled until request creation succeeds with a non-empty ID; verify pending and failed creation states never render a request-scoped URL.
- [ ] 5.3 Clear stale request IDs and request-scoped fidelity windows when material preview/profile inputs change; verify a new configuration cannot reuse an ID created for the previous configuration while replacement registration is pending.
- [ ] 5.4 Preserve cancellation handling in preview/fidelity consumers; verify aborted consumers detach without a misleading generic error while real transport/encode errors remain visible.
- [ ] 5.5 Preserve credentialless behavior for request creation and resource/fidelity fetches; verify the existing `credentials: omit` contract remains intact for the shared request helper and direct video preview fetches.

## 6. Verification and handoff

- [ ] 6.1 Run focused backend and frontend suites covering identity, descriptor reuse, lifecycle split, lock boundaries, unrelated-request concurrency, shared consumers, zero-consumer cancellation, ephemeral cleanup, diagnostics, legacy compatibility, and request-ID guards; verify all acceptance scenarios pass.
- [ ] 6.2 Run `openspec validate compatible-preview-lifecycle-cache --type change --strict` and verify the change reports valid with all planning artifacts present.
- [ ] 6.3 For implementation completion, run repository-prescribed `./scripts/verify.sh --auto`, inspect only reported failures, and verify `ALL CHECKS PASSED` before updating task checkboxes.
