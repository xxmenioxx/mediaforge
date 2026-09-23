## Purpose

Provide a status-aware Compatible Preview contract that keeps legitimately
slow software encodes healthy and followable without holding one HTTP media
request open or presenting a client wait interruption as encoder failure.

This capability intentionally refines the existing Compatible Preview behavior
owned by #1 and #14. It does not replace their artifact identity,
single-flight, persistent request, restart, or terminal lifecycle contracts.

## ADDED Requirements

### Requirement: Registered preview status SHALL use one canonical projection

The system SHALL expose
`GET /api/assets/preview/compatible/requests/:requestId/status` as the
canonical read model for a registered Compatible Preview request. Its exact
HTTP projection SHALL be:

| State | HTTP | `code` |
| --- | ---: | --- |
| `idle` | 200 | `idle` |
| `generating` | 200 | `generating` |
| `ready` | 200 | `ready` |
| `failed` | 200 | `failed` |
| `canceled` | 200 | `canceled` |
| `interrupted` | 200 | `interrupted` |
| `stale` | 200 | `stale_artifact` |
| `expired` | 410 | `expired` |
| `unknown` | 404 | `unknown_request` |

Known terminal states SHALL remain successfully observable on `/status`. Their
body SHALL preserve #14 `state`, `code`, `retryable`, `recreate`, and bounded
`reason` data. Status observation SHALL not be interpreted as a resource/action
success or failure.

#### Scenario: Active and terminal states are observable

- **WHEN** a registered request is idle, generating, ready, failed, canceled,
  interrupted, or stale
- **THEN** `/status` returns HTTP 200 with the corresponding state and code,
  rather than converting active work or a known terminal state into a generic
  timeout

#### Scenario: Expired and unknown identity remain distinct

- **WHEN** the request identity is an expired #14 tombstone or is not known
- **THEN** `/status` returns HTTP 410 with `code: "expired"` or HTTP 404 with
  `code: "unknown_request"`, respectively

### Requirement: Resource and start actions SHALL preserve a separate lifecycle matrix

Registered video, frame, inspect, metrics, and start actions SHALL use this
matrix:

| Action condition | HTTP | Required behavior |
| --- | ---: | --- |
| `idle`, or healthy generation after the wait budget | 202 | Return a followable `generating` envelope with logical ownership. |
| Ready artifact/cache hit | 200 | Serve the media/JSON or return the ready action result. |
| `failed`, `canceled`, `interrupted`, or `stale_artifact` | 409 | Preserve the #14 lifecycle action conflict and terminal body. |
| `expired` | 410 | Preserve the #14 expired tombstone. |
| `unknown_request` | 404 | Preserve unknown registered identity behavior. |

The `cancel` action SHALL return an idempotent release result for a valid lease,
and SHALL use 404/410 for unknown/expired request identity plus a lifecycle
conflict for an invalid or already expired lease token. It SHALL not cancel
another owner. This action matrix SHALL not be conflated with the successful
known-state `/status` projection.

#### Scenario: Status and resource observe the same failure differently

- **WHEN** #14 reports `interrupted`, `stale_artifact`, or another known
  terminal lifecycle state
- **THEN** `/status` returns HTTP 200 with that state, while a resource or
  start action returns HTTP 409 with the existing lifecycle error semantics

#### Scenario: Ready cache hit does not enter generating UX

- **WHEN** the final artifact already passes the existing validity checks
- **THEN** status, start, and resources return ready/HTTP 200 immediately and
  do not create a generating lease merely because the endpoint was called

### Requirement: Every resource `202 generating` SHALL complete ownership handoff before waiter release

The registered video, frame, inspect, and metrics resources SHALL retain the
documented 5-second HTTP wait budget, but that budget SHALL never be an FFmpeg
deadline. If a generation remains healthy when the budget ends, the endpoint
MUST reuse the caller's valid logical lease or create one before releasing the
HTTP waiter. If the direct caller supplied no owner identity, the endpoint
MUST create a bounded opaque handoff owner and return its lease token. The
lease installation and waiter release SHALL be one synchronized ownership
handoff with no zero-consumer gap.

