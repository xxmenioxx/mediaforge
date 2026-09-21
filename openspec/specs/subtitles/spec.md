# subtitles Specification

## Purpose
Provide one observable subtitle conversion contract so Asset Info, Track Profiles,
Profile Lab, and Queue produce compatible SRT, ASS, and original SSA artifacts.

## Requirements

### Requirement: Canonical converted subtitle formats
The system SHALL resolve a converted subtitle request through the canonical
pipeline and produce SRT, ASS, or both according to the requested format. A
logical `both` request SHALL be one subtitle transform that performs text
normalization or bitmap OCR once, then renders both artifacts from the same
normalized cue result. SRT SHALL be the default when no converted format is
specified. This change SHALL introduce no new OCR semantics, sidecar
discovery or matching behavior, or parallel subtitle pipeline.

#### Scenario: Converted SRT
- **WHEN** a text or OCR-capable bitmap subtitle is requested as converted SRT
- **THEN** the canonical pipeline produces a `.srt` artifact with normalized cues

#### Scenario: Converted ASS
- **WHEN** a text or OCR-capable bitmap subtitle is requested as converted ASS
- **THEN** the canonical pipeline produces a `.ass` artifact with the converted cues

#### Scenario: Default format
- **WHEN** a converted subtitle request omits an output format
- **THEN** the pipeline selects SRT and preserves existing SRT behavior

#### Scenario: Both converted formats share one cue result
- **WHEN** a text or OCR-capable bitmap subtitle is requested as one logical converted `both` operation
- **THEN** the canonical pipeline performs text normalization or bitmap OCR once and produces both `.srt` and `.ass` artifacts from the same normalized cues

### Requirement: Text and bitmap source handling
The system SHALL route text subtitle streams directly through canonical
conversion and SHALL evaluate PGS and VobSub streams through OCR before deciding
whether SRT or ASS output can be generated. Bitmap streams MUST NOT be rejected
before OCR capability is evaluated.

#### Scenario: Text source
- **WHEN** an SRT, ASS, or SSA-compatible text stream is converted
- **THEN** normalized cues are emitted in the requested SRT or ASS format

#### Scenario: PGS OCR
- **WHEN** a PGS stream is requested as converted SRT or ASS
- **THEN** the stream reaches OCR and its recognized cues feed the requested artifact

#### Scenario: VobSub OCR
- **WHEN** a VobSub stream is requested as converted SRT or ASS
- **THEN** the stream reaches OCR and its recognized cues feed the requested artifact

#### Scenario: Unsupported bitmap conversion
- **WHEN** OCR capability is unavailable or cannot produce a supported result
- **THEN** the operation reports an explicit conversion failure without creating a misleading artifact

### Requirement: Lossless original ASS and SSA extraction
The system SHALL preserve original ASS and SSA representation when original
extraction is requested, including source formatting supported by stream copy,
without routing it through converted normalization.

#### Scenario: Original ASS
- **WHEN** an ASS stream is requested in original mode
- **THEN** an `.ass` artifact is extracted losslessly with its styles, positioning, override tags, and karaoke metadata preserved

#### Scenario: Original SSA
- **WHEN** an SSA stream is requested in original mode
- **THEN** an `.ssa` artifact is extracted losslessly rather than converted to SRT or ASS

### Requirement: Shared output decisions and artifact metadata
Asset Info, Track Profiles, Profile Lab, and Queue SHALL use the same resolved
subtitle source type, mode, format, OCR requirement, stages, extension, naming,
and artifact metadata. A canonical plan SHALL take precedence over entry-point
specific heuristics.

#### Scenario: Entry-point parity
- **WHEN** equivalent subtitle policy is requested from Asset Info, Track Profiles, Lab, and Queue
- **THEN** each entry point selects the same output format, mode, naming, and conversion stages

#### Scenario: Artifact identity
- **WHEN** an artifact is planned or generated
- **THEN** its metadata records whether it is original or converted and its extension matches `.srt`, `.ass`, or original `.ssa`

#### Scenario: Existing canonical artifact satisfaction
- **WHEN** a valid artifact already identified by the existing canonical pipeline satisfies an explicit transform
- **THEN** the canonical plan reuses that artifact while avoiding duplicate extraction or OCR, without introducing new sidecar discovery or matching

### Requirement: Asset Info subtitle conversion cancellation

Asset Info SHALL expose cancellation for every running subtitle conversion
operation represented by its existing subtitle extraction operation model. This
includes converted SRT and ASS requests for text sources and OCR-backed SRT and
ASS requests for PGS and VobSub sources. Cancellation SHALL end the operation
with status `cancelled`, which is distinct from `error`/`failed`, and SHALL NOT
change the canonical source classification, format selection, OCR semantics, or
artifact identity decisions. The operation SHALL reach `cancelled` only after
cancellation cleanup has been attempted. A terminal `cancelled` operation SHALL
remain terminal and MUST NOT transition back to `running`; retrying SHALL
create a new operation identity.

#### Scenario: Running text SRT conversion can be cancelled

- **WHEN** a text subtitle conversion to SRT is running from Asset Info and the
  user requests Cancel
- **THEN** the existing operation reports `cancelled` and the UI stops treating
  it as running without reporting a conversion failure

#### Scenario: Running text ASS conversion can be cancelled

- **WHEN** a text subtitle conversion to ASS is running from Asset Info and the
  user requests Cancel
- **THEN** the existing operation reports `cancelled` and no incomplete ASS
  artifact is reported as ready

#### Scenario: Running PGS OCR conversion can be cancelled

- **WHEN** a PGS subtitle conversion to SRT or ASS is running and the user
  requests Cancel
- **THEN** OCR and its child processes stop, the operation reports `cancelled`,
  and the source asset remains unchanged

#### Scenario: Running VobSub OCR conversion can be cancelled

- **WHEN** a VobSub subtitle conversion to SRT or ASS is running and the user
  requests Cancel
- **THEN** OCR and its child processes stop, the operation reports `cancelled`,
  and the source asset remains unchanged

#### Scenario: Cancellation is distinct from conversion failure

- **WHEN** cancellation causes a running operation to stop before completion
- **THEN** its terminal status is `cancelled` with cancellation information,
  while an independent tool or validation error continues to report the
  existing failure status and error information

#### Scenario: A terminal operation is not cancelled retroactively

- **WHEN** Cancel is requested after an operation has already completed or
  failed
- **THEN** the terminal result remains unchanged and no completed artifact is
  removed

#### Scenario: Cancellation cleanup is attempted before the cancelled state

- **WHEN** cancellation stops a running operation that owns staged outputs or
  temporary conversion/OCR workspace
- **THEN** cleanup is attempted before the operation is marked `cancelled`,
  cleanup diagnostics do not reclassify user cancellation as `error`/`failed`,
  and the final operation status remains `cancelled` even if cleanup reaches
  its bounded limit

### Requirement: Context and child-process cancellation

Cancellation SHALL propagate through the operation's `context.Context` to every
canonical extraction, conversion, OCR, normalization, and rendering step. Any
FFmpeg, SeConv, Tesseract, or other child process started for that operation
MUST terminate with its process tree within a bounded cleanup interval after
cancellation is requested. Cancellation cleanup MUST use an independent bounded
cleanup context or equivalent cleanup path that is not already cancelled; the
cancelled work context MUST NOT prevent cleanup from being attempted.

#### Scenario: FFmpeg extraction or rendering is cancelled

- **WHEN** an FFmpeg step is running for a subtitle operation and the operation
  is cancelled
- **THEN** FFmpeg and its descendants terminate, the operation does not wait for
  normal completion, and the operation proceeds to cancellation cleanup

#### Scenario: SeConv or Tesseract OCR is cancelled

- **WHEN** SeConv or Tesseract is running for PGS or VobSub OCR and the
  operation is cancelled
- **THEN** the complete OCR process tree terminates, including descendants that
  were started by an intermediate tool, and the operation reports `cancelled`
  after cleanup

#### Scenario: Cancellation during a multi-step canonical conversion

- **WHEN** cancellation occurs between extraction, OCR, normalization, or SRT/
  ASS rendering steps
- **THEN** the shared canonical operation context reaches the next step and
  prevents any further conversion work before publication

#### Scenario: Cleanup uses a non-cancelled bounded path

- **WHEN** the cancelled work context has stopped conversion and child
  processes but staged files or temporary workspace still exist
- **THEN** cleanup runs with an independent bounded context, is attempted before
  the operation reaches `cancelled`, and any cleanup diagnostic is reported as
  cancellation information rather than as a conversion failure

### Requirement: Cancellation-safe subtitle artifacts and workspace

A cancelled operation MUST NOT publish a partial or unvalidated sidecar. New
sidecars SHALL be committed only after all requested artifacts for the operation
have passed the canonical validation and publication barrier. Cancellation
cleanup SHALL remove only temporary workspace and staged outputs owned by the
cancelled operation, while preserving the source asset, valid sidecars that
already existed, and sidecars completed before this operation. For a logical
`both` conversion, the complete requested SRT and ASS output set is one
publication unit: neither newly generated output from the current operation may
be published independently before both outputs are validated and cross the
publication barrier.

#### Scenario: Cancellation removes partial staged output

- **WHEN** cancellation occurs after a temporary SRT or ASS file has been
  partially written but before the operation's publication barrier
- **THEN** the partial file is removed, no partial sidecar is visible at the
  destination, and the operation reports `cancelled`

#### Scenario: Cancellation during the second `both` output is atomic

- **WHEN** a logical `both` conversion has staged its first output and
  cancellation occurs while its second output is being generated or validated
- **THEN** every staged output owned by the current operation is removed,
  neither newly generated SRT nor ASS is published by that operation, and
  sidecars that existed before it or were published by a previously completed
  operation remain available

#### Scenario: Cancellation cleans bitmap OCR workspace

- **WHEN** a bitmap OCR operation is cancelled while its temporary media,
  images, raw OCR output, or normalized cue files exist
- **THEN** those operation-owned temporary files are removed safely without
  touching unrelated files or source media

#### Scenario: Existing and completed sidecars are preserved

- **WHEN** a cancelled operation targets a sidecar that already existed before
  it began or was published by a previously completed operation
- **THEN** that sidecar remains available and is not deleted or replaced by
  partial output

#### Scenario: A cancelled conversion can be retried

- **WHEN** the user starts the same canonical subtitle conversion after it has
  ended as `cancelled`
- **THEN** a new operation identity may run from the canonical plan, existing
  valid sidecars are satisfied or preserved, the retry is not blocked by the
  cancelled operation, and the cancelled operation remains terminal

### Requirement: Asset Info cancellation and progress contract

Asset Info SHALL provide a Cancel action only while the existing subtitle
operation is running, expose cancellation through the operation API, and stop
frontend progress polling cleanly once the operation reaches `cancelled`.
The UI SHALL provide a retry path after cancellation without presenting the
cancelled operation as a failed conversion.

