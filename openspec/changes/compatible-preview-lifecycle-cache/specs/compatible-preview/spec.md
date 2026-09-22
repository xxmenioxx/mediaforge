## Purpose

Provide a bounded, request-scoped Compatible Preview lifecycle whose registered
resources share one canonical encoded artifact, remain safe under cancellation,
and cannot accidentally derive resource URLs from an empty or stale request ID.

## ADDED Requirements

### Requirement: Registered requests have distinct request and artifact identities

The system SHALL normalize a valid Compatible Preview request before assigning
its identity. A successful registration MUST return a non-empty `requestId`,
the request expiry information, and an opaque `cacheIdentity` that represents
the canonical artifact scope rather than serving as an alias for `requestId`.
The canonical encode signature and artifact identity MUST remain distinct from
the request handle even when the API exposes the artifact identity as
`cacheIdentity`.

For non-ephemeral requests, equivalent normalized request intent MUST produce a
deterministic request identity for as long as the in-memory request record is
available. An ephemeral request MUST have a stable ID for its own registered
request, but MAY use a fresh request scope so independent registrations do not
become an unintended persistent cache share.

The registered request record MAY remain in memory with bounded TTL and size
cleanup. This requirement does not add request-record persistence across a
backend restart; an unavailable post-restart request ID is handled as unknown.

#### Scenario: Request creation returns a usable handle

- **WHEN** a valid request is registered
- **THEN** the response contains a non-empty `requestId`, a non-empty opaque
  artifact/cache identity, and an explicit expiration duration

#### Scenario: Equivalent normalized intent is deterministic

- **WHEN** two non-ephemeral registrations contain equivalent values that
  normalize to the same request intent
- **THEN** they address the same deterministic request identity while that
  request record is retained, without implying that the request ID is the
  encoded artifact identity

#### Scenario: Empty, unknown, or expired request IDs are rejected

- **WHEN** a resource endpoint receives an empty, unknown, or expired request
  ID
- **THEN** it returns a clear client-visible not-found/expired error and does
  not start an encode

### Requirement: Canonical encode identity is operation-independent

The system SHALL derive one canonical encode signature from a normalized,
backend-authoritative execution description. The signature MUST include every
material input that can change encoded output, including as applicable the
resolved source path, source revision/fingerprint (at least size and
modification time or existing canonical revision evidence), start, duration,
effective resolved video profile, effective encoder, effective codec arguments,
video filters, cadence/output FPS, geometry/upscale, output color or other
normalization, subtitle burn-in selection, rate-control settings,
frame-structure settings, and other actual encode-semantic inputs.

The signature MUST NOT include consumer-only behavior such as `video` versus
`inspect` versus `metrics`, output-frame selection, diagnostic logging
verbosity, response headers, HTTP timing, or random timestamps. The existing
backend effective profile/video resolver SHALL remain authoritative; this
capability MUST NOT introduce a second video policy resolver.

`artifactId`/`cacheIdentity` SHALL be derived from the canonical signature and
the retention scope. A persistent artifact MAY be reused by independent
requests with the same signature. An ephemeral artifact MAY include the
registered request scope so it is not reused by independent requests, but all
consumers of that one request MUST resolve to its same artifact identity.

#### Scenario: QSV inspection does not change the artifact identity

- **WHEN** `/video` and `/inspect` use the same registered request whose
  effective encoder is `hevc_qsv`
- **THEN** both operations resolve the same canonical artifact identity even
  if inspection requires richer bounded encoder stderr diagnostics

#### Scenario: Source revision invalidates an old artifact identity

- **WHEN** the source file's canonical revision evidence changes after an
  artifact was created
- **THEN** a later preparation derives a different artifact identity and does
  not serve the old encoded bytes as the current result

#### Scenario: Ephemeral scope is shared only within one request

- **WHEN** `/video`, `/inspect`, `/metrics`, and output-frame consumers use the
  same ephemeral request
- **THEN** they all address one canonical artifact and one retention scope,
  while an independent ephemeral registration MAY receive a different
  artifact identity and MUST NOT create an unbounded cross-request cache

### Requirement: Registered consumers share one disk-backed artifact

The system SHALL produce at most one canonical encoded artifact for a given
artifact identity, including when consumers arrive concurrently. `/video`,
`/inspect`, `/metrics`, and output-frame resources MUST acquire the same
artifact before serving or analyzing it. Once the artifact is ready, repeated
consumers MUST use it without another video encode.

Inspection and metrics MAY run additional bounded ffprobe, frame, or other
lightweight analysis against the ready artifact, but such analysis MUST NOT
start another video encode. Source-frame resources MAY use the canonical source
asset directly because they do not depend on encoded output.

The artifact MUST remain on disk and existing video/range serving behavior MUST
be preserved. Implementations MUST NOT load an unbounded encoded payload into
memory merely to coordinate or serve shared consumers.

#### Scenario: Video followed by inspection reuses one encode

- **WHEN** `/video` completes for a registered request and `/inspect` is then
  requested for that request
- **THEN** inspection uses the ready artifact and the backend performs one
  video encode total

#### Scenario: Inspection followed by video reuses one encode

- **WHEN** `/inspect` completes first for a registered request and `/video` is
  then requested
- **THEN** video uses the same ready artifact and the backend performs one
  video encode total

#### Scenario: Concurrent metrics and video share generation

- **WHEN** `/metrics` and `/video` arrive concurrently for one registered
  request
- **THEN** both consumers wait for and use one canonical generation, with no
  duplicate video encode

#### Scenario: Output frame, inspection, and video share generation

- **WHEN** output-frame, `/inspect`, and `/video` consumers arrive concurrently
  for one registered request
