## ADDED Requirements

### Requirement: Canonical OCR identity and provenance

Every reusable bitmap OCR result SHALL have a versioned canonical identity that
includes the source identity and fast-validatable fingerprint evidence, subtitle
stream index, normalized OCR language, normalized OCR mode, prepared-input
scope, and canonical subtitle/OCR pipeline provenance. Prepared-input scope
SHALL distinguish full-asset OCR from segmented/Test Encode OCR and SHALL
include segment start, segment duration, prepared stream mapping/extraction
representation, and its version. Where a smaller prepared subtitle
representation exists, its bounded fingerprint SHALL be included. The normal
identity path MUST NOT require hashing an entire large media file merely to
construct an OCR cache key. The identity SHALL exclude the requested output
format only after recognition, cleanup, and accurate-mode scoring use one
internal canonical representation independent of SRT or ASS. The retained
evidence SHALL record the identity, effective OCR language, recognition
provenance, and the canonical cue/timing/text/geometry result it represents.

#### Scenario: Equivalent output formats share an OCR identity

- **WHEN** equivalent SRT, ASS, and logical `both` requests use the same source
  stat/snapshot evidence, stream identity, OCR language, OCR mode,
  prepared-input scope, prepared-subtitle fingerprint where applicable, and
  pipeline provenance
- **THEN** both requests resolve to one canonical OCR identity and may consume
  one reusable cue result; their internal canonical cue/timing/text/geometry
  evidence is identical and each requested serializer produces its own final
  format

#### Scenario: Identity includes all OCR-affecting inputs

- **WHEN** two requests differ in source identity or fast source
  stat/snapshot evidence, stream index, normalized OCR language, normalized OCR
  mode, full-versus-segmented preparation scope, segment start or duration,
  prepared mapping/representation/version, prepared-subtitle fingerprint, or
  canonical pipeline/runtime provenance
- **THEN** they resolve to different OCR identities and the second request
  cannot reuse the first request's evidence

#### Scenario: Segmented OCR cannot satisfy full-asset OCR

- **WHEN** evidence was produced for a segmented/Test Encode input and a later
  request targets the full asset, or the preparation scope otherwise differs
- **THEN** the evidence is rejected as a cache miss even when stream index and
  OCR settings are equal

#### Scenario: Fingerprint uncertainty fails closed

- **WHEN** source stat/snapshot evidence, prepared-subtitle fingerprint, or
  provenance is missing, ambiguous, or changed
- **THEN** the cache does not reuse prior evidence and the canonical lifecycle
  may produce fresh evidence

#### Scenario: Effective language and provenance remain observable

- **WHEN** OCR succeeds with the requested language or an existing bounded
  language fallback
- **THEN** the canonical evidence records the effective language and the
  provenance used to produce it without changing the requested language or
  published artifact identity

### Requirement: Concurrent equivalent OCR requests are deduplicated

The canonical subtitle pipeline SHALL deduplicate concurrent equivalent bitmap
OCR requests by canonical OCR identity. One in-flight recognition/evidence
production SHALL serve all equivalent consumers, including consumers that later
serialize SRT and ASS separately. The deduplication layer SHALL reuse the
existing canonical subtitle planner and SHALL NOT create a second subtitle
planner, public job type, or operation model.

#### Scenario: Equivalent concurrent requests execute OCR once

- **WHEN** equivalent OCR requests for the same canonical identity arrive while
  no reusable evidence is complete
- **THEN** one OCR execution produces the canonical evidence and all consumers
  wait for or receive that result without starting duplicate recognition

#### Scenario: Shared progress reaches every subscriber

- **WHEN** equivalent operations attach to an in-flight OCR execution
- **THEN** every attached operation receives the existing meaningful `phase`,
  `progress`, `processed`, `total`, and diagnostic/message updates from the
  shared flight, including the current snapshot when joining after work began

#### Scenario: Cache reuse is observable without a new operation model

- **WHEN** a request is satisfied by valid internal canonical OCR evidence
- **THEN** the existing operation reports a clear reuse/cache-hit diagnostic and
  terminal progress through its existing API without starting OCR or creating
  another operation type

