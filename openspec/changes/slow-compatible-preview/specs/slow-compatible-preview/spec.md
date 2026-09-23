## Purpose

Provide a status-aware Compatible Preview contract that lets legitimately slow
encodes remain healthy and observable without holding one HTTP media request
open or presenting a client wait interruption as encoder failure.

## ADDED Requirements

### Requirement: Registered preview requests expose one canonical lifecycle status

The system SHALL expose `GET /api/assets/preview/compatible/requests/:requestId/status`
as the canonical status projection for a registered Compatible Preview request.
The status response SHALL distinguish `idle`, `generating`, `ready`, `failed`,
`canceled`, `interrupted`, `stale`, `expired`, and `unknown` request outcomes.
For the stale outcome, the machine-readable code SHALL be `stale_artifact`; for
the expired and unknown outcomes, the codes SHALL be `expired` and
`unknown_request` respectively. A known active or terminal request SHALL return
its status representation with HTTP 200; an expired tombstone SHALL preserve
#14's HTTP 410 contract and an unknown request SHALL preserve HTTP 404.

#### Scenario: Newly registered request is observable before generation

- **WHEN** a valid request is created but no generation has been started
- **THEN** the status endpoint returns HTTP 200 with `state: "idle"`, the same
  `requestId` and `artifactId`, and no error claiming that encoding failed

#### Scenario: Active software encode is reported as generating

- **WHEN** the canonical artifact generation is alive and has not published a
  complete artifact
- **THEN** status returns HTTP 200 with `code: "generating"`,
  `state: "generating"`, `retryable: true`, and the current attempt metadata

#### Scenario: Recovery and lookup states remain distinct

- **WHEN** a request is failed, canceled, interrupted, stale, expired, or
  unknown
- **THEN** status and resource responses preserve the corresponding #14
  machine-readable code, HTTP status, retry/recreate metadata, and bounded
  reason instead of collapsing the result into `timeout` or generic failure

### Requirement: Resource waits SHALL be bounded independently from generation

The registered video, frame, inspection, and metrics resources SHALL start or
join the canonical artifact generation without requiring one HTTP response to
remain open until encoding completes. If the artifact is still generating when
the documented resource wait budget is reached, the resource SHALL return HTTP
202 with a JSON envelope containing `code: "generating"`,
`state: "generating"`, `retryable: true`, `requestId`, `artifactId`,
`statusUrl`, `startedAt`, `elapsedSeconds`, and the known effective encoder and
preset. It MUST NOT return HTTP 500, `failed`, or `canceled` solely because the
HTTP waiter budget or client connection ended. No resource wait budget SHALL
cancel FFmpeg or impose a fixed short encode lifetime.

#### Scenario: Slow x265 exceeds the HTTP wait budget

- **WHEN** a controlled `libx265` `veryslow` generation remains healthy beyond
  the resource wait budget
- **THEN** the resource returns HTTP 202 with `state: "generating"`, the
  generation remains alive under valid ownership, and the response does not
  report a timeout, failed encode, or canceled encode

#### Scenario: Generation later becomes ready

- **WHEN** a client observes `generating` and later requests the resource after
  the same artifact is published
- **THEN** the resource returns the media or JSON result with HTTP 200 and no
  second generation is started

#### Scenario: Client disconnect does not misclassify healthy work

- **WHEN** one HTTP waiter disconnects while a logical preview lease or another
  artifact consumer remains active
- **THEN** only that HTTP waiter detaches, the shared generation remains
  governed by artifact ownership, and later status reports `generating` or
  `ready` rather than a generic timeout failure

### Requirement: Logical preview ownership SHALL be explicit and bounded

The system SHALL provide an explicit start/renew interaction for an actively
waiting Compatible Preview UI and an explicit cancel interaction. Starting a
request SHALL return a bounded logical lease identifier when generation is
needed; renewing that lease SHALL be possible without starting a duplicate
generation. The logical lease SHALL be released on explicit cancel, component
unmount where cancellation is owned by the UI, lease expiry, or terminal
generation completion. Lease expiry SHALL eventually allow #1's existing
zero-consumer cancellation grace to cancel abandoned work.

`GET .../status` without a lease SHALL be observational only: it MUST NOT
increment artifact waiter/consumer ownership, reset zero-consumer cancellation,
or keep an otherwise abandoned encode alive. A lease-bearing renewal is an
explicit ownership operation, not an implicit side effect of status polling.

#### Scenario: Active UI owns a slow generation

- **WHEN** the UI explicitly starts a request and renews its lease while the
  user remains on the preview surface
- **THEN** the artifact generation remains eligible to continue beyond normal
  HTTP request lifetimes and status polling does not create additional artifact
  consumers

#### Scenario: Status observer does not pin abandoned work

- **WHEN** a client repeatedly calls the status endpoint without an active
  lease after all real consumers have detached
- **THEN** the calls do not prevent the bounded zero-consumer handoff from
  canceling the generation

#### Scenario: Explicit cancel respects shared ownership

- **WHEN** preview request A releases its logical lease while request B still
  consumes the same artifact identity
- **THEN** A stops waiting, B's generation remains active, and no requester
  unconditionally kills the shared FFmpeg process

### Requirement: Status SHALL provide honest slow-work metadata

