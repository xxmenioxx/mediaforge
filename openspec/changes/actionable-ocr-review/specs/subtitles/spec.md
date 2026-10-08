## MODIFIED Requirements

### Requirement: Asset Info review and correction are safe to publish

Asset Info SHALL expose `Analyze OCR` results with clean, suspicious, and
review counts and per-cue original-versus-suggestion details. For each
suspicious or reviewable cue, users SHALL be able to Correct the cue by
applying a validated suggestion or entering a text edit, Accept as correct
without changing its text, or Leave pending. Accept as correct SHALL persist a
durable approval even when the accepted text is unchanged. Leave pending SHALL
not change the cue, record an approval, or satisfy Translation review. A review
surface SHALL show cue ID, bounded reason and provenance, original text,
deterministic suggestion, any already-available AI suggestion, and an editable
text field; cue timing, order, ASS structure, geometry, and positioning SHALL
remain protected. Analysis SHALL be read-only. Correction and approval SHALL
use the existing subtitle operation identity, status, progress, cancellation,
and retry model rather than introducing a second job system. Batch decisions
SHALL be bounded, explicitly preview the affected cue IDs and actions, and
require confirmation before they are applied.

#### Scenario: Review shows a cue-level decision

- **WHEN** analysis returns a suspicious or reviewable cue
- **THEN** Asset Info shows its cue ID, original text, any deterministic or
  already-available AI suggestion, bounded reason and provenance, protected
  timing/layout details, an editable text field, and the Correct, Accept as
  correct, and Leave pending actions

#### Scenario: Accepting unchanged text creates a durable approval

- **WHEN** a user accepts a cue as correct without editing its text
- **THEN** the current sidecar text and structure remain unchanged and a
  revision-bound approval is recorded for that exact cue text

#### Scenario: Leaving a cue pending does not approve it

- **WHEN** a user leaves a finding pending
- **THEN** the cue and published artifacts remain unchanged, no approval is
  stored, and Translation continues to require review for that finding

#### Scenario: Safe batch repair applies validated findings only

- **WHEN** a user previews and confirms a bounded batch repair
- **THEN** only findings that pass deterministic validation are corrected,
  explicitly selected approval decisions are recorded, and uncertain or
  conflicting findings remain unchanged and reviewable

#### Scenario: Failed correction preserves the published sidecar

- **WHEN** correction fails validation, encounters an execution failure, or is
  cancelled before publication
- **THEN** the currently published SRT/ASS sidecar remains untouched, all
  operation-owned staging is cleaned, and the result is distinct from a
  successful correction

#### Scenario: Publication is atomic for requested outputs

- **WHEN** a correction changes text for SRT, ASS, or logical `both` outputs
- **THEN** every requested output is validated before the existing publication
  barrier commits, both serializers use the same corrected canonical cues, and
  no output from the current attempt is published alone

#### Scenario: No temporary repair artifacts are library-visible

- **WHEN** analysis or correction creates intermediate evidence or staged
  outputs
- **THEN** temporary files, `.bak`, and `.raw` artifacts remain internal and
  are cleaned or retained only in the internal provenance/evidence store; none
  appears beside the media as a Library-visible sidecar

#### Scenario: A cancelled repair can be retried

- **WHEN** a correction or approval operation ends as cancelled or failed and
  the user retries it
- **THEN** the prior operation remains terminal, a new operation identity is
  used, existing published sidecars are preserved, and valid canonical OCR
  evidence may be reused without violating the existing cancellation rules

### Requirement: Analysis and correction are bound to the published revision

Quality findings SHALL record a bounded published-artifact revision for every
SRT or ASS sidecar they describe. The revision SHALL include artifact/path
identity, size, mtime, full sidecar content hash, and logical `both` membership
where applicable. Findings and decisions SHALL also be bound to the canonical
source revision and identity, including the selected embedded stream, OCR
language and mode, preparation scope, and logical SRT/ASS transform. Each
approval SHALL identify the cue and exact cue-text hash it approves. Before
applying a correction or approval, before an approval is reused for
Translation, and again immediately before publication or durable decision
commit, the system SHALL verify that the source identity, cue hash, and
currently published output set still match the recorded revisions. A mismatch
SHALL fail closed as stale analysis, require re-analysis, clean operation
staging, preserve current sidecars, and invalidate the affected approval.

#### Scenario: External sidecar edit invalidates analysis

- **WHEN** a published SRT or ASS sidecar changes after analysis but before a
  correction or approval is applied