- **THEN** they observe one terminal artifact result and the backend performs
  one video encode total

#### Scenario: Ready consumers are cache hits

- **WHEN** a registered request's artifact is ready and any supported consumer
  is repeated
- **THEN** the response is served or analyzed from that artifact without
  starting another encode

### Requirement: Artifact lifecycle separates cancellation from failure

The system SHALL expose an explicit lifecycle sufficient to distinguish
registered, generating, ready, canceled, failed, and expired states (exact
internal names may differ). `ready` MUST mean that the complete canonical
artifact is published and usable. Concurrent waiters MUST observe one terminal
result for a generation.

Consumer cancellation MUST detach that consumer's wait without canceling a
shared generation that still has another active consumer. If backend policy
intentionally cancels a generation, the result MUST be classified as canceled,
not as a generic FFmpeg failure. A canceled waiter MUST receive cancellation
semantics and MUST NOT be reported as a generic encode 500.

Failed or canceled generations MUST NOT publish a ready cache entry, MUST clean
owned partial files safely, and MUST allow a later valid request to retry.
Actual FFmpeg failures MUST remain failures with bounded diagnostics and MUST
not be hidden as cancellation.

#### Scenario: One canceled waiter leaves another consumer usable

- **WHEN** one consumer stops waiting while another consumer still waits for
  the same generating artifact
- **THEN** the first consumer detaches with cancellation semantics and the
  shared generation remains usable for the remaining consumer

#### Scenario: Backend cancellation is distinct from failure

- **WHEN** the backend intentionally cancels a generation
- **THEN** all remaining waiters observe a canceled result and not a generic
  FFmpeg/HTTP 500 failure

#### Scenario: Partial output is never published

- **WHEN** generation is canceled or fails after writing a staging file
- **THEN** the partial file is removed or quarantined safely and no later
  consumer treats it as a ready artifact

#### Scenario: Cancellation can be retried

- **WHEN** a generation is canceled and a later valid consumer requests the
  same artifact identity
- **THEN** the later request can start a fresh generation successfully

#### Scenario: Actual FFmpeg failure remains retryable

- **WHEN** FFmpeg exits with a real encode error
- **THEN** the generation is failed with bounded diagnostics, no corrupt cache
  entry is published, and a later attempt can retry

### Requirement: Diagnostic evidence belongs to the canonical artifact

The system SHALL capture the bounded encoder evidence required by downstream
inspection consumers as part of canonical artifact generation or artifact
metadata. Diagnostic detail MAY be richer than ordinary response logging, but
it MUST NOT alter the encoded artifact identity or cause `/inspect` to launch a
second encode. Inspection SHALL reuse available encoder evidence, including
QSV frame-structure evidence when present, and MAY supplement it with analysis
of the canonical artifact.

#### Scenario: Inspection reuses QSV encoder evidence

- **WHEN** a registered request uses QSV and an inspection consumer reads the
  generated artifact
- **THEN** inspection can use bounded artifact-associated encoder diagnostics
  without changing the artifact key or rerunning FFmpeg video encoding

### Requirement: Frontend resource URLs require a validated request ID

The frontend SHALL make a registered Compatible Preview resource URL or fetch
only after request creation succeeds with a validated non-empty `requestId`.
It MUST never construct or render `/requests//video`, `/requests//inspect`,
`/requests//metrics`, or `/requests//frame`, and MUST reject an empty or
whitespace-only ID before transport. A source URL using the documented legacy
scalar endpoint is not a request-scoped resource, but structured profiles MUST
continue to use registered requests.

When material request inputs change, the frontend SHALL invalidate or clear
the old request-scoped IDs before using a new configuration. It MUST NOT reuse
an old ID or render a request-scoped resource while a replacement request is
missing or pending. Consumer cancellation SHALL not be presented as a
misleading generic preview error. Existing credentialless (`credentials:
omit`) transport behavior MUST remain intact.

#### Scenario: Resource use starts after request creation

- **WHEN** request creation succeeds with a non-empty `requestId`
- **THEN** video, inspection, metrics, and frame URLs/fetches may be created
  using that validated ID

#### Scenario: Empty ID cannot create a request-scoped URL

- **WHEN** a component or API helper receives an empty request ID
- **THEN** it does not construct or fetch any `/requests/<resource>` URL and
  exposes a local validation state instead

#### Scenario: Stale ID is cleared after input change

- **WHEN** draft/request inputs materially change before a replacement request
  is ready
- **THEN** the previous request-scoped ID is not used for the new configuration
  and no request-scoped resource URL is rendered without the new valid ID

#### Scenario: Cancellation is not shown as a generic failure

- **WHEN** a preview or fidelity consumer is aborted by its caller
- **THEN** the frontend stops waiting without showing a generic encode failure,
  while non-cancellation HTTP or encode failures remain visible

#### Scenario: Credentialless behavior is preserved

- **WHEN** a Compatible Preview JSON or resource fetch is issued
- **THEN** the request remains credentialless with `credentials: omit`

### Requirement: Legacy scalar preview has one compatibility boundary

The legacy `/api/assets/preview/compatible?...query...` endpoint SHALL either
reuse the same canonical preparation, artifact identity, single-flight, and
publication primitive as registered requests or return an explicitly
documented compatibility result that does not create a competing encode/cache
engine. Its operation-specific calls MUST still exclude consumer-only
diagnostics from artifact identity. Structured profile requests SHALL continue
to use registered request creation rather than placing large profiles in the
legacy scalar URL.

#### Scenario: Legacy scalar video uses the canonical primitive

- **WHEN** a valid legacy scalar request asks for a compatible video
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
