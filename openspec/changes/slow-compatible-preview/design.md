## Context

This design is based on the checkout at
`f87233cc5572ff20421ec0779998c973ef2c21e7`, where merged #14 is present. It
refines the existing Compatible Preview lifecycle from #1 and #14; it does not
replace either lifecycle owner.

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
path, so any of them can start or join the same single-flight encode. A source
frame is separate: it reads the source directly and does not require the
Compatible Preview artifact.

The timeout audit keeps these lifetimes separate:

| Lifetime | Current observation | #15 treatment |
| --- | --- | --- |
| Generation | `acquireCompatiblePreviewArtifact` reaches an artifact entry whose background context owns FFmpeg. #1's zero-consumer grace can cancel it. There is no short encode deadline. | Preserve the generation context, single-flight entry, completion signal, publication, and #1 cancellation policy. Add only bounded logical ownership and operational timestamps. |
| HTTP waiter | The acquisition path selects between the request context and `entry.done`. A canceled request releases its waiter; current error mapping can make context cancellation look like HTTP 408/canceled or another generic failure. | Split begin/join from bounded waiting. A healthy active entry returns a machine-readable `202 generating` only after a logical owner is installed or reused. |
| Frontend wait/poll | Current Compatible Preview callers perform sequential inspection/metrics work. Video currently downloads a Blob and aborts its fetch on unmount/cancel; there is no status polling contract. | Use registered start, lease renewal, status polling, and ready-only native media attachment. Polls observe and do not acquire ownership. |
| Proxy/network | `frontend/nginx.conf` has 300-second `proxy_read_timeout` and `proxy_send_timeout`; browser, client, and external proxy limits are additional independent boundaries. | Keep transport limits separate from generation. The resource path normally returns its 5-second response before those limits matter. |

The current FFmpeg path stages output in the target directory, captures bounded
stderr diagnostics, atomically publishes the final artifact, and stores the
bounded encoder log. It does not expose a reliable machine-readable progress
stream. This change therefore reports elapsed time and frozen encoder/preset
metadata, not a fabricated percentage or ETA.

## Goals / Non-Goals

**Goals:**

- Make healthy long-running generation observable without requiring one HTTP
  response to remain open until FFmpeg completes.
- Guarantee that every followable `202 generating` response has a bounded
  logical owner before its HTTP waiter is released.
- Preserve #1 as the owner of artifact identity, single-flight generation,
  waiter/consumer accounting, zero-consumer grace, cancellation, publication,
  and shared artifact reuse.
- Preserve #14 as the owner of persistent request identity, frozen request to
  artifact relationship, generation token/version, restart recovery,
  tombstones, stale/interrupted handling, and terminal action errors.
- Give Profile Lab and other registered Compatible Preview callers explicit
  generating, slow-presentation, ready, and terminal recovery behavior.
- Keep ready compatible video delivery native and Range-capable while stating
  the browser-cookie limitation documented by #13.

**Non-Goals:**

- No new scheduler, queue, background-operation framework, or process-resume
  mechanism.
- No persistent lease table, durable browser-owner restoration, or second
  lifecycle engine.
- No persistent `slow` state, x265-specific state machine, or arbitrary short
  encode timeout.
- No live progress percentage, speed, FPS, ETA, or `-progress` pipe in v1.
- No #19 LAB preview-first orchestration or primary-artifact evidence reuse.
- No cache identity, canonical encode signature, frozen profile policy, media
  authorization, or CORS policy changes.

## Decisions

### 1. Use one canonical registered status and ownership contract

Add thin routes to the registered request group:

```text
POST /api/assets/preview/compatible/requests/:requestId/start
GET  /api/assets/preview/compatible/requests/:requestId/status
POST /api/assets/preview/compatible/requests/:requestId/cancel
```

`start` is an ownership operation, not a job-submission API. It resolves the
frozen descriptor, checks the existing final-file cache gate, and otherwise
begins or joins the existing artifact generation without waiting for
completion. It accepts a stable client-generated `ownerId` and an optional
current opaque `leaseToken`.

Registered resource calls may carry the same `ownerId` and `leaseToken`. A
direct resource caller that supplies neither can still use the compatibility
path: the server creates an opaque bounded handoff owner and returns its lease
token in the generating response. The frontend's canonical start path uses a
stable `ownerId` so lost initial responses remain idempotent.

