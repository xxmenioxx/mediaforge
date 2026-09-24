## ADDED Requirements

### Requirement: Canonical multi-source subtitle composition

The system SHALL support a composition request containing exactly one primary
subtitle source and one or more explicitly selected supplemental sources. The
initial supported source combinations SHALL include ASS+ASS, SSA/ASS
combinations, SRT+ASS, ASS+SRT, and SRT+SRT. Bitmap sources SHALL be eligible
only after the existing canonical bitmap/OCR path has produced canonical text
cues. The result SHALL be one canonical ASS artifact, and source media,
source sidecars, and source track contents SHALL remain untouched by default.

Each simultaneous dialogue, sign, or song cue SHALL remain an independent ASS
event. The system MUST NOT concatenate overlapping text into one event merely
because the events overlap or have equivalent text.

#### Scenario: Supported text composition produces canonical ASS

- **WHEN** a valid primary and one or more valid supplemental sources use any
  of the initial ASS/SSA/SRT combinations
- **THEN** the operation produces one validated canonical ASS artifact whose
  source set and resolved language metadata identify every contributor

#### Scenario: Bitmap source uses the existing canonical OCR result

- **WHEN** a selected source is bitmap-only and the canonical OCR/text path
  produces valid cues
- **THEN** composition consumes those cues and does not start a composition-
  specific OCR or bitmap conversion path

#### Scenario: Simultaneous top and bottom events remain independent

- **WHEN** a primary or supplemental source contains a top-positioned event
  overlapping a bottom-positioned event
- **THEN** the canonical ASS contains two independent events with their
  respective timing and layout evidence

#### Scenario: Invalid source type is rejected

- **WHEN** a request contains an unsupported source, an unavailable OCR result,
  or no explicitly resolved primary source
- **THEN** the backend rejects the request before staging or publication and
  reports the bounded source-specific reason

### Requirement: Frozen source identity and revision safety

Before normalization or reconciliation, the system SHALL resolve and freeze
the identity and revision of every contributing embedded stream, external
sidecar, or canonical OCR artifact. The frozen operation state SHALL include
the ordered source set, source representation, resolved language, resolved
track classification/role, source revision, composition policy version, style
policy version, output identity, and publication mode.

The operation MUST revalidate every frozen source revision before publication.
If any contributing source is missing, replaced, or changed, the operation
MUST fail as stale, clean only its owned staging, publish no new artifact, and
leave all sources and previously published composites unchanged.

#### Scenario: One stale source invalidates the composition

- **WHEN** one of several frozen sources changes after composition begins and
  before the publication barrier
- **THEN** the operation reports stale input, publishes no composite, and
  preserves the changed source and any prior composite

#### Scenario: Frozen retry does not silently re-resolve inputs

- **WHEN** a failed or cancelled composition is retried
- **THEN** the retry uses the previously frozen source identities, revisions,
  language/role decisions, composition policy, style policy, and output mode;
  a changed source is rejected as stale rather than silently substituted

#### Scenario: Sources remain protected

- **WHEN** composition completes, fails, is cancelled, or is retried
- **THEN** source media, source subtitle sidecars, and source attachments are
  not modified, renamed, deleted, or overwritten by the composition operation

### Requirement: Safe language and role resolution

The backend SHALL resolve language and track/cue classification before
reconciliation. Sources with the same resolved language MAY be composed
automatically when the frozen policy allows it. Cross-language composition
MUST require an explicit manual/profile override and MUST record all
contributing languages. Unknown or ambiguous language MUST NOT be auto-
composed.

The existing `SubtitleSemanticRole` values SHALL remain authoritative and
backward compatible for existing track rules. Composition SHALL use a separate
bounded composition classification for individual cue/style intent, with
stable values equivalent to `dialogue`, `sign`, `lyrics`, `commentary`, and
`unknown`; `lyrics` SHALL be a composition classification, not a new
`SubtitleSemanticRole`. Track classification and cue/style intent SHALL remain
distinct, so labels such as `Signs & Songs` or `Signs & Other` MUST NOT be
treated as compound semantic roles.

#### Scenario: Same-language sources are selected automatically

- **WHEN** the primary and eligible supplemental sources resolve to the same
  non-ambiguous language and the policy permits automatic selection
- **THEN** the backend may compose them without a manual language override and
  records that language for the output and contributing sources

#### Scenario: Cross-language composition needs explicit authorization

- **WHEN** a supplemental source resolves to a different language from the
  primary
- **THEN** automatic composition rejects it, while an explicit manual/profile
  policy may authorize it and the output records the primary and every
  contributing language