The `202` body SHALL include, at minimum, `requestId`, `artifactId`,
`code: "generating"`, `state: "generating"`, `retryable: true`,
`recreate: false`, `leaseToken`, `leaseExpiresAt`, `statusUrl`, a renewal URL,
a cancel URL, `startedAt`, non-negative `elapsedSeconds`, and effective
`encoder`/`preset` when known. The response SHALL include `Retry-After: 2`.

#### Scenario: Direct registered resource promotes its only waiter

- **WHEN** a direct registered resource call starts or joins generation, is
  the only consumer, and remains healthy beyond five seconds
- **THEN** a logical lease is installed while the HTTP waiter still owns the
  artifact, the waiter is released afterward, and the response is HTTP 202
  with a real follow-up lease; consumer ownership never reaches zero between
  those operations

#### Scenario: Existing lease is reused at the handoff

- **WHEN** the resource caller already supplies a valid lease for the same
  request and generation when the five-second budget ends
- **THEN** the response reuses that lease and does not add a duplicate
  artifact consumer reference

#### Scenario: Ready wins before the budget

- **WHEN** generation completes before the resource wait budget
- **THEN** the resource serves the ready result with HTTP 200 and does not
  create a logical lease solely because it waited briefly

#### Scenario: HTTP wait interruption before handoff is not a false 202

- **WHEN** the HTTP context ends before the endpoint selects a generating
  response
- **THEN** the temporary waiter is released using #1 rules and the endpoint
  does not emit a followable `202` without ownership

### Requirement: The logical lease SHALL be explicit, idempotent, and bounded

The registered request lifecycle SHALL provide start/renew and cancel
operations over runtime-only logical leases. The frontend SHALL send a stable
opaque `ownerId` for one preview surface/request. Runtime state SHALL map
`(requestId, ownerId)` to at most one active lease. Repeated initial start with
the same owner identity SHALL renew/return that lease and SHALL NOT increment
artifact consumers twice, including after a lost start response.

The default lease lifetime SHALL be 45 seconds and the renewal target SHALL be
15 seconds. `ownerId` and `leaseToken` SHALL not be authentication, request ID,
artifact ID, encode signature, frozen request data, or persisted restart
state. No lease table or browser-owner restoration SHALL be added.

#### Scenario: Lost initial start response is retry-safe

- **WHEN** `start(ownerId=A)` creates a lease but its response is lost and the
  client retries `start(ownerId=A)` without a token
- **THEN** the server returns or renews the same active logical lease and the
  artifact has exactly one reference for owner A

#### Scenario: Renew does not add ownership

- **WHEN** a valid owner renews its lease
- **THEN** expiry is extended without adding an HTTP waiter or artifact
  consumer reference

#### Scenario: Repeated cancel releases once

- **WHEN** owner A cancels its lease and repeats the same cancel request
- **THEN** the first request releases A's reference and the second is an
  idempotent result that cannot release another owner

#### Scenario: Expiry releases once

- **WHEN** owner A stops renewing and its bounded lease expires
- **THEN** A's reference is released exactly once and #1's existing
  zero-consumer grace decides whether generation cancels

#### Scenario: Shared owners remain independent

- **WHEN** owners A and B reference the same artifact and A cancels or expires
- **THEN** B's reference remains, the shared generation continues, and one
  requester cannot unconditionally kill FFmpeg

#### Scenario: Terminal generation removes runtime lease bookkeeping

- **WHEN** the generation reaches ready, failed, or canceled
- **THEN** logical lease records are released/removed without changing the
  already-owned #1/#14 terminal artifact lifecycle

### Requirement: Status observation SHALL not own generation

Repeated `GET .../status` calls without a valid lease SHALL be observational
only. They SHALL not acquire an artifact waiter, increment a consumer,
renew/extend a lease, reset the zero-consumer handoff, or restore a browser
owner after restart. A valid lease may be renewed only by the explicit renewal
operation.

#### Scenario: Polling does not pin abandoned work