- **THEN** the action is rejected as stale, the changed sidecar is not
  overwritten, operation staging is cleaned, and the user must analyze again

#### Scenario: Logical both checks every published member

- **WHEN** analysis targets a logical `both` artifact and either the published
  SRT or ASS member no longer matches its recorded revision
- **THEN** the complete correction or approval is rejected before commit and
  neither output or decision from that attempt is committed

#### Scenario: Concurrent corrections cannot overwrite each other

- **WHEN** operation A and operation B both analyze revision R, A publishes a
  valid correction, and B reaches its final publication check afterward
- **THEN** B observes that revision R is stale, fails closed, cleans its
  staging, and does not overwrite A's published result or approval state

#### Scenario: Source or cue change invalidates approval

- **WHEN** the media revision, selected stream/OCR identity, logical transform,
  published artifact revision, or approved cue text changes
- **THEN** the old approval no longer satisfies Translation review, and the
  changed cue must be analyzed and reviewed against its current revision

## ADDED Requirements

### Requirement: Translation-triggered OCR review is visible and identity-exact

When Translation reports that canonical OCR review is required, its Review OCR
action SHALL open a visible review surface while preserving the selected
Translation source and settings. The review target SHALL match the blocked
request's exact media revision, embedded stream identity, requested OCR
language, OCR mode, preparation scope, and canonical preparation provenance.
The published SRT/ASS artifact or logical transform SHALL also match the
current published revision and exact logical provenance. SRT and ASS members
may be reviewed as one target only when they belong to the same logical
transform and contain equivalent canonical cue evidence. A target SHALL NOT
be selected from filename, language, or stream index alone. Missing,
incompatible, or conflicting evidence SHALL be shown as an actionable error
inside the review surface without closing or losing Translation context.

#### Scenario: Review OCR opens the exact target

- **WHEN** a blocked Translation request has one compatible canonical OCR
  review target
- **THEN** Review OCR visibly opens that target's cue-level review and does
  not analyze a neighboring sidecar

#### Scenario: Missing compatible sidecar remains actionable

- **WHEN** no published OCR sidecar matches the blocked request's exact source
  and OCR identity
- **THEN** the review surface explains that no compatible canonical sidecar is
  available, preserves the Translation source and settings, and offers a
  clear way to return to Translation or resolve the missing evidence

#### Scenario: Conflicting sidecars fail closed

- **WHEN** multiple candidates claim the same source but have incompatible
  revisions, logical transforms, or canonical cue evidence
- **THEN** the review surface reports the conflict and analyzes or modifies no
  candidate until the canonical target is unambiguous

#### Scenario: Review loading, empty, error, and cancel states stay visible

- **WHEN** review analysis is loading, completes with no findings, fails, or
  is cancelled
- **THEN** the review surface displays the corresponding state and a usable
  return or retry action; no state silently closes or loses Translation
  context

### Requirement: Canonical Translation honors current human OCR approvals

Canonical Translation preparation SHALL honor an approval only when its
canonical source revision and identity, selected stream, requested OCR
language and mode, preparation scope, logical SRT/ASS transform, published
artifact revision, cue ID, and exact cue-text hash match the current request.
After existing canonical dictionary and selective-correction work completes,
matching approvals SHALL resolve only their approved findings. Unapproved or
stale findings SHALL continue to block Translation. The translation provider
SHALL NOT be called until every required OCR finding is either safely resolved
by existing canonical correction or covered by a current explicit approval.
After review, returning to Translation SHALL preserve the selected source,
source language, target locale, and output representation; retry SHALL use the
same canonical OCR and Translation pipeline and SHALL not re-block on the same
approved finding for the same revision.

#### Scenario: Current approval prevents the same review loop

- **WHEN** a user approves the exact cue text for the current canonical source
  and published artifact revisions, then retries Translation without changing
  the request
- **THEN** preparation recognizes that approval, the reviewed finding no
  longer blocks Translation, and the provider receives the approved canonical
  cue text only after all other findings are resolved

#### Scenario: Pending or stale approval does not reach the provider

- **WHEN** any finding remains pending or an approval no longer matches the
  current source, OCR identity, artifact revision, or cue-text hash
- **THEN** Translation remains blocked for review and makes no provider call

#### Scenario: Translation retry preserves the selected request

- **WHEN** the user returns from OCR review and retries the blocked
  Translation
- **THEN** the selected embedded stream or external source, source language,
  target locale, and output representation remain selected, and the retry uses
  the existing terminal-operation retry contract
