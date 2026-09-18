## ADDED Requirements

### Requirement: Canonical subtitle semantic roles

The system SHALL assign every subtitle stream one canonical semantic role from
`full`, `forced`, `signs`, `commentary`, `sdh`, or `unknown`. The role SHALL be
independent from language, stream index, and ordinary codec/disposition fields;
stream index SHALL remain the authoritative identity of the subtitle track.
Explicit semantic metadata and explicit dispositions SHALL take precedence over
bounded heuristics. Heuristics SHALL use only available bounded snapshot or
track evidence, SHALL NOT invoke OCR, and SHALL resolve ambiguous or
insufficient evidence to `unknown`. A `signs` role SHALL NOT imply `forced`.

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
to set or reset a per-asset, per-stream role override. A valid manual override
SHALL take precedence over all automatic classification and SHALL remain tied to
the stream index rather than language alone. Reset-to-auto SHALL remove only the
override and recompute the role from the current snapshot without changing the
stream identity.

#### Scenario: Manual override changes one stream only

- **WHEN** a user assigns `commentary` to one subtitle stream on an asset
- **THEN** that stream reports `commentary` with manual provenance and other
  same-language streams retain their independently resolved roles

#### Scenario: Manual override wins over automatic evidence

- **WHEN** a manual role override exists and later metadata or heuristic
  evidence suggests another role
- **THEN** the resolved role remains the manual value until the override is
  reset

#### Scenario: Reset returns to current automatic classification

- **WHEN** a user resets a stream's manual role override
- **THEN** the override is cleared, the role is recomputed from current
  evidence, and the UI identifies the result as automatic

### Requirement: Optional role-aware Track Profile filtering

Track Profiles MAY configure subtitle semantic-role filters in addition to their
existing language and stream-selection rules. Role filtering SHALL apply only
when explicitly configured. A profile that contains only existing language-based
configuration SHALL retain its current behavior, including selection of multiple
same-language streams when its existing rules select them.

#### Scenario: Role filter selects matching subtitles

- **WHEN** a Track Profile explicitly requests one or more semantic roles
- **THEN** subtitle selection includes only streams whose resolved roles match
  the configured role filter, subject to the profile's existing selection and
  disposition rules

#### Scenario: Unconfigured role filter preserves compatibility

- **WHEN** a legacy or current Track Profile has no semantic-role filter
- **THEN** resolution uses the existing language and stream rules without
  requiring role classification for selection or changing its selected tracks

#### Scenario: Unknown role is not silently treated as a configured role

- **WHEN** a profile explicitly filters for a role and a stream resolves to
  `unknown`
- **THEN** that stream is excluded unless the profile explicitly includes
  `unknown`

### Requirement: Resolved roles are frozen for Queue execution

The canonical resolved track plan SHALL include each subtitle stream's role,
provenance, confidence, and override state. When Queue freezes a resolved plan,
those role decisions SHALL be stored in the immutable Queue snapshot alongside
the stream index and existing subtitle decisions. Workers SHALL consume the
frozen role decisions and SHALL NOT reclassify streams from mutable profiles or
new runtime metadata during execution.

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
and publishing. Where a target supports established conventions, `forced` and
`sdh` MAY be mapped to sidecar or container metadata, but the canonical role
value SHALL remain player-agnostic and shall not be replaced by a convention.
Role metadata SHALL not alter subtitle text conversion, OCR, bitmap geometry,
ASS positioning, native ASS/SSA extraction, atomic publication, or existing
sidecar identity unless an explicit role-aware publishing rule requires it.

#### Scenario: Converted sidecar retains role metadata

- **WHEN** a subtitle stream is converted to SRT, ASS, or logical `both`
- **THEN** the resulting artifact metadata identifies the source stream and
  resolved role while the canonical text/OCR pipeline remains unchanged

#### Scenario: Supported publishing convention is derived from role

- **WHEN** a publication target supports a documented forced or SDH convention
- **THEN** publishing may emit that convention from the frozen canonical role
  while retaining the canonical role in artifact metadata

#### Scenario: Unsupported convention preserves canonical role

- **WHEN** a publication target does not support a role-specific convention
- **THEN** the artifact is published using existing semantics with the
  canonical role metadata preserved and no heuristic renaming or role loss

#### Scenario: Existing subtitle pipeline remains authoritative

- **WHEN** role metadata is resolved for a text or bitmap subtitle
- **THEN** the existing canonical subtitle/OCR pipeline, ASS positioning,
  native ASS/SSA extraction, cancellation behavior, and publication barriers
  continue to govern conversion and output generation