#### Scenario: Unknown language is not silently composed

- **WHEN** any candidate source has unknown or ambiguous language evidence
  without an explicit override
- **THEN** the backend reports an eligibility/conflict result and does not
  stage or publish a composite

#### Scenario: Lyrics classification does not change existing role rules

- **WHEN** a cue or source is explicitly classified as `lyrics`
- **THEN** composition may use the classification to select the frozen
  lyrics/style intent, but existing `full`, `forced`, `signs`, `commentary`,
  `sdh`, and `unknown` role matching remains unchanged

#### Scenario: Compound track labels remain classifications

- **WHEN** a track is described as `Signs & Songs` or `Signs & Other`
- **THEN** the track may be selected as a supplemental classification, but
  each cue retains its own bounded composition classification and no compound
  label is persisted as a semantic cue role

### Requirement: Deterministic event reconciliation

The backend SHALL initialize an evolving canonical event set from primary
events and process supplemental sources in frozen precedence order:
`primary → supplemental 1 → supplemental 2 → ... → canonical event set`.
For each supplemental event, reconciliation SHALL compare against eligible
canonical events already accepted, not only against original primary events.
Primary-owned evidence has highest preservation precedence; among supplemental
owners, the earlier frozen supplemental source has precedence. A later source
MAY repair missing/default evidence but MUST NOT silently replace meaningful
higher-precedence evidence.

Candidate comparison SHALL use at minimum normalized protected/text content,
start and end times within a bounded tolerance, resolved language, semantic
role, bounded composition classification/style intent, layout evidence, and
source provenance evidence. Text equality alone MUST NOT establish event
identity. The tolerance and normalization version SHALL be part of the frozen
composition policy. Every supplemental event SHALL have at most one canonical
owner, and each source event ordinal SHALL be consumed at most once.

Source provenance is evidence and deterministic tie/conflict input, not an
identity-equality requirement. Primary and supplemental source identities are
expected to differ, and a different source identity alone MUST NOT make
otherwise equivalent events different. Frozen source identity/revision remains
auditable; source identity plus same-source ordinal prevents accidental
self-matching; and unresolved cross-source relationships MUST remain
ambiguous/conflicting.

For each supplemental event, reconciliation SHALL apply these outcomes in
order: keep a primary-owned equivalent event when its layout is meaningful;
enrich/repair an equivalent canonical event when its layout or style is
missing/default; deduplicate an equivalent event already owned by a primary or
earlier supplemental source; add a supplemental event when no eligible
canonical event is equivalent; and surface an ambiguous/conflicting match
when more than one plausible identity or incompatible meaningful evidence
remains. Ambiguous events MUST NOT be silently merged or discarded.

#### Scenario: Equivalent meaningful primary cue is kept once

- **WHEN** a supplemental event matches one primary-owned canonical event on
  protected/text content, bounded timing, language, role/classification, and
  layout evidence, while source provenance identifies the frozen contributing
  sources but does not require equal source identity
- **THEN** the output keeps the primary event, emits no duplicate, and records
  a `kept-primary` decision

#### Scenario: Equivalent default cue is enriched

- **WHEN** a supplemental event matches an eligible canonical event but that
  event lacks meaningful position/style evidence or only has default layout
- **THEN** the output keeps one event, enriches the canonical event with the approved
  supplemental evidence, and records an `enriched/repaired` decision

#### Scenario: Supplemental-only cue is added

- **WHEN** a supplemental event has no eligible identity match in the evolving
  canonical event set
- **THEN** the output adds it as an independent event and records an `added`
  decision

#### Scenario: Ambiguous timing/text match is surfaced

- **WHEN** an event has multiple plausible candidates after protected/text,
  bounded timing, language, role/classification, layout, and provenance
  evidence are considered
- **THEN** the operation records a bounded conflict/review result, does not
  silently merge either candidate, and publishes only according to the
  explicit conflict policy

#### Scenario: Equivalent cue across two supplemental sources is deduplicated

- **WHEN** the primary lacks cue X, supplemental source A adds X, and
  supplemental source B later contains an equivalent X
- **THEN** the evolving canonical set contains one X, B records a
  `deduplicated` reconciliation against A's accepted event, and no duplicate X
  is added

#### Scenario: Primary cue dominates duplicate supplementals

- **WHEN** the primary contains cue X and supplemental sources A and B both
  contain equivalent X
- **THEN** A and B reconcile to the primary-owned X, primary evidence remains
  authoritative, and neither supplemental creates a second X

#### Scenario: Conflicting supplemental copies are surfaced

