## Purpose

Preserve enough Compatible Preview request and shared-artifact identity across
backend restarts to reuse valid published previews and expose deterministic
recovery states without resuming a dead media process.

## ADDED Requirements

### Requirement: Persistent request identity has bounded expiration tombstones

The system SHALL persist each non-ephemeral Compatible Preview request while
active, with its request identity, normalized request identity, prepared
artifact relationship, lifecycle state, timestamps, and expiration metadata.
When an active request expires, the system SHALL replace its active state with
a bounded expired tombstone rather than immediately deleting all evidence of
the request ID. The active request bound SHALL remain 500 records, the expired
tombstone retention SHALL be two hours from expiration, and the tombstone
bound SHALL remain 500 records. Tombstones are ephemeral cache state, not
historical audit data.

#### Scenario: Persistent request is restored after restart

- **WHEN** a non-ephemeral request is active and the backend restarts
- **THEN** the same request ID is addressable with its prepared artifact
  relationship and expiration metadata restored

#### Scenario: Request expires into a tombstone

- **WHEN** an active request reaches its expiration time
- **THEN** it leaves active lookup, becomes an expired tombstone, and remains
  distinguishable from an unknown ID during the bounded tombstone window

#### Scenario: Tombstone retention ends

- **WHEN** an expired tombstone reaches its tombstone-retention deadline or is
  evicted to enforce the 500-tombstone bound
- **THEN** the tombstone row is purged and subsequent lookup of that ID returns
  the unknown-request result

#### Scenario: Pre-feature requests are absent

- **WHEN** an installation is upgraded without durable Compatible Preview
  request records
- **THEN** request IDs that existed only before the upgrade remain unknown after
  restart, while requests created after the upgrade use the new persistence

### Requirement: Explicit creation reactivates deterministic request IDs

Persistent request IDs SHALL remain deterministic for the same normalized
request. An explicit create/recreate operation SHALL be distinct from resource
lookup: it MAY replace an expired tombstone, reset the active lifecycle and
timestamps, and prepare a current request-to-artifact relationship. A resource
GET SHALL never revive an expired, stale, interrupted, failed, or canceled
request by itself.

#### Scenario: Create repeats an expired deterministic request

- **WHEN** a client POSTs the same normalized request while its deterministic
  request ID is an expired tombstone
- **THEN** creation returns the same request ID as a fresh active lifecycle,
  resets the appropriate timestamps and recovery state, and uses normal #1
  acquisition semantics for any later resource consumer

#### Scenario: Resource lookup does not revive expiration

- **WHEN** a client GETs a resource using an expired tombstone request ID
- **THEN** the backend returns `410` with `code=expired` and
  `retryable/recreate=true` without changing the request back to active

#### Scenario: Explicit recreate after expiration uses current configuration

- **WHEN** a request has expired, its profile or configuration has changed, and
  the client explicitly creates the same normalized request again
- **THEN** the new active lifecycle may prepare the current configuration and
  may reference a different artifact identity

### Requirement: Frozen prepared relationships survive restart

While an active persistent request's source revision remains valid, restart
recovery SHALL restore the prepared/frozen request-to-artifact relationship
created for that request rather than re-running mutable ProfileID,
configuration, or policy resolution. The versioned frozen snapshot SHALL contain
the minimum descriptor data required by existing video, source/output frame,
inspect, metrics, and API paths: resolved source path and source revision;
artifact ID, canonical encode signature, and final cache path; start, seconds,
preview mode, and normalization decision; requested and effective preview
profile snapshots where used; resolved video decision; requested/effective
video encoder; requested/effective QSV rate control; and the canonical FFmpeg
argument array required by existing inspection output. Contexts, waiters,
timers, goroutines, process state, and unbounded encoder logs SHALL NOT be
persisted.

#### Scenario: Profile changes do not mutate an active recovered request

- **WHEN** request R was prepared with ProfileID 7 and artifact X, profile 7
  changes while R remains active, and the backend restarts with the same source
  revision
- **THEN** R restores artifact X and its frozen prepared snapshot without
  re-resolving profile 7 to artifact Y

#### Scenario: Frozen snapshot is unavailable or invalid