#### Scenario: Cancel action is available only while running

- **WHEN** Asset Info displays a running SRT, ASS, PGS, or VobSub conversion
- **THEN** it shows an enabled Cancel action for that operation, and the action
  is disabled or absent after any terminal status

#### Scenario: Polling stops after cancellation

- **WHEN** the cancellation response or the next operation status reports
  `cancelled`
- **THEN** the frontend clears the operation's polling/progress loop, removes
  its running presentation, and does not continue issuing status requests

#### Scenario: Cancel request is safe to repeat

- **WHEN** duplicate Cancel requests or a Cancel request racing with completion
  reach the operation API
- **THEN** the operation has one stable terminal outcome, no artifact cleanup is
  repeated destructively, and the API returns the current operation state

#### Scenario: Retry is offered after cancellation

- **WHEN** Asset Info receives a terminal `cancelled` operation
- **THEN** the UI can start the same conversion again and keeps any existing
  sidecar status visible; the retry uses a new operation identity and does not
  restart the terminal cancelled operation

### Requirement: Runtime Japanese OCR capability and bounded orientation fallback

For a canonical converted PGS or VobSub subtitle request, the system SHALL
resolve the effective OCR language and verify that the running OCR runtime
actually supports that language before starting recognition. The runtime image
SHALL provide the Japanese traineddata required by the Japanese options it
exposes, but the runtime capability result SHALL remain authoritative. A
missing or unusable language/runtime capability SHALL be reported separately
from a completed OCR attempt that recognized zero subtitle text, and
diagnostics SHALL include the effective OCR language used or rejected.

When a SeConv recognition process exits, the system SHALL inspect bounded
SeConv stdout/stderr and exit diagnostics before classifying the result. A
non-zero exit SHALL become `zero_recognition` only when the strict known
canonical SeConv zero-recognition signal is present and no cancellation,
malformed-input, crash, or unrelated-failure diagnostic takes precedence. A
non-zero exit without that strict signal SHALL remain a tool/input failure.

When the effective language is `jpn` or `jpn_vert` and the complete language
attempt ends with the canonical genuine zero-recognition result, the system
MAY perform at most one bounded fallback as a distinct opposite Japanese
language/orientation attempt, only when that fallback language is available in
the runtime. The fallback budget SHALL count distinct language/orientation
attempts, not raw SeConv process invocations. Existing raw, clean, and
accurate OCR-mode semantics SHALL remain authoritative inside each language
attempt, including any subprocesses required by that mode. The fallback SHALL
reuse the already extracted bitmap/PGS input and the existing OCR workspace,
SHALL remain within Japanese language data, and SHALL NOT run for missing
language support, tool failure, malformed input, cancellation, or other
non-zero-recognition failures. If either orientation recognizes text, that
successful canonical recognition result SHALL feed SRT, ASS, or logical `both`
serialization without causing another OCR pass. Serializers SHALL never start
or retry OCR. If neither available orientation recognizes text, the operation
SHALL return a concise actionable recognition failure that identifies the
attempted Japanese language or languages and distinguishes it from missing
runtime support. OCR behavior for non-Japanese languages SHALL remain
unchanged.

#### Scenario: Supported effective Japanese language passes runtime preflight

- **WHEN** a PGS or VobSub conversion resolves effective OCR language `jpn` or
  `jpn_vert` and the running Tesseract capability report contains that exact
  language
- **THEN** OCR starts with the effective language, diagnostics identify it, and
  the canonical subtitle pipeline continues normally

#### Scenario: Missing effective Japanese language fails before OCR

- **WHEN** a Japanese conversion resolves an effective language that the
  running Tesseract capability report does not contain
- **THEN** the operation fails before SeConv/Tesseract recognition starts with
  an actionable missing-language/runtime diagnostic naming the effective
  language

#### Scenario: Runtime support is not inferred from package intent

- **WHEN** the container or host configuration suggests a Japanese package is
  installed but the running Tesseract capability report does not list the
  requested language
- **THEN** the capability report controls the result and the operation reports
  missing runtime support rather than attempting OCR with an unusable language

#### Scenario: Genuine Japanese zero-recognition is distinct from missing support

- **WHEN** the effective Japanese language passes preflight, OCR runs against
  the extracted bitmap input, SeConv emits the strict known
  zero-recognition signal in bounded diagnostics, and the process may exit
  non-zero without a cancellation, malformed-input, crash, or unrelated
  failure diagnostic
- **THEN** the operation reports a zero-recognition diagnostic that names the
  effective language and does not report the result as missing language data

#### Scenario: Non-zero SeConv exit without the strict signal remains a failure

- **WHEN** SeConv exits non-zero without the strict known zero-recognition
  signal, or its bounded diagnostics identify cancellation, malformed input, a
  crash, or another unrelated failure
- **THEN** the operation reports the existing tool/input or cancellation
  classification, does not relabel it as zero recognition, and does not start
  a Japanese orientation fallback

#### Scenario: Horizontal Japanese falls back once to vertical Japanese

- **WHEN** `jpn` is available and produces the canonical genuine
  zero-recognition result while `jpn_vert` is also available
- **THEN** the operation performs at most one distinct `jpn_vert`
  language/orientation attempt using the already extracted bitmap/PGS input;
  any raw/clean/accurate subprocesses within that attempt do not consume extra
  fallback budget, and a successful fallback result becomes the canonical
  recognized cue result

#### Scenario: Vertical Japanese falls back once to horizontal Japanese

- **WHEN** `jpn_vert` is available and produces the canonical genuine
  zero-recognition result while `jpn` is also available
- **THEN** the operation performs at most one distinct `jpn`
  language/orientation attempt using the already extracted bitmap/PGS input;
  any raw/clean/accurate subprocesses within that attempt do not consume extra
  fallback budget, and a successful fallback result becomes the canonical
  recognized cue result

#### Scenario: Japanese fallback is not attempted for unavailable alternate data

- **WHEN** the primary Japanese language is available but recognition returns
  genuine zero recognition and the alternate Japanese orientation is absent
  from the runtime capability report
- **THEN** no fallback OCR is started and the operation returns a concise
  zero-recognition diagnostic that identifies the unavailable alternate

#### Scenario: Japanese fallback is not a generic OCR retry

- **WHEN** OCR fails because the selected language is unavailable, a child tool
  exits with an error without the strict zero-recognition signal, the bitmap
  input is malformed, work is cancelled, or OCR reports a result other than
  genuine zero recognition
- **THEN** the operation does not invoke the opposite Japanese orientation as a
  fallback and preserves the existing failure classification

#### Scenario: One successful Japanese recognition result feeds all requested formats

- **WHEN** Japanese PGS or VobSub OCR succeeds with the primary language or its
  one allowed orientation fallback and the request is SRT, ASS, or logical
  `both`
- **THEN** the canonical recognized cues produce the requested artifacts, a
  logical `both` request shares one recognition result, and no serializer
  starts or retries a duplicate OCR pass

#### Scenario: Neither Japanese orientation recognizes text

- **WHEN** both available Japanese orientations complete with genuine
  zero-recognition results
- **THEN** the operation fails with a concise actionable message that names the
  effective/attempted Japanese languages and advises checking subtitle
  orientation or source content, without presenting the failure as missing
  runtime language support

#### Scenario: Non-Japanese OCR semantics remain unchanged

- **WHEN** a PGS or VobSub conversion resolves a non-Japanese OCR language
- **THEN** the existing canonical OCR language selection, recognition, cleanup,
  failure classification, and SRT/ASS output behavior remain unchanged and no
  Japanese orientation fallback is attempted

### Requirement: Bitmap cue geometry is preserved for OCR-generated ASS

For a converted bitmap subtitle, the canonical pipeline SHALL retain optional
spatial metadata for each recognized cue independently from its text and
timing. Geometry SHALL be obtained only from a bitmap representation that the
running implementation can actually parse or otherwise verify; stream-level
dimensions or undocumented ffprobe/SeConv fields SHALL NOT be treated as cue
geometry. When geometry is unavailable, invalid, or cannot be correlated
unambiguously, the affected cue SHALL remain valid and use the existing
bottom-center layout fallback.

Spatial metadata SHALL remain authoritative canonical cue metadata through OCR
cleanup. Raw OCR ASS, when produced, SHALL be rendered from the pre-clean
canonical cues plus their geometry. Final cleaned ASS SHALL be rendered, or
have layout applied, from the cleaned canonical cues plus the same retained
geometry; correctness SHALL NOT depend on SeConv preserving already-serialized
ASS alignment tags.

Every geometry event and recognized OCR cue SHALL have timing provenance.
prepared-input-local timing SHALL be used directly. original-source-absolute
timing SHALL be translated exactly once into the prepared-input timeline using
the operation's known preparation mapping. unknown or ambiguous timing
provenance SHALL receive no invented offset and SHALL NOT produce an
authoritative geometry correlation; the affected cue SHALL use the safe
fallback.

#### Scenario: Bitmap cue geometry is available

- **WHEN** a bitmap subtitle cue has verifiable canvas and bitmap bounds and
  the recognized cue can be correlated to that source event
- **THEN** the canonical cue result retains the cue's relative spatial region
  for an OCR-generated ASS serializer without changing its text or timing

#### Scenario: Geometry is unavailable

- **WHEN** the source representation or runtime does not provide parseable cue
  geometry, or the geometry provider cannot establish a reliable correlation
- **THEN** the OCR conversion succeeds with the existing bottom-center ASS
  fallback and does not invent coordinates or fail solely because metadata is
  unavailable

#### Scenario: Simultaneous cues remain distinct

- **WHEN** two bitmap cues overlap in time but occur in different source
  regions
- **THEN** the canonical result keeps both recognized cues as separate events
  and assigns their independently correlated coarse regions without merging
  them because their timestamps overlap

#### Scenario: Prepared-input-local timings are not double-adjusted

- **WHEN** bitmap source events and recognized OCR cues are produced from a
  prepared or segmented input with a non-zero segment or sample start and both
  are marked prepared-input-local
- **THEN** both timing sets are matched directly in that local timeline, the
  segment/sample start is not applied again, and the expected geometry remains
  associated with the cue rather than being shifted by a second offset

#### Scenario: Original-source-absolute timing is translated once

- **WHEN** bitmap source-event timing is original-source-absolute and recognized
  OCR cue timing is prepared-input-local
- **THEN** the source event is translated exactly once through the known
  preparation mapping before correlation, without changing emitted subtitle
  timing

#### Scenario: Ambiguous timing provenance falls back safely

- **WHEN** geometry-event or OCR-cue timing provenance is unknown or ambiguous
- **THEN** no offset is invented, no authoritative geometry match is made, and
  the affected OCR ASS cue uses the documented safe fallback

