## Context

See `proposal.md` for the motivation. This design is based on the checkout at
`f87233cc5572ff20421ec0779998c973ef2c21e7`, where merged #14 is present and
the working tree is otherwise clean.

The current registered request flow is:

```text
POST /api/assets/preview/compatible/requests
  -> normalize and freeze the request/artifact relationship
  -> return requestId and cacheIdentity without encoding

GET /api/assets/preview/compatible/requests/:requestId/{video,frame,inspect,metrics}
  -> resolve the frozen descriptor
  -> acquireCompatiblePreviewArtifact(request context, descriptor)
  -> start or join one artifact generation
  -> wait for request context or entry.done
  -> serve the ready file or result
```

The four request-backed resource operations share the same artifact acquisition
path. An output frame, inspection, metrics, or video request can therefore
start the same single-flight encode. A source frame is different: it reads the
source directly and does not require the compatible output artifact.

The observed lifetimes are distinct:

| Lifetime | Current owner/behavior | Issue #15 treatment |
| --- | --- | --- |
| Generation | The artifact entry owns a background-derived context and FFmpeg process. It has no short wall-clock deadline. #1's zero-waiter grace calls its cancel function. | Preserve this owner and cancellation policy. Add only an observational start timestamp and a bounded logical lease adapter. |
| HTTP waiter | `acquireCompatiblePreviewArtifact` selects between the request context and `entry.done`. A canceled request releases one waiter; `writeCompatiblePreviewError` currently maps `context.Canceled` to HTTP 408/canceled, while another context error can become a generic 500. | Split begin/join from bounded waiting. A healthy still-generating artifact returns 202 instead of an error. |
| Frontend wait/poll | Request creation is followed by sequential inspection/metrics calls. `VideoPreviewRequest` fetches the entire video to a Blob and aborts on unmount or its cancel signal. There is no Compatible Preview status polling. | Start/renew a logical lease, poll status with bounded backoff, and attach native media URLs only after ready. |
| Proxy/network | `frontend/nginx.conf` currently has 300-second `proxy_read_timeout` and `proxy_send_timeout`. External proxies and browser/fetch/media behavior add further independent limits. | Keep these transport limits separate from generation. The resource path should usually return a short 202/status response before they matter. |

The current FFmpeg path writes stdout to a same-directory partial artifact,
captures stderr through a bounded buffer, atomically renames the final file,
and stores the bounded stderr as `.encoder.log`. It does not currently expose a
reliable machine-readable progress stream. No current Compatible Preview caller
has a status route or a lease/cancel route.

## Goals / Non-Goals

**Goals:**

- Make a healthy long-running generation observable without holding one HTTP
  response open until FFmpeg completes.
- Preserve #1 as the owner of artifact identity, single-flight generation,
  artifact waiters, zero-consumer grace, cancellation, publication, and shared
  reuse.
- Preserve #14 as the owner of durable request identity, frozen relationships,
  generation token/version, restart recovery, tombstones, and terminal API
  errors.
- Give Profile Lab and other registered Compatible Preview callers explicit
  generating, slow-presentation, ready, and terminal recovery behavior.
- Keep compatible video delivery streaming-capable and keep x265 profile/preset
  selection unchanged.

**Non-Goals:**

- No new scheduler, queue, background-operation framework, or process-resume
  mechanism.
- No persistent `slow` lifecycle state, x265-specific state machine, or
  arbitrary encode safety timeout.
- No live progress percentage, speed, FPS, ETA, or extra FFmpeg pipe in v1.
- No #19 LAB preview-first orchestration or primary-artifact evidence reuse.
- No cache identity, canonical encode signature, or frozen profile policy
  changes.

## Decisions

### 1. Use one status projection with explicit start, renewal, and cancel

Existing resource endpoints cannot safely become status observers by themselves:
they currently acquire an artifact waiter and block until ready, and a status
poll that repeated that operation would either hold HTTP open or repeatedly
reset #1's zero-consumer handoff. The current frontend also needs a place to
keep a logical preview active while no single media request is waiting.

Add the following thin routes to the registered request route group:

```text
POST /api/assets/preview/compatible/requests/:requestId/start
GET  /api/assets/preview/compatible/requests/:requestId/status
POST /api/assets/preview/compatible/requests/:requestId/cancel
```