The frontend creates one random `ownerId` for one logical Compatible Preview
surface/request. Runtime state maps `(requestId, ownerId)` to at most one active
logical lease. Repeating `start` with the same pair renews and returns the same
active lease; it never increments the artifact consumer reference twice. The
server may return a different opaque `leaseToken`, but the token is runtime
operational state rather than authentication.

`status` is the canonical read model. It never starts generation, acquires an
artifact waiter, renews a lease, or resets #1's zero-consumer handoff. Repeated
status polls therefore do not keep abandoned work alive. Lease renewal is an
explicit `start` renewal or the equivalent dedicated renewal action if the
implementation exposes one; it is not an effect of observation.

`cancel` releases only the caller's logical lease. It is idempotent for the
same lease and never unconditionally kills an FFmpeg process. If another
logical owner or artifact consumer remains, the shared generation continues.

These routes are a projection and ownership adapter over the existing request
and artifact registries. They do not add a job scheduler, persistent lease
record, lease goroutine, or second generation engine.

### 2. Make the HTTP and lifecycle contracts unambiguous

The status endpoint is intentionally a successful read of known state. Its
canonical HTTP matrix is:

| `/status` condition | HTTP | `code` | `state` | `retryable` | `recreate` |
| --- | ---: | --- | --- | --- | --- |
| Request prepared, not started | 200 | `idle` | `idle` | true | false |
| Healthy active attempt | 200 | `generating` | `generating` | true | false |
| Valid final artifact | 200 | `ready` | `ready` | false | false |
| Genuine FFmpeg/publication failure | 200 | `failed` | `failed` | false or #14-defined | #14-defined |
| Explicit/zero-consumer cancellation | 200 | `canceled` | `canceled` | #14-defined | #14-defined |
| Restart recovery | 200 | `interrupted` | `interrupted` | #14-defined | #14-defined |
| Invalidated artifact relationship | 200 | `stale_artifact` | `stale` | #14-defined | #14-defined |
| Expired request tombstone | 410 | `expired` | `expired` | false | true |
| Unknown request ID | 404 | `unknown_request` | `unknown` | false | true |

Known terminal states remain HTTP 200 on `/status` so clients can observe the
state without treating observation as an action failure. Their body preserves
the #14 `code`, `state`, `retryable`, `recreate`, and bounded `reason` fields.

Resource and start actions have a separate matrix:

| Resource or `start` condition | HTTP | Meaning |
| --- | ---: | --- |
| `idle`, or healthy generation after the wait budget | 202 | Generation is followable; the response includes a logical lease and status/renew/cancel identity. |
| Ready cache hit or completed artifact | 200 | Serve media/JSON or return the ready action result. |
| `failed`, `canceled`, `interrupted`, or `stale_artifact` | 409 | Preserve the #14 lifecycle action conflict and terminal body. |
| `expired` | 410 | Preserve the #14 expired tombstone semantics. |
| `unknown_request` | 404 | No registered lifecycle record is addressable. |

The `cancel` action returns a successful idempotent release result for a valid
lease, including the current state. It returns 404/410 for unknown/expired
request identity and a lifecycle conflict for an invalid, expired, or
already-released token without affecting another owner. This does not change
the resource/start matrix above.

A generating response uses an envelope like:

```json
{
  "requestId": "...",
  "artifactId": "...",
  "code": "generating",
  "state": "generating",
  "retryable": true,
  "recreate": false,
  "leaseToken": "...",
  "leaseExpiresAt": "2026-09-23T12:00:45Z",
  "statusUrl": "/api/assets/preview/compatible/requests/.../status",
  "renewUrl": "/api/assets/preview/compatible/requests/.../start",
  "cancelUrl": "/api/assets/preview/compatible/requests/.../cancel",
  "startedAt": "2026-09-23T11:59:58Z",
  "elapsedSeconds": 42,
  "encoder": "libx265",
  "preset": "veryslow",
  "cacheHit": false
}
```

The exact JSON type names may follow repository conventions, but the semantic
fields and matrix are canonical. `Retry-After: 2` is returned for a healthy
generating resource response. It is a retry hint, never an FFmpeg deadline.

### 3. Install a logical lease before every followable `202`

The existing artifact consumer/waiter remains the ownership primitive. Its
current combined begin/join-and-wait operation is conceptually split into a
short begin/join step and a bounded wait step.