### Requirement: OCR-generated ASS uses deterministic coarse layout

OCR-generated ASS SHALL consume retained bitmap cue geometry through a
deterministic coarse relative-region resolver. The first layout contract SHALL
use vertical thirds and, when reliable horizontal data is available, left,
center, or right alignment mapped to supported ASS alignment tags. It SHALL
not claim exact per-pixel positioning. Newly generated raw OCR ASS and the
cleaned OCR ASS SHALL retain the resolved layout metadata.

#### Scenario: Upper-region cue

- **WHEN** a correlated bitmap cue's relative center is in the upper screen
  region
- **THEN** OCR-generated ASS places that event at a top alignment while
  preserving its recognized text, line breaks, and timing

#### Scenario: Middle-region cue

- **WHEN** a correlated bitmap cue's relative center is in the middle screen
  region
- **THEN** OCR-generated ASS places that event at a middle alignment while
  preserving its recognized text, line breaks, and timing

#### Scenario: Lower-region cue

- **WHEN** a correlated bitmap cue's relative center is in the lower screen
  region
- **THEN** OCR-generated ASS places that event at a bottom alignment while
  preserving its recognized text, line breaks, and timing

#### Scenario: Cleanup retains layout

- **WHEN** OCR-generated ASS passes through the existing cleanup mode
- **THEN** cleanup operates on canonical cue text/timing while geometry remains
  attached to cue identity, and the cleaned ASS is rendered with each retained
  coarse alignment rather than relying on SeConv to preserve serialized ASS
  tags

#### Scenario: Raw and cleaned ASS use the same geometry

- **WHEN** an OCR conversion produces both a raw ASS representation and a
  cleaned ASS artifact
- **THEN** raw ASS uses pre-clean canonical cues plus geometry, cleaned ASS uses
  cleaned canonical cues plus that same geometry, and cleanup cannot silently
  replace spatial metadata with its default style

### Requirement: Spatial metadata does not alter SRT or native subtitle behavior

The canonical SRT serializer SHALL remain text-and-timing only and SHALL
ignore bitmap spatial metadata. Original ASS and SSA extraction SHALL remain
lossless and SHALL not consume OCR geometry resolution. A logical converted
both request SHALL perform one canonical bitmap recognition and produce SRT
and ASS from the same recognized cue result, with only the ASS serializer
consuming spatial metadata.

#### Scenario: SRT remains text and timing only

- **WHEN** a bitmap subtitle is converted to SRT with correlated geometry
- **THEN** the SRT contains the recognized text and timing without ASS
  alignment or position tags, and geometry does not alter the artifact identity

#### Scenario: Native ASS is unchanged

- **WHEN** an original ASS subtitle is extracted
- **THEN** the existing native ASS representation, styles, override tags,
  positioning, and timing are preserved without OCR geometry processing

#### Scenario: Native SSA is unchanged

- **WHEN** an original SSA subtitle is extracted
- **THEN** the existing native SSA representation is preserved without
  conversion or bitmap layout inference

#### Scenario: Shared both consumes one enriched cue result

- **WHEN** a bitmap subtitle requests converted SRT and ASS as one logical
  both operation
- **THEN** recognition and geometry correlation occur once, SRT serializes the
  shared text/timing result, ASS serializes the same result with coarse layout,
  and neither serializer starts another OCR operation

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
separate shared workspace and shared execution context. The shared flight owns
prepared bitmap/subtitle input, extraction intermediates, raw and accurate OCR
passes, cleanup intermediates, and evidence staging. Each subscriber subtitle
operation owns only serializer output, validation, and publication staging.
Operation-owned staging SHALL be cleaned after terminal success, failure, or
cancellation; shared-flight workspace SHALL be cleaned only by the flight after
its own terminal success, failure, or cancellation. Reusable committed
evidence is exempt because it belongs to the internal cache. Cancelling one
consumer SHALL detach it and clean only its operation-owned staging; it MUST
NOT delete the active shared-flight workspace. A shared in-flight OCR execution
MAY continue for other non-cancelled equivalent consumers, but when its last
consumer detaches it SHALL cancel its shared context, terminate child processes
through the existing process-tree mechanism, clean the shared-flight workspace,
and SHALL not commit evidence.

#### Scenario: Successful completion removes temporary workspace

- **WHEN** an OCR conversion reaches terminal completion after final sidecars
  pass validation and publication
- **THEN** the subscriber operation's serializer/validation/publication staging
  workspace is gone, and the shared flight independently cleans its prepared
  input, extraction/OCR/cleanup intermediates, and evidence staging after the
  flight reaches its own terminal state, while any committed internal evidence
  remains only in the internal cache. A subscriber terminal state does not imply
  deletion of an active shared-flight workspace still serving another
  subscriber

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

### Requirement: Canonical subtitle semantic roles

The system SHALL assign every subtitle stream one canonical semantic role from
`full`, `forced`, `signs`, `commentary`, `sdh`, or `unknown`. The role SHALL be
independent from language, stream index, and ordinary codec/disposition fields;
stream index SHALL remain the authoritative identity of the subtitle track.
Explicit semantic metadata and explicit dispositions SHALL take precedence over
bounded heuristics. Heuristics SHALL use only available bounded snapshot or
track evidence, SHALL NOT invoke OCR, and SHALL resolve ambiguous or
insufficient evidence to `unknown`. The automatic result SHALL identify the
source snapshot and relevant stream-signature evidence used to classify it. A
`signs` role SHALL NOT imply `forced`.

#### Scenario: Explicit semantic metadata wins

- **WHEN** a subtitle stream has explicit semantic role metadata
- **THEN** the canonical role matches that metadata even when language, title,
  codec, order, or heuristic signals suggest another role

#### Scenario: Dispositions classify supported semantics

- **WHEN** a stream has an explicit forced, commentary, or hearing-impaired
  disposition and no higher-precedence manual or semantic-role metadata
- **THEN** the canonical role is respectively `forced`, `commentary`, or `sdh`
  while preserving the original disposition values

#### Scenario: Signs remain independent from forced

- **WHEN** bounded metadata identifies a stream as signs without an explicit
  forced disposition
- **THEN** its canonical role is `signs` and it is not treated as `forced`

#### Scenario: Ambiguous evidence fails safely

- **WHEN** available title, language, disposition, codec, order, cue, duration,
  or bounded size evidence cannot distinguish a role reliably
- **THEN** the stream receives role `unknown` with provenance explaining the
  ambiguity, and no OCR is invoked for classification

#### Scenario: Same-language streams remain distinct

- **WHEN** an asset contains multiple subtitle streams with the same language
  but different indexes or role evidence
- **THEN** each stream retains its own stream-index identity and independently
  resolved role, metadata, and confidence

### Requirement: Manual semantic-role overrides are authoritative

Asset Info SHALL expose the resolved role, provenance, confidence, and whether
the role is manually overridden for each subtitle stream. A user SHALL be able
to set or reset a per-asset, per-stream role override. A manual override SHALL
be bound to the asset/source snapshot identity and a relevant stream signature
in addition to the authoritative stream index. A valid, current manual
override SHALL take precedence over all automatic classification; a stale or
non-matching override SHALL fail closed, SHALL NOT move to another stream, and
automatic classification SHALL be used instead. Asset Info SHOULD expose the
stale/ignored state for review. Reset-to-auto SHALL remove only the override and
recompute the role from the current snapshot without changing stream identity.

#### Scenario: Manual override changes one stream only

- **WHEN** a user assigns `commentary` to one subtitle stream on an asset
- **THEN** that stream reports `commentary` with manual provenance and other
  same-language streams retain their independently resolved roles

#### Scenario: Manual override wins over automatic evidence

- **WHEN** a manual role override exists and later metadata or heuristic
  evidence suggests another role
- **THEN** the resolved role remains the manual value until the override is
  reset

#### Scenario: Stale manual override is ignored

- **WHEN** the source snapshot or relevant stream signature changes while a
  manual override remains stored for the old stream index
- **THEN** the override is marked stale or ignored, is not applied to the new
  stream, and automatic classification resolves the current stream instead

#### Scenario: Stream-index reuse does not inherit an override

- **WHEN** a later source snapshot reuses an old stream index for a different
  codec, language, title, order, or disposition signature
- **THEN** the old manual role is not transferred and the new stream receives
  only its current automatic classification until explicitly overridden

#### Scenario: Reset returns to current automatic classification

- **WHEN** a user resets a stream's manual role override
- **THEN** the override is cleared, the role is recomputed from current
  evidence, and the UI identifies the result as automatic

### Requirement: Role-aware subtitle rules in Track Profiles

Track Profiles SHALL use the existing `subtitleRules` model for optional
semantic-role matching. Each rule MAY match `language`, `streamIndex`, and
`roles`, and SHALL retain its existing action, sidecar-format, and OCR fields.
An absent role selector SHALL be a wildcard and SHALL preserve current behavior.
Role matching SHALL preserve existing rule precedence, including a matching
stream-specific rule taking precedence over language/path-safe rules. A profile
that contains only existing language-based rules SHALL retain its current
behavior, including selection of multiple same-language streams when its
existing rules select them.

#### Scenario: Rule matches language and role

- **WHEN** a Track Profile contains a subtitle rule matching `language: spa` and
  `roles: [full]`
- **THEN** that rule applies only to Spanish streams with role `full`, while
  the rule's existing action, sidecar, and OCR fields are applied unchanged

#### Scenario: Different role policies coexist by language

- **WHEN** a profile defines `spa + full -> keep`, `spa + forced/signs -> keep`,
  `spa + commentary -> remove`, and `eng + full -> keep`
- **THEN** each subtitle stream is evaluated against its language and role and
  receives the action from its matching rule without merging same-language
  streams

#### Scenario: Absent role selector remains a wildcard

- **WHEN** an existing subtitle rule has language and/or stream selectors but no
  `roles` field
- **THEN** the rule matches any resolved role exactly as it did before semantic
  roles were introduced

#### Scenario: Stream-specific rule precedence is preserved

- **WHEN** a stream matches both a language/path-safe rule and a stream-specific
  rule, with compatible or different role selectors
- **THEN** the stream-specific rule wins according to existing precedence and
  role matching does not reorder or weaken that rule

#### Scenario: Unknown role is explicit in filtering

- **WHEN** a rule explicitly lists `unknown` in `roles` and a stream resolves to
  `unknown`
- **THEN** the rule may match it, while an explicit list that omits `unknown`
  does not match it

### Requirement: Resolved roles are frozen for Queue execution

The canonical resolved track plan SHALL include each subtitle stream's role,
provenance, confidence, override state, source snapshot binding, and relevant
stream signature. When Queue freezes a resolved plan, those role decisions
SHALL be stored in the immutable Queue snapshot alongside the stream index and
existing subtitle decisions. Workers SHALL consume the frozen role decisions
and SHALL NOT reclassify streams from mutable profiles or new runtime metadata
during execution.