- **WHEN** a client polls status once per second after all logical and HTTP
  consumers have detached
- **THEN** polling does not prevent #1's bounded zero-consumer cancellation

#### Scenario: Restart does not restore browser ownership

- **WHEN** the backend restarts while a browser lease was active
- **THEN** the runtime lease is gone, #14 projects the persisted attempt as
  ready or interrupted, and startup does not recreate that lease

### Requirement: Slow status SHALL be useful without asserting fake progress

Generating status SHALL expose `requestId`, `artifactId`, `startedAt`,
non-negative `elapsedSeconds`, effective `encoder`, and effective `preset` when
known. It MAY expose `cacheHit` and bounded terminal diagnostics. A slow UX
presentation MAY begin at 30 seconds, but `generating_slow` SHALL not be a
durable artifact lifecycle state and SHALL not imply that the encoder is
stalled. V1 SHALL not promise progress percentage, speed, FPS, frame count,
encoded time, or ETA because the current FFmpeg invocation does not expose a
reliable machine-readable progress stream.

Operational timestamps and any future progress samples SHALL not participate in
request identity, artifact identity, frozen request data, generation
token/version, or encode signature. The selected effective x265 `slow` or
`veryslow` preset SHALL not be replaced with a faster preset.

#### Scenario: Slow x265 remains healthy

- **WHEN** a controlled `libx265` `veryslow` encode remains alive beyond the
  HTTP wait budget
- **THEN** status and UI show generating, elapsed time, encoder, and preset,
  without a fake percentage, timeout, stalled assertion, or failure

#### Scenario: Elapsed time does not select failure

- **WHEN** generation remains alive for longer than the 30-second slow UX
  threshold
- **THEN** the backend state remains `generating` unless explicit ownership,
  shutdown/restart, or genuine encoder failure selects another state

### Requirement: Frontend polling and controllable fetches SHALL remain bounded and credentialless

Compatible Preview surfaces SHALL start ownership explicitly, poll at a
non-aggressive interval (about 1 second initially, then 2 seconds capped at 5
seconds), renew every 15 seconds while mounted and interested, and stop on
ready, failed, canceled, interrupted, stale, expired, unknown, explicit
cancel, unmount, or lease expiry. Temporary network errors SHALL use bounded
backoff capped at 15 seconds and show reconnecting/generating rather than
backend failure. The UI SHALL not show “Preview timed out” or “Preview failed”
while canonical status is healthy generating.

All controllable API, status, start, cancel, renewal, and polling fetches MUST
explicitly use `credentials: omit`.

#### Scenario: Profile Lab shows slow work and then ready

- **WHEN** Profile Lab owns a Compatible Preview that remains generating for
  multiple polls and later becomes ready
- **THEN** it shows generating/slow messaging with elapsed encoder metadata,
  stops polling, releases the lease, and attaches the ready preview without a
  second generation

#### Scenario: Temporary network failure is recoverable

- **WHEN** status or lease-renewal fetches fail transiently while generation
  may still be active
- **THEN** the UI uses bounded retry/backoff and a reconnecting/generating
  presentation, without canceling or labeling backend work failed solely from
  the transport failure

### Requirement: Ready native media SHALL preserve Range while documenting cookie limits

After canonical status is ready, compatible video and frame resources SHALL be
attached through native media/image URLs so browser Range, streaming, and seek
behavior remain available. The frontend SHALL not claim that native `<video>`,
`<audio>`, or `<img>` requests guarantee `credentials: omit`. Native
subrequests may carry same-host cookies according to browser policy;
`crossOrigin="anonymous"` SHALL remain where applicable.

Backend `stripInboundCookies()` SHALL remain before the media handlers, remove
inbound `Cookie`, preserve `Range` and unrelated headers, and retain logging
redaction for `Cookie` and `Authorization`. Complete browser isolation SHALL
continue to require the dedicated-hostname/subdomain deployment guidance from
#13. This capability SHALL not change media authorization or CORS policy.

#### Scenario: Controllable fetches differ from native media