- **WHEN** an active request cannot restore a complete, version-compatible
  prepared snapshot
- **THEN** the request is exposed as an explicit stale/recovery state and is
  not silently re-prepared or regenerated by a resource GET

#### Scenario: Explicit retry may replace the frozen relationship

- **WHEN** the client explicitly recreates an interrupted, stale, failed, or
  canceled request
- **THEN** the backend may prepare current inputs and replace the request's
  prepared relationship before normal #1 acquisition begins

### Requirement: Ready artifacts remain reusable after restart

The system SHALL keep a restored request usable under its existing request ID
when its frozen artifact is the canonical final artifact, is within the
controlled Compatible Preview cache root, and passes the existing lightweight
published-file validity checks. A valid restored ready artifact SHALL be
served or inspected through the existing #1 artifact path without starting a
new encode.

#### Scenario: Ready request and final artifact survive restart

- **WHEN** a request is ready, its final artifact is present and non-empty, and
  the backend restarts
- **THEN** the existing request ID continues to serve video, inspect, metrics,
  and supported output-frame consumers without a second encode or mutable
  descriptor re-resolution

#### Scenario: Ready metadata points to a missing artifact

- **WHEN** persisted metadata says an artifact is ready but the final file is
  missing or empty after restart
- **THEN** the request is not advertised as ready and returns an explicit
  stale/recovery response that does not serve the missing path

### Requirement: Generation attempts have durable identity and conditional writes

Each artifact generation attempt SHALL have a durable identity/version in
addition to the canonical artifact ID. A new attempt for the same artifact
MUST receive a new generation token before it becomes generating. Durable
state transitions SHALL be conditional on both artifact identity and the
expected generation token, or use equivalent transactional compare-and-swap
behavior. Runtime entry-object identity protection from #1 SHALL remain in
place; the durable generation token supplements it and does not replace it.

#### Scenario: Late completion cannot overwrite a replacement

- **WHEN** generation N for artifact X becomes terminal, explicit retry starts
  generation N+1, and generation N later attempts to persist completion
- **THEN** the late transition is rejected by generation identity and the
  durable state for generation N+1 remains unchanged

#### Scenario: Restart interrupts the current attempt

- **WHEN** startup finds artifact X at generation N with state `generating`
  and no valid published final artifact
- **THEN** generation N becomes `interrupted`, is not joinable, and an explicit
  retry creates generation N+1 rather than resurrecting generation N

#### Scenario: Crash-window completion promotes the same attempt

- **WHEN** generation N is persisted as `generating` but its canonical final
  artifact is complete and valid at startup
- **THEN** reconciliation conditionally promotes generation N to `ready`
  without creating a replacement attempt

### Requirement: Request and artifact lifecycle ownership remains distinct

The request record SHALL own request identity, frozen request-to-artifact
relationship, active/stale/expired-tombstone addressability, and request
timestamps. The artifact record SHALL own canonical artifact identity, the
current generation token, generation state (`generating`, `ready`, `failed`,
`canceled`, `interrupted`, or `stale`), publication/cache metadata, and
generation recovery reason. The system SHALL NOT duplicate one lifecycle into
both rows. Request expiration SHALL NOT imply artifact failure or deletion.

#### Scenario: Multiple requests share one artifact generation

- **WHEN** requests A, B, and C reference artifact X and its current
  generation N
- **THEN** all request rows retain their own addressability while one artifact
  generation owns publication and shared single-flight state

#### Scenario: One request expires while another remains active

- **WHEN** request A expires but request B still references valid artifact X
- **THEN** only A becomes an expired tombstone; artifact X and B remain usable
  subject to normal artifact retention and generation state

#### Scenario: Retry replaces a generation, not artifact identity by default

- **WHEN** a failed, canceled, or interrupted generation is explicitly
  retried without changing the prepared canonical descriptor
- **THEN** the artifact ID remains X, the old attempt stays terminal and
  non-joinable, and a new generation token owns the fresh attempt

### Requirement: Restart reconciliation never revives dead generation work

