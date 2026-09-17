## ADDED Requirements

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