#### Scenario: Queue snapshot freezes role decisions

- **WHEN** a queued asset reaches its preparation/freeze boundary
- **THEN** the Queue snapshot contains the role decision for each relevant
  subtitle stream and later profile or metadata changes do not alter that job's
  role-aware execution

#### Scenario: Worker uses frozen role

- **WHEN** a Worker executes a job with a frozen resolved track plan
- **THEN** it uses the stored role and override decisions for selection,
  artifact planning, and publication without running a second classifier or
  resolver path

### Requirement: Role metadata follows subtitle artifacts and publishing

Resolved semantic role metadata SHALL remain associated with the authoritative
stream index through canonical subtitle artifact planning, sidecar generation,
and publishing. Existing/source `forced=true` SHALL remain authoritative
publication metadata. The effective forced disposition SHALL be true when the
source disposition is true or the canonical role is `forced`; canonical role
and source dispositions remain independent metadata.

For supported external subtitle sidecars, `.forced` naming SHALL derive from
the effective forced disposition, while role `sdh` SHALL use the supported
`.sdh` semantic suffix in the canonical sidecar naming position before the
subtitle extension. Where a supported convention can represent both SDH and
forced, both semantics SHALL be retained. For supported muxed/container
outputs, the effective forced disposition SHALL map to forced, role
`commentary` SHALL map to commentary, and role `sdh` SHALL map to
hearing-impaired where those target dispositions exist. Original source
dispositions SHALL be preserved independently. The canonical role value SHALL
remain player-agnostic and shall not be replaced by a destination convention.
`signs` SHALL remain independent from forced; this change SHALL NOT invent a
`.signs` suffix.

Role metadata SHALL not alter subtitle text conversion, OCR, bitmap geometry,
ASS positioning, native ASS/SSA extraction, atomic publication, or existing
sidecar identity except for the required supported forced/SDH publication
mapping.

#### Scenario: Converted sidecar retains role metadata

- **WHEN** a subtitle stream is converted to SRT, ASS, or logical `both`
- **THEN** the resulting artifact metadata identifies the source stream and
  resolved role while the canonical text/OCR pipeline remains unchanged

#### Scenario: Existing forced disposition preserves forced sidecar naming

- **WHEN** a supported external sidecar target publishes a subtitle with
  source `forced=true`, including a stream whose canonical role is `signs` or
  `sdh`
- **THEN** the published sidecar retains the existing `.forced` naming before
  its subtitle extension, and the source disposition and canonical role remain
  separately observable

#### Scenario: Canonical forced role sets effective forced disposition

- **WHEN** a supported target publishes a subtitle with canonical `role=forced`
  and source `forced=false`
- **THEN** the effective forced disposition is true, supported muxed output
  carries forced semantics, and supported external sidecar output uses `.forced`

#### Scenario: Signs with source forced disposition remains forced

- **WHEN** a subtitle has `role=signs` and source `forced=true`
- **THEN** supported publication retains forced semantics and existing `.forced`
  sidecar naming, without inventing a `.signs` suffix

#### Scenario: Signs without source forced disposition is not forced

- **WHEN** a subtitle has `role=signs` and source `forced=false`
- **THEN** supported publication does not add forced semantics or `.forced`
  naming solely from the `signs` role

#### Scenario: SDH with source forced disposition retains both semantics

- **WHEN** a subtitle has `role=sdh` and source `forced=true`
- **THEN** supported publication retains both SDH and forced semantics, using
  the supported `.sdh` and `.forced` representation or corresponding container
  metadata wherever the destination convention allows both

#### Scenario: Supported muxed outputs map role dispositions

- **WHEN** a supported muxed/container target publishes a subtitle with frozen
  role `forced`, `commentary`, or `sdh`
- **THEN** it emits the corresponding forced, commentary, or hearing-impaired
  disposition while preserving the source disposition fields independently

#### Scenario: Signs does not inherit forced publication

- **WHEN** a supported target publishes a subtitle whose canonical role is
  `signs` without a forced role
- **THEN** it does not add forced semantics or invent a `.signs` suffix solely
  from the canonical role

#### Scenario: Unsupported convention preserves canonical role

- **WHEN** a publication target does not support a role-specific convention
- **THEN** the artifact is published using existing semantics with the
  canonical role metadata preserved and no heuristic renaming or role loss

#### Scenario: Existing subtitle pipeline remains authoritative

- **WHEN** role metadata is resolved for a text or bitmap subtitle
- **THEN** the existing canonical subtitle/OCR pipeline, ASS positioning,
  native ASS/SSA extraction, cancellation behavior, and publication barriers
  continue to govern conversion and output generation

### Requirement: Published OCR analysis baseline is separate from execution cache

After successful OCR sidecar publication, the system SHALL retain a bounded
internal canonical cue snapshot associated with the published artifact. The
snapshot SHALL include canonical cue IDs, text, timing/order, geometry/layout
needed for ASS, OCR language/mode, a canonical OCR identity/provenance
reference, artifact or logical-transform identity, and versions/integrity
metadata. It SHALL NOT retain large prepared bitmap inputs or arbitrary per-cue
images indefinitely. The existing #6 reusable OCR execution cache SHALL remain
bounded and evictable, including its current seven-day and 256 MiB limits.

#### Scenario: Analysis works after execution-cache eviction

- **WHEN** a successfully published OCR artifact's #6 execution-cache entry
  has expired or been evicted but its internal canonical cue snapshot remains
- **THEN** `Analyze OCR` analyzes the snapshot without requiring a full OCR
  rerun and without exposing an evidence file beside the media

#### Scenario: Snapshot retains serializer-neutral baseline data

- **WHEN** a published OCR artifact is later analyzed or corrected
- **THEN** the internal snapshot supplies cue identity, text, timing/order,
  required ASS geometry/layout, OCR and provenance identity, and integrity
  metadata without retaining the cleaned shared-flight bitmap workspace

#### Scenario: Legacy artifact reconstructs only bounded text evidence

- **WHEN** an older OCR artifact has no snapshot but an MVForge-owned SRT/ASS
  and sufficient provenance can be read
- **THEN** the system may reconstruct a bounded text-only analysis snapshot;
  image-level/selective OCR remains unavailable unless valid cue input is
  safely reacquired, and Analyze OCR does not silently rerun full OCR

#### Scenario: Legacy SRT baseline cannot become an ASS repair source

- **WHEN** a legacy SRT-only baseline has bounded text, timing, and order but
  no durable canonical geometry/layout evidence
- **THEN** OCR repair may analyze and correct only the published SRT
  representation; it cannot silently synthesize ASS geometry or use that
  baseline as a new ASS or logical `both` repair source

#### Scenario: Legacy ASS preserves only safely parsed structure

- **WHEN** a legacy MVForge-owned ASS artifact is used to reconstruct a
  baseline
- **THEN** repair preserves only structure that can be parsed and validated
  from that artifact, and missing geometry/layout or provenance causes the
  affected ASS correction to fail closed

#### Scenario: Missing representation evidence uses canonical conversion

- **WHEN** a requested correction needs a representation or layout field that
  the legacy analysis baseline does not contain
- **THEN** OCR repair does not invent it or act as a format converter, and any
  generation requiring that evidence is delegated to the existing canonical
  subtitle conversion pipeline

#### Scenario: Missing legacy provenance fails closed

- **WHEN** a legacy OCR artifact has neither a valid snapshot nor sufficient
  provenance for a bounded reconstruction
- **THEN** analysis reports the unavailable baseline and leaves the published
  sidecar unchanged rather than guessing, scanning the complete media, or
  starting full OCR automatically

### Requirement: Successful correction advances the durable canonical baseline

After a correction passes serializer validation, the final stale-revision
check, and the existing atomic publication barrier, the system SHALL commit the
corrected canonical cue set as the current lightweight published snapshot. The
newly published SRT/ASS bytes SHALL define a new published-artifact revision,
and bounded provenance SHALL record the previous revision, new revision, and
correction operation identity. The publication and internal snapshot/revision
commit SHALL fail safe so newly published sidecars are never current while the
old snapshot remains authoritative.

#### Scenario: Successful correction advances the baseline

- **WHEN** correction starts from revision R, validates the requested
  serializers, passes the final stale-revision check, and publishes
- **THEN** the corrected canonical cues become the durable analysis baseline,
  the new sidecar bytes define a new revision, and provenance records R to the
  new revision with the correction operation identity

#### Scenario: Subsequent analysis sees corrected text

- **WHEN** a correction successfully publishes text different from the
  snapshot used for analysis
- **THEN** the next `Analyze OCR` reads the corrected canonical text rather
  than the pre-correction snapshot

#### Scenario: Previous findings become stale

- **WHEN** correction advances the baseline from revision R to a new revision
- **THEN** findings produced against R cannot be applied again and are marked
  stale or require a new analysis

#### Scenario: Logical both advances one corrected baseline

- **WHEN** a logical `both` correction publishes validated SRT and ASS outputs
- **THEN** one corrected canonical cue baseline is committed for the complete
  output set, both new member revisions are recorded, and neither serializer
  retains an independent pre-correction baseline

#### Scenario: Baseline commit failure is fail-safe

- **WHEN** publication or the internal snapshot/revision/provenance commit
  cannot complete as one managed transition
- **THEN** the prior published state and prior authoritative baseline remain
  consistent, no partial correction is exposed, and no Library-visible backup
  is required

### Requirement: Published OCR snapshots have bounded ownership and cleanup

The current lightweight published OCR snapshot SHALL be owned by its
MVForge-managed published OCR artifact or logical transform. Replacement SHALL
advance that current baseline and retain only bounded provenance/history under
the retention policy. Artifact removal SHALL invalidate or remove its active
snapshot; an ownership or source-identity change SHALL fail closed. Orphaned
internal snapshots SHALL be eligible for bounded cleanup, and cleanup SHALL
never delete published sidecars or create a Library-visible artifact class.

#### Scenario: Artifact removal invalidates its snapshot

- **WHEN** the managed published OCR artifact or logical transform is removed
- **THEN** its active snapshot is invalidated or removed and cannot be used for
  later analysis or correction

#### Scenario: Ownership change does not reuse a snapshot

- **WHEN** a snapshot is found under a different source or artifact owner than
  the published artifact it describes
- **THEN** the snapshot fails closed as unusable and no correction or analysis
  is applied from it

#### Scenario: Orphan cleanup preserves published sidecars

- **WHEN** bounded cleanup finds an internal snapshot with no valid managed
  artifact owner
- **THEN** the snapshot is eligible for cleanup without deleting, replacing, or
  renaming any published sidecar

### Requirement: OCR quality analysis uses canonical evidence