#### Scenario: Logical both retains one canonical execution

- **WHEN** one logical converted `both` request reaches the canonical pipeline
- **THEN** its existing shared transformation performs OCR once and fans the
  canonical result out to the requested SRT and ASS serializers

#### Scenario: Failed or cancelled evidence is not reusable

- **WHEN** an in-flight OCR execution fails or is cancelled before its evidence
  commit
- **THEN** no reusable evidence is exposed for that identity and a later retry
  may start a new execution

### Requirement: OCR modes publish only requested final sidecars

The canonical OCR lifecycle SHALL preserve the existing normalized `raw`,
`clean`, and `accurate` mode meanings while making canonical cue fan-out the
only publication path. Bitmap recognition, cleanup, and accurate-mode scoring
SHALL first produce or consume one internal canonical representation of cue
text, timing, source identity, and optional geometry that is independent of
requested output format. `raw` SHALL serialize the requested final format or
formats directly from the canonical recognized cues without cleanup. `clean`
SHALL perform cleanup once, normalize the cleaned result back into canonical
cues, and fan that result out to the requested serializers. `accurate` SHALL
keep its internal OCR passes and selection evidence private, select one
canonical recognized result, perform cleanup once, and publish only the
requested final sidecars. SRT SHALL remain text/timing-only, while OCR-generated
ASS SHALL be rendered from canonical cues plus retained geometry/layout
metadata; requesting ASS SHALL NOT cause an ASS-shaped OCR execution.

#### Scenario: Raw mode publishes requested final formats directly

- **WHEN** a bitmap conversion requests raw mode as SRT, ASS, or logical `both`
- **THEN** the requested final sidecar or sidecars are serialized directly from
  the format-independent canonical recognized cues without cleanup and no
  `.raw.srt` or `.raw.ass` sidecar is published beside the media

#### Scenario: Clean mode cleans once and fans out

- **WHEN** a bitmap conversion requests clean mode for SRT, ASS, or logical
  `both`
- **THEN** recognition and cleanup each occur once for the canonical request,
  the cleaned canonical cues feed every requested serializer, and only the
  requested final sidecar formats are published

#### Scenario: Accurate mode keeps selection evidence internal

- **WHEN** a bitmap conversion requests accurate mode
- **THEN** all accurate-mode OCR passes and selection intermediates remain
  internal and format-independent, one selected canonical result is cleaned
  once, and only the requested final SRT and/or ASS sidecars are published

#### Scenario: Equivalent SRT, ASS, and both share canonical evidence

- **WHEN** standalone SRT, standalone ASS, and logical `both` requests have
  equivalent OCR identity inputs
- **THEN** recognition, cleanup, and accurate-mode scoring produce identical
  canonical evidence before serializers run, and ASS is rendered from that
  evidence rather than causing a separate format-specific OCR result

#### Scenario: Existing shared geometry and serializer semantics remain

- **WHEN** OCR-generated ASS is produced through any supported mode
- **THEN** the canonical cue result retains existing bitmap geometry/layout for
  ASS, SRT ignores geometry, and native ASS/SSA extraction remains outside the
  OCR lifecycle

### Requirement: Reusable OCR evidence is internal and provenance-validated

Completed canonical OCR evidence SHALL be retained only in a backend-managed
internal cache or equivalent internal evidence store with explicit identity,
provenance, completeness, and integrity metadata. Evidence SHALL be committed
atomically and SHALL be reusable only when the stored identity and provenance
match the current request and the evidence validates. Temporary operation
workspace, shared-flight workspace, and staged publication files SHALL remain
distinct from reusable evidence and SHALL never be treated as cache entries
merely because they exist. The shared flight owns the evidence commit decision;
subscriber cancellation alone SHALL NOT invalidate a successful shared commit
for another attached subscriber.

#### Scenario: Provenance changes invalidate reusable evidence

- **WHEN** a source fingerprint, OCR runtime/provenance, canonical parser or
  pipeline provenance, stream identity, OCR language, or OCR mode changes
- **THEN** prior evidence is rejected for reuse and the request produces fresh
  canonical evidence under the new identity

#### Scenario: Internal evidence survives output fan-out without publication

