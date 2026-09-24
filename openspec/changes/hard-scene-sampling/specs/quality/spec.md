## Purpose

Provide one deterministic, backend-authoritative and resource-bounded
`SamplingPlan` that identifies representative and potentially difficult media
windows for Test Encode evaluation and all participating quality consumers.

## ADDED Requirements

### Requirement: The system SHALL resolve one canonical versioned SamplingPlan

For an asset, the backend SHALL resolve a `SamplingPlan` from the asset/source
fingerprint, available canonical evidence, and an explicit sampling
policy/version. The plan SHALL contain a plan/schema identity, source revision
identity, evidence identities and statuses, selected normalized windows, each
window's stable identity, roles/reasons, bounded score/provenance summaries,
fallback state, and the maximum-window policy used. The plan SHALL distinguish
selection evidence from authoritative encode or Fidelity verdicts.

#### Scenario: Equivalent inputs resolve the same plan

- **WHEN** the asset fingerprint, canonical evidence, sampling policy/version,
  and normalization inputs are identical
- **THEN** the selected window identities, roles/reasons, scores, fallback
  state, and ordering SHALL be identical across repeated resolutions and
  backend entry points

#### Scenario: A plan records its policy identity

- **WHEN** a plan is returned to LAB, Queue, Test Encode, Validation, or a
  future quality consumer
- **THEN** the response SHALL identify the plan schema and sampling policy
  versions and SHALL NOT require a consumer to infer them from window values

### Requirement: Window identity and time normalization SHALL be canonical

Each selected window SHALL have a stable identity derived from the source
fingerprint/stream identity, canonical source-timeline normalization, and the
normalized start and duration. Role, score, and consumer SHALL NOT change that
identity. Start SHALL be clamped to the asset timeline, duration SHALL be
positive and bounded by the policy and available duration, and the plan SHALL
record both canonical media-time values and any compatible-preview request
normalization used by a consumer.

Two intervals SHALL be equivalent only under the versioned overlap/boundary
tolerance: the measured source-frame duration when reliable, otherwise the
fixed fallback tolerance. Non-equivalent overlapping intervals SHALL remain
distinct unless the policy explicitly versioned a consolidation rule.

#### Scenario: The same physical interval has one identity

- **WHEN** baseline evidence, a Fidelity/reference window, and a hard-scene
  candidate normalize to the same interval within the configured tolerance
- **THEN** they SHALL consolidate to one window identity while retaining the
  union of bounded roles and reasons

#### Scenario: Nearby hard scenes remain observable

- **WHEN** two candidates overlap but their normalized boundaries exceed the
  equivalence tolerance
- **THEN** the plan SHALL retain them as distinct candidates or selected
  windows subject to the bounded maximum, rather than silently treating all
  overlap as duplication

#### Scenario: Invalid interval input is bounded

- **WHEN** evidence proposes a negative start, an interval beyond the asset, or
  a duration larger than the policy limit
- **THEN** the backend SHALL clamp or reject that candidate deterministically,
  record a bounded normalization/discard reason, and SHALL never emit an
  out-of-range selected window

### Requirement: Selection SHALL reuse canonical evidence with explicit precedence

The selector SHALL prefer semantically equivalent, version-matched evidence
already available for the same source revision. Reusable inputs MAY include the
existing distributed frame/GOP/frame-signal windows, cadence and field evidence,
interlace evidence, restoration noise/grain/blocking/ringing/detail evidence,
stream metadata and bitrate, bounded packet/timing evidence, existing
Fidelity/reference windows, and a future canonical scene or motion evidence
source. Evidence SHALL retain availability, confidence, source, and version
identity.

Interval-specific measured evidence SHALL outrank stream-global metadata for
window classification. A frozen primary user selection SHALL outrank inferred
selection for primary ownership. Missing, stale, ambiguous, or merely
diagnostic evidence SHALL remain unavailable or corroborating; it SHALL NOT be
promoted to authoritative evidence by a selector.

#### Scenario: Existing evidence is reused

- **WHEN** a matching snapshot already contains versioned frame-structure,
  cadence, restoration, or source-fingerprint evidence for the asset revision
- **THEN** SamplingPlan resolution SHALL consume that evidence and SHALL NOT
  launch a semantically duplicate full-file analyzer merely to select windows

#### Scenario: A global bitrate cannot become a hard-scene claim

