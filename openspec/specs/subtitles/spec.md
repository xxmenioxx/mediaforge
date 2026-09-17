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