For an OCR-generated subtitle artifact, the system SHALL analyze the canonical
OCR evidence or the lightweight published canonical cue snapshot produced by
the existing subtitle pipeline.
Analysis SHALL be independent of the requested final sidecar format, SHALL
not start a full OCR lifecycle a second time, and SHALL not modify or publish
any sidecar. The result SHALL include aggregate clean, suspicious, and review
counts plus bounded per-cue findings containing the cue identity, original
text, finding status, evidence, and any candidate suggestion.

#### Scenario: Analyze generated OCR without changing the sidecar

- **WHEN** Asset Info requests `Analyze OCR` for a generated SRT, ASS, or
  logical `both` artifact
- **THEN** the system analyzes the existing canonical OCR evidence, returns
  quality counts and per-cue findings, and leaves the currently published
  sidecar bytes and metadata unchanged

#### Scenario: Analysis reuses canonical evidence

- **WHEN** equivalent SRT, ASS, or `both` requests refer to the same canonical
  OCR evidence
- **THEN** quality analysis consumes that shared evidence without running full
  OCR again or creating a format-specific OCR result

#### Scenario: Analysis reports bounded cue findings

- **WHEN** analysis completes for a subtitle stream
- **THEN** every reported finding identifies its canonical cue, original text,
  status, bounded reason/evidence, and optional suggestion, and the aggregate
  counts equal the per-cue statuses

### Requirement: Quality analysis fails safely and scores candidates

The quality analyzer SHALL use deterministic bounded evidence, including
structural character sanity, replacement or suspicious-character penalties,
known OCR confusion patterns, supported-language lexical or dictionary checks,
unknown-word evidence, and available recognition evidence. The canonical
`accurate` candidate scorer SHALL use a versioned deterministic contract that
selects its candidate before canonical cleanup and serialization. Dictionary
membership SHALL not be authoritative; unsupported or unknown languages SHALL
not receive English dictionary semantics. Proper nouns, unknown words, and
conflicting evidence SHALL remain clean or reviewable rather than being
silently rewritten. Analysis SHALL never treat uncertainty as permission to
change canonical cue text.

#### Scenario: Suspicious OCR confusion is identified

- **WHEN** a canonical cue contains a bounded OCR-confusion pattern supported
  by lexical or recognition evidence
- **THEN** analysis marks the cue suspicious or reviewable, reports the reason,
  and may provide a bounded candidate without changing the cue

#### Scenario: Accurate scoring changes a candidate when evidence supports it

- **WHEN** isolated and color-preserving accurate-mode OCR candidates include
  one valid-looking recognition error and one candidate supported by
  deterministic structural, confusion, and supported-language lexical
  evidence
- **THEN** the versioned accurate scorer may select the supported candidate
  before cleanup and serialization, while a case without sufficient evidence
  remains reviewable

#### Scenario: Accurate scoring provenance invalidates incompatible evidence

- **WHEN** the accurate candidate-scoring contract/version changes
- **THEN** cached OCR evidence produced under the older scorer is not reused as
  an equivalent accurate result and fresh compatible selection evidence is
  required

#### Scenario: Proper nouns and unknown words fail safe

- **WHEN** a token is a proper noun, an unknown word, or lacks reliable
  dictionary/context evidence
- **THEN** the analyzer does not auto-correct it solely because it is unknown
  and reports clean or review status with bounded evidence

#### Scenario: Analysis is deterministic for unchanged evidence

- **WHEN** the same canonical evidence, analyzer version, and bounded runtime
  inputs are analyzed again
- **THEN** the findings, statuses, counts, and deterministic candidate scores
  are equivalent

#### Scenario: Unsupported language does not use English dictionary semantics

- **WHEN** a cue's OCR language is unsupported or unknown to the lexical
  resources
- **THEN** scoring uses only applicable structural/confusion evidence and
  fails safe without treating dictionary absence as an OCR error

### Requirement: OCR repair is selective, provenance-aware, and not translation

OCR repair SHALL start from canonical OCR evidence and SHALL avoid rerunning
full OCR. Selective secondary OCR SHALL be available only when MVForge can
obtain the suspicious cue bitmap/input through a bounded supported mechanism,
such as a prepared subtitle representation, persisted lightweight source/cue
locator, or another explicit safe extraction path. For eligible bitmap cues,
the system MAY use bounded DVD/VobSub OCR-only preprocessing and selective
secondary OCR. Candidate comparison SHALL include the original text and the
provenance of each generated suggestion. An optional local or contextual
correction provider SHALL be subordinate to deterministic validation, SHALL be
unable to modify structural cue data, and SHALL not translate subtitles.

#### Scenario: Selective secondary OCR repairs an eligible suspicious bitmap cue

- **WHEN** a suspicious DVD/VobSub cue has cue input available through a
  bounded supported representation or locator and evidence supports a
  secondary OCR attempt
- **THEN** repair may preprocess only the bounded cue input, compare the
  resulting candidate with canonical evidence, and apply it only after
  deterministic validation accepts it

#### Scenario: Unavailable selective cue input remains reviewable

- **WHEN** a suspicious cue lacks a safely reacquirable bounded bitmap/input
- **THEN** the canonical text remains unchanged and the cue is marked for
  review without selective OCR

#### Scenario: Selective OCR does not trigger broad OCR

- **WHEN** selective cue input is unavailable or bounded extraction fails
- **THEN** repair leaves the cue unchanged or marks it for review without
  rerunning full-stream OCR or performing unbounded media analysis

#### Scenario: Optional contextual correction remains subordinate

- **WHEN** an optional local or contextual provider returns a text suggestion
- **THEN** the suggestion is treated as a candidate, is accepted only through
  the same deterministic validation and review rules, cannot alter timing,
  cue structure, or layout metadata, and is not used as a translation result

#### Scenario: Repair records provenance

- **WHEN** a candidate is kept, applied, or rejected
- **THEN** internal provenance records the canonical evidence identity, cue
  identity, analyzer/repair contract versions, candidate source, relevant OCR
  language/mode and runtime evidence, validation outcome, and whether a user
  or provider selected it

### Requirement: Corrected canonical cues feed every requested serializer

The correction boundary SHALL be:

`canonical OCR evidence -> quality/correction -> canonical final cues -> SRT/ASS serializers`

Corrected canonical cues SHALL preserve cue IDs, timing, order, ASS override
  tags, styles, bitmap geometry, and positioning metadata. Correction MAY
  change cue text and correction metadata only. SRT SHALL serialize the
  corrected text and timing without geometry; ASS SHALL serialize the same
  corrected text with retained styles, geometry, and positioning. A logical
  `both` request SHALL use one corrected canonical cue set for both outputs.

#### Scenario: Text-only correction preserves cue structure

- **WHEN** a user or validated candidate corrects canonical cue text
- **THEN** cue IDs, timing, order, ASS tags/styles, geometry, and positioning
  are unchanged from the canonical evidence

#### Scenario: SRT and ASS share corrected text

- **WHEN** corrected SRT and ASS outputs are requested for equivalent canonical
  OCR input
- **THEN** both serializers consume the same corrected cue text, while SRT
  omits geometry and ASS retains the canonical layout metadata

#### Scenario: Logical both does not duplicate correction

- **WHEN** a logical `both` conversion is repaired and published
- **THEN** quality/correction runs once for the logical transformation and
  both final sidecars are rendered from the resulting canonical cues

#### Scenario: Native ASS/SSA remains outside OCR repair

- **WHEN** an original native ASS or SSA extraction is requested
- **THEN** the existing lossless native extraction behavior remains in force
  and OCR quality repair does not normalize or rewrite that native artifact

### Requirement: Asset Info review and correction are safe to publish

Asset Info SHALL expose `Analyze OCR` results with clean, suspicious, and
review counts and per-cue original-versus-suggestion details. Users SHALL be
able to keep a cue, apply a validated suggestion, or enter a manual text edit;
safe batch application SHALL apply only candidates meeting the deterministic
validation policy. Analysis SHALL be read-only. Correction SHALL use the
existing subtitle operation identity, status, progress, cancellation, and
retry model rather than introducing a second job system.

#### Scenario: Review shows a cue-level decision

- **WHEN** analysis returns a suspicious or reviewable cue
- **THEN** Asset Info shows the original text, any suggestion, bounded reason
  and provenance, and actions to keep, apply, or edit only the cue text

#### Scenario: Safe batch repair applies validated findings only

- **WHEN** a user requests a batch repair
- **THEN** only findings that pass deterministic validation are applied, while
  uncertain or conflicting findings remain unchanged and reviewable

#### Scenario: Failed correction preserves the published sidecar

- **WHEN** correction fails validation, encounters an execution failure, or is
  cancelled before publication
- **THEN** the currently published SRT/ASS sidecar remains untouched, all
  operation-owned staging is cleaned, and the result is distinct from a
  successful correction

#### Scenario: Publication is atomic for requested outputs

- **WHEN** a correction requests SRT, ASS, or logical `both`
- **THEN** every requested output is validated before the existing publication
  barrier commits, and no output from the current attempt is published alone

#### Scenario: No temporary repair artifacts are library-visible

- **WHEN** analysis or correction creates intermediate evidence or staged
  outputs
- **THEN** temporary files, `.bak`, and `.raw` artifacts remain internal and
  are cleaned or retained only in the internal provenance/evidence store; none
  appears beside the media as a Library-visible sidecar

#### Scenario: A cancelled repair can be retried

- **WHEN** a correction operation ends as cancelled or failed and the user
  retries it
- **THEN** the prior operation remains terminal, a new operation identity is
  used, existing published sidecars are preserved, and valid canonical OCR
  evidence may be reused without violating the existing cancellation rules

### Requirement: Analysis and correction are bound to the published revision

Quality findings SHALL record a bounded published-artifact revision for every
SRT or ASS sidecar they describe. The revision SHALL include artifact/path
identity, size, mtime, full sidecar content hash, and logical `both` membership
where applicable. Before applying an individual, manual, or batch correction
and again immediately before publication, the system SHALL verify that the
currently published output set still matches the recorded revision. A mismatch
SHALL fail closed as stale analysis, require re-analysis, clean operation
staging, and preserve the current sidecar.

#### Scenario: External sidecar edit invalidates analysis

- **WHEN** a published SRT or ASS sidecar changes after analysis but before a
  correction is applied
- **THEN** the correction is rejected as stale, the changed sidecar is not
  overwritten, operation staging is cleaned, and the user must analyze again

#### Scenario: Logical both checks every published member

- **WHEN** analysis targets a logical `both` artifact and either the published
  SRT or ASS member no longer matches its recorded revision
- **THEN** the complete correction is rejected before publication and neither
  output from that correction is committed

#### Scenario: Concurrent corrections cannot overwrite each other

- **WHEN** operation A and operation B both analyze revision R, A publishes a
  valid correction, and B reaches its final publication check afterward