- **WHEN** the only complexity input is asset- or stream-global bitrate without
  bounded interval evidence
- **THEN** it MAY support baseline or weak complexity scoring, but SHALL NOT be
  presented as proof that a particular window is high complexity or worst
  predicted

#### Scenario: An unavailable analyzer does not block planning

- **WHEN** scene-change, motion, dark/gradient, blur, or another optional
  evidence source is absent or stale
- **THEN** its role contribution SHALL be marked unavailable and the plan SHALL
  continue with the remaining canonical evidence and deterministic fallback

### Requirement: Roles and scores SHALL be bounded selection evidence

Selected windows MAY carry one or more of the roles `typical`, `high_motion`,
`high_texture_or_grain`, `dark_or_gradient`, `high_complexity`, and
`worst_predicted`. A role SHALL be omitted when no usable evidence supports it;
the plan SHALL NOT manufacture one window per role. A window MAY satisfy several
roles, and a score SHALL describe ranking evidence only, not a guaranteed visual
property or a final quality verdict.

The backend SHALL normalize scores to a bounded, versioned range, apply missing
evidence as unavailable rather than as a zero-quality truth, and retain bounded
role/reason evidence references and confidence. Candidate generation and
per-role ranking SHALL have explicit limits before global selection.

#### Scenario: One window satisfies multiple roles

- **WHEN** the highest-ranked candidate has strong available texture/grain and
  complexity evidence
- **THEN** one selected window MAY carry both `high_texture_or_grain` and
  `high_complexity`, with separate bounded reasons and contributions

#### Scenario: No evidence does not create a false role

- **WHEN** no canonical evidence supports `dark_or_gradient`
- **THEN** the plan SHALL omit that role and SHALL identify the role as
  unavailable or unselected rather than assigning it to an arbitrary window

### Requirement: Ranking, consolidation, ordering, and maximum windows SHALL be deterministic

The plan SHALL rank candidates using a versioned bounded scoring contract and
deterministic tie-breaking. Ties SHALL be resolved by the canonical role
priority, evidence confidence/availability, normalized start, normalized
duration, and stable window identity in that order, or by an explicitly
versioned equivalent ordering. Consolidation SHALL union roles/reasons without
duplicating a window, and discarded/consolidated candidates SHALL be represented
only by bounded summaries.

The plan SHALL enforce a hard maximum that includes the primary window. The
default SHALL preserve the current LAB distributed behavior's bounded capacity
(the balanced policy's primary plus its configured distributed positions when
the primary is not equivalent); a policy MAY choose a lower bound but SHALL
NOT bypass the maximum. Selected ordering SHALL be stable: the preserved
primary first, followed by canonical baseline/hard-scene order defined by the
policy.

#### Scenario: Equal scores use a stable tie-break

- **WHEN** two candidates have equal quantized role scores and equivalent
  evidence confidence
- **THEN** the same candidate SHALL win on every run according to the documented
  tie-break, independent of map, filesystem, goroutine, or input enumeration
  order

#### Scenario: The bounded maximum is reached

- **WHEN** more candidates qualify than the plan maximum
- **THEN** the backend SHALL retain only the deterministically ranked bounded
  set, preserve the primary, and record bounded discarded-role/reason counts

### Requirement: Insufficient evidence SHALL use deterministic distributed fallback

If the asset has insufficient usable evidence for hard-scene selection, the
backend SHALL retain the existing distributed sampling positions and duration
adaptation as the baseline/fallback. The fallback SHALL be deterministic,
bounded, non-random, and non-fatal to LAB or Test Encode. The plan SHALL
identify the fallback reason, which evidence was unavailable or stale, and
whether the selected windows are baseline-only, without claiming hard-scene
coverage.

#### Scenario: A source has only metadata and duration

- **WHEN** no usable interval-level hard-scene evidence exists
- **THEN** the plan SHALL return the normalized distributed baseline in stable
  order, complete with fallback provenance, and LAB SHALL remain runnable

#### Scenario: Short media collapses baseline windows

- **WHEN** the asset duration is shorter than the configured distributed window
- **THEN** the plan SHALL apply the existing deterministic duration adaptation,
  deduplicate equivalent intervals, and return at least one valid window when
  the source duration permits it

### Requirement: LAB SHALL preserve preview-first primary ownership