`start` is an ownership operation, not a job submission API. It resolves the
frozen descriptor, checks for a valid final artifact, and otherwise begins or
joins the existing artifact generation without waiting for completion. With no
lease token it returns a new opaque logical lease token; with the same valid
token it renews that lease and never starts a duplicate generation. A response
for a ready cache hit is HTTP 200; an active generation is HTTP 202 with the
status envelope; #14 terminal recovery states retain their existing 409/410
responses.

`status` is the canonical read model. It returns HTTP 200 for a known active or
terminal request, with `state`, `code`, `retryable`, `recreate`, and bounded
metadata. Expired tombstones remain HTTP 410 and unknown IDs remain HTTP 404.
Status does not start generation, acquire an artifact waiter, renew a lease, or
reset cancellation. The lease-bearing `start` call is the only renewal
heartbeat.

`cancel` accepts the logical lease token and releases only that logical owner.
It returns HTTP 200 with `leaseReleased` and the current artifact state; if the
generation is shared, the state can remain `generating`. Invalid or already
expired lease tokens are reported as a client lifecycle conflict and do not
cancel another owner. Cancellation after the artifact is ready is a no-op for
the ready artifact.

This is a projection and ownership adapter over the existing request/artifact
registries. It is not a second lifecycle engine and does not persist a live
lease, goroutine, mutex, context, or PID.

### 2. Keep lifecycle codes and HTTP meanings explicit

The status/resource envelope uses the following stable distinctions. `stale`
is the lifecycle state retained by #14 while its API code remains
`stale_artifact`.

| Condition | Status code/state | Resource HTTP behavior | Meaning |
| --- | --- | --- | --- |
| Request prepared, not started | `idle` / `idle` | 200 JSON status only | No generation has been requested yet. |
| Healthy active attempt | `generating` / `generating` | 202 JSON with `Retry-After: 2` | Work is alive; wait or poll later. |
| Complete valid file | `ready` / `ready` | 200 media/JSON | The canonical artifact is usable. |
| FFmpeg or publication failure | `failed` / `failed` | Existing #14 lifecycle error | Bounded diagnostics may explain a real failure. |
| Explicit/zero-consumer cancellation | `canceled` / `canceled` | Existing #14 lifecycle error | Generation was canceled by ownership semantics. |
| Restart recovery | `interrupted` / `interrupted` | Existing #14 lifecycle error | No old process is alive after restart. |
| Invalidated relationship/file | `stale_artifact` / `stale` | Existing #14 lifecycle error | Explicit recreate is required. |
| Request tombstone | `expired` / `expired` | 410 | The deterministic request expired; recreate is allowed. |
| No request record | `unknown_request` / `unknown` | 404 | No lifecycle record is addressable. |

The resource wait budget is a deliberately separate default of 5 seconds. It
limits how long one video/frame/inspect/metrics response waits after joining an
active artifact; it never wraps the FFmpeg context. A generating response
includes:

```json
{
  "requestId": "...",
  "artifactId": "...",
  "code": "generating",
  "state": "generating",
  "retryable": true,
  "recreate": false,
  "statusUrl": "/api/assets/preview/compatible/requests/.../status",
  "startedAt": "2026-09-23T12:00:00Z",
  "elapsedSeconds": 42,
  "encoder": "libx265",
  "preset": "veryslow",
  "cacheHit": false
}
```

`Retry-After: 2` is a client hint, not an encode deadline. The same envelope
is used by the legacy scalar adapter when it can expose a registered follow-up
identity. The frontend API client must treat 202 as a successful JSON response
and terminal 409/410/404 bodies as `ApiRequestError` lifecycle data.

### 3. Extend #1 with a bounded logical consumer, not a parallel engine

The existing artifact lease/waiter is the correct ownership primitive. The
implementation should split its current combined operation into a short
begin/join step and a wait step. The logical preview lease retains one existing
artifact consumer lease in the request runtime entry while its token is valid;
resource HTTP waiters use separate short-lived leases. This preserves the
existing counter and zero-consumer timer rather than inventing a second process
owner.

The default logical lease lifetime is 45 seconds and the frontend renews it
every 15 seconds while the preview surface is active. The values are bounded,
documented operational defaults, not encode timeouts. If renewals stop because
of navigation, browser failure, or a dead client, the lease expires and #1's
existing handoff grace eventually cancels the generation when no other consumer
remains. If the generation reaches ready, failed, or canceled, the logical
lease is released and its terminal result is retained by the existing request
and artifact lifecycle.