The system SHALL reconcile persisted in-flight generation conservatively. A
persisted `generating` state SHALL never be restored as an active joinable
generation because its owner process and cancellation context no longer exist.
If the canonical final artifact is complete and valid, reconciliation SHALL
promote the current generation to ready; otherwise it SHALL expose an
interrupted state. No resource GET SHALL silently start a replacement
generation.

#### Scenario: Generating metadata has a complete final artifact

- **WHEN** a request was persisted as generating at restart and its canonical
  final artifact is present and valid
- **THEN** the current generation is restored as ready and later consumers
  reuse that artifact without joining the dead process

#### Scenario: Generating metadata has no complete final artifact

- **WHEN** a request was persisted as generating at restart and its final
  artifact is missing, empty, or only a staging/partial file exists
- **THEN** the current generation becomes interrupted, no consumer joins it,
  and an explicit retry can start a fresh generation

### Requirement: Source revision invalidation is explicit

The restored frozen relationship SHALL be valid only while its persisted source
revision matches the source used to prepare it. If the source revision changes,
the request SHALL become stale and SHALL NOT serve the old artifact or silently
regenerate under the old request relationship. An explicit recreate/retry MAY
revalidate current source inputs and update the relationship through normal
creation semantics.

#### Scenario: Source changes after request preparation

- **WHEN** request R references artifact X prepared against source revision S1
  and the source is now revision S2
- **THEN** R returns `409` with `code=stale_artifact`, X is not served, and the
  frozen S1 relationship is not silently replaced by a resource GET

#### Scenario: Explicit recreate after source change

- **WHEN** the client explicitly recreates R after the source changes
- **THEN** current source revision and canonical preparation may produce a new
  artifact relationship Y under the same deterministic request ID

### Requirement: Terminal generation states remain deterministic but retryable

Failed, canceled, and interrupted generation attempts SHALL remain terminal and
non-joinable. This terminal rule applies to the generation token, not forever
to the canonical artifact ID. An explicit recreate/retry MAY create a fresh
generation for the same artifact ID or update the request to a new artifact
ID; a normal resource use SHALL return the existing terminal state instead of
silently retrying.

#### Scenario: Failed generation survives restart

- **WHEN** a failed generation has not been replaced or expired and the backend
  restarts
- **THEN** the same request returns a deterministic failed result with its
  bounded reason and does not join the failed generation

#### Scenario: Canceled generation survives restart

- **WHEN** a canceled generation has not been replaced or expired and the
  backend restarts
- **THEN** the same request returns a deterministic canceled result and does
  not resume or silently restart FFmpeg

#### Scenario: Explicit retry starts a new attempt

- **WHEN** the client explicitly retries a failed, canceled, or interrupted
  request
- **THEN** the old generation remains terminal/non-joinable and the fresh
  generation receives a new durable token through #1 coordination

### Requirement: Shared artifact identity remains canonical

The persisted request layer SHALL reference the canonical artifact identity
owned by #1 rather than deriving a new cache key or creating a second
single-flight lifecycle. Multiple request records that reference one artifact
SHALL be restored to that one artifact relationship and SHALL not cause
duplicate encodes solely because the backend restarted.

#### Scenario: Multiple requests share one artifact across restart

- **WHEN** two or more persistent request IDs reference the same canonical
  artifact and that artifact remains valid across restart
- **THEN** all restored requests reuse one ready artifact and no duplicate
  encode is started for the shared identity

#### Scenario: Recreating a tombstoned request does not duplicate a valid cache

- **WHEN** a deterministic request ID is explicitly recreated after its
  tombstone exists and its current preparation resolves to an already valid
  canonical artifact
- **THEN** the new request relationship acquires that artifact through #1
  rather than starting duplicate FFmpeg work

### Requirement: Expiration and cleanup remain bounded and coordinated

The system SHALL apply the existing two-hour access-based active-request
retention and 500 active-record bound, then retain expired tombstones for two
hours with a 500-tombstone bound. A tombstone is the only bounded evidence
needed to return explicit expiration; after its retention deadline or capacity
eviction, the request ID becomes unknown. Artifact cleanup SHALL continue to
honor #1's active-consumer/generation protections, persistent cache age/item/
size limits, and active-request references.

#### Scenario: Request expires while the backend is stopped

