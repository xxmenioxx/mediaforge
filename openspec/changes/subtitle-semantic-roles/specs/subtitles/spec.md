## ADDED Requirements

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
and publishing. For supported external subtitle sidecars, role `forced` SHALL
use the `.forced` suffix and role `sdh` SHALL use the `.sdh` suffix in the
canonical sidecar naming position before the subtitle extension. For supported
muxed/container outputs, role `forced` SHALL map to the forced disposition,
role `commentary` SHALL map to the commentary disposition, and role `sdh` SHALL
map to the hearing-impaired disposition where those target dispositions exist.
Original source dispositions SHALL be preserved independently. The canonical
role value SHALL remain player-agnostic and shall not be replaced by a
destination convention. `signs` SHALL remain independent from `forced`; this
change SHALL NOT invent a `.signs` suffix.

Role metadata SHALL not alter subtitle text conversion, OCR, bitmap geometry,
ASS positioning, native ASS/SSA extraction, atomic publication, or existing
sidecar identity except for the required supported forced/SDH publication
mapping.

#### Scenario: Converted sidecar retains role metadata

- **WHEN** a subtitle stream is converted to SRT, ASS, or logical `both`
- **THEN** the resulting artifact metadata identifies the source stream and
  resolved role while the canonical text/OCR pipeline remains unchanged

#### Scenario: Forced and SDH external sidecars use canonical suffixes

- **WHEN** a supported external sidecar target publishes subtitles with frozen
  role `forced` or `sdh`
- **THEN** the published sidecar uses `.forced` or `.sdh`, respectively, before
  its `.srt`, `.ass`, or other supported subtitle extension, and retains the
  canonical role in artifact metadata

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
