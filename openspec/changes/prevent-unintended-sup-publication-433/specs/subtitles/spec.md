## ADDED Requirements

### Requirement: Publisher enforces frozen subtitle output authorization

The system SHALL authorize generated subtitle publication from the frozen
resolved per-stream output decisions. A ready artifact SHALL be promoted only
when its source stream identity, output format, mode, and artifact identity
match an authorized sidecar decision. OCR intermediates SHALL remain
operation-owned and SHALL NOT be published as final sidecars. Native original
extraction, including PGS `.sup` and text SRT/ASS/SSA, SHALL remain available
when explicitly authorized. Existing required, optional, and no-cue policies
remain authoritative.

#### Scenario: Converted PGS publishes only the requested representation

- **WHEN** the frozen plan authorizes PGS as converted SRT and OCR succeeds
- **THEN** only the SRT artifact is eligible for publication, and any
  temporary `.sup` used by OCR remains outside the final package

#### Scenario: Logical both publishes the requested pair

- **WHEN** the frozen plan authorizes a logical converted both output for one
  PGS stream and OCR succeeds
- **THEN** the SRT and ASS artifacts from that plan are eligible together, and
  no original `.sup` artifact is eligible

#### Scenario: Explicit original PGS extraction remains supported

- **WHEN** the frozen plan explicitly authorizes PGS original extraction
- **THEN** the native `.sup` artifact is eligible for publication without OCR

#### Scenario: Extra ready artifacts do not expand the frozen output set

- **WHEN** a job contains a ready subtitle artifact whose stream, format, mode,
  or artifact identity does not match an authorized frozen decision
- **THEN** Publisher does not include or promote that artifact, even when its
  extension matches another planned output

#### Scenario: No selected sidecars produce no subtitle publication

- **WHEN** the frozen plan contains no authorized subtitle sidecar outputs
- **THEN** no subtitle artifact is promoted from the job

#### Scenario: Preflight explains original and converted format provenance

- **WHEN** Preflight displays frozen subtitle sidecar decisions
- **THEN** it distinguishes original formats such as `.sup` from converted SRT
  or ASS and identifies the resolved source of the format decision

#### Scenario: Legacy publication does not infer authorization from a source file

- **WHEN** a historical job lacks a resolved Track Plan and a matching
  operation artifact record
- **THEN** an unrelated preexisting `.sup` is not copied into the package,
  while the existing supported legacy external SRT/ASS/SSA behavior remains
  available