- **WHEN** an active request's expiration time passes before startup
- **THEN** startup converts it to an expired tombstone, retains it within the
  tombstone bounds, and a later resource use returns `410 expired`

#### Scenario: Tombstone is purged

- **WHEN** the tombstone retention deadline passes or the tombstone bound
  requires eviction
- **THEN** its row is deleted and a later resource use returns `404` with
  `code=unknown_request`

#### Scenario: Shared artifact outlives one request

- **WHEN** one request expires but another active request references the same
  valid persistent artifact
- **THEN** request cleanup removes only the expired active handle and does not
  delete or invalidate the shared artifact

### Requirement: Recovery states have explicit API and frontend semantics

The existing request resource routes SHALL return normal media responses for a
ready request and machine-readable lifecycle responses for expired, stale,
interrupted, failed, and canceled requests. Unknown IDs SHALL remain distinct
from expired IDs. Recovery responses SHALL include a stable `code`, `state`,
bounded `reason` when available, and `retryable` or `recreate` semantics, and
normal lifecycle conditions SHALL NOT use a generic 500 response.

#### Scenario: Frontend resumes with a ready request ID

- **WHEN** the frontend keeps a request ID while the backend restarts and the
  frozen artifact reconciles ready
- **THEN** existing polling/resource calls continue to work without requiring
  the frontend to know how persistence is implemented

#### Scenario: Interrupted or stale resource use is explicit

- **WHEN** a resource call uses a request whose generation was interrupted or
  whose frozen artifact is stale/missing
- **THEN** the backend returns `409` with `code=interrupted` or
  `code=stale_artifact`, marks it retryable/recreateable, and the frontend
  presents a useful explicit retry path

#### Scenario: Failed or canceled resource use is terminal

- **WHEN** a resource call uses a request whose current generation is failed or
  canceled
- **THEN** the backend returns deterministic `409` lifecycle data and the
  frontend does not hide it as a timeout or automatically restart it

#### Scenario: Expired and unknown lookups differ

- **WHEN** a resource call uses an expired tombstone or an ID whose tombstone
  has already been purged
- **THEN** the backend returns `410` with `code=expired` during tombstone
  retention and `404` with `code=unknown_request` afterward

### Requirement: Restored cache paths are safe and startup work is bounded

The system SHALL trust a persisted artifact path only after validating that it
is a canonical final path under the configured Compatible Preview cache root,
corresponds to the persisted artifact identity, and is covered by the frozen
source/encode relationship. It SHALL reject or ignore paths outside controlled
roots, arbitrary filenames, zero-byte files, staging files, and partial files.
Startup reconciliation SHALL use bounded metadata loading and direct
filesystem existence/stat/identity checks and SHALL NOT probe or decode every
cached preview.

#### Scenario: Persisted path escapes the preview cache root

- **WHEN** durable metadata contains an artifact path outside the canonical
  Compatible Preview cache/staging roots
- **THEN** the path is rejected, the request is not restored as ready, and no
  arbitrary file is served or deleted

#### Scenario: Large cache is restored

- **WHEN** many persisted preview records exist at startup
- **THEN** reconciliation remains bounded by metadata and lightweight final
  file checks, deferring deeper validation until consumption where safe

### Requirement: Filesystem and metadata crash windows reconcile safely

The system SHALL tolerate a crash between generation start, final publication,
durable state updates, cleanup, or cancellation. It SHALL treat the final
published artifact and durable metadata as independently recoverable evidence
and SHALL choose a safe stale, interrupted, ready, expired, failed, or
canceled state on the next reconciliation rather than assuming a cross-system
atomic transaction. All generation-owned durable updates SHALL remain
conditional on the generation token.

#### Scenario: Final file exists while durable state is still generating

- **WHEN** the process dies after atomically publishing a complete final file
  but before recording ready
- **THEN** startup validates the canonical final file and conditionally
  promotes the same generation to ready

#### Scenario: Metadata says ready after cleanup removed the file

- **WHEN** cleanup deletes a final artifact before the corresponding durable
  row is fully updated
- **THEN** the next reconciliation detects the missing file and removes the
  ready advertisement, returning stale/recovery semantics instead of serving a
  bad path

