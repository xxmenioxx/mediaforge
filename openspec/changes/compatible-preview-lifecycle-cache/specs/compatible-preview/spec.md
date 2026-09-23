## Purpose

Provide a bounded, request-scoped Compatible Preview contract whose registered
resources share one canonical encoded artifact, remain safe under cancellation,
and cannot derive resource URLs from an empty or stale request ID.

## ADDED Requirements

### Requirement: Request records have an independent bounded lifecycle

The request-record domain SHALL contain only a registered state and an
expired/unknown state. A successful registration MUST normalize a valid
Compatible Preview request and return a non-empty `requestId`, expiration
information, and an opaque `cacheIdentity` that describes artifact identity or
scope rather than aliasing the request handle.

For non-ephemeral requests, equivalent normalized intent MUST produce a
deterministic request identity for as long as its in-memory record is retained.
An ephemeral registration MUST have a stable ID for its own registered request
but MAY use a fresh request scope for each independent registration.

Request expiry SHALL remove only the request handle. It MUST NOT imply deletion
of a persistent artifact that remains valid for another request. Artifact
generation failure or cancellation SHALL NOT expire a still-registered request;
that request MUST be able to retry generation.

This requirement does not add request-record persistence across backend restart;
an unavailable post-restart request ID is handled as unknown until #14.

#### Scenario: Valid creation returns a usable handle

- **WHEN** a valid request is registered
- **THEN** the response contains a non-empty `requestId`, a non-empty opaque
  artifact/cache identity, the normalized path, and an explicit expiration
  duration

#### Scenario: Equivalent normalized intent is deterministic

- **WHEN** two non-ephemeral registrations contain equivalent values that
  normalize to the same request intent
- **THEN** they address the same deterministic request identity while the
  request record is retained, without implying that the request ID is the
  encoded artifact identity

#### Scenario: Expiry removes only the request handle

- **WHEN** a request record expires while its persistent artifact remains valid
  for another request
- **THEN** the expired request ID is rejected but the unrelated valid request
  continues to use the artifact

#### Scenario: Artifact cancellation does not expire a request

- **WHEN** generation for a registered request is canceled or fails
- **THEN** the request remains registered and a later valid consumer can retry
  generation without registering a new request solely because of that artifact
  result

#### Scenario: Empty, unknown, or expired IDs are rejected

- **WHEN** a resource endpoint receives an empty, unknown, or expired request
  ID
- **THEN** it returns a clear client-visible not-found/expired error and does
  not start artifact generation

### Requirement: Registration prepares a bounded reusable canonical descriptor

Registration MAY perform the bounded normalization, source revision capture,
and existing backend effective-profile/video resolution required to return the
artifact `cacheIdentity`, but it MUST NOT encode media. The prepared canonical
descriptor SHALL be stored on the request record and acquisitions SHALL reuse
it for `/video`, `/inspect`, `/metrics`, and output-frame consumers.

Registration MUST NOT cause the same expensive preparation to run once per
consumer. Only a source revision change or another material invalidation MAY
cause descriptor recomputation. Where available, preparation MUST reuse
persisted analysis and existing effective-resolver evidence rather than adding
unnecessary repeated probes. Request creation MUST NOT become a Fidelity or
background-analysis workflow owned by #19.

The current backend effective video decision from merged #12/#128 SHALL remain
authoritative, including its resolved HEVC Level/Tier result and provenance.
Compatible Preview MUST consume that decision and MUST NOT introduce another
video resolver or alter #12 policy.

#### Scenario: Registration does not encode

- **WHEN** a valid request is registered and its canonical descriptor is
  prepared
- **THEN** registration performs no FFmpeg video encode and returns without
  starting a consumer generation

#### Scenario: Consumers reuse the registered descriptor

- **WHEN** `/video`, `/inspect`, `/metrics`, and output-frame consumers use one
  registered request without material invalidation
- **THEN** they reuse the stored descriptor and do not repeat expensive profile,
  analysis, or probe preparation for each consumer

#### Scenario: Existing effective video decision is consumed

- **WHEN** the merged #12 effective video decision contains an encoder,
  effective HEVC Level/Tier state, or related warning/provenance
- **THEN** the Compatible Preview descriptor includes the effective decision
  needed by its command and artifact identity without creating a second resolver