- **THEN** B observes that revision R is stale, fails closed, cleans its
  staging, and does not overwrite A's published result

### Requirement: Contextual correction providers are bounded and subordinate

If a contextual correction provider is enabled, the system SHALL send only
suspicious or reviewable candidates, SHALL batch them within bounded request
limits rather than issuing one request per cue, and SHALL propagate the
existing subtitle operation cancellation context. Provider output SHALL remain
text-only and SHALL pass deterministic validation before application. Provider
output SHALL never change structural subtitle data or perform translation.

#### Scenario: Provider receives bounded suspicious batches

- **WHEN** a correction operation has suspicious or reviewable cues and a
  contextual provider is enabled
- **THEN** only those candidates are sent in bounded batches, clean cues are
  excluded, and each suggestion retains provider provenance for review

#### Scenario: Provider cancellation leaves canonical text unchanged

- **WHEN** the subtitle operation is cancelled or the provider times out while
  a contextual request is in progress
- **THEN** the operation cancellation context stops the provider work, no
  provider suggestion is applied, and the existing sidecar remains unchanged

#### Scenario: Provider suggestions remain text-only

- **WHEN** a provider returns a correction suggestion
- **THEN** deterministic validation may accept only its cue text, while cue IDs,
  timing, order, tags, styles, geometry, and positioning remain canonical and
  no translation is performed

### Requirement: Quality repair is bounded and backward compatible

Quality analysis and repair SHALL use bounded reads and bounded candidate work
appropriate for NAS-hosted media. The #6 OCR execution/reuse cache SHALL retain
its seven-day and 256 MiB bounds and SHALL be treated as evictable; the
lightweight published canonical cue snapshot is the later analysis baseline.
Cache lookup and provenance SHALL be based on the existing canonical OCR
evidence identity and, only where available, bounded prepared-input provenance,
not a whole-media fingerprint as the normal path. Temporary workspaces SHALL
be cleaned after terminal completion, failure, or cancellation. Subtitle
conversion behavior for non-OCR text, native ASS/SSA extraction, existing
Japanese OCR fallback, bitmap positioning, and existing publication semantics
SHALL remain unchanged when quality repair is not explicitly requested.

#### Scenario: Large NAS media is not fully hashed for analysis

- **WHEN** analysis or selective repair looks up reusable OCR evidence for a
  large asset
- **THEN** it uses existing bounded source/prepared-input identity and does not
  hash or scan the complete media file solely to perform quality analysis

#### Scenario: Terminal cleanup removes temporary repair work

- **WHEN** analysis or correction completes, fails, or is cancelled
- **THEN** its temporary workspace is cleaned using the existing bounded
  cleanup path, while committed internal evidence and provenance remain
  available according to their retention policy

#### Scenario: Existing behavior is preserved without repair

- **WHEN** a subtitle conversion does not request OCR quality analysis or
  repair
- **THEN** the existing canonical subtitle/OCR lifecycle, cancellation,
  Japanese fallback, geometry/ASS positioning, native ASS/SSA extraction, and
  artifact publication behavior are unchanged

### Requirement: Persisted ASS-to-SRT fallback setting

The system SHALL persist a boolean subtitle setting equivalent to
`generateSRTFallbackForASS`. A missing value SHALL resolve to `false`, and the
default SHALL preserve existing subtitle output behavior until a user enables
the setting.

#### Scenario: Legacy settings default to fallback off

- **WHEN** an installation has no stored ASS-to-SRT fallback value
- **THEN** subtitle planning SHALL behave as if the setting were `false`

#### Scenario: Setting persists across settings reload

- **WHEN** a user enables or disables the fallback setting and settings are
  loaded again
- **THEN** the same effective boolean value SHALL be used by subsequent
  canonical subtitle planning

### Requirement: Canonical effective subtitle format resolution

The canonical subtitle planner SHALL combine explicit requested formats,
requested mode, source codec, and the resolved fallback setting before
producing the `ResolvedTrackPlan`. Fallback eligibility SHALL be computed as:

```text
fallbackEligible =
    generateSRTFallbackForASS
    AND
    (
        (mode == converted AND effective converted formats include ASS)
        OR
        (mode == original AND source codec is native ASS or SSA)
    )
```

Only an eligible request SHALL gain an implicit SRT. Explicit SRT requests
SHALL remain SRT-only when no converted ASS output is effective, and normalized
effective formats SHALL be deduplicated.

#### Scenario: Converted ASS is fallback eligible

- **WHEN** a subtitle request has `mode=converted`, effective converted format
  ASS, and the fallback setting is enabled
- **THEN** the resolved plan SHALL contain exactly one converted ASS output and
  one converted SRT output using the existing logical `both` semantics

#### Scenario: SRT does not create another output

- **WHEN** the explicit subtitle request is SRT and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain exactly one SRT output and no ASS
  output

#### Scenario: Explicit ASS and SRT are normalized

- **WHEN** the explicit request contains ASS and SRT and the fallback setting
  is enabled
- **THEN** the resolved plan SHALL contain one ASS output and one SRT output
  without duplicate transforms

#### Scenario: ASS remains unchanged when fallback is disabled

- **WHEN** the explicit request is ASS and the fallback setting is disabled or
  absent
- **THEN** the resolved plan SHALL contain ASS without an added SRT output

#### Scenario: Original native ASS is fallback eligible

- **WHEN** a native ASS source has `mode=original` and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain the lossless original ASS output and
  one converted SRT output, without treating the pair as logical `both`

#### Scenario: Original native SSA is fallback eligible

- **WHEN** a native SSA source has `mode=original` and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain the lossless original SSA output and
  one converted SRT output, without treating the pair as logical `both`

#### Scenario: Original non-ASS/SSA sources are not fallback eligible

- **WHEN** an original-mode source is SRT, PGS, or VobSub and the fallback
  setting is enabled
- **THEN** the planner SHALL NOT add an implicit SRT output

#### Scenario: Disabled fallback never adds implicit SRT

- **WHEN** the fallback setting is disabled or absent for either converted ASS
  or original native ASS/SSA
- **THEN** the planner SHALL NOT add an implicit SRT output

### Requirement: Shared canonical planning across entry points

Asset Info, Track Profiles, Profile Lab, Test Encode, and Queue SHALL use the
same canonical resolver and effective-format resolution contract. Interactive
entry points MAY resolve equivalent current plans; Queue SHALL own and freeze
its execution snapshot. Where an entry point exposes ASS selection, it SHALL
make the resulting SRT fallback visible before execution when the persisted
setting is enabled. No client capability or playback policy SHALL alter the
resolved formats.

#### Scenario: Asset Info shows the effective fallback

- **WHEN** Asset Info selects ASS for a subtitle stream and fallback is enabled
- **THEN** the displayed/resolved subtitle outputs SHALL show that SRT will also
  be produced before the operation is started

#### Scenario: Profile and Lab selections agree

- **WHEN** a Track Profile or Profile Lab/Test Encode request selects ASS under
  the same settings and source conditions as Asset Info
- **THEN** its resolved effective outputs SHALL match the canonical plan,
  including the added SRT output

### Requirement: Source codec and requested subtitle mode remain distinct

The system SHALL treat source codec and requested extraction mode as separate
dimensions. Converted text and bitmap outputs SHALL use the canonical
normalization/serializer or OCR/cue lifecycle respectively. Native ASS/SSA
SHALL remain lossless only when `mode=original` is requested; an effective SRT
fallback for that mode SHALL be a separate converted output and SHALL NOT
rewrite or replace the native representation.

#### Scenario: Converted text ASS receives converted SRT fallback

- **WHEN** a text source is requested with `mode=converted`, `format=ass`, and
  fallback enabled
- **THEN** the operation SHALL produce converted ASS and converted SRT through
  the canonical normalization and serializer path

#### Scenario: Converted bitmap ASS receives converted SRT fallback

- **WHEN** a bitmap source is requested with `mode=converted`, `format=ass`,
  and fallback enabled
- **THEN** the operation SHALL produce converted ASS and converted SRT from
  one canonical OCR/cue lifecycle

#### Scenario: Native ASS in original mode publishes original plus converted SRT

- **WHEN** a native ASS subtitle is selected with `mode=original` and fallback
  enabled
- **THEN** the operation SHALL preserve the native ASS artifact and produce a
  separate converted SRT fallback through the existing canonical conversion
  path without treating the pair as logical `both`

#### Scenario: Native SSA in original mode preserves its source representation

- **WHEN** a native SSA subtitle is selected with `mode=original` and fallback
  enabled
- **THEN** the operation SHALL preserve the native SSA artifact and produce the
  additional converted SRT output without converting the original into a
  different styled format

#### Scenario: Converted logical both remains one transformation

- **WHEN** a converted subtitle request explicitly selects logical `both`
- **THEN** its SRT and ASS serializers SHALL share one normalized cue/OCR
  result, independently of the original-mode native ASS/SSA fallback case

### Requirement: Bitmap ASS and SRT share canonical OCR evidence

For bitmap subtitles, effective ASS+SRT output SHALL be one logical subtitle
transformation. OCR, Japanese fallback, accurate candidate selection, and
existing OCR quality corrections SHALL produce one canonical cue/evidence set
before SRT and ASS serialization. ASS SHALL retain existing geometry and
positioning behavior, while SRT SHALL serialize the same corrected text and
timing without geometry.

#### Scenario: Bitmap fallback performs one OCR lifecycle

- **WHEN** a bitmap subtitle is requested as ASS with fallback enabled
- **THEN** OCR and any existing correction lifecycle SHALL execute once and
  both serializers SHALL consume the resulting canonical cues

#### Scenario: Corrections are shared by both outputs

- **WHEN** the canonical OCR quality lifecycle corrects a bitmap cue before
  publication of effective ASS+SRT outputs
- **THEN** the SRT and ASS artifacts SHALL contain the same corrected cue text
  and the ASS artifact SHALL retain the cue's canonical geometry/positioning

### Requirement: Queue freezes effective fallback outputs

Queue SHALL retain the existing lifecycle `QUEUED → CLAIM/PREPARING → resolve
effective configuration → build/freeze ResolvedTrackPlan → PROCESSING`.
Effective fallback outputs SHALL be frozen in the immutable
`ResolvedTrackPlan`/Queue snapshot during Claim/Preparing. Workers SHALL
consume that frozen plan and SHALL NOT read or reinterpret Settings for
subtitle output selection.

Before a successful freeze, preparation or retry MAY resolve the current
settings. After a successful freeze, the execution and any retry of that
frozen job SHALL use the same snapshot. A new/future job SHALL resolve current
settings.

#### Scenario: Settings changes before freeze are observed during preparation

- **WHEN** a job remains `QUEUED`, the fallback setting changes, and Claim/
  Preparing has not successfully frozen the plan
