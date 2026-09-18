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