For a shared artifact, each logical lease maps to one ownership reference. A
cancel from A releases only A's reference; B's reference keeps the same
single-flight generation alive. Status polls without a lease never affect the
reference count. Lease tokens are runtime-only and are intentionally not
restored after restart.

### 4. Bound HTTP waiting without binding generation to HTTP

For a registered resource request:

1. Validate the request/recovery state through the existing #14 lookup.
2. Resolve the frozen descriptor and begin/join the same canonical artifact.
3. If the artifact is ready, acquire a short serving reference and return the
   existing media/JSON result.
4. If it is generating, wait on the existing completion signal for at most the
   5-second resource budget using the HTTP request context as the waiter
   context only.
5. If the budget or HTTP context ends while the artifact remains healthy,
   release only the temporary HTTP waiter and return the 202 generating
   envelope. A valid logical lease, if present, remains in place.
6. If completion is observed first, serve the ready result or return the real
   bounded terminal error.

The generation goroutine continues to use the #1 background-derived context
and `exec.CommandContext`. It is canceled only by explicit logical ownership
release/lease expiry reaching zero consumers, backend shutdown/restart, or
other existing #1 failure semantics. There is no `context.WithTimeout` around
FFmpeg. A source-frame request continues to use its direct source operation and
does not pretend that source-frame rendering is an output-artifact generation.

When the old scalar query route is used, it should prepare or recover the same
canonical registered identity for any non-ephemeral follow-up. Ready behavior
stays unchanged; a slow response becomes the same 202 envelope instead of a
second direct generation path. Ephemeral legacy requests remain bounded by
their existing request scope and are not made durable merely to support #15.

### 5. Project status from frozen backend data

The status projection reads the request's frozen descriptor and the current
artifact entry/record. It must not reread a mutable ProfileID, rerun effective
encoder policy, or derive the encoder/preset in TypeScript. Add operational
generation timestamps to runtime/durable artifact metadata as needed:

- `generationStartedAt` is recorded when the current token/version begins;
- `generationFinishedAt` is optional terminal evidence;
- neither field participates in request identity, artifact identity, or the
  frozen encode signature.

`elapsedSeconds` is computed from the active attempt's wall-clock start and is
clamped at zero. `encoder` and `preset` come from the frozen effective plan;
requested values may be included separately when useful. `cacheHit` is a
response hint: it is true only when a valid final file was found before the
current interaction started a generation, and it is not used for cache
identity. Terminal diagnostics reuse the existing bounded FFmpeg/persistence
reason limits.

Do not add `-progress pipe:...` in this change. The current subprocess path
uses stderr for bounded diagnostics and stdout for the staging artifact; a
live progress pipe would require a concurrent reader, parsing contract, and
careful interaction with stderr capture. Since x265 stderr is not currently a
reliable machine-readable contract, the v1 response omits progress percentage,
speed, FPS, frames, and ETA rather than inventing them. Progress can be added
later as operational metadata without entering identity or request snapshots.

### 6. Update frontend orchestration and retain native media delivery

The API client adds typed start, status, and cancel operations plus the
generating envelope. Profile Lab's Compatible Preview flow changes as follows:

```text
create request (201, no encode)
  -> start/renew lease (202 generating or 200 ready)
  -> status GET at 1s, then 2s, capped at 5s while active
  -> renew lease explicitly every 15s
  -> inspect/metrics only after ready; retry a 202 race through status
  -> attach native video/frame URLs after ready
  -> release lease on ready, cancel, unmount, or terminal state
```

The polling loop stops on every terminal/recovery state, explicit cancellation,
unmount, or lease expiry. Network errors use bounded exponential backoff up to
15 seconds and show reconnecting/generating rather than failure; they do not
turn a backend generation into canceled. The user-facing slow threshold is a
presentation rule, initially 30 seconds: it adds “taking longer than usual,
but still running” without creating a `slow` backend state.

`VideoPreviewRequest` must no longer fetch the entire video into a Blob. Once
status is ready it sets the registered resource URL directly on the native
`video` element with the existing credentialless/cross-origin policy. Frame
URLs are attached to native image elements only after readiness. This keeps
Range/seek behavior and avoids making a large NAS artifact consume browser
memory. Existing Process Video cancellation remains the UI trigger, but it
also calls the backend cancel route for each owned logical lease; aborting a
local fetch/poll alone is not treated as encoder cancellation.

### 7. Keep #14 restart and terminal recovery authoritative