- **THEN** preparation SHALL resolve and freeze the new setting's effective
  output

#### Scenario: Settings changes after freeze do not affect execution or retry

- **WHEN** a job has successfully frozen ASS+SRT, the setting changes to off,
  and the job executes or is retried
- **THEN** execution and retry SHALL continue to use the frozen ASS+SRT
  snapshot

#### Scenario: Incomplete freeze may resolve again

- **WHEN** preparation or retry has not successfully completed the freeze
  boundary and the persisted fallback setting changes
- **THEN** the next preparation attempt MAY resolve current settings before
  freezing a plan

#### Scenario: A future job resolves current settings

- **WHEN** a new job is created after the setting changes
- **THEN** its Claim/Preparing phase SHALL resolve the current setting

### Requirement: Atomic fallback publication and metadata preservation

The fallback SHALL use the existing validation, cancellation, naming, metadata,
and atomic all-output publication boundaries. Semantic roles, source and
effective forced/SDH dispositions, sidecar qualifiers, logical `both` identity,
and existing cancellation behavior SHALL be preserved independently of whether
SRT was added by fallback.

#### Scenario: Fallback outputs publish atomically

- **WHEN** one output in an effective ASS+SRT operation fails validation or the
  operation is cancelled before the publication barrier
- **THEN** no newly generated member of that operation SHALL be published and
  previously existing sidecars SHALL remain intact

#### Scenario: Role and disposition metadata survive fallback

- **WHEN** a subtitle with semantic role, forced, or SDH metadata receives an
  SRT fallback
- **THEN** the canonical filenames, artifact metadata, and supported
  publication dispositions SHALL preserve that metadata for the output set

### Requirement: Explicit Asset Info subtitle translation

Asset Info SHALL expose an explicit `Translate...` operation for an eligible
embedded text subtitle, embedded PGS/VobSub subtitle, or supported external
SRT/ASS subtitle artifact. V1 SHALL support source language `Auto`, `English`,
or `Japanese`, target locale `es-419` or `es-ES`, and provider `Ollama`.
Translation SHALL be opt-in from Asset Info and SHALL NOT be introduced into
Queue, Track Profiles, Profile Lab, or automatic processing policy.

#### Scenario: Embedded English text subtitle can be translated

- **WHEN** Asset Info starts translation for an embedded English text subtitle
  with target `es-419` and an enabled, ready Ollama configuration
- **THEN** one subtitle translation operation is created and the resulting
  Spanish artifact is produced without changing the embedded source or its
  source classification

#### Scenario: Embedded Japanese bitmap subtitle can be translated

- **WHEN** Asset Info starts translation for an embedded Japanese PGS or
  VobSub subtitle with target `es-ES` and an enabled, ready Ollama
  configuration
- **THEN** the existing canonical OCR path supplies the source cues to one
  translation operation and a separate Spanish artifact is planned

#### Scenario: External sidecar can be translated

- **WHEN** Asset Info selects an eligible external SRT or ASS artifact and
  starts translation
- **THEN** the operation uses that exact artifact and its ownership/revision
  identity rather than guessing another same-language sidecar

#### Scenario: Translation is not automatic Queue behavior

- **WHEN** a Queue job, Track Profile, or Profile Lab request is processed
  without an explicit Asset Info translation operation
- **THEN** no translation provider is invoked and existing subtitle planning
  and publication behavior remains unchanged

### Requirement: Asset Info translation controls reflect backend capabilities

The Asset Info `Translate...` dialog SHALL expose source language (`Auto`,
`English`, `Japanese`), target locale (`Spanish (Latin America) / es-419`,
`Spanish (Spain) / es-ES`), and provider. It SHALL expose a model control only
when the selected provider reports model selection as supported. The backend
response SHALL provide informational media-context status and source, and the
dialog SHALL display them when available. Missing context SHALL remain
informational and SHALL NOT block translation.

The backend SHALL remain authoritative for canonical valid output formats.
The frontend SHALL show an output-format selector only when the backend
reports more than one valid format; with one valid format it SHALL use that
format without requiring a choice. The frontend SHALL NOT invent, infer, add,
or suppress output-format eligibility independently. For an intrinsically
eligible subtitle, the backend SHALL keep `Translate...` discoverable when
translation infrastructure is unavailable, but SHALL report it disabled with
an actionable reason. The frontend SHALL not probe Ollama or infer provider
readiness from endpoint/model fields.

#### Scenario: Dialog displays available media context

- **WHEN** the selected subtitle has backend-reported media context with a
  status and source
- **THEN** the dialog displays that context status and source as informational
  fields alongside the source language, target locale, provider, and supported
  model controls

#### Scenario: Missing media context does not block translation

- **WHEN** the backend reports that media context is unavailable or empty for
  an otherwise eligible subtitle
- **THEN** the dialog indicates that context is unavailable, leaves the
  translation action enabled, and does not invent replacement context

#### Scenario: Multiple backend-authorized formats show a selector

- **WHEN** the backend reports more than one canonical valid output format for
  the exact subtitle entry
- **THEN** the dialog shows an output-format selector containing only those
  backend-reported formats

#### Scenario: One backend-authorized format needs no selector

- **WHEN** the backend reports exactly one canonical valid output format
- **THEN** the dialog does not require an output-format choice and submits the
  single backend-authorized format

#### Scenario: Frontend does not decide output eligibility

- **WHEN** a source, filename, client capability, or provider state could
  suggest an output format that is absent from the backend capability response
- **THEN** the dialog does not add or enable that format and relies on the
  backend-authorized set

#### Scenario: Disabled Translate remains discoverable

- **WHEN** a subtitle is intrinsically eligible but translation is disabled,
  unconfigured, untested, unavailable, or missing its configured model
- **THEN** Asset Info shows `Translate...` as disabled with the backend's
  bounded actionable reason, and the frontend does not probe or enable the
  provider itself

### Requirement: Translation provider readiness and explicit enablement

Translation SHALL be disabled by default. Configuration, readiness, and
enablement SHALL remain distinct backend-authoritative states. A configured
provider SHALL not become usable merely because its endpoint and model are
non-empty. The backend SHALL provide an explicit `Test Connection` operation
for the configured provider. For Ollama, readiness SHALL verify both
reachability of the configured endpoint and availability of the configured
model through a provider-native lightweight readiness/model request, without
generating a subtitle translation or requiring GPU capability. The frontend
SHALL not probe the provider or infer readiness.

The backend SHALL support bounded states equivalent to `disabled`,
`not_configured`, `untested`, `ready`, `unavailable`, `model_not_found`, and
`invalid_configuration`, with secret-safe diagnostics. A readiness timestamp,
if exposed, SHALL be informational and SHALL NOT be treated as a live health
guarantee. Explicit enablement SHALL require successful readiness for the
current configuration. Changing provider, endpoint, or model SHALL invalidate
readiness and enablement.

#### Scenario: Translation disabled by default

- **WHEN** translation settings are absent or newly initialized
- **THEN** translation is disabled and Asset Info cannot start provider work

#### Scenario: Configured but untested provider

- **WHEN** an Ollama endpoint and model are configured but have not passed
  readiness testing
- **THEN** translation remains disabled and backend capabilities report that
  provider verification is required

#### Scenario: Successful Ollama readiness

- **WHEN** `Test Connection` reaches the configured Ollama endpoint and
  confirms that the configured model is available
- **THEN** readiness becomes `ready`, no subtitle translation is generated,
  and translation remains disabled until explicitly enabled

#### Scenario: Missing model

- **WHEN** the configured Ollama endpoint is reachable but the configured
  model is unavailable
- **THEN** readiness reports a bounded actionable `model_not_found` state and
  translation cannot be enabled

#### Scenario: Enable after readiness

- **WHEN** the current provider configuration passes readiness testing and the
  user explicitly enables translation
- **THEN** backend capabilities may expose translation as available for an
  otherwise eligible subtitle

#### Scenario: Configuration change invalidates readiness

- **WHEN** provider, endpoint, or model changes after successful verification
- **THEN** the prior readiness and enablement are invalidated and translation
  cannot run until the new configuration is tested successfully and enabled

#### Scenario: Runtime outage does not trigger a preflight loop

- **WHEN** an enabled provider becomes unavailable after successful setup
- **THEN** a translation attempt calls the normal provider request directly,
  uses the existing timeout/unavailable/malformed/cancellation semantics, and
  does not run an additional health or model preflight first

### Requirement: Source-language selection is reliable and bounded

The operation SHALL accept `Auto`, `English`, or `Japanese` as source
language. Auto SHALL use reliable bounded source metadata or cue evidence and
SHALL proceed only when English or Japanese is identified with sufficient
confidence. Ambiguous, unsupported, or insufficient evidence SHALL require an
explicit source language and SHALL not invoke a provider. Missing media
context SHALL not prevent translation when source language is otherwise valid.

#### Scenario: Auto selects reliable English

- **WHEN** Auto language evidence reliably identifies English
- **THEN** the operation records effective source language English and sends
  English cues to the provider

#### Scenario: Auto selects reliable Japanese

- **WHEN** Auto language evidence reliably identifies Japanese
- **THEN** the operation records effective source language Japanese and sends
  Japanese cues to the provider

#### Scenario: Auto rejects ambiguity

- **WHEN** Auto evidence cannot distinguish English from Japanese or is not
  reliable for the selected subtitle
- **THEN** the operation fails before provider work with an actionable request
  for explicit source language selection

#### Scenario: Explicit source language overrides weak evidence

- **WHEN** the user explicitly selects English or Japanese and bounded source
  metadata is incomplete but the selected source is supported
- **THEN** translation may proceed using the explicit language and records the
  requested and effective source language

### Requirement: Translation operates on canonical cues

The translation boundary SHALL be:

```text
source -> canonical cues -> optional bounded context -> TranslationProvider
       -> structured cue-ID response -> strict validation
       -> canonical final cues -> existing serializer -> staging -> publication
```

Providers SHALL receive and return text only. MVForge SHALL retain ownership
of cue IDs, order, timing, semantic metadata, ASS/SSA structure, styles,
override tags, geometry, positioning, artifact identity, validation, and
publication. Translation SHALL never create a second subtitle planner,
serializer, OCR lifecycle, operation system, or publication path.

#### Scenario: SRT translation preserves canonical timing

- **WHEN** an SRT source is translated successfully
- **THEN** the translated SRT contains provider-approved text with the same
  cue IDs, order, start/end timing, and cue boundaries as the source

#### Scenario: ASS translation preserves immutable structure

- **WHEN** an ASS source is translated successfully
- **THEN** the translated ASS uses the translated dialogue text while
  preserving styles, layer, margins, positioning, override tags, line breaks,
  karaoke/drawing structure, cue order, and timing from MVForge's canonical
  representation

#### Scenario: SSA translation preserves source representation