For LAB, an explicit user-selected `Start` and `Seconds` interval SHALL remain
the primary window even when it is not a baseline or hard-scene winner. The
primary SHALL retain its existing Compatible Preview request, artifact, source
revision, generation identity, and Frame Fidelity role. SamplingPlan hard-scene
windows SHALL complement that primary and SHALL NOT silently replace it or
cause a second primary encode. Additional windows SHALL consume the frozen
plan and execute sequentially within the existing bounded Fidelity capacity.

#### Scenario: The explicit primary is not a hard-scene winner

- **WHEN** the user selects a valid LAB interval that receives no hard-scene
  role
- **THEN** the selected interval SHALL still be first in the plan and primary
  preview/Fidelity SHALL use its existing artifact; additional windows MAY be
  selected after it

#### Scenario: The primary overlaps a candidate

- **WHEN** a hard-scene candidate is equivalent to the explicit primary after
  normalization
- **THEN** the plan SHALL reuse the primary identity, merge its bounded roles
  and reasons, and SHALL NOT request or encode that interval again

#### Scenario: Additional Fidelity work is bounded and sequential

- **WHEN** more than one additional selected window requires Compatible Preview
  or Fidelity evidence
- **THEN** additional windows SHALL be processed one at a time, in canonical
  plan order, while retaining bounded partial results and primary readiness

### Requirement: Frozen plans SHALL be immutable across later changes

When LAB, Queue, Validation, Test Encode, or another operation requires a
persisted plan, the frozen record SHALL include the asset/source revision,
sampling policy/version, relevant evidence version/identity, selected windows,
roles/reasons, fallback state, normalization contract, and plan identity. A
later source revision, evidence revision, or policy/version SHALL make a prior
unfrozen plan stale and require a new plan; it SHALL NOT silently reinterpret a
frozen operation or execution.

#### Scenario: The source revision changes

- **WHEN** the same path is replaced or its persisted size/mtime/source revision
  no longer matches a frozen plan
- **THEN** the plan SHALL be reported stale and SHALL NOT be reused as current
  evidence or silently remapped to the replacement asset

#### Scenario: The sampling policy changes

- **WHEN** the configured sampling policy or policy version changes after Queue
  preparation or LAB operation start
- **THEN** the frozen operation SHALL keep its original windows and provenance,
  while a new operation MAY resolve a new plan with a distinct identity

### Requirement: Future consumers SHALL consume, not reselect, canonical windows

The #69 perceptual-quality consumer SHALL accept the canonical SamplingPlan and
evaluate the same window identities, without selecting its own hard scenes. The
#9 QSV pumping consumer SHALL be able to request the plan's
`high_motion`, `high_texture_or_grain`, and `worst_predicted` windows, subject
to the frozen plan and bounded maximum, without introducing another selector.
Validation and future quality consumers SHALL preserve window identity and
provenance when reporting results.

#### Scenario: #69 evaluates the same windows

- **WHEN** #69 receives a frozen SamplingPlan from Test Encode or Queue
- **THEN** it SHALL evaluate the supplied window identities and SHALL report
  plan/policy provenance rather than generating a second hard-scene set

#### Scenario: #9 requests a role subset

- **WHEN** #9 requests high-motion, texture/grain, or worst-predicted coverage
- **THEN** the backend SHALL return only matching windows already present in the
  canonical plan, with their original identities and reasons, and SHALL report
  unavailable roles without selecting replacements

### Requirement: Persisted provenance SHALL remain bounded and explainable

Plan provenance SHALL explain why each selected window was retained, including
role/reason, contributing evidence identities and versions, score/confidence
summary, normalization, policy/version, fallback use, and bounded
discard/consolidation summaries. It MUST NOT persist unlimited frame-level
evidence, complete packet datasets, full decoded frames, or full analyzer logs
as part of a plan.

#### Scenario: A user inspects a hard-scene reason

- **WHEN** a selected additional window is displayed by LAB or a future quality
  consumer
- **THEN** the consumer SHALL be able to show its role/reason and bounded
  evidence provenance without rerunning selection or exposing an unbounded
  dataset

#### Scenario: Plan provenance exceeds its bound

- **WHEN** candidate discovery produces more evidence or discarded candidates
  than the provenance budget
- **THEN** the plan SHALL retain bounded counts and deterministic top summaries,
  mark provenance as truncated when applicable, and SHALL remain valid and
  explainable