Startup reconciliation remains unchanged in ownership: persisted `generating`
without a valid final artifact becomes `interrupted`, while a valid final file
can be promoted to `ready`. No logical lease, HTTP waiter, context, or process
is restored. The status route projects these outcomes rather than adding a
second restart state machine.

For `interrupted`, `stale_artifact`, `expired`, `failed`, or `canceled`, the
frontend stops active polling and uses the existing recreate/retry semantics.
An explicit recreate through the #14 request-registration path establishes the
new frozen relationship and generation token/version; `start` does not silently
retry a terminal recovery record. A real FFmpeg non-zero exit remains `failed`
with bounded diagnostics. A zero-consumer cancellation remains `canceled`.

### 8. Preserve effective x265 plan and cache-hit behavior

The status and resource paths consume the already frozen backend-resolved plan.
They do not change `libx265`, quality values, filters, or `slow`/`veryslow`
presets to meet the 5-second response budget. The current effective encoder and
preset are diagnostic fields only and do not add x265-specific lifecycle
branches.

Before beginning a generation, the existing final-file validation remains the
cache-hit gate. If the file is valid, status/start/resource calls return ready
immediately and no timestamp touch or re-encode is introduced solely by status
observation. Generation timestamps and progress metadata remain operational
state and cannot alter artifact identity.

### 9. Validate concurrency with deterministic controls

The test design uses an injected fake FFmpeg runner or controlled generation
barrier/channels. It must not use multi-second sleeps to make the slow case
pass. Tests exercise the resource wait timer with a deterministic signal, the
logical lease TTL with the existing timer abstraction/convention, and the
artifact completion signal for readiness.

The backend coverage includes slow x265-like generation, eventual ready with
one encode, genuine failure, explicit cancel, shared logical consumers,
zero-consumer cancellation, observer polling, cache hit, restart/interrupted,
explicit retry/new token, disconnected HTTP waiter, all four registered
resources, and legacy adapter behavior. Frontend coverage includes 202 JSON
transport, polling/backoff/terminal stops, slow messaging, cancel/renew calls,
native media URL attachment, no Blob download, and non-fatal network errors.

## Risks / Trade-offs

- **[Risk]** A 5-second resource budget can surprise direct API consumers that
  expect a video response in one request. **Mitigation:** return a stable
  machine-readable status URL and preserve 200 behavior for ready/cache-hit
  resources; document the registered status flow as canonical.
- **[Risk]** A 45-second lease can expire during a long network outage and let
  #1 cancel work that a user still expects. **Mitigation:** renew every 15
  seconds while mounted, retry renewal with bounded backoff, and show
  reconnecting rather than failure; the bound prevents abandoned NAS work.
- **[Risk]** Adding operational timestamps to persistence can be confused with
  #14 generation identity. **Mitigation:** keep token/version and frozen
  snapshot authoritative; explicitly exclude timestamps from all identities and
  use additive metadata only.
- **[Risk]** Existing clients may treat a 202 JSON response as media. **Mitigation:**
  update the frontend transport and media components together; ready responses
  remain backward-compatible, and the legacy adapter includes a followable
  identity for clients that understand the status contract.
- **[Risk]** Removing Blob staging changes current browser test assumptions.
  **Mitigation:** test native media element attachment, Range-capable server
  delivery, credentialless requests, and generation status separately rather
  than preserving whole-file buffering.
- **[Risk]** NAS disk usage can increase if active UI leases retain more
  generations. **Mitigation:** reuse #1 single-flight/artifact identity,
  release leases on terminal/unmount, and retain #1/#14 bounded cleanup and
  expiration rules.

## Migration Plan

1. Add the status/start/cancel routes, status response types, and additive
   operational metadata while keeping request creation, artifact IDs, and
   frozen descriptors compatible with #14.
2. Implement the backend split between begin/join and bounded HTTP wait, then
   update frontend Profile Lab transport and native media presentation.
3. Run focused deterministic backend/frontend tests followed by the canonical
   repository verifier. Deploy backend and frontend together so generating 202
   responses are understood by their caller.
4. Rollback is code-only for the lifecycle behavior: existing ready artifacts
   and request rows remain valid, active leases expire/release through their
   bounds, and additive timestamp columns/metadata can remain unused by older
   code. Do not delete preview artifacts or request records as part of rollback.

## Open Questions

None. The response budget, lease bounds, status routes, no-progress v1 policy,
and #14 retry/restart boundaries are selected for this change.