- **WHEN** an embedded native SSA source is translated through the existing
  text subtitle path
- **THEN** the translated artifact uses the existing SSA-aware serializer and
  preserves the source representation's structural fields while changing only
  eligible dialogue text

#### Scenario: ASS/SSA protected anchors make one provider result reversible

- **WHEN** an ASS or SSA dialogue cue contains override blocks, positioning,
  karaoke, drawing data, or `\\N`/`\\n`/`\\h` escapes
- **THEN** the provider-facing cue text contains only eligible human dialogue
  plus bounded MVForge-owned opaque anchors, and exact-once ordered anchor
  validation restores the retained structure before serialization

#### Scenario: Invalid protected anchors fail closed

- **WHEN** a provider removes, duplicates, reorders, malforms, or invents a
  protected ASS/SSA anchor
- **THEN** MVForge rejects the response before reconstruction and publishes no
  translated artifact

#### Scenario: Provider text cannot inject ASS/SSA structure

- **WHEN** provider-owned translated text outside the exact expected anchors
  contains override braces, `\\N`, `\\n`, `\\h`, or physical CR/LF line breaks
- **THEN** MVForge rejects the response before reconstruction while allowing
  ordinary translated punctuation such as commas

#### Scenario: Providers cannot change subtitle structure

- **WHEN** a provider response attempts to add markup, alter timing, merge or
  split cues, reorder cues, or change ASS/SSA structural tokens
- **THEN** validation rejects the response and no translated artifact is
  published

### Requirement: Structured provider responses are strictly validated

Each translation batch SHALL carry stable canonical cue IDs and the provider
response SHALL contain exactly one text result for each requested cue ID.
Missing, duplicate, unexpected, empty-invalid, or structurally mutated cue
results SHALL fail the operation. A provider response SHALL not be accepted by
position alone when cue IDs are available.

#### Scenario: Valid cue-ID response is accepted

- **WHEN** a provider returns exactly the requested cue IDs with text-only
  translations
- **THEN** MVForge attaches each translation to its matching canonical cue and
  continues to serializer validation

#### Scenario: Missing cue ID fails closed

- **WHEN** a provider omits one requested cue ID
- **THEN** the batch fails validation, the source remains unchanged, and no
  partial translated sidecar is published

#### Scenario: Duplicate or unexpected cue ID fails closed

- **WHEN** a provider returns a duplicate or unknown cue ID
- **THEN** the response is rejected as malformed and no output from that
  operation is published

#### Scenario: Provider structural mutation fails closed

- **WHEN** provider output includes timing, ordering, ASS tags, style fields,
  geometry, or other fields outside translated text
- **THEN** the operation rejects the response and preserves existing source and
  translated artifacts

### Requirement: Ollama is the bounded V1 translation provider

The V1 provider SHALL be Ollama with a configurable endpoint and model. The
endpoint SHALL be usable over a NAS, LAN, or container network and SHALL NOT
assume localhost. CPU-only Ollama deployments SHALL be supported. Requests
SHALL contain bounded batches of cues and SHALL propagate the existing subtitle
operation context. The provider contract SHALL be replaceable by future
OpenAI, DeepL, Google, or Azure adapters without changing canonical cue or
operation contracts; those adapters are not part of V1.

#### Scenario: Configured remote Ollama is used

- **WHEN** translation is enabled after the configured Ollama endpoint and
  model have passed readiness testing
- **THEN** the operation sends bounded structured batches to that endpoint and
  records the configured provider/model identity

#### Scenario: Runtime Ollama outage fails clearly

- **WHEN** an enabled translation request reaches an endpoint that is missing,
  invalid, or unreachable
- **THEN** the operation ends with an actionable provider failure before
  publication and does not silently use localhost or another provider

#### Scenario: Translation is batched

- **WHEN** a subtitle contains more cues than one bounded provider request can
  carry
- **THEN** the operation sends multiple bounded batches, preserves global cue
  identity/order, and never issues one provider request per cue

#### Scenario: Provider cancellation propagates

- **WHEN** the user cancels while an Ollama request is in progress
- **THEN** the request stops through the existing operation context, the
  operation reaches `cancelled` after cleanup, and no translated output is
  published

### Requirement: Translation context is bounded and advisory

The operation MAY provide bounded neighboring canonical cues and optional
media metadata such as title/year, movie or series name, season/episode,
synopsis, and known character names. Context SHALL be advisory only; it SHALL
not authorize structural subtitle changes, replace source-language evidence,
or block translation when absent. Context sent to a provider SHALL be bounded
and SHALL not be persisted as a full prompt or full response.

#### Scenario: Neighboring cues provide bounded context

- **WHEN** context is enabled for a cue batch
- **THEN** only the configured bounded neighboring cues and permitted metadata
  are sent, and cue ownership remains based on explicit IDs

#### Scenario: Missing metadata does not block translation

- **WHEN** title, synopsis, character, or episode metadata is unavailable
- **THEN** translation proceeds without that context when the source and
  provider contracts are otherwise valid

#### Scenario: Context cannot alter structure

- **WHEN** provider output influenced by context proposes a timing, tag, style,
  geometry, or cue-boundary change
- **THEN** canonical validation rejects the structural change and preserves the
  source representation

### Requirement: Bitmap translation reuses canonical OCR and correction

PGS and VobSub translation SHALL first use the existing canonical bitmap OCR
and OCR-quality/correction lifecycle. Japanese `jpn`/`jpn_vert` fallback and
accurate candidate selection SHALL occur once at the canonical OCR boundary.
Translation SHALL consume the resulting canonical corrected cues and SHALL
never invoke OCR itself or cause serializers to invoke OCR again.

#### Scenario: PGS translation reuses one OCR result

- **WHEN** a PGS subtitle is translated
- **THEN** canonical OCR produces the source cues once, correction is applied
  before translation, and the provider receives only canonical cue text

#### Scenario: VobSub translation reuses one OCR result

- **WHEN** a VobSub subtitle is translated
- **THEN** the existing VobSub OCR path supplies canonical cues to translation
  without a translation-specific OCR or bitmap extraction path

#### Scenario: Japanese fallback precedes translation

- **WHEN** Japanese OCR requires its existing bounded opposite-orientation
  fallback
- **THEN** the successful canonical language attempt becomes the source for
  translation and the provider is called only after OCR/correction completes

#### Scenario: OCR failure prevents translation publication

- **WHEN** OCR, OCR correction, or required canonical cue validation fails
- **THEN** the provider is not invoked and no translated artifact is staged as
  publishable

### Requirement: Locale-specific translated artifact identity and source safety

Translation SHALL publish a separate artifact and SHALL never overwrite the
source sidecar by default. `es-419` and `es-ES` SHALL have distinct canonical
artifact and logical-transform identities. Artifact naming SHALL reuse existing
subtitle naming and ownership helpers while adding a deterministic locale
qualifier. An existing translated artifact MAY satisfy only the exact source
identity, source revision, target locale, provider contract, and requested
representation; it SHALL not satisfy another locale or stale source.

#### Scenario: Target locales remain distinct

- **WHEN** the same source is translated once to `es-419` and once to `es-ES`
- **THEN** two distinct artifact identities and locale-qualified sidecars are
  available without either operation overwriting the other

#### Scenario: Source sidecar is preserved

- **WHEN** translation succeeds for an SRT, ASS, or SSA source
- **THEN** the source artifact remains byte-for-byte untouched and the Spanish
  artifact is published separately

#### Scenario: Existing exact translation is reusable

- **WHEN** an existing translated artifact matches the exact canonical source
  identity, revision, locale, provider contract, and representation
- **THEN** the operation may report it satisfied without duplicating publication

### Requirement: Translation is protected by source revision and atomic publication

The operation SHALL capture the selected embedded stream or external artifact
revision before provider work and SHALL recheck that revision immediately
before publication. A stale or replaced source SHALL fail closed, preserve the
source and any previously published translated artifact, and publish nothing
new. All requested translated outputs SHALL pass canonical validation before
the existing all-output publication barrier commits them.

#### Scenario: External source edit invalidates translation

- **WHEN** an external SRT or ASS source changes after translation starts
  before publication
- **THEN** the operation reports stale source, cleans its staging, publishes
  nothing new, and leaves the changed source and prior translated artifact
  intact

#### Scenario: Embedded source replacement invalidates translation

- **WHEN** the media source or selected subtitle stream snapshot changes before
  publication
- **THEN** the operation fails the final revision check and does not publish a
  translation for the old source revision

#### Scenario: Atomic publication prevents partial translated output

- **WHEN** one requested translated representation fails validation or
  publication
- **THEN** no newly generated member of that operation is published and
  previously existing artifacts remain intact

### Requirement: Translation uses the existing operation lifecycle

Translation SHALL use the existing subtitle operation identity, status,
progress, cancellation, cleanup, retry, staging, and publication contracts.
Progress SHALL expose meaningful preparation, batching/translation,
validation, and publication phases with bounded processed/total values where
the existing operation API supports them. A terminal `cancelled` or failed
translation SHALL remain terminal; retry SHALL create a new operation identity.

#### Scenario: Translation reports progress

- **WHEN** batches are prepared, translated, and validated
- **THEN** Asset Info can display the existing operation progress and current
  phase without creating a second job model

#### Scenario: Cancelled translation preserves all prior artifacts

- **WHEN** a translation is cancelled before the publication barrier
- **THEN** provider work and child processes stop, operation-owned staging is
  cleaned, no new artifact is published, and existing source/translated
  artifacts remain unchanged

#### Scenario: Failed translation can be retried

- **WHEN** translation ends as failed or cancelled
- **THEN** the terminal operation remains unchanged and a retry may create a new
  operation identity without overwriting existing artifacts

### Requirement: Translation provenance is bounded and secret-safe

Successful or terminal translation provenance SHALL record bounded metadata for
provider, model/version, requested and effective source language, target
locale, context source, input revision, operation identity, contract version,
and available token/request metrics. It MUST NOT persist secrets, credentials,
authorization headers, full prompts, full provider responses, or unbounded
translated payloads in artifact metadata or the Asset Info UI.

#### Scenario: Audit metadata identifies a translation

- **WHEN** a translation operation completes or fails after provider contact
- **THEN** bounded provenance identifies its provider/model, languages, locale,
  input revision, context mode, operation, and contract version

#### Scenario: Secrets and full provider content are excluded

- **WHEN** provider configuration or responses contain credentials, prompts,
  or large payloads
- **THEN** those values are excluded or redacted from persisted provenance,
  sidecar metadata, logs intended for Asset Info, and UI responses

#### Scenario: Existing non-translation provenance remains readable

- **WHEN** an existing subtitle artifact has no translation provenance
- **THEN** it remains readable and publishable through the existing subtitle
  pipeline without migration or being mislabeled as translated
