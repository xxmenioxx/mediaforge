## Purpose

Lets operators review likely text defects in exact subtitle sidecar revisions with bounded AI assistance while preserving format structure and requiring explicit approval before publication.

## ADDED Requirements

### Requirement: Review eligible text sidecars without changing them

Asset Info SHALL offer AI proofreading for an exact eligible external SRT, ASS, or SSA sidecar, regardless of whether its text was imported, translated, or produced by canonical OCR. Analysis SHALL leave the source bytes unchanged and SHALL report detected language, source provenance, model/provider readiness, and the captured revision identity. Embedded bitmap subtitles SHALL be ineligible unless represented by an existing canonical OCR text sidecar.

#### Scenario: Imported or translated text sidecar is reviewed
- **WHEN** an operator selects an owned SRT, ASS, or SSA sidecar and starts Review with AI
- **THEN** the backend binds the operation to that exact sidecar revision and reports its language and known source provenance without requiring translation or OCR

#### Scenario: Unsupported or ambiguous target is rejected
- **WHEN** a request names a sidecar outside the asset's authorized inventory, a bitmap source, or an unsupported representation
- **THEN** the backend rejects the request without reading or modifying an unrelated path

#### Scenario: Analysis never publishes edits
- **WHEN** an operator analyzes a sidecar or cancels analysis
- **THEN** the original bytes remain unchanged and any completed findings remain advisory

### Requirement: Bound proofreading work and model input

Quick review SHALL run deterministic checks first and send only suspicious cues with bounded neighboring context to the configured local reviewer. Extended review SHALL be explicit, warn about added resource use, process bounded chunks, enforce cue and byte caps, and be cancellable. Both modes SHALL use a proofreading contract distinct from translation, validate response cue identity and source-text hash one-to-one, and treat model confidence as advisory. No request SHALL send an unbounded subtitle file or make a per-cue provider call.

#### Scenario: Quick review limits provider input
- **WHEN** Quick review runs on a sidecar
- **THEN** deterministic findings are retained and only suspicious cues plus bounded neighbor context are sent in bounded chunks

#### Scenario: Extended review remains bounded
- **WHEN** an operator opts into Extended review
- **THEN** the UI states the higher runtime/resource cost and the backend enforces configured cue, chunk, request, and response limits

#### Scenario: Provider output is invalid
- **WHEN** a provider omits, duplicates, mismatches, or rewrites a cue identity or source hash, changes language, or introduces protected ASS/SSA controls
- **THEN** affected suggestions are rejected and no sidecar is changed

#### Scenario: Provider is unavailable or operation is cancelled
- **WHEN** the configured provider is unavailable, times out, or the operator cancels
- **THEN** deterministic findings remain visible, the operation reports its outcome, and no partial provider result is published

### Requirement: Preserve subtitle meaning and structure

Proofreading SHALL propose same-language spelling, punctuation, encoding, or terminology candidates only. It SHALL preserve cue identity, order, timestamps, line boundaries, ASS/SSA styles, positioning, and override/control semantics. It SHALL preserve slang, names, honorifics, intentional symbols, and mixed-language text when uncertain. A single sidecar SHALL NOT be used to claim audio agreement or source-to-translation fidelity.

#### Scenario: ASS or SSA text is reviewed
- **WHEN** a cue contains ASS/SSA styling, positioning, signs, songs, or protected control tags
- **THEN** only canonical text spans are considered and the original structure remains intact

#### Scenario: Language or intent is uncertain
- **WHEN** a cue is mixed-language, romanized, intentionally unusual, or not safely classifiable
- **THEN** the system flags uncertainty for operator review and does not assert an automatic correction

#### Scenario: Review cannot establish semantic fidelity
- **WHEN** no exact paired source dialogue or audio transcript is available
- **THEN** the system does not claim that subtitle text matches audio or a translated source

### Requirement: Require explicit decisions before applying suggestions

AI suggestions SHALL remain pending by default. The review interface SHALL provide summary counts, issue filters, original/suggested comparisons, reasons, provenance, and bounded neighboring context for changed or suspicious cues. Applying a selected batch SHALL show its exact cue count and diff preview and require confirmation. Keep-original, reject, edit, and accept decisions SHALL remain distinct and attributable to the operator.

#### Scenario: Operator reviews a suggested batch
- **WHEN** an operator selects suggestions to apply
- **THEN** the system presents the exact affected cue count and before/after diff and publishes only after explicit confirmation

#### Scenario: Operator keeps or rejects a suggestion
- **WHEN** an operator rejects a suggestion or keeps the original text
- **THEN** the source text is preserved and the decision is recorded against the exact cue and revision

### Requirement: Publish atomically with revision binding and restore support

The backend SHALL validate the full selected edit set against the captured sidecar revision, preserve the prior revision and bounded decision provenance through the canonical internal revision mechanism, and publish atomically only after explicit approval. Stale identity, malformed text, unsupported encoding, failed staging, or failed publication SHALL leave the current sidecar unchanged. A compatible prior revision SHALL be restorable through an explicit operation.

#### Scenario: Source changed after analysis
- **WHEN** an operator applies suggestions after the sidecar revision has changed
- **THEN** the backend rejects the entire batch as stale and leaves the changed sidecar untouched

#### Scenario: Valid approved edit is published
- **WHEN** the selected changes pass revision, format, and protected-text validation and the operator confirms
- **THEN** the sidecar is atomically updated and the previous bytes plus model, contract, revision, cue, and operator-decision provenance remain internally restorable

#### Scenario: Publication fails or prior revision is restored
- **WHEN** staging or publication fails, or an operator explicitly restores a compatible stored revision
- **THEN** the failed edit does not partially publish, and a successful restore uses the same validated atomic publication boundary

### Requirement: Keep specialized subtitle quality owners separate

Generic proofreading SHALL reuse existing canonical OCR findings when available but SHALL NOT perform OCR correction or claim imported text defects are OCR errors. It SHALL NOT implement source-versus-translation semantic fidelity checks owned by translation quality review.

#### Scenario: OCR evidence is available
- **WHEN** the exact sidecar has compatible canonical OCR quality evidence
- **THEN** existing OCR findings are merged by exact canonical cue identity and retain OCR provenance

#### Scenario: OCR evidence is absent
- **WHEN** the sidecar has no compatible canonical OCR evidence
- **THEN** generic deterministic and proofreading findings remain available without inventing OCR provenance