- **WHEN** canonical OCR evidence is reused for SRT, ASS, or logical `both`
- **THEN** serializers read the internal canonical cues and geometry, publish
  only requested final sidecars, and do not expose the evidence path or create
  adjacent raw intermediates beside the media

#### Scenario: Invalid or incomplete cache evidence is ignored

- **WHEN** an internal evidence entry has a missing payload, incomplete commit,
  mismatched manifest, or failed integrity validation
- **THEN** it is not used for conversion, is eligible for safe internal cleanup,
  and the canonical pipeline may regenerate evidence

#### Scenario: Remaining subscriber permits a valid cache commit

- **WHEN** subscriber A cancels while subscriber B remains attached and the
  shared flight completes canonical recognition/cleanup successfully with
  valid integrity
- **THEN** A detaches and does not publish, B may receive the result, and the
  shared flight may atomically commit reusable evidence for B

#### Scenario: Shared-flight cancellation prevents cache commit

- **WHEN** the shared flight itself fails, is cancelled because no subscribers
  remain, or fails canonical validation/integrity before its commit barrier
- **THEN** no reusable evidence is committed, the shared-flight workspace is
  cleaned, and a later retry may execute fresh OCR

### Requirement: OCR workspace cleanup and shared cancellation

The existing subtitle operation and cancellation model SHALL remain authoritative
for OCR lifecycle execution. A shared in-flight OCR execution SHALL own a
separate shared workspace and shared execution context for prepared bitmap
input, OCR passes, cleanup intermediates, and evidence production. Each
subscriber subtitle operation SHALL own only its own serializer
staging/publication workspace. Operation-owned temporary OCR workspace, raw
pass outputs, extraction intermediates, and staged final sidecars SHALL be
cleaned after terminal success, failure, or cancellation; reusable committed
evidence is exempt because it belongs to the internal cache. Cancelling one
consumer SHALL detach it and clean only its operation-owned workspace; it MUST
NOT delete the active shared-flight workspace. A shared in-flight OCR execution
MAY continue for other non-cancelled equivalent consumers, but when its last
consumer detaches it SHALL cancel its shared context, terminate child processes
through the existing process-tree mechanism, clean the shared-flight workspace,
and SHALL not commit evidence.

#### Scenario: Successful completion removes temporary workspace

- **WHEN** an OCR conversion reaches terminal completion after final sidecars
  pass validation and publication
- **THEN** its temporary OCR workspace and non-reusable intermediates are gone,
  while any committed internal evidence remains only in the internal cache

#### Scenario: Failure or cancellation removes temporary workspace

- **WHEN** OCR fails or an operation is cancelled before the publication
  barrier
- **THEN** operation-owned temporary files and staged outputs are cleaned using
  the existing bounded cleanup path, no partial sidecar is published, and
  existing sidecars remain preserved

#### Scenario: Subscriber cleanup does not delete shared workspace

- **WHEN** subscriber A cancels while subscriber B remains attached to the same
  in-flight OCR identity
- **THEN** A's operation workspace and staged outputs are cleaned, the shared
  flight workspace remains active for B, and A's cleanup cannot remove B's
  prepared input or OCR intermediates

#### Scenario: One cancelled consumer does not cancel remaining consumers

- **WHEN** one operation waiting on shared in-flight OCR is cancelled while an
  equivalent non-cancelled consumer remains attached
- **THEN** the cancelled operation becomes terminal under the existing
  cancellation contract without publishing output, while the shared execution
  may finish for the remaining consumer

#### Scenario: Last cancellation terminates shared OCR

- **WHEN** all consumers of an in-flight canonical OCR execution cancel or fail
  before the evidence commit
- **THEN** the shared OCR context cancels FFmpeg, SeConv, Tesseract, and other
  child processes through the existing process-tree handling, cleans its
  workspace, and leaves no reusable evidence

#### Scenario: Retry creates a new operation and may rerun OCR

- **WHEN** the same conversion is retried after a cancelled or failed operation
- **THEN** the retry receives a new operation identity, the terminal prior
  operation remains unchanged, and the retry either reuses valid matching
  internal evidence or starts fresh OCR when none is reusable
