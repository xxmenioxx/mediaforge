## ADDED Requirements

### Requirement: Canonical OCR identity and provenance

Every reusable bitmap OCR result SHALL have a versioned canonical identity that
includes the source identity and fingerprint, subtitle stream index, normalized
OCR language, normalized OCR mode, and canonical subtitle/OCR pipeline
provenance. The identity SHALL exclude the requested output format so equivalent
SRT, ASS, and logical `both` requests can consume the same canonical result.
The retained evidence SHALL record the identity, effective OCR language,
recognition provenance, and the canonical cue/geometry result it represents.

#### Scenario: Equivalent output formats share an OCR identity

- **WHEN** equivalent SRT and ASS requests use the same source fingerprint,
  subtitle stream, OCR language, OCR mode, and pipeline provenance
- **THEN** both requests resolve to one canonical OCR identity and may consume
  one reusable cue result while each requested serializer produces its own final
  format

#### Scenario: Identity includes all OCR-affecting inputs

- **WHEN** two requests differ in source identity or fingerprint, stream index,
  normalized OCR language, normalized OCR mode, or canonical pipeline
  provenance
- **THEN** they resolve to different OCR identities and the second request
  cannot reuse the first request's evidence

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
only publication path. `raw` SHALL serialize the requested final format or
formats directly from the canonical recognized cues without cleanup. `clean`
SHALL perform cleanup once, normalize the cleaned result back into canonical
cues, and fan that result out to the requested serializers. `accurate` SHALL
keep its internal OCR passes and selection evidence private, select one
canonical recognized result, perform cleanup once, and publish only the
requested final sidecars. SRT SHALL remain text/timing-only, while OCR-generated
ASS SHALL continue to consume retained geometry/layout metadata.

#### Scenario: Raw mode publishes requested final formats directly

- **WHEN** a bitmap conversion requests raw mode as SRT, ASS, or logical `both`
- **THEN** the requested final sidecar or sidecars are serialized directly from
  canonical recognized cues without cleanup and no `.raw.srt` or `.raw.ass`
  sidecar is published beside the media

#### Scenario: Clean mode cleans once and fans out

- **WHEN** a bitmap conversion requests clean mode for SRT, ASS, or logical
  `both`
- **THEN** recognition and cleanup each occur once for the canonical request,
  the cleaned canonical cues feed every requested serializer, and only the
  requested final sidecar formats are published

#### Scenario: Accurate mode keeps selection evidence internal

- **WHEN** a bitmap conversion requests accurate mode
- **THEN** all accurate-mode OCR passes and selection intermediates remain
  internal, one selected canonical result is cleaned once, and only the
  requested final SRT and/or ASS sidecars are published

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
workspace and staged publication files SHALL remain distinct from reusable
evidence and SHALL never be treated as cache entries merely because they exist.

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

### Requirement: OCR workspace cleanup and shared cancellation

The existing subtitle operation and cancellation model SHALL remain authoritative
for OCR lifecycle execution. Operation-owned temporary OCR workspace, raw pass
outputs, extraction intermediates, and staged final sidecars SHALL be cleaned
after terminal success, failure, or cancellation; reusable committed evidence
is exempt because it belongs to the internal cache. A cancelled consumer SHALL
stop its own publication and finish with the existing `cancelled` semantics. A
shared in-flight OCR execution MAY continue for other non-cancelled equivalent
consumers, but when its last consumer detaches it SHALL cancel child processes
through the existing context path and SHALL not commit evidence.

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