#### Scenario: Source revision invalidates the descriptor

- **WHEN** source revision evidence changes after a descriptor or artifact was
  prepared
- **THEN** the next acquisition recomputes the descriptor and derives a new
  artifact identity instead of serving old encoded bytes

### Requirement: Canonical encode identity is operation-independent

The system SHALL derive one canonical encode signature from the normalized,
backend-authoritative execution description. The signature MUST include every
material input that can change encoded output, including as applicable the
resolved source path, source revision/fingerprint (at least size and
modification time or existing canonical revision evidence), start, duration,
effective resolved video profile and effective video decision, effective
encoder, codec arguments, video filters, cadence/output FPS, geometry/upscale,
output color or other normalization, subtitle burn-in selection, rate-control
settings, frame-structure settings, and other actual encode-semantic inputs.

The signature MUST NOT include consumer-only behavior such as `video` versus
`inspect` versus `metrics`, output-frame selection, diagnostic logging
verbosity, response headers, HTTP timing, or random timestamps.

`artifactId`/`cacheIdentity` SHALL be derived from the canonical signature and
retention scope. A persistent artifact MAY be reused by independent requests
with the same signature. An ephemeral artifact MAY include the registered
request scope so it is not reused by independent requests, but all consumers
of that one request MUST resolve to its same artifact identity.

#### Scenario: QSV inspection does not change identity

- **WHEN** `/video` and `/inspect` use the same registered request whose
  effective encoder is `hevc_qsv`
- **THEN** both operations resolve the same artifact identity even if
  inspection requires richer bounded encoder diagnostics

#### Scenario: Source change prevents stale reuse

- **WHEN** the source file revision changes after an artifact was created
- **THEN** a later preparation derives a different artifact identity and does
  not serve the old encoded bytes as the current result

#### Scenario: Ephemeral identity is request-scoped

- **WHEN** two independent ephemeral requests have equivalent encode intent
- **THEN** each may have its own artifact identity and neither is entered as a
  persistent cross-request cache artifact

### Requirement: Artifact generation has an independent lifecycle

The artifact-generation domain SHALL distinguish `absent`, `generating`,
`ready`, `canceled`, and `failed` states independently from the request-record
domain. `ready` SHALL mean only that the complete canonical encoded artifact
has been atomically published and is usable.

The system SHALL produce at most one generation for a given artifact identity,
including concurrent consumers. `/video`, `/inspect`, `/metrics`, and
output-frame resources MUST acquire the same artifact before serving or
analyzing it. Failed or canceled artifact generations MUST NOT expire the
registered request, publish a ready cache entry, or poison a later retry.

#### Scenario: Video followed by inspection uses one artifact

- **WHEN** `/video` completes for a registered request and `/inspect` is then
  requested
- **THEN** inspection uses the ready artifact and the backend performs one
  video encode total

#### Scenario: Inspection followed by video uses one artifact

- **WHEN** `/inspect` completes first and `/video` is then requested for the
  same request
- **THEN** video uses the same ready artifact and the backend performs one
  video encode total

#### Scenario: Concurrent metrics and video share generation

- **WHEN** `/metrics` and `/video` arrive concurrently for one registered
  request
- **THEN** both wait for and use one canonical generation with no duplicate
  video encode

#### Scenario: Output frame, inspection, and video share generation

- **WHEN** output-frame, `/inspect`, and `/video` consumers arrive concurrently
- **THEN** they observe one terminal artifact result and the backend performs
  one video encode total

#### Scenario: Ready consumers do not regenerate

- **WHEN** an artifact is ready and any supported consumer is repeated
- **THEN** the consumer uses that artifact without starting another encode

#### Scenario: Source frame remains source-backed

- **WHEN** a source-frame resource is requested
- **THEN** it may use the canonical source asset directly and does not require
  an output-artifact generation

### Requirement: Zero-consumer generation cancellation is deterministic

An in-flight artifact generation with at least one attached consumer MUST NOT
be canceled because another consumer detaches. When the attached-consumer
count reaches zero, the artifact manager SHALL cancel the in-flight generation.
It MAY use a short bounded grace period to absorb an immediate handoff, but it
MUST NOT allow an orphan encode to run indefinitely.