For a registered video, frame, inspect, or metrics resource:

1. Validate request/recovery state through the existing #14 lookup.
2. Resolve the frozen descriptor and begin or join the same canonical artifact.
3. If a valid final artifact exists, serve it immediately. No logical lease is
   created merely because the endpoint briefly waited.
4. If an active generation is found, use the HTTP request context only for a
   maximum 5-second waiter budget.
5. If generation completes before that budget, serve the ready result or the
   genuine terminal result and release the temporary HTTP waiter.
6. If the budget ends while generation is healthy, atomically reuse the
   caller's valid logical lease or create one for the supplied owner identity
   while the HTTP waiter still holds its reference. If the direct caller
   supplied no owner identity, create a bounded opaque handoff owner for this
   follow-up response.
7. Only after that logical reference is installed does the endpoint release
   the HTTP waiter and return `202 generating` with the lease token and
   follow-up URLs.

The promotion in step 6 and waiter release in step 7 are one ownership
handoff under the artifact entry's synchronization. There must be no observable
zero-consumer interval between them. A direct registered resource call does
not require a prior `/start`; it receives a real bounded follow-up lease on
the slow path. If the caller supplied a valid lease, it is reused and no
duplicate consumer is added.

If the HTTP context is canceled before a `202` is selected, the temporary
waiter is released normally and no followable generating response is emitted.
The existing #1 zero-consumer grace then decides whether generation cancels.
Once `202` is selected, the handoff has already happened, so the response is
not advertising work that is about to be canceled solely because the waiter
ended.

The generation goroutine keeps the #1 background-derived context and
`exec.CommandContext`. It has no `context.WithTimeout` tied to five seconds or
any other short HTTP budget. It is governed by explicit ownership release,
#1 zero-consumer cancellation, backend shutdown/restart, genuine encoder
failure, and existing safety semantics only.

### 4. Make the legacy scalar adapter honest

Non-ephemeral legacy scalar calls route through the same registered request
identity, frozen descriptor, single-flight artifact, owner identity, and lease
handoff. Ready behavior remains compatible; a slow result uses the canonical
`202 generating` envelope and includes a followable request/lease identity.
It must not invoke a second generation implementation.

An ephemeral legacy call that cannot expose a registered request identity and
cannot safely return a lease/status follow-up must not return a followable
`202`. Its explicit compatibility response is HTTP 409 with
`code=legacy_non_followable`, `state=generating`, `retryable=true`, and a
reason directing the caller to the registered request flow. This response
describes a transport limitation, not an FFmpeg failure, and prevents the
legacy adapter from claiming that doomed work is followable.

### 5. Define runtime-only, idempotent lease lifecycle

Logical leases are bounded runtime ownership records. They do not get a
`CompatiblePreviewLease` table, durable owner row, or startup restoration.
#14 persists request/artifact/generation recovery; #15 browser ownership
disappears on restart.

The lifecycle invariants are:

| Operation | Required behavior |
| --- | --- |
| Acquire | Same `(requestId, ownerId)` has one active lease and one artifact consumer reference. A fresh owner gets one new reference. |
| Renew | Extends expiry to the bounded lease lifetime, with a 15-second frontend target; it adds no waiter or consumer reference. |
| Cancel | Releases exactly that owner's reference once. Repeated cancel is an idempotent no-op/result and cannot release another owner. |
| Expire | Releases exactly once when the lease expires; cleanup is safe if cancel raced with expiry. |
| Terminal generation | On `ready`, `failed`, or `canceled`, remove/release logical lease bookkeeping without changing the already-owned #1/#14 terminal artifact lifecycle. |
| Shared artifact | Owners A and B each reference the same artifact entry. Releasing A leaves B and the generation alive. |

The default logical lease is 45 seconds. Renewal targets every 15 seconds.
After a legitimate owner disappears and all other consumers are gone, #1's
existing bounded handoff grace, approximately 100ms where that is the current
convention, remains authoritative. The lease does not turn status polling into
a hidden permanent consumer.

`ownerId` is a bounded, opaque operational nonce. It is not authentication,
does not participate in request ID, artifact ID, encode signature, or frozen
request data, and is not persisted across restart. A retry with the same owner
identity can renew/return the same active lease after a lost response. Once the
old lease has expired, a later explicit acquire may create a new bounded lease
for that owner identity; it still cannot create two active references for one
owner at once.