- **WHEN** supplemental sources A and B agree on the identity evidence for one
  cue but contain incompatible meaningful style/layout evidence that cannot be
  treated as a repair
- **THEN** the earlier frozen supplemental source is the deterministic owner,
  the conflict is recorded for review, and the later source does not silently
  overwrite or disappear

#### Scenario: Supplemental precedence is stable

- **WHEN** the same equivalent cue is supplied by multiple supplementals and
  the request is repeated with the same frozen policy
- **THEN** source order and ownership are identical, regardless of map or
  filesystem enumeration order

#### Scenario: Repeated identical text at different times stays separate

- **WHEN** two events have identical normalized text but distinct non-matching
  timestamps or role/class/layout evidence
- **THEN** they remain separate canonical ASS events and are not deduplicated

#### Scenario: Overlap is not a merge instruction

- **WHEN** two events overlap in time but have distinct position, role,
  classification, or conflicting provenance/tie evidence
- **THEN** both remain independent events even if their normalized text is
  equal or partially equal

### Requirement: Canonical ASS structure, positioning, and style preservation

Every input SHALL be normalized into the shared canonical ASS event/document
model before reconciliation. Native ASS/SSA style definitions, event style
assignments, override tags, layers, margins, timing, line breaks, karaoke or
drawing structure, and explicit positioning SHALL be preserved by default.
SRT normalization SHALL create canonical text events with no fabricated
native style or position claims. Bitmap-derived events SHALL retain the
existing #26 canonical geometry/layout evidence produced by OCR.

Position completeness SHALL be evaluated from existing ASS evidence including
source style/alignment, `\\an`, `\\pos`, `\\move`, and #26 canonical geometry or
layout evidence. Meaningful explicit evidence SHALL outrank a default style or
fallback alignment. Composition MUST NOT invent precise position evidence when
none exists.

Style and layout resolution SHALL use the frozen #123 style policy and
`MVF_Default`, `MVF_Top`, `MVF_Lyrics`, and `MVF_Sign` identities only through
the existing style resolver contract. When `CompositionCueClass=lyrics`, the
frozen composition/style mapping SHALL pass `ExplicitStyleID=MVF_Lyrics`
through that existing resolver boundary; it MUST NOT create a
Composition-specific style resolver. Geometry SHALL remain layout evidence and
MUST NOT change the semantic role or imply a lyrics role. A native source event
SHALL remain source-owned unless the frozen policy explicitly authorizes a
style mutation.

#### Scenario: Native ASS styles and overrides survive composition

- **WHEN** an ASS/SSA source supplies custom styles, event assignments, and
  inline override tags
- **THEN** the canonical ASS preserves those definitions and event-level
  structures unless an explicit frozen style policy authorizes a bounded
  mutation

#### Scenario: SRT receives deterministic canonical styling

- **WHEN** an SRT event is added or enriched into the output
- **THEN** its style/layout is resolved from the frozen composition/style
  policies and available canonical evidence, without claiming source-native
  ASS positioning that SRT did not provide

#### Scenario: Explicit position evidence wins

- **WHEN** an event contains `\\an`, `\\pos`, or `\\move`, or has meaningful
  #26 geometry evidence, while a fallback/default style suggests another
  alignment
- **THEN** the output retains the explicit/canonical evidence and does not
  replace it with the fallback alignment

#### Scenario: Missing layout evidence remains missing

- **WHEN** neither the source style/alignment nor `\\an`, `\\pos`, `\\move`,
  nor #26 geometry provides meaningful layout evidence
- **THEN** the output records layout as unresolved/default according to policy
  and does not infer a precise position from text alone

#### Scenario: Lyrics class selects the existing lyrics preset

- **WHEN** a cue has `CompositionCueClass=lyrics` and the frozen composition
  style mapping authorizes the lyrics appearance
- **THEN** composition passes `MVF_Lyrics` as the existing explicit style
  selection and `resolveSubtitleStyle` resolves the style, without a second
  style authority or semantic-role mutation

### Requirement: Collision-safe styles and bounded font attachment reconciliation

When multiple ASS/SSA sources define the same style name with different
definitions, the output SHALL preserve each source meaning by using
deterministic collision-safe identities and updating only the corresponding
event references. A source style name SHALL NOT be overwritten by a configured
MVForge style of the same display name. Native override tags such as style
resets SHALL remain structurally valid after any permitted base-style remap.

Required font attachments from all contributing sources SHALL be reconciled as
bounded publication members. Byte-identical attachments MAY be deduplicated;
different content with the same filename SHALL receive deterministic distinct
names when safe. An attachment that cannot be safely reconciled SHALL produce
an actionable unresolved-conflict result and MUST NOT be silently dropped.