- **WHEN** the frontend starts/polls/cancels through `fetch` and later assigns
  a ready URL to a native video element
- **THEN** fetch calls use `credentials: omit`, while the native element is
  documented as browser-controlled and the backend cookie-stripping defense
  remains in force

#### Scenario: Native ready video remains seekable

- **WHEN** status reports ready and the video element requests the artifact
- **THEN** the frontend uses the native URL and the backend preserves Range
  behavior without a mandatory whole-file Blob transfer

### Requirement: Native media errors SHALL recover through canonical status

When a native media/image request races with artifact staleness, removal, or a
new lifecycle response, the frontend SHALL re-read canonical status rather than
interpret every element error as an encode failure. If status is ready, it MAY
retry native attachment within a small fixed bound (for example, two attempts
per ready observation). If status is generating, it SHALL resume bounded
polling. If status is failed, canceled, interrupted, stale, expired, or
unknown, it SHALL stop and use normal lifecycle recovery. Status network errors
shall use the bounded reconnecting policy.

#### Scenario: Ready-to-generating media race resumes polling

- **WHEN** status first reports ready, the native video request errors, and a
  status reread reports generating
- **THEN** the UI resumes polling and does not show generic preview failure or
  create an additional generation

#### Scenario: Native media retry is bounded

- **WHEN** a native ready attachment repeatedly errors
- **THEN** the frontend retries only within the documented small bound and then
  presents normal recovery instead of entering an infinite retry loop

### Requirement: Restart, retry, cache, and genuine error states SHALL remain #14-authoritative

If a backend restart finds durable generating state without a final artifact,
the request SHALL become `interrupted`; the old process SHALL not be reported
as still generating. An explicit #14 retry/recreate SHALL create a new frozen
relationship and generation token/version before returning to `generating`.
A real non-zero FFmpeg exit SHALL become `failed` with bounded diagnostics.
Explicit or zero-consumer ownership cancellation SHALL remain `canceled`.

#### Scenario: Restart interrupts a slow attempt

- **WHEN** the backend restarts during a slow generation and no valid final
  artifact exists
- **THEN** status reports `interrupted`, no old lease/process is restored, and
  only an explicit retry starts a new attempt

#### Scenario: Explicit retry starts a new attempt

- **WHEN** a client retries an interrupted/stale lifecycle record through the
  #14 action
- **THEN** the new generation token/version is observable as `generating` and
  its eventual result is independent of the old attempt

#### Scenario: Genuine encoder failure is not slow work

- **WHEN** FFmpeg exits non-zero without an ownership cancellation
- **THEN** status reports `failed`, active polling stops, and the client
  receives bounded encoder diagnostics rather than an indefinitely generating
  state

### Requirement: Legacy scalar callers SHALL not advertise doomed generation

Non-ephemeral legacy scalar calls SHALL route through the same registered
request identity, frozen descriptor, single-flight artifact, and logical lease
contract. If such a caller reaches the wait budget while generation is healthy,
it SHALL return the canonical followable `202 generating` envelope. An
ephemeral legacy call that cannot expose a registered identity and lease SHALL
return HTTP 409 with `code: "legacy_non_followable"`,
`state: "generating"`, `retryable: true`, and a reason directing the caller to
the registered flow; it SHALL not return a misleading `202`, 500, or 408.

#### Scenario: Legacy ready behavior remains compatible

- **WHEN** a scalar caller requests an already ready artifact
- **THEN** it receives its existing successful media/JSON behavior and the
  canonical artifact is reused

#### Scenario: Legacy non-ephemeral slow request is followable

- **WHEN** a non-ephemeral scalar request reaches the wait budget while the
  shared generation remains healthy
- **THEN** it returns the canonical 202 envelope with registered request,
  status, lease, renewal, and cancel identity, without a duplicate encoder

#### Scenario: Ephemeral legacy limitation is explicit

- **WHEN** a scalar request cannot safely expose a registered follow-up
  identity
- **THEN** it returns the explicit `legacy_non_followable` compatibility
  response and does not claim that a healthy generation can be followed