### 6. Project slow status without inventing encoder progress

Status reads the request's frozen descriptor and current artifact entry/record.
It never rereads a mutable profile or reruns effective encoder policy in the
frontend. Operational fields are:

- `requestId` and `artifactId`;
- `state`, `code`, `retryable`, `recreate`, and bounded `reason`;
- active-attempt `startedAt` and computed non-negative `elapsedSeconds`;
- frozen effective `encoder` and `preset`;
- `cacheHit` when a valid artifact was found before this interaction started
  generation.

Generation timestamps are operational metadata only. They do not participate
in request identity, artifact identity, generation token/version, or the
frozen encode signature. A ready cache hit returns immediately and does not
touch timestamps or start generation merely because status was observed.

V1 does not add `-progress pipe:...`. The current subprocess path uses stderr
for bounded diagnostics and stdout for staging; a live progress pipe would
require an additional concurrent reader and parsing contract. The contract
therefore promises no encoded-frame count, encoded-time percentage, speed,
FPS, or ETA. `generating_slow` is not a durable state: after 30 seconds the UI
may present “taking longer than usual, but still running” while the backend
state remains `generating`.

### 7. Keep frontend polling bounded and credential semantics explicit

The canonical frontend flow is:

```text
create registered request (no encode)
  -> POST start with stable ownerId
  -> if 202, retain leaseToken and poll status
  -> poll at about 1s, then 2s, capped at 5s while active
  -> renew explicitly every 15s while mounted and interested
  -> show slow presentation after 30s without changing backend state
  -> attach ready native media URL only after status says ready
  -> release/cancel ownership on ready, explicit cancel, unmount, or terminal state
```

All controllable API, start, status, cancel, renew, and polling fetches
explicitly use `credentials: omit`. Temporary network failures use bounded
backoff (capped at 15 seconds) and retain the active-generation presentation;
they do not turn healthy backend work into failed or canceled. Polling stops on
ready, failed, canceled, interrupted, stale, expired, unknown, explicit
cancellation, unmount, or lease expiry. It is not an endless loop.

After ready, the video resource is assigned directly to the native `<video>`
element and frame output to the native image element where applicable. This is
intentional: browser-native delivery preserves Range/seek behavior and avoids
fetching an entire NAS artifact into a Blob. `crossOrigin="anonymous"` remains
where applicable.

This native transport is not equivalent to `credentials: omit`. Browser-native
`<video>`, `<audio>`, and `<img>` subrequests may send same-host cookies under
browser policy, and the frontend cannot guarantee browser-level credentialless
native media on a shared hostname. The backend `stripInboundCookies()`
middleware remains installed before handlers, including media Range requests;
it removes inbound `Cookie` while preserving `Range` and unrelated headers.
Logging must continue not to expose `Cookie` or `Authorization`. Complete
browser isolation still requires the dedicated-hostname/subdomain deployment
guidance from #13. This change does not weaken middleware, media
authorization, CORS policy, or that documentation.

### 8. Recover from the native-media readiness race

Status `ready` is a read observation, not a lock on the file. The artifact can
become stale or unavailable before a native media element issues its request,
and the element cannot interpret a JSON `202`/`409` body like `fetch` can.

Native media error handling therefore re-reads canonical status instead of
mapping every element error to “preview failed”:

```text
native media error
  -> GET canonical status
  -> ready       : retry native attachment within a small fixed attempt bound
  -> generating  : return to bounded polling
  -> terminal    : stop and use normal recovery UI
  -> status network error: bounded retry/backoff, then show reconnecting state
```

The ready-attachment retry count is small and explicit (for example, two
attempts per ready observation); it is not an infinite media retry loop. A
terminal response from the status endpoint retains #14's state and does not
become a generic timeout.

### 9. Preserve #14 restart, retry, cache, and encoder boundaries

Startup reconciliation remains #14-authoritative. A persisted generating
attempt without a valid final artifact becomes `interrupted`; a valid final
artifact may become `ready`. The old process, HTTP waiter, logical lease, and
browser owner are not restored. Status exposes the recovered state and start
does not silently retry it.