#### Scenario: Distinct same-named styles remain distinct

- **WHEN** two sources define `Default` with different ASS style values
- **THEN** both definitions remain available under deterministic identities,
  each event refers to the definition it originally meant, and provenance
  records the collision

#### Scenario: Native style name does not capture MVForge style identity

- **WHEN** a source already defines a style named `MVF_Default` and the frozen
  policy also references the built-in `MVF_Default`
- **THEN** the source definition remains source-owned and the MVForge style
  identity is namespaced or otherwise kept distinct

#### Scenario: Identical attachments are bounded

- **WHEN** multiple contributing sources require byte-identical font
  attachments
- **THEN** the publication set contains one deterministic attachment identity
  and provenance records the deduplication without retaining duplicate bodies

#### Scenario: Conflicting attachment cannot be resolved silently

- **WHEN** two required attachments have the same effective name but different
  content and no safe deterministic rename is possible
- **THEN** composition reports an unresolved attachment conflict and publishes
  no incomplete canonical output

### Requirement: Versioned Track Profile composition policy and Queue snapshot

Track Profile composition settings SHALL use a versioned, bounded policy that
explicitly resolves primary selection, supplemental selection and order,
language authorization, source-track and cue-role rules, style/layout behavior,
preferred/default publication, and explicit overrides. The backend SHALL
apply existing Track Profile subtitle rules and transforms before composition
selection, then resolve this policy through the canonical Track Profile plan
and freeze the
resolved policy and all resulting source decisions into the Queue preparation
snapshot before execution.

Equivalent inputs and the same policy version SHALL produce the same frozen
source order, output identity, reconciliation decisions, and style policy.
Policy-version changes SHALL invalidate stale snapshots rather than changing a
processing execution after its freeze boundary. Existing Queue snapshots that
do not contain composition state SHALL retain their existing authority and
behavior.

Only a composite containing the configured primary/full-dialogue source MAY
become preferred/default, and only when the frozen profile/manual policy
explicitly requests promotion. Both conditions are structural invariants;
manual policy MUST NOT waive the primary/full-dialogue requirement. A
supplemental-only, signs-only, or lyrics-only composite MUST NOT become
preferred/default under any policy.

#### Scenario: Profile snapshot is deterministic

- **WHEN** the same scanned sources and versioned Track Profile composition
  policy are resolved more than once
- **THEN** the frozen Queue snapshot contains the same ordered contributors,
  resolved roles/languages, style policy version, output identity, and default
  decision

#### Scenario: Post-freeze profile edits do not alter execution

- **WHEN** a Track Profile or global style setting changes after the Queue
  composition snapshot is frozen
- **THEN** the in-flight composition uses the frozen policy and style snapshot,
  while a later request resolves a new snapshot

#### Scenario: Preferred output requires configured primary dialogue

- **WHEN** a composition contains only supplemental signs/lyrics or omits the
  configured primary/full-dialogue source
- **THEN** it MUST NOT become preferred/default, even when a manual/profile
  policy requests promotion, and the result records the structural denial

#### Scenario: Promotion requires an explicit frozen request

- **WHEN** a composition contains the configured primary/full-dialogue source
  but the frozen profile/manual policy does not explicitly request promotion
- **THEN** it MUST NOT become preferred/default

#### Scenario: Explicit cross-language policy is retained

- **WHEN** a profile explicitly authorizes a cross-language supplemental source
- **THEN** the Queue snapshot records that authorization and all resolved
  languages, and later execution does not reinterpret it as automatic
  same-language selection

### Requirement: Backend-authoritative Asset Info eligibility and request

Asset Info SHALL expose a backend-authoritative `Compose subtitles` capability
result for the selected asset. The result SHALL identify eligible primary and
supplemental sources, resolved revisions, languages, existing roles and
bounded composition classifications, supported output mode, and actionable
ineligibility/conflict reasons. The frontend MUST NOT independently decide
source validity, language compatibility, role compatibility, or attachment
eligibility.

A manual compose request SHALL identify the asset, one primary source, the
ordered supplemental source set, and any explicit bounded policy/style or
cross-language override. The backend SHALL revalidate every selection and
reject an invalid, stale, ambiguous, unsupported, or unauthorized request
before creating an operation. The request and result SHALL use the existing
subtitle operation/status contract.

#### Scenario: Asset Info reports eligible sources

- **WHEN** an asset has a valid primary and compatible supplemental sources
- **THEN** Asset Info receives a backend response describing the eligible set,
  resolved languages/roles, revisions, and the allowed compose request