The zero-consumer transition SHALL be classified as `canceled`, never
`failed`. A later consumer MAY start a fresh generation. Detaching a consumer
MUST NOT remove or invalidate a ready artifact.

Each consumer SHALL be able to stop waiting using its own HTTP context. A
canceled waiter MUST receive cancellation semantics and MUST NOT turn a valid
shared generation into a generic encode 500. Concurrent waiters MUST observe
one terminal result for the shared generation.

#### Scenario: One canceled waiter leaves another consumer usable

- **WHEN** one consumer detaches while another remains attached to the same
  generating artifact
- **THEN** the first consumer receives cancellation semantics and generation
  remains usable for the remaining consumer

#### Scenario: Zero consumers cancel the generation

- **WHEN** the attached-consumer count reaches zero for an in-flight artifact
  after any bounded handoff grace period
- **THEN** the manager cancels the generation, cleans its staging files, and
  reports canceled rather than failed

#### Scenario: Canceled generation can be retried

- **WHEN** a generation is canceled and a later valid consumer requests the
  same artifact identity
- **THEN** a fresh generation can start successfully while the request record
  remains valid

#### Scenario: Ready artifact survives detachment

- **WHEN** a consumer detaches after an artifact reaches ready
- **THEN** the ready artifact remains usable under its retention policy and is
  not canceled or deleted merely because that consumer stopped waiting

#### Scenario: Backend cancellation is distinct from failure

- **WHEN** the backend intentionally cancels a generation
- **THEN** waiters observe a canceled result and not a generic FFmpeg/HTTP 500
  failure

### Requirement: Publication, cleanup, and diagnostic evidence are safe

Generation SHALL keep encoded artifacts on disk and MUST NOT load an unbounded
encoded payload into memory merely to coordinate or serve consumers. A
canceled or failed generation MUST NOT publish a partial file as ready; owned
staging files and bounded diagnostic output SHALL be cleaned safely. Actual
FFmpeg failures MUST remain failures with bounded diagnostics and MUST be
retryable.

The canonical generation SHALL capture the bounded encoder evidence required by
inspection consumers as artifact metadata. Diagnostic detail MAY be richer than
ordinary response logging, but it MUST NOT alter artifact identity or cause
`/inspect` to launch a second video encode. Inspection and metrics MAY perform
additional lightweight analysis against the ready artifact.

#### Scenario: Partial output is never ready

- **WHEN** generation is canceled or fails after writing a staging file
- **THEN** the partial file is removed or quarantined safely and no later
  consumer treats it as a ready artifact

#### Scenario: Actual FFmpeg failure is retryable

- **WHEN** FFmpeg exits with a real encode error
- **THEN** the artifact is failed with bounded diagnostics, no corrupt cache
  entry is published, and a later attempt can retry

#### Scenario: Inspection reuses QSV evidence

- **WHEN** a QSV request is inspected after its artifact is generated
- **THEN** inspection can use bounded artifact-associated encoder diagnostics
  without changing the artifact identity or rerunning video encoding

### Requirement: Ephemeral artifacts have explicit bounded retention

An ephemeral artifact SHALL be shared by all consumers of its one registered
ephemeral request. It MUST NOT be reused as a persistent cross-request cache
entry and MUST NOT be deleted at the end of each individual HTTP consumer.

The artifact SHALL be protected from cleanup while generation, serving, or
consumer analysis is active. It SHALL be cleaned by a bounded idle/TTL policy
and remain subject to bounded cache item/size limits. It becomes eligible for
cleanup when its request scope has expired and no consumer is active, subject
to the same bounded housekeeping rules.

#### Scenario: Sequential ephemeral consumers reuse one artifact

- **WHEN** `/video` completes and `/inspect`, `/metrics`, or output-frame is
  later requested for the same ephemeral request
- **THEN** all consumers use the one request-scoped artifact without another
  encode

#### Scenario: Active ephemeral work is protected

- **WHEN** an ephemeral artifact is generating, serving, or being analyzed
- **THEN** cleanup does not remove it or its required diagnostics

#### Scenario: Expired idle ephemeral scope is boundedly cleaned

- **WHEN** an ephemeral request scope has expired and no consumer remains
  active
- **THEN** the artifact is eligible for bounded idle/TTL or item/size cleanup
  and is not retained indefinitely

### Requirement: Registry synchronization does not serialize unrelated work