An explicit retry/recreate follows #14: it creates a new frozen relationship
and generation token/version, after which #15 reports the new attempt as
`generating` and eventually `ready` or a genuine terminal state. A real
non-zero FFmpeg exit is `failed` with bounded diagnostics; a user or
zero-consumer ownership cancellation is `canceled`; elapsed time alone never
selects either state.

The existing final-file validation remains the cache-hit gate. If an artifact
already exists, status/resource/start returns `ready` immediately with
`cacheHit=true` where that field is applicable. No timestamp touch or re-encode
is introduced solely by status observation. The effective `libx265`/`slow` or
`veryslow` plan remains unchanged; x265 is diagnostic metadata, not a special
lifecycle branch.

### 10. Deterministic verification design

Concurrency tests use an injected fake FFmpeg runner, barriers, channels, and
controlled completion/error signals. They do not use multi-second sleeps to
make lifecycle tests pass. If the repository's existing #1 timer abstraction
requires a tiny grace-period test, use that abstraction rather than an
arbitrary wall-clock delay.

Backend tests cover the 5-second resource wait and atomic lease handoff,
existing-lease reuse, unrenewed lease expiry, lost initial start response,
repeated cancel, shared consumers, zero-consumer cancellation, eventual ready,
genuine failure, explicit cancel, observer polling, cache hit, restart,
interrupted retry, disconnected HTTP waiter, all four registered resources,
and the legacy adapter limitation/followable path.

Frontend/API tests cover the exact status/action matrices, `202` JSON and
`Retry-After`, owner-idempotent start, renewal/cancel, bounded polling and
network backoff, 30-second slow messaging, terminal stops, native URL
attachment, readiness-race recovery, and no Blob transport for ready video.
Cookie-policy tests keep `credentials: omit` on controllable fetches, retain
`crossOrigin` behavior without claiming native credentiallessness, verify
backend Cookie stripping preserves Range, and retain logging redaction.

## Risks / Trade-offs

- **[Risk]** A 5-second resource budget surprises callers that expect one
  request to return media. **Mitigation:** the response is a successful,
  followable `202` with status, renewal, and cancel URLs; ready/cache-hit
  behavior remains 200.
- **[Risk]** A 45-second lease can expire during a long network outage.
  **Mitigation:** renew every 15 seconds with bounded retry and show
  reconnecting rather than failure; the bound prevents abandoned NAS work.
- **[Risk]** A lost start response can otherwise duplicate ownership.
  **Mitigation:** stable frontend `ownerId` makes initial acquire/renew
  idempotent for one request and preview surface.
- **[Risk]** Removing Blob staging can be mistaken for preserving the old
  credentialless media guarantee. **Mitigation:** document controllable-fetch
  and native-media rules separately; keep backend Cookie stripping, Range,
  logging, and dedicated-host guidance.
- **[Risk]** Native readiness races can produce transient element errors.
  **Mitigation:** re-read status and use a small bounded ready retry instead of
  declaring encoder failure.
- **[Risk]** Active leases can retain more work on NAS storage. **Mitigation:**
  reuse #1 single-flight identity, release on terminal/unmount/cancel, expire
  abandoned leases, and preserve #1/#14 cleanup bounds.

## Migration Plan

1. Add the status/start/cancel response types and routes as an adapter over the
   existing #1/#14 request/artifact registries. Keep request IDs, artifact IDs,
   frozen descriptors, token/version, and ready media authorization compatible.
2. Implement the runtime owner/lease map and split begin/join from bounded
   resource wait. Install/reuse the logical owner before releasing the final
   HTTP waiter for any generating `202`.
3. Update the registered frontend flow, controllable fetch options, bounded
   polling, cancellation/renewal, native ready media, and native error recovery
   together so a 202 response is understood by its caller.
4. Run focused deterministic backend/frontend tests followed by strict
   OpenSpec validation and the repository's canonical verifier. Do not change
   the selected preview preset or add an FFmpeg deadline.
5. Rollback is code-only for lifecycle behavior. Existing ready artifacts and
   request records remain valid; runtime leases expire/release through their
   bounds. Do not delete preview artifacts, request records, or source media.

## Open Questions

None. The canonical choices for this change are the 5-second resource wait,
45-second logical lease, 15-second renewal target, `Retry-After: 2`, 30-second
slow presentation threshold, status 200 projection for known states, action
409/410/404 terminal behavior, runtime-only leases, no `-progress` v1, and
#14-authoritative restart/retry recovery.