#### Scenario: UI cannot enable an invalid source

- **WHEN** a source has an unsupported representation, ambiguous language,
  incompatible role, stale revision, or unresolved attachment conflict
- **THEN** the backend excludes or marks it ineligible with a bounded reason,
  and the UI cannot enable it through local heuristics

#### Scenario: Manual request is revalidated

- **WHEN** a manual request names a source that became stale or invalid after
  the eligibility response
- **THEN** the backend rejects the request without creating a composition
  operation or publishing an artifact

#### Scenario: Existing sidecar discovery remains bounded

- **WHEN** Asset Info evaluates composition candidates
- **THEN** it uses sources already identified by the canonical pipeline or
  explicitly selected by the request and does not introduce a new adjacent
  sidecar discovery/matching rule

### Requirement: Shared subtitle operation lifecycle and atomic publication

Composition SHALL reuse the existing subtitle operation registry, progress and
status model, cancellation, retry, staging, validation, cleanup, publication
barrier, and publication mutex. A distinct composition operation kind MAY be
added only if the existing kind cannot represent the request; it MUST NOT
create a second lifecycle or registry. The complete canonical ASS output and
any required reconciled attachment members SHALL be one publication unit.

The operation SHALL pass source revision checks, event/style/attachment
validation, conflict policy, and deterministic identity checks before crossing
the publication barrier. Cancellation before the barrier SHALL clean only
operation-owned staging and leave existing artifacts unchanged. Cancellation
or completion racing at the barrier SHALL have one stable terminal outcome and
MUST NOT publish a partial composite.

#### Scenario: Composition uses existing operation visibility

- **WHEN** Asset Info starts, polls, cancels, retries, or displays a
  composition
- **THEN** it uses the existing subtitle operation identity/status/progress
  contract and does not create a separate composition job model

#### Scenario: Cancel before publication is atomic

- **WHEN** cancellation occurs after one event/output or attachment is staged
  but before the publication barrier
- **THEN** all operation-owned staged members are cleaned, no partial ASS or
  attachment is visible, and prior artifacts remain unchanged

#### Scenario: Cancel races with publication

- **WHEN** cancellation arrives while composition is entering the publication
  barrier
- **THEN** the existing publication barrier determines one stable committed or
  cancelled terminal result, and cleanup cannot remove a committed artifact

#### Scenario: Retry creates a new terminally independent operation

- **WHEN** composition ends failed or cancelled and the user retries
- **THEN** the old operation remains terminal, the retry has a new operation
  identity, and exact frozen-input/idempotency rules prevent duplicate output

### Requirement: Deterministic output identity and bounded provenance

The output logical identity SHALL be a deterministic digest of the ordered or
policy-canonicalized contributing source identities and revisions, resolved
languages and roles/classifications, composition policy version, style policy
version, output representation, and publication mode. The identity MUST NOT
depend on operation ID, wall-clock time, or nondeterministic map/order
iteration. An existing valid composite with the exact identity MAY satisfy a
retry; a path with a different identity MUST NOT be overwritten silently.

Bounded provenance SHALL record, at minimum, counts and decisions for
`kept-primary`, `deduplicated`, `enriched/repaired`, `added`, and
`ambiguous/conflicting`; contributing source tracks/sidecars; resolved
languages and roles/classifications; attachment decisions; and output
identity/path. Provenance SHALL retain bounded source identifiers, revisions,
policy/version identifiers, and conflict summaries, but MUST NOT persist full
cue bodies, full ASS documents, OCR payloads, or unbounded duplicate lists.

#### Scenario: Exact retry is idempotent

- **WHEN** a retry resolves the same frozen sources, revisions, policies,
  representation, and publication mode as an existing valid composite
- **THEN** it reports or reuses the existing artifact identity without creating
  a second composite at another path

#### Scenario: Changed source has a different identity

- **WHEN** one source revision or composition/style policy version changes
- **THEN** the resolved output identity changes and the old composite remains
  separately identifiable and untouched

#### Scenario: Provenance records reconciliation outcomes

- **WHEN** composition completes or ends with a bounded conflict
- **THEN** Asset Info and artifact metadata expose the required decision
  counts, contributing source references, language/role decisions, and output
  identity/path without exposing full cue bodies

#### Scenario: Provenance is bounded

- **WHEN** sources contain many events, conflicts, or attachments
- **THEN** persisted provenance remains within documented bounds using counts,
  stable identifiers, and bounded summaries rather than storing full source
  documents or every cue body