For an active generation, the status contract SHALL expose `requestId`,
`artifactId`, `startedAt`, non-negative `elapsedSeconds`, the frozen effective
encoder, and the frozen effective preset when known. It MAY expose
`cacheHit` and terminal bounded diagnostics. A `generating_slow` presentation
label MAY be derived once elapsed time crosses the documented UX threshold,
but it MUST remain presentation-only and MUST NOT become a durable lifecycle
state or imply that the encoder is stalled. The v1 contract SHALL omit or leave
null progress percentage, speed, FPS, frame counts, and ETA when the current
FFmpeg invocation cannot supply reliable machine-readable progress.

#### Scenario: x265 NAS status is useful without fake progress

- **WHEN** a slow software x265 preview is generating on NAS hardware
- **THEN** the UI can display a generic generating message, elapsed time,
  `libx265`, and `veryslow` from backend status, without inventing a percentage
  or declaring the encode stalled

#### Scenario: Progress data is not identity data

- **WHEN** operational progress metadata is sampled or updated
- **THEN** it does not change `requestId`, `artifactId`, canonical encode
  identity, frozen request data, or cache reuse

### Requirement: Frontend Compatible Preview surfaces SHALL poll bounded status

The frontend SHALL start and renew ownership explicitly, poll status at a
non-aggressive interval with bounded backoff, and stop polling on `ready`,
`failed`, `canceled`, `interrupted`, `stale`, `expired`, `unknown`, explicit
cancel, unmount, or lease expiry. Temporary network failures SHALL produce a
reconnecting/generating presentation and bounded retry backoff; they MUST NOT
be converted into backend generation failure. While status is `generating`,
the UI SHALL use wording such as “Generating preview” and, after the UX
threshold, explain that the encode is taking longer than usual but is still
running. It MUST NOT show “Preview timed out” or “Preview failed” for a
healthy generating response.

#### Scenario: Profile Lab shows slow work and then ready

- **WHEN** Profile Lab starts a Compatible Preview whose status remains
  generating for multiple polls and later becomes ready
- **THEN** it shows the backend encoder/preset and elapsed time while active,
  then loads the ready preview and stops polling without creating another
  request or encode

#### Scenario: Temporary network failure is recoverable

- **WHEN** one or more status or lease-renewal calls fail transiently while
  the backend generation may still be active
- **THEN** the UI backs off within the documented bounds, preserves a
  reconnecting/generating state, and does not label the preview failed solely
  because of that transport interruption

### Requirement: Ready media SHALL preserve native streaming behavior

The frontend SHALL not fetch a complete compatible video into a Blob merely to
wait for generation. After status is ready, video and frame resources SHALL be
attached to native media/image elements using the registered request resource
URLs so normal browser Range and streaming behavior remains available. A
resource response that races with generation and returns `202 generating` SHALL
cause the caller to re-read status and retry within the bounded polling policy,
not to install a JSON response as media or report a generic decode failure.

#### Scenario: Video is attached only after readiness

- **WHEN** a registered preview is generating
- **THEN** Profile Lab displays status/progress messaging without attaching the
  generating resource URL to a video element

#### Scenario: Ready video remains seekable

- **WHEN** status reports ready and the video element requests the artifact
- **THEN** the frontend uses the native resource URL and the backend serves the
  complete artifact without a mandatory whole-file Blob transfer

### Requirement: Restart, retry, cache, and encoder policy remain authoritative

The status contract SHALL consume #14 restart reconciliation and SHALL NOT
restore a dead FFmpeg owner. A durable generating record without a valid final
artifact after restart SHALL be reported as `interrupted`; explicit recreate
or retry SHALL establish a new generation token/version before returning to
`generating`. A valid existing artifact SHALL report `ready` immediately with
`cacheHit: true` where the response can establish that no generation was
started. Compatible Preview SHALL continue to represent the backend-resolved
effective encode plan, including a selected x265 `slow` or `veryslow` preset;
it MUST NOT silently substitute a faster preset or create encoder-specific
lifecycle states.

#### Scenario: Restart interrupts a live attempt

- **WHEN** the backend restarts during a slow generation and no valid final
  artifact exists
- **THEN** the old request reports `interrupted`, no old process is claimed to
  be generating, and the next explicit retry uses a new attempt identity

#### Scenario: Cache hit bypasses slow UX

- **WHEN** the canonical artifact already exists and passes the existing
  validity checks
- **THEN** status and resources report `ready` immediately, no generation is
  started, and the response may mark `cacheHit: true`

#### Scenario: Real FFmpeg failure is not hidden as slow work

- **WHEN** FFmpeg exits unsuccessfully without an ownership cancellation
- **THEN** the artifact transitions to `failed`, status stops presenting active
  generation, and the client receives only bounded encoder diagnostics with the
  existing recreate/retry semantics

### Requirement: Legacy Compatible Preview callers use the same lifecycle

The existing scalar Compatible Preview route SHALL remain a compatibility
adapter to the canonical request/artifact lifecycle. Ready requests SHALL keep
their current media/JSON behavior. If a scalar request cannot finish within
the resource wait budget, it SHALL return the same machine-readable `202
generating` envelope and a followable registered request/status identity when
available, rather than creating a second generation engine or returning a
misleading 500/408. Registered request URLs remain the preferred contract for
clients that need long-running status, ownership, and cancellation.

#### Scenario: Legacy ready request remains compatible

- **WHEN** an existing scalar Compatible Preview caller requests an already
  ready artifact
- **THEN** it receives the same successful media or JSON response and the
  canonical artifact is reused

#### Scenario: Legacy slow request is followable

- **WHEN** an existing scalar caller reaches the bounded wait budget while the
  canonical generation remains healthy
- **THEN** it receives a `generating` response that identifies how to observe
  the same request/artifact instead of an unexplained timeout or duplicate
  encode