Global request and artifact registry locks SHALL protect only in-memory state
changes such as lookup, state transition, waiter accounting, and descriptor
pointer replacement. They MUST NOT be held while performing filesystem I/O,
database queries, ffprobe, media analysis, effective profile/video resolution,
or FFmpeg.

Source revision checks and descriptor recomputation SHALL use per-entry or
per-artifact synchronization, or copy/prepare/update semantics, so unrelated
Compatible Preview requests can prepare and acquire artifacts independently.
The implementation MUST NOT introduce a scheduler or distributed job engine to
provide this isolation.

#### Scenario: Unrelated requests prepare independently

- **WHEN** two different registered requests require source/profile
  preparation or artifact acquisition concurrently
- **THEN** slow I/O, resolver, probe, analysis, or FFmpeg work for one request
  does not hold a global registry lock that blocks the other request's
  in-memory progress and preparation

#### Scenario: State transitions remain synchronized

- **WHEN** concurrent consumers attach, detach, publish, fail, or cancel one
  artifact
- **THEN** its state and waiter accounting remain consistent without holding a
  global lock during external work

### Requirement: Frontend resource URLs require a validated request ID

The frontend SHALL make a registered Compatible Preview resource URL or fetch
only after request creation succeeds with a validated non-empty `requestId`.
It MUST never construct or render `/requests//video`, `/requests//inspect`,
`/requests//metrics`, or `/requests//frame`, and MUST reject an empty or
whitespace-only ID before transport. A documented legacy scalar source URL is
not request-scoped, but structured profiles MUST continue to use registered
requests.

When material request inputs change, the frontend SHALL clear or invalidate
old request-scoped IDs before using a new configuration. It MUST NOT reuse an
old ID or render a request-scoped resource while a replacement request is
missing or pending. Consumer cancellation SHALL not be presented as a
misleading generic preview error. Existing credentialless (`credentials:
omit`) behavior MUST remain intact.

#### Scenario: Resource use starts after creation

- **WHEN** request creation succeeds with a non-empty `requestId`
- **THEN** video, inspection, metrics, and frame URLs/fetches may be created
  using that validated ID

#### Scenario: Empty ID cannot create a resource URL

- **WHEN** a component or API helper receives an empty request ID
- **THEN** it does not construct or fetch any `/requests/<resource>` URL and
  exposes a local validation or pending state

#### Scenario: Stale ID is cleared after input change

- **WHEN** draft/request inputs materially change before a replacement request
  is ready
- **THEN** the previous request-scoped ID is not used for the new configuration
  and no request-scoped URL is rendered without the new valid ID

#### Scenario: Cancellation is not a generic frontend failure

- **WHEN** a preview or fidelity consumer is aborted by its caller
- **THEN** the frontend stops waiting without showing a generic encode error,
  while non-cancellation HTTP or encode errors remain visible

#### Scenario: Credentialless behavior is preserved

- **WHEN** a Compatible Preview JSON or resource fetch is issued
- **THEN** the request remains credentialless with `credentials: omit`

### Requirement: Legacy scalar preview has one compatibility boundary

The legacy `/api/assets/preview/compatible?...query...` endpoint SHALL reuse
the same canonical preparation, artifact identity, single-flight, and atomic
publication primitive as registered requests. Its operation-specific calls
MUST exclude consumer-only diagnostics from artifact identity. Structured
profile requests SHALL continue to use registered request creation rather than
placing large profiles in the legacy scalar URL.

The scalar endpoint does not provide a registered request handle for sharing
across separate legacy calls; identical persistent signatures MAY still hit a
ready persistent artifact. It MUST NOT create a competing encode/cache engine.

#### Scenario: Legacy scalar video uses the canonical primitive

- **WHEN** a valid legacy scalar request asks for compatible video
- **THEN** it uses the same canonical artifact policy and does not create a
  second encode/cache implementation

#### Scenario: Legacy scalar inspection does not fork the artifact key

- **WHEN** the legacy endpoint requests inspection for the same effective
  scalar encode as video
- **THEN** diagnostic operation details do not force a separate video encode

#### Scenario: Structured profiles remain registered

- **WHEN** a caller supplies a structured profile for Compatible Preview
- **THEN** the caller registers a request and uses its request ID rather than
  serializing that profile into the legacy scalar URL
